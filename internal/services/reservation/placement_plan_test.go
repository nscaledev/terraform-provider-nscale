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

package reservation_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/provider"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/reservation"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/uuidtype"
)

const (
	planTestImageID        = "5c9b2e7f-1a4d-4b8c-8f2e-3d6a9b0c1e52"
	planTestUpdatedImageID = "2f8a1d4e-6b3c-4f5a-9e0d-7c1b8a2f3d40"
)

func planTestPlacement() *reservationapi.PlacementV2Read {
	return &reservationapi.PlacementV2Read{
		Metadata: reservationapi.ProjectScopedResourceReadMetadata{
			Id:                 "placement-1",
			Name:               "training-workers",
			ProjectId:          "project-1",
			CreationTime:       time.Date(2026, time.April, 28, 11, 3, 12, 0, time.UTC),
			ProvisioningStatus: reservationapi.ResourceProvisioningStatusProvisioned,
		},
		Spec: reservationapi.PlacementV2Spec{
			Count: 2,
			Constraints: reservationapi.PlacementConstraintsV2{
				Policy: reservationapi.PlacementPolicyV2Pack,
			},
			ServerSpec: reservationapi.PlacementServerSpecV2{
				ImageId: planTestImageID,
				Networking: &reservationapi.PlacementServerNetworkingV2{
					SecurityGroups: &[]string{"sg-1"},
				},
			},
			UpdateStrategy: &reservationapi.PlacementUpdateStrategyV2{
				Type: reservationapi.PlacementUpdateStrategyTypeV2Manual,
			},
		},
		Status: reservationapi.PlacementV2Status{
			RegionId:       "region-1",
			ReservationId:  "reservation-1",
			NetworkId:      "network-1",
			ReadyHostCount: new(2),
		},
	}
}

func planPlacement(
	t *testing.T,
	prior *reservationapi.PlacementV2Read,
	next *reservationapi.PlacementV2Read,
	unsetConfig func(config, proposed *reservation.PlacementResourceModel),
) (*tfprotov6.PlanResourceChangeResponse, reservation.PlacementResourceModel) {
	t.Helper()

	ctx := context.Background()

	schemaResponse := placementSchema()
	schemaType := schemaResponse.Schema.Type().TerraformType(ctx)

	encode := func(model reservation.PlacementResourceModel) *tfprotov6.DynamicValue {
		t.Helper()

		value, err := tfprotov6.NewDynamicValue(schemaType, encodePlacement(t, model))
		if err != nil {
			t.Fatalf("encode model: %v", err)
		}

		return &value
	}

	priorModel := placementResourceModel(prior)
	proposedModel := placementResourceModel(next)

	configModel := proposedModel
	configModel.ID = types.StringNull()
	configModel.RegionID = types.StringNull()
	configModel.ReadyHostCount = types.Int64Null()
	configModel.UpdatedHostCount = types.Int64Null()
	configModel.DriftedHostCount = types.Int64Null()
	configModel.ProjectID = types.StringNull()
	configModel.CreationTime = types.StringNull()
	configModel.ProvisioningStatus = types.StringNull()

	proposedModel.ID = priorModel.ID
	proposedModel.RegionID = priorModel.RegionID
	proposedModel.ReadyHostCount = priorModel.ReadyHostCount
	proposedModel.UpdatedHostCount = priorModel.UpdatedHostCount
	proposedModel.DriftedHostCount = priorModel.DriftedHostCount
	proposedModel.ProjectID = priorModel.ProjectID
	proposedModel.CreationTime = priorModel.CreationTime
	proposedModel.ProvisioningStatus = priorModel.ProvisioningStatus

	if unsetConfig != nil {
		unsetConfig(&configModel, &proposedModel)
	}

	proposedModel.UpdateStrategy, _ = proposeAttribute(
		ctx,
		schemaResponse.Schema.Attributes["update_strategy"],
		priorModel.UpdateStrategy,
		configModel.UpdateStrategy,
	).(types.Object)

	server, err := providerserver.NewProtocol6WithError(provider.New())()
	if err != nil {
		t.Fatalf("provider server: %v", err)
	}

	response, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "nscale_placement",
		PriorState:       encode(priorModel),
		ProposedNewState: encode(proposedModel),
		Config:           encode(configModel),
	})
	if err != nil {
		t.Fatalf("PlanResourceChange() error = %v", err)
	}

	for _, diagnostic := range response.Diagnostics {
		if diagnostic.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("PlanResourceChange() diagnostic: %s: %s", diagnostic.Summary, diagnostic.Detail)
		}
	}

	plannedValue, err := response.PlannedState.Unmarshal(schemaType)
	if err != nil {
		t.Fatalf("decode planned state: %v", err)
	}

	var planned reservation.PlacementResourceModel

	plan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: plannedValue}
	if diagnostics := plan.Get(ctx, &planned); diagnostics.HasError() {
		t.Fatalf("decode planned state: %v", diagnostics)
	}

	return response, planned
}

