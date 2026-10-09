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

package reservation

import (
	"context"
	"fmt"
	"regexp"
	"time"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/uuidtype"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

// maxUnavailablePattern matches the API's count or percentage form of
// rolling_update.max_unavailable. Zero is rejected by the API, and a leading
// zero would read back in a different spelling.
var maxUnavailablePattern = regexp.MustCompile(`^[1-9][0-9]*%?$`)

var (
	_ resource.Resource                   = &PlacementResource{}
	_ resource.ResourceWithConfigure      = &PlacementResource{}
	_ resource.ResourceWithImportState    = &PlacementResource{}
	_ resource.ResourceWithModifyPlan     = &PlacementResource{}
	_ resource.ResourceWithValidateConfig = &PlacementResource{}
)

type PlacementResourceModel struct {
	PlacementModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// PlacementResource embeds the generic CRUD base. Only server_spec.image_id and
// update_strategy can change in place; every other configurable attribute is
// marked for replacement, as the API rejects any other change.
type PlacementResource struct {
	*nscale.GenericResource[PlacementResourceModel, reservationapi.PlacementV2Read]
}

const defaultPlacementUpdateTimeout = time.Hour

func NewPlacementResource() resource.Resource {
	return &PlacementResource{
		GenericResource: nscale.NewGenericResource(placementAdapter()),
	}
}

func placementAdapter() nscale.ResourceAdapter[PlacementResourceModel, reservationapi.PlacementV2Read] {
	return nscale.ResourceAdapter[PlacementResourceModel, reservationapi.PlacementV2Read]{
		TypeNameSuffix: "_placement",
		Title:          "Placement",
		Name:           "placement",
		Create:         placementCreate,
		// The update endpoint ignores tags, so the operation-tag watcher behind
		// Update cannot be used.
		Update:               nil,
		UpdateAndWait:        placementUpdateAndWait,
		DefaultUpdateTimeout: defaultPlacementUpdateTimeout,
		Delete:               placementDelete,
		Get: func(
			ctx context.Context,
			client *nscale.Client,
			id string,
		) (*reservationapi.PlacementV2Read, nscale.ResourceStatus, error) {
			return getPlacementStatus(ctx, id, client)
		},
		ToModel: func(api *reservationapi.PlacementV2Read, dst *PlacementResourceModel) {
			dst.PlacementModel = NewPlacementModel(api)
		},
		IDFromModel:       func(m PlacementResourceModel) string { return m.ID.ValueString() },
		TimeoutsFromModel: func(m PlacementResourceModel) tftimeouts.Value { return m.Timeouts },
	}
}

// ModifyPlan plans no change when nothing configured changes, as when the
// configuration respells the image UUID or leaves update_strategy unset.
func (r *PlacementResource) ModifyPlan(
	ctx context.Context,
	request resource.ModifyPlanRequest,
	response *resource.ModifyPlanResponse,
) {
	nscale.KeepStateWhenUnchanged(ctx, request, response)
}

func (r *PlacementResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Placement",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the placement.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the placement. Changing this forces a new placement to be created.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the placement. Changing this forces a new placement to be created.",
				Optional:            true,
				// Removing a configured description must replace too: the update
				// endpoint ignores metadata, so it cannot clear one.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the placement. Changing this forces a new placement to be created.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"reservation_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the reservation to allocate hosts from. Changing this forces a new placement to be created.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the network to attach hosts to. This also determines the InfiniBand partition boundary; all hosts in a placement share a single partition key. Changing this forces a new placement to be created.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"host_count": schema.Int64Attribute{
				MarkdownDescription: "The number of hosts to allocate from the reservation. Must be at least 1. Changing this forces a new placement to be created.",
				Required:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"constraints": schema.SingleNestedAttribute{
				MarkdownDescription: "The scheduling policy applied when selecting hosts from the reservation. Changing this forces a new placement to be created.",
				Required:            true,
				// Replacement is decided per attribute, not on the whole object: the
				// framework marks unconfigured computed attributes unknown whenever
				// the placement changes, and an object holding an unknown never
				// equals its prior state, so an image update would replace. Only a
				// wholly unknown object, whose attributes the framework does not
				// plan, replaces as a whole.
				PlanModifiers: []planmodifier.Object{
					requiresReplaceIfUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"policy": schema.StringAttribute{
						MarkdownDescription: "The placement policy. `pack` fills domains sequentially, maximising locality; `spread` distributes hosts as evenly as possible across domains.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.OneOf("pack", "spread"),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"max_skew": schema.Int64Attribute{
						MarkdownDescription: "The maximum difference in host count between any two domains. Applicable only when `policy` is `spread`.",
						Optional:            true,
						Computed:            true,
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
							int64planmodifier.RequiresReplaceIfConfigured(),
						},
					},
					"min_domains": schema.Int64Attribute{
						MarkdownDescription: "The minimum number of domains that must receive at least one selected host. Applicable only when `policy` is `spread`. Must be less than or equal to `host_count`.",
						Optional:            true,
						Computed:            true,
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
							int64planmodifier.RequiresReplaceIfConfigured(),
						},
					},
					"when_unsatisfiable": schema.StringAttribute{
						MarkdownDescription: "The behaviour when the constraint cannot be fully satisfied. `fail` returns an error; `bestEffort` applies the constraint as closely as it can.",
						Optional:            true,
						Computed:            true,
						Validators: []validator.String{
							stringvalidator.OneOf("fail", "bestEffort"),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							stringplanmodifier.RequiresReplaceIfConfigured(),
						},
					},
				},
			},
			"server_spec": schema.SingleNestedAttribute{
				MarkdownDescription: "Region server options applied to each pinned server. Changing `image_id` updates the placement in place; changing anything else forces a new placement to be created.",
				Required:            true,
				// Replacement is decided per attribute, as for constraints.
				PlanModifiers: []planmodifier.Object{
					requiresReplaceIfUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"image_id": schema.StringAttribute{
						MarkdownDescription: "The image to use for each pinned server. Changing this updates the placement in place: it marks the servers drifted, and they converge onto the new image according to the placement's update strategy. Under the default `Manual` strategy they stay on their current image until each is reconciled explicitly.",
						CustomType:          uuidtype.Type{},
						Required:            true,
						PlanModifiers: []planmodifier.String{
							uuidtype.UseStateForSameUUID(),
						},
					},
					"ssh_certificate_authority_id": schema.StringAttribute{
						MarkdownDescription: "The SSH certificate authority ID. Changing this forces a new placement to be created.",
						Optional:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"user_data": schema.StringAttribute{
						MarkdownDescription: "Base64-encoded configuration information or scripts to use upon launch. Changing this forces a new placement to be created.",
						Optional:            true,
						Validators: []validator.String{
							validators.Base64Validator{},
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"networking": schema.SingleNestedAttribute{
						MarkdownDescription: "Region server networking options applied to each pinned server. Changing this forces a new placement to be created.",
						Optional:            true,
						PlanModifiers: []planmodifier.Object{
							requiresReplaceIfPresenceChanges(),
						},
						Attributes: map[string]schema.Attribute{
							"enable_public_ip": schema.BoolAttribute{
								MarkdownDescription: "Whether or not to provision a public IP for each server.",
								Optional:            true,
								Computed:            true,
								PlanModifiers: []planmodifier.Bool{
									boolplanmodifier.UseStateForUnknown(),
									boolplanmodifier.RequiresReplaceIfConfigured(),
								},
							},
							"security_group_ids": schema.ListAttribute{
								MarkdownDescription: "A list of security group IDs to apply to each server.",
								ElementType:         types.StringType,
								Optional:            true,
								Computed:            true,
								PlanModifiers: []planmodifier.List{
									listplanmodifier.UseStateForUnknown(),
									listplanmodifier.RequiresReplaceIfConfigured(),
								},
							},
							"allowed_source_addresses": schema.ListAttribute{
								MarkdownDescription: "A list of network prefixes that are allowed to egress from each server. By default, only packets from the server's network interface's IP address are allowed to enter the network.",
								ElementType:         types.StringType,
								Optional:            true,
								Computed:            true,
								PlanModifiers: []planmodifier.List{
									listplanmodifier.UseStateForUnknown(),
									listplanmodifier.RequiresReplaceIfConfigured(),
								},
							},
						},
					},
				},
			},
			"update_strategy": schema.SingleNestedAttribute{
				MarkdownDescription: "How the placement's servers converge onto a new `server_spec.image_id`. The API defaults it to `Manual`. Changing this updates the placement in place; setting `RollingUpdate` on a placement whose servers are already drifted starts converging them. Removing it keeps the current strategy: set `type` to `Manual` to switch back.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						MarkdownDescription: "`Manual` leaves each server on its current image until it is reconciled explicitly. `RollingUpdate` converges drifted servers automatically, at most `rolling_update.max_unavailable` at a time.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.OneOf(
								string(reservationapi.PlacementUpdateStrategyTypeV2Manual),
								string(reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate),
							),
						},
					},
					"rolling_update": schema.SingleNestedAttribute{
						MarkdownDescription: "Tuning for the `RollingUpdate` strategy.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"max_unavailable": schema.StringAttribute{
								MarkdownDescription: "The number of servers that may converge at once, as a count (`\"1\"`) or a percentage of `host_count` (`\"25%\"`, rounded up). Must resolve to at least one server, and the service caps it. Required when `type` is `RollingUpdate`.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.RegexMatches(
										maxUnavailablePattern,
										"must be a count such as \"1\" or a percentage such as \"25%\"",
									),
								},
							},
						},
					},
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region the placement belongs to.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ready_host_count": schema.Int64Attribute{
				MarkdownDescription: "The number of hosts whose Region server resources are ready.",
				Computed:            true,
			},
			"updated_host_count": schema.Int64Attribute{
				MarkdownDescription: "The number of servers running `server_spec.image_id` and provisioned.",
				Computed:            true,
			},
			"drifted_host_count": schema.Int64Attribute{
				MarkdownDescription: "The number of servers not yet running `server_spec.image_id`, including servers still re-provisioning onto it.",
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project the placement is provisioned in.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the placement was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the placement.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// requiresReplaceIfUnknown replaces the placement when an object is wholly
// unknown at plan time. The framework does not run the plan modifiers of an
// unknown object's attributes, so they cannot decide; the object might change
// in any of them.
func requiresReplaceIfUnknown() planmodifier.Object {
	return objectplanmodifier.RequiresReplaceIf(
		func(
			_ context.Context,
			request planmodifier.ObjectRequest,
			response *objectplanmodifier.RequiresReplaceIfFuncResponse,
		) {
			response.RequiresReplace = request.PlanValue.IsUnknown()
		},
		"Changing this forces a new placement to be created.",
		"Changing this forces a new placement to be created.",
	)
}

// requiresReplaceIfPresenceChanges replaces the placement when an optional
// object is added, removed or wholly unknown. Changes within it are left to
// its attributes' own plan modifiers, which see their values after
// UseStateForUnknown.
func requiresReplaceIfPresenceChanges() planmodifier.Object {
	return objectplanmodifier.RequiresReplaceIf(
		func(
			_ context.Context,
			request planmodifier.ObjectRequest,
			response *objectplanmodifier.RequiresReplaceIfFuncResponse,
		) {
			response.RequiresReplace = request.PlanValue.IsUnknown() ||
				request.StateValue.IsNull() != request.PlanValue.IsNull()
		},
		"Adding or removing this forces a new placement to be created.",
		"Adding or removing this forces a new placement to be created.",
	)
}

// ValidateConfig surfaces constraint/policy mismatches at plan time rather than
// letting them fail as an API 400 at apply. The spread-only fields (max_skew,
// min_domains, when_unsatisfiable) are meaningful only when policy is "spread",
// and min_domains cannot exceed host_count. A RollingUpdate strategy must set
// its budget.
func (r *PlacementResource) ValidateConfig(
	ctx context.Context,
	request resource.ValidateConfigRequest,
	response *resource.ValidateConfigResponse,
) {
	var model PlacementResourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &model)...)
	if response.Diagnostics.HasError() {
		return
	}

	response.Diagnostics.Append(validatePlacementUpdateStrategy(ctx, model.UpdateStrategy)...)

	if model.Constraints.IsNull() || model.Constraints.IsUnknown() {
		return
	}

	var constraints PlacementConstraintsModel
	response.Diagnostics.Append(model.Constraints.As(ctx, &constraints, basetypes.ObjectAsOptions{})...)
	if response.Diagnostics.HasError() {
		return
	}

	response.Diagnostics.Append(validatePlacementConstraints(constraints, model.HostCount)...)
}

