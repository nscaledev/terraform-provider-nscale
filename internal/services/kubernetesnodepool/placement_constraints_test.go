/*
Copyright 2026 Nscale

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubernetesnodepool

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// spreadReservationModel is a reservation pool with every constraint set.
func spreadReservationModel(t *testing.T) *KubernetesNodePoolModel {
	t.Helper()

	return &KubernetesNodePoolModel{
		Name:             types.StringValue("gpu"),
		ClusterID:        types.StringValue("cluster-1"),
		ProvisioningMode: types.StringValue("reservation"),
		Replicas:         types.Int64Value(4),
		Tags:             types.MapNull(types.StringType),
		Compute:          types.ObjectNull(computeAttrTypes()),
		Reservation: objectValue(t, reservationAttrTypes(), map[string]attr.Value{
			"reservation_id": types.StringValue("res-1"),
			"constraints": objectValue(t, constraintsAttrTypes(), map[string]attr.Value{
				"policy":             types.StringValue("spread"),
				"max_skew":           types.Int64Value(1),
				"min_domains":        types.Int64Value(2),
				"when_unsatisfiable": types.StringValue("bestEffort"),
			}),
		}),
		Taints: types.ListNull(taintObjectType()),
		Labels: types.MapNull(types.StringType),
	}
}

// TestConstraintsRoundTrip sends every constraint and reads the payload back
// through the read converter: what goes out must come back identical, or the
// next plan would propose replacing the pool.
func TestConstraintsRoundTrip(t *testing.T) {
	t.Parallel()

	model := spreadReservationModel(t)

	params, diagnostics := model.NscaleNodePoolCreateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building create params: %v", diagnostics)
	}

	encoded, err := json.Marshal(params.Spec.Reservation)
	if err != nil {
		t.Fatalf("marshalling reservation: %s", err)
	}
	want := `{"constraints":{"maxSkew":1,"minDomains":2,"policy":"spread","whenUnsatisfiable":"bestEffort"},"reservationId":"res-1"}`
	if string(encoded) != want {
		t.Errorf("reservation payload =\n  %s\nwant\n  %s", encoded, want)
	}

	readBack := reservationObjectValue(params.Spec.Reservation)
	if !readBack.Equal(model.Reservation) {
		t.Errorf("read back %s, want %s", readBack, model.Reservation)
	}
}

// TestUpdateParamsCarryConstraints: constraints are immutable, including
// whether they are set, so a PUT that dropped them would be rejected.
func TestUpdateParamsCarryConstraints(t *testing.T) {
	t.Parallel()

	params, diagnostics := spreadReservationModel(t).NscaleNodePoolUpdateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building update params: %v", diagnostics)
	}

	if params.Spec.Reservation == nil || params.Spec.Reservation.Constraints == nil {
		t.Fatal("constraints were dropped from the update payload")
	}
	if got := params.Spec.Reservation.Constraints.Policy; got != "spread" {
		t.Errorf("policy = %q, want spread", got)
	}
}

func TestSpreadOnlyConstraintsFindings(t *testing.T) {
	t.Parallel()

	all := map[string]bool{"max_skew": true, "min_domains": true, "when_unsatisfiable": true}

	tests := map[string]struct {
		policy types.String
		set    map[string]bool
		want   []string
	}{
		"spread allows everything": {types.StringValue("spread"), all, nil},
		"pack rejects each":        {types.StringValue("pack"), all, spreadOnlyAttributes()},
		"pack alone is fine":       {types.StringValue("pack"), map[string]bool{}, nil},
		"pack names only the set": {
			types.StringValue("pack"),
			map[string]bool{"min_domains": true},
			[]string{"min_domains"},
		},
		"unknown policy is skipped": {types.StringUnknown(), all, nil},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := spreadOnlyConstraintsFindings(test.policy, test.set); !reflect.DeepEqual(got, test.want) {
				t.Errorf("findings = %v, want %v", got, test.want)
			}
		})
	}
}

// nodePoolConfig builds a resource config with replicas = 2 and optionally the
// reservation block set, which is all validateMinDomainsWithinReplicas reads.
func nodePoolConfig(t *testing.T, minDomains *int64, withReservation bool) tfsdk.Config {
	t.Helper()

	ctx := context.Background()

	var response resource.SchemaResponse
	NewKubernetesNodePoolResource().Schema(ctx, resource.SchemaRequest{}, &response)

	objectType := response.Schema.Type().TerraformType(ctx).(tftypes.Object) //nolint:forcetypeassert // schema root is an object
	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}

	values["replicas"] = tftypes.NewValue(tftypes.Number, 2)

	if withReservation {
		reservationType := objectType.AttributeTypes["reservation"].(tftypes.Object)      //nolint:forcetypeassert // reservation is an object
		constraintsType := reservationType.AttributeTypes["constraints"].(tftypes.Object) //nolint:forcetypeassert // constraints is an object

		var minDomainsValue tftypes.Value
		if minDomains == nil {
			minDomainsValue = tftypes.NewValue(tftypes.Number, nil)
		} else {
			minDomainsValue = tftypes.NewValue(tftypes.Number, *minDomains)
		}

		values["reservation"] = tftypes.NewValue(reservationType, map[string]tftypes.Value{
			"reservation_id": tftypes.NewValue(tftypes.String, "res-1"),
			"constraints": tftypes.NewValue(constraintsType, map[string]tftypes.Value{
				"policy":             tftypes.NewValue(tftypes.String, "spread"),
				"max_skew":           tftypes.NewValue(tftypes.Number, nil),
				"min_domains":        minDomainsValue,
				"when_unsatisfiable": tftypes.NewValue(tftypes.String, nil),
			}),
		})
	}

	return tfsdk.Config{Schema: response.Schema, Raw: tftypes.NewValue(objectType, values)}
}

func TestValidateMinDomainsWithinReplicas(t *testing.T) {
	t.Parallel()

	three, two := int64(3), int64(2)

	tests := map[string]struct {
		config    func(*testing.T) tfsdk.Config
		wantError bool
	}{
		"above replicas":    {func(t *testing.T) tfsdk.Config { return nodePoolConfig(t, &three, true) }, true},
		"equal to replicas": {func(t *testing.T) tfsdk.Config { return nodePoolConfig(t, &two, true) }, false},
		"unset":             {func(t *testing.T) tfsdk.Config { return nodePoolConfig(t, nil, true) }, false},
		"compute pool":      {func(t *testing.T) tfsdk.Config { return nodePoolConfig(t, nil, false) }, false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var diagnostics diag.Diagnostics
			validateMinDomainsWithinReplicas(context.Background(), test.config(t), &diagnostics)

			if got := diagnostics.HasError(); got != test.wantError {
				t.Errorf("HasError = %t, want %t: %v", got, test.wantError, diagnostics)
			}
		})
	}
}