func placementSchema() resource.SchemaResponse {
	var schemaResponse resource.SchemaResponse
	reservation.NewPlacementResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)

	return schemaResponse
}

func placementResourceModel(placement *reservationapi.PlacementV2Read) reservation.PlacementResourceModel {
	return reservation.PlacementResourceModel{
		PlacementModel: reservation.NewPlacementModel(placement),
		Timeouts: tftimeouts.Value{
			Object: types.ObjectNull(map[string]attr.Type{
				"create": types.StringType,
				"update": types.StringType,
				"delete": types.StringType,
			}),
		},
	}
}

func encodePlacement(t *testing.T, model reservation.PlacementResourceModel) tftypes.Value {
	t.Helper()

	ctx := context.Background()
	schemaResponse := placementSchema()

	plan := tfsdk.Plan{
		Schema: schemaResponse.Schema,
		Raw:    tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil),
	}
	if diagnostics := plan.Set(ctx, model); diagnostics.HasError() {
		t.Fatalf("encode model: %v", diagnostics)
	}

	return plan.Raw
}

// proposeAttribute proposes a value for an attribute the way Terraform does
// before asking the provider to plan (objchange.proposedNewAttributes): a
// computed attribute left unconfigured is proposed at its prior value, unless
// the prior value holds a nested attribute that is not computed, which only
// configuration could have set; a configured nested object is proposed
// attribute by attribute; anything else is proposed as configured.
func proposeAttribute(ctx context.Context, attribute schema.Attribute, prior, config attr.Value) attr.Value {
	if attribute.IsComputed() && config.IsNull() {
		if setByConfiguration(attribute, prior) {
			return config
		}

		return prior
	}

	nested, ok := attribute.(schema.SingleNestedAttribute)
	if !ok || config.IsNull() || config.IsUnknown() {
		return config
	}

	configObject, _ := config.(types.Object)
	priorObject, _ := prior.(types.Object)

	values := make(map[string]attr.Value, len(nested.Attributes))
	for name, child := range nested.Attributes {
		childPrior := nullValue(ctx, child.GetType())
		if !priorObject.IsNull() && !priorObject.IsUnknown() {
			childPrior = priorObject.Attributes()[name]
		}

		values[name] = proposeAttribute(ctx, child, childPrior, configObject.Attributes()[name])
	}

	return types.ObjectValueMust(configObject.AttributeTypes(ctx), values)
}

// setByConfiguration reports whether a prior value holds a non-null nested
// attribute that is not computed (objchange.optionalValueNotComputable).
func setByConfiguration(attribute schema.Attribute, prior attr.Value) bool {
	nested, ok := attribute.(schema.SingleNestedAttribute)
	if !ok || prior.IsNull() || prior.IsUnknown() {
		return false
	}

	priorObject, _ := prior.(types.Object)

	for name, child := range nested.Attributes {
		value := priorObject.Attributes()[name]
		if value.IsNull() {
			continue
		}

		if !child.IsComputed() || setByConfiguration(child, value) {
			return true
		}
	}

	return false
}

func nullValue(ctx context.Context, attributeType attr.Type) attr.Value {
	value, err := attributeType.ValueFromTerraform(ctx, tftypes.NewValue(attributeType.TerraformType(ctx), nil))
	if err != nil {
		panic(err)
	}

	return value
}

func plansNoChange(
	t *testing.T,
	response *tfprotov6.PlanResourceChangeResponse,
	prior *reservationapi.PlacementV2Read,
) bool {
	t.Helper()

	planned, err := response.PlannedState.Unmarshal(placementSchema().Schema.Type().TerraformType(context.Background()))
	if err != nil {
		t.Fatalf("decode planned state: %v", err)
	}

	return planned.Equal(encodePlacement(t, placementResourceModel(prior)))
}

func TestPlacementImageChangePlansInPlaceUpdate(t *testing.T) {
	next := planTestPlacement()
	next.Spec.ServerSpec.ImageId = planTestUpdatedImageID

	response, planned := planPlacement(t, planTestPlacement(), next, nil)

	if len(response.RequiresReplace) != 0 {
		t.Errorf("RequiresReplace = %v, want none for an image change", response.RequiresReplace)
	}

	if !planned.UpdatedHostCount.IsUnknown() {
		t.Errorf("planned updated_host_count = %s, want unknown", planned.UpdatedHostCount)
	}
}

