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

package instance_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	computeapi "github.com/nscaledev/nscale-sdk-go/compute"

	"github.com/nscaledev/terraform-provider-nscale/internal/provider"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/instance"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/uuidtype"
)

func TestInstanceRespelledUUIDsPlanNoChange(t *testing.T) {
	ctx := context.Background()
	planner := newInstancePlanHarness(ctx, t)

	imageID := uuid.MustParse("5c9b2e7f-1a4d-4b8c-8f2e-3d6a9b0c1e52")
	flavorID := uuid.MustParse("2f8a1d4e-6b3c-4f5a-9e0d-7c1b8a2f3d40")
	caID := uuid.MustParse("8e3f4a1b-2c5d-4e7f-9a0b-1c2d3e4f5a60")

	prior := instance.InstanceResourceModel{
		InstanceModel: instance.NewInstanceModel(&computeapi.InstanceRead{
			Metadata: computeapi.ProjectScopedResourceReadMetadata{
				Id:           "instance-1",
				Name:         "test-instance",
				ProjectId:    "project-1",
				CreationTime: time.Date(2026, time.April, 28, 11, 3, 12, 0, time.UTC),
			},
			Spec: computeapi.InstanceSpec{
				FlavorId:                  flavorID,
				ImageId:                   imageID,
				SshCertificateAuthorityId: &caID,
				Networking:                &computeapi.InstanceNetworking{},
			},
			Status: computeapi.InstanceStatus{
				NetworkId: "network-1",
				RegionId:  "region-1",
			},
		}),
		Timeouts: tftimeouts.Value{
			Object: types.ObjectNull(map[string]attr.Type{
				"create": types.StringType,
				"update": types.StringType,
				"delete": types.StringType,
			}),
		},
	}

	proposed := prior
	proposed.ImageID = uuidtype.NewValue(strings.ToUpper(imageID.String()))
	proposed.FlavorID = uuidtype.NewValue(strings.ToUpper(flavorID.String()))
	proposed.SSHCertificateAuthorityID = uuidtype.NewValue(strings.ToUpper(caID.String()))

	config := proposed
	config.ID = types.StringNull()
	config.PublicIP = types.StringNull()
	config.PrivateIP = types.StringNull()
	config.PowerState = types.StringNull()
	config.RegionID = types.StringNull()
	config.CreationTime = types.StringNull()

	response := planner.plan(prior, proposed, config)

	if len(response.RequiresReplace) != 0 {
		t.Errorf("RequiresReplace = %v, want none", response.RequiresReplace)
	}

	planned, err := response.PlannedState.Unmarshal(planner.schemaType)
	if err != nil {
		t.Fatalf("decode planned state: %v", err)
	}

	if !planned.Equal(planner.value(prior)) {
		t.Error("plan changes the instance, want no change")
	}
}