// validatePlacementConstraints holds the constraint/policy rules so they can be
// exercised directly in unit tests without constructing a tfsdk.Config.
func validatePlacementConstraints(
	constraints PlacementConstraintsModel,
	hostCount types.Int64,
) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	constraintsPath := path.Root("constraints")

	if constraints.Policy.ValueString() == "pack" {
		spreadOnly := []struct {
			name  string
			value attr.Value
		}{
			{"max_skew", constraints.MaxSkew},
			{"min_domains", constraints.MinDomains},
			{"when_unsatisfiable", constraints.WhenUnsatisfiable},
		}
		for _, field := range spreadOnly {
			if isKnownSet(field.value) {
				diagnostics.AddAttributeError(
					constraintsPath.AtName(field.name),
					"Invalid Placement Constraint",
					fmt.Sprintf("%q is only applicable when policy is \"spread\".", field.name),
				)
			}
		}
	}

	if isKnownSet(constraints.MinDomains) &&
		!hostCount.IsNull() && !hostCount.IsUnknown() &&
		constraints.MinDomains.ValueInt64() > hostCount.ValueInt64() {
		diagnostics.AddAttributeError(
			constraintsPath.AtName("min_domains"),
			"Invalid Placement Constraint",
			fmt.Sprintf(
				"min_domains (%d) must be less than or equal to host_count (%d).",
				constraints.MinDomains.ValueInt64(),
				hostCount.ValueInt64(),
			),
		)
	}

	return diagnostics
}