func TestPlacementImageChangeOnErroredPlacementPlansStatusUnknown(t *testing.T) {
	prior := planTestPlacement()
	prior.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusError

	next := planTestPlacement()
	next.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusError
	next.Spec.ServerSpec.ImageId = planTestUpdatedImageID

	_, planned := planPlacement(t, prior, next, nil)

	if !planned.ProvisioningStatus.IsUnknown() {
		t.Errorf(
			"planned provisioning_status = %s, want unknown: the update waits for provisioned",
			planned.ProvisioningStatus,
		)
	}
}

func TestPlacementUpdateStrategyChangePlansInPlaceUpdate(t *testing.T) {
	next := planTestPlacement()
	next.Spec.UpdateStrategy = &reservationapi.PlacementUpdateStrategyV2{
		Type: reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate,
		RollingUpdate: &reservationapi.PlacementRollingUpdateV2{
			MaxUnavailable: new("25%"),
		},
	}

	response, _ := planPlacement(t, planTestPlacement(), next, nil)

	if len(response.RequiresReplace) != 0 {
		t.Errorf("RequiresReplace = %v, want none for an update strategy change", response.RequiresReplace)
	}
}

func TestPlacementUnconfiguredUpdateStrategyKeepsState(t *testing.T) {
	prior := planTestPlacement()
	prior.Spec.UpdateStrategy = &reservationapi.PlacementUpdateStrategyV2{
		Type: reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate,
	}

	next := planTestPlacement()
	next.Spec.ServerSpec.ImageId = planTestUpdatedImageID

	_, planned := planPlacement(t, prior, next, func(config, _ *reservation.PlacementResourceModel) {
		config.UpdateStrategy = types.ObjectNull(reservation.PlacementUpdateStrategyModelAttributeType.AttrTypes)
	})

	want := reservation.NewPlacementModel(prior).UpdateStrategy
	if !planned.UpdateStrategy.Equal(want) {
		t.Errorf("planned update_strategy = %s, want the prior %s", planned.UpdateStrategy, want)
	}
}

func TestPlacementUnconfiguredUpdateStrategyPlansNoChange(t *testing.T) {
	testCases := []struct {
		name     string
		strategy reservationapi.PlacementUpdateStrategyV2
	}{
		{"manual", reservationapi.PlacementUpdateStrategyV2{
			Type: reservationapi.PlacementUpdateStrategyTypeV2Manual,
		}},
		{"rolling update", reservationapi.PlacementUpdateStrategyV2{
			Type: reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate,
			RollingUpdate: &reservationapi.PlacementRollingUpdateV2{
				MaxUnavailable: new("1"),
			},
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			prior := planTestPlacement()
			prior.Spec.UpdateStrategy = &testCase.strategy

			response, _ := planPlacement(t, prior, prior, func(config, _ *reservation.PlacementResourceModel) {
				config.UpdateStrategy = types.ObjectNull(
					reservation.PlacementUpdateStrategyModelAttributeType.AttrTypes,
				)
			})

			if !plansNoChange(t, response, prior) {
				t.Error("plan changes the placement, want no change")
			}
		})
	}
}

func TestPlacementRespelledImagePlansNoChange(t *testing.T) {
	next := planTestPlacement()
	next.Spec.ServerSpec.ImageId = strings.ToUpper(planTestImageID)

	response, _ := planPlacement(t, planTestPlacement(), next, nil)

	if !plansNoChange(t, response, planTestPlacement()) {
		t.Error("plan changes the placement, want no change")
	}
}

func TestPlacementServerSpecChangesPlanReplacement(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*reservationapi.PlacementV2Read)
	}{
		{"ssh certificate authority", func(p *reservationapi.PlacementV2Read) {
			p.Spec.ServerSpec.SshCertificateAuthorityId = new("ca-1")
		}},
		{"user data", func(p *reservationapi.PlacementV2Read) {
			p.Spec.ServerSpec.UserData = &[]byte{'x'}
		}},
		{"networking removed", func(p *reservationapi.PlacementV2Read) {
			p.Spec.ServerSpec.Networking = nil
		}},
		{"security groups", func(p *reservationapi.PlacementV2Read) {
			p.Spec.ServerSpec.Networking.SecurityGroups = &[]string{"sg-2"}
		}},
		{"policy", func(p *reservationapi.PlacementV2Read) {
			p.Spec.Constraints.Policy = reservationapi.PlacementPolicyV2Spread
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			next := planTestPlacement()
			next.Spec.ServerSpec.ImageId = planTestUpdatedImageID
			testCase.mutate(next)

			response, _ := planPlacement(t, planTestPlacement(), next, nil)

			if len(response.RequiresReplace) == 0 {
				t.Error("RequiresReplace = none, want a replacement")
			}
		})
	}
}