func TestInstancePlanUpdatesServerInputs(t *testing.T) {
	ctx := context.Background()
	planner := newInstancePlanHarness(ctx, t)
	imageID := uuid.MustParse("5c9b2e7f-1a4d-4b8c-8f2e-3d6a9b0c1e52")
	flavorID := uuid.MustParse("2f8a1d4e-6b3c-4f5a-9e0d-7c1b8a2f3d40")
	userData := []byte("old")

	prior := instance.InstanceResourceModel{
		InstanceModel: instance.NewInstanceModel(&computeapi.InstanceRead{
			Metadata: computeapi.ProjectScopedResourceReadMetadata{
				Id:           "instance-1",
				Name:         "test-instance",
				ProjectId:    "project-1",
				CreationTime: time.Date(2026, time.April, 28, 11, 3, 12, 0, time.UTC),
			},
			Spec: computeapi.InstanceSpec{
				FlavorId: flavorID,
				ImageId:  imageID,
				UserData: &userData,
				Networking: &computeapi.InstanceNetworking{
					SecurityGroups: &[]string{"sg-1"},
				},
			},
			Status: computeapi.InstanceStatus{NetworkId: "network-1", RegionId: "region-1"},
		}),
		Timeouts: tftimeouts.Value{Object: types.ObjectNull(map[string]attr.Type{
			"create": types.StringType,
			"update": types.StringType,
			"delete": types.StringType,
		})},
	}

	for _, test := range []struct {
		name   string
		change func(*instance.InstanceResourceModel)
	}{
		{
			name: "image",
			change: func(model *instance.InstanceResourceModel) {
				model.ImageID = uuidtype.NewValue("8e3f4a1b-2c5d-4e7f-9a0b-1c2d3e4f5a60")
			},
		},
		{
			name: "user data",
			change: func(model *instance.InstanceResourceModel) {
				model.UserData = types.StringValue("bmV3")
			},
		},
		{
			name: "flavor",
			change: func(model *instance.InstanceResourceModel) {
				model.FlavorID = uuidtype.NewValue("9f4a1b2e-6c3d-4f5a-8e0d-7c1b8a2f3d40")
			},
		},
		{
			name: "security group",
			change: func(model *instance.InstanceResourceModel) {
				model.NetworkInterface = types.ObjectValueMust(
					instance.InstanceNetworkInterfaceModelAttributeType.AttrTypes,
					map[string]attr.Value{
						"network_id":           types.StringValue("network-1"),
						"enable_public_ip":     types.BoolNull(),
						"security_group_ids":   types.ListValueMust(types.StringType, []attr.Value{types.StringValue("sg-2")}),
						"allowed_destinations": types.ListNull(types.StringType),
					},
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			proposed := prior
			test.change(&proposed)
			config := proposed
			config.ID = types.StringNull()
			config.PublicIP = types.StringNull()
			config.PrivateIP = types.StringNull()
			config.PowerState = types.StringNull()
			config.RegionID = types.StringNull()
			config.CreationTime = types.StringNull()

			response := planner.plan(prior, proposed, config)
			if len(response.RequiresReplace) != 0 {
				t.Errorf("RequiresReplace = %v, want none", response.RequiresReplace)
			}
		})
	}
}

type instancePlanHarness struct {
	t          *testing.T
	ctx        context.Context
	schema     schema.Schema
	schemaType tftypes.Type
}

func newInstancePlanHarness(ctx context.Context, t *testing.T) instancePlanHarness {
	t.Helper()
	var schemaResponse resource.SchemaResponse
	instance.NewInstanceResource().Schema(ctx, resource.SchemaRequest{}, &schemaResponse)

	return instancePlanHarness{
		t:          t,
		ctx:        ctx,
		schema:     schemaResponse.Schema,
		schemaType: schemaResponse.Schema.Type().TerraformType(ctx),
	}
}

func (h instancePlanHarness) value(model instance.InstanceResourceModel) tftypes.Value {
	h.t.Helper()

	plan := tfsdk.Plan{Schema: h.schema, Raw: tftypes.NewValue(h.schemaType, nil)}
	if diagnostics := plan.Set(h.ctx, model); diagnostics.HasError() {
		h.t.Fatalf("encode model: %v", diagnostics)
	}

	return plan.Raw
}

func (h instancePlanHarness) dynamic(model instance.InstanceResourceModel) *tfprotov6.DynamicValue {
	h.t.Helper()

	value, err := tfprotov6.NewDynamicValue(h.schemaType, h.value(model))
	if err != nil {
		h.t.Fatalf("encode model: %v", err)
	}

	return &value
}

func (h instancePlanHarness) plan(
	prior, proposed, config instance.InstanceResourceModel,
) *tfprotov6.PlanResourceChangeResponse {
	h.t.Helper()

	server, err := providerserver.NewProtocol6WithError(provider.New())()
	if err != nil {
		h.t.Fatalf("provider server: %v", err)
	}
	response, err := server.PlanResourceChange(h.ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "nscale_instance",
		PriorState:       h.dynamic(prior),
		ProposedNewState: h.dynamic(proposed),
		Config:           h.dynamic(config),
	})
	if err != nil {
		h.t.Fatalf("PlanResourceChange() error = %v", err)
	}
	for _, diagnostic := range response.Diagnostics {
		if diagnostic.Severity == tfprotov6.DiagnosticSeverityError {
			h.t.Fatalf("PlanResourceChange() diagnostic: %s: %s", diagnostic.Summary, diagnostic.Detail)
		}
	}
	return response
}