// validatePlacementUpdateStrategy requires a RollingUpdate strategy to set
// rolling_update.max_unavailable, so the service's own default never applies.
// Unknown values are deferred to apply-time validation.
func validatePlacementUpdateStrategy(ctx context.Context, updateStrategy types.Object) diag.Diagnostics {
	if !isKnownSet(updateStrategy) {
		return nil
	}

	var strategy PlacementUpdateStrategyModel
	diagnostics := updateStrategy.As(ctx, &strategy, basetypes.ObjectAsOptions{})
	if diagnostics.HasError() {
		return diagnostics
	}

	if strategy.Type.ValueString() != string(reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate) ||
		strategy.RollingUpdate.IsUnknown() {
		return diagnostics
	}

	if !strategy.RollingUpdate.IsNull() {
		var rollingUpdate PlacementRollingUpdateModel
		diagnostics.Append(strategy.RollingUpdate.As(ctx, &rollingUpdate, basetypes.ObjectAsOptions{})...)

		if diagnostics.HasError() || !rollingUpdate.MaxUnavailable.IsNull() {
			return diagnostics
		}
	}

	diagnostics.AddAttributeError(
		path.Root("update_strategy").AtName("rolling_update").AtName("max_unavailable"),
		"Missing Rolling Update Budget",
		`update_strategy of type RollingUpdate requires rolling_update.max_unavailable, e.g. "1" or "25%"`,
	)

	return diagnostics
}