func TestPlacementUnknownObjectsPlanReplacement(t *testing.T) {
	unknownServerNetworking := types.ObjectValueMust(
		reservation.PlacementServerSpecModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"image_id":                     uuidtype.NewValue(planTestImageID),
			"ssh_certificate_authority_id": types.StringNull(),
			"user_data":                    types.StringNull(),
			"networking": types.ObjectUnknown(
				reservation.PlacementServerNetworkingModelAttributeType.AttrTypes,
			),
		},
	)

	testCases := []struct {
		name        string
		makeUnknown func(model *reservation.PlacementResourceModel)
	}{
		{"constraints", func(model *reservation.PlacementResourceModel) {
			model.Constraints = types.ObjectUnknown(reservation.PlacementConstraintsModelAttributeType.AttrTypes)
		}},
		{"server_spec", func(model *reservation.PlacementResourceModel) {
			model.ServerSpec = types.ObjectUnknown(reservation.PlacementServerSpecModelAttributeType.AttrTypes)
		}},
		{"networking", func(model *reservation.PlacementResourceModel) {
			model.ServerSpec = unknownServerNetworking
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response, _ := planPlacement(
				t,
				planTestPlacement(),
				planTestPlacement(),
				func(config, proposed *reservation.PlacementResourceModel) {
					testCase.makeUnknown(config)
					testCase.makeUnknown(proposed)
				},
			)

			if len(response.RequiresReplace) == 0 {
				t.Error("RequiresReplace = none, want a replacement")
			}
		})
	}
}

func TestPlacementMetadataChangesPlanReplacement(t *testing.T) {
	testCases := []struct {
		name   string
		prior  func(*reservationapi.PlacementV2Read)
		mutate func(*reservationapi.PlacementV2Read)
	}{
		{
			"description removed",
			func(p *reservationapi.PlacementV2Read) { p.Metadata.Description = new("workers") },
			func(p *reservationapi.PlacementV2Read) { p.Metadata.Description = nil },
		},
		{
			"description changed",
			func(p *reservationapi.PlacementV2Read) { p.Metadata.Description = new("workers") },
			func(p *reservationapi.PlacementV2Read) { p.Metadata.Description = new("trainers") },
		},
		{
			"tags changed",
			func(p *reservationapi.PlacementV2Read) {
				p.Metadata.Tags = &[]reservationapi.Tag{{Name: "workload", Value: "training"}}
			},
			func(p *reservationapi.PlacementV2Read) {
				p.Metadata.Tags = &[]reservationapi.Tag{{Name: "workload", Value: "inference"}}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			prior := planTestPlacement()
			testCase.prior(prior)

			next := planTestPlacement()
			testCase.prior(next)
			testCase.mutate(next)

			response, _ := planPlacement(t, prior, next, nil)

			if len(response.RequiresReplace) == 0 {
				t.Error("RequiresReplace = none, want a replacement")
			}
		})
	}
}

func TestPlacementRollingUpdateWithoutBudgetFailsValidation(t *testing.T) {
	ctx := context.Background()

	var schemaResponse resource.SchemaResponse
	reservation.NewPlacementResource().Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
	schemaType := schemaResponse.Schema.Type().TerraformType(ctx)

	placement := planTestPlacement()
	placement.Spec.UpdateStrategy = &reservationapi.PlacementUpdateStrategyV2{
		Type: reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate,
	}

	model := reservation.PlacementResourceModel{
		PlacementModel: reservation.NewPlacementModel(placement),
		Timeouts: tftimeouts.Value{
			Object: types.ObjectNull(map[string]attr.Type{
				"create": types.StringType,
				"update": types.StringType,
				"delete": types.StringType,
			}),
		},
	}

	plan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: tftypes.NewValue(schemaType, nil)}
	if diagnostics := plan.Set(ctx, model); diagnostics.HasError() {
		t.Fatalf("encode model: %v", diagnostics)
	}

	value, err := tfprotov6.NewDynamicValue(schemaType, plan.Raw)
	if err != nil {
		t.Fatalf("encode model: %v", err)
	}

	server, err := providerserver.NewProtocol6WithError(provider.New())()
	if err != nil {
		t.Fatalf("provider server: %v", err)
	}

	response, err := server.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{
		TypeName: "nscale_placement",
		Config:   &value,
	})
	if err != nil {
		t.Fatalf("ValidateResourceConfig() error = %v", err)
	}

	want := `update_strategy of type RollingUpdate requires rolling_update.max_unavailable, e.g. "1" or "25%"`
	for _, diagnostic := range response.Diagnostics {
		if diagnostic.Severity == tfprotov6.DiagnosticSeverityError && diagnostic.Detail == want {
			return
		}
	}

	t.Errorf("ValidateResourceConfig() diagnostics = %v, want an error %q", response.Diagnostics, want)
}