// isKnownSet reports whether a configured value is present, i.e. neither null
// nor unknown. Unknown values are deferred to apply-time validation.
func isKnownSet(value attr.Value) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func placementCreate(
	ctx context.Context,
	client *nscale.Client,
	plan PlacementResourceModel,
) (*reservationapi.PlacementV2Read, diag.Diagnostics) {
	params, diagnostics := plan.NscalePlacementCreateParams(ctx)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	createResponse, err := client.Reservation.CreatePlacement(ctx, params)
	if err != nil {
		diagnostics.AddError(
			"Failed to Create Placement",
			fmt.Sprintf("An error occurred while creating the placement: %s", err),
		)
		return nil, diagnostics
	}
	defer createResponse.Body.Close()

	placement, err := nscale.ReadJSONResponsePointer[reservationapi.PlacementV2Read](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Create Placement",
			fmt.Sprintf("An error occurred while creating the placement: %s", err),
		)
		return nil, diagnostics
	}

	return placement, nil
}

func placementDelete(ctx context.Context, client *nscale.Client, id string) error {
	deleteResponse, err := client.Reservation.DeletePlacement(ctx, id)
	if err != nil {
		return err
	}
	defer deleteResponse.Body.Close()

	return nscale.ReadEmptyResponse(deleteResponse)
}
