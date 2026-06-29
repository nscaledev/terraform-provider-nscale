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

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.Resource                = &ReservationResource{}
	_ resource.ResourceWithConfigure   = &ReservationResource{}
	_ resource.ResourceWithImportState = &ReservationResource{}
)

type ReservationResourceModel struct {
	ReservationModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// ReservationResource embeds the generic CRUD base; only Schema and the adapter
// wiring below are reservation-specific. The reservation API has no update
// endpoint, so the adapter's Update is nil and every configurable attribute is
// marked for replacement.
type ReservationResource struct {
	*nscale.GenericResource[ReservationResourceModel, reservationapi.ReservationV2Read]
}

func NewReservationResource() resource.Resource {
	return &ReservationResource{
		GenericResource: nscale.NewGenericResource(reservationAdapter()),
	}
}

func reservationAdapter() nscale.ResourceAdapter[ReservationResourceModel, reservationapi.ReservationV2Read] {
	return nscale.ResourceAdapter[ReservationResourceModel, reservationapi.ReservationV2Read]{
		TypeNameSuffix: "_reservation",
		Title:          "Reservation",
		Name:           "reservation",
		Create:         reservationCreate,
		// Update is intentionally nil: reservations are immutable.
		Update: nil,
		Delete: reservationDelete,
		Get: func(
			ctx context.Context,
			client *nscale.Client,
			id string,
		) (*reservationapi.ReservationV2Read, nscale.ResourceStatus, error) {
			return getReservationStatus(ctx, id, client)
		},
		ToModel: func(api *reservationapi.ReservationV2Read, dst *ReservationResourceModel) {
			dst.ReservationModel = NewReservationModel(api)
		},
		IDFromModel:       func(m ReservationResourceModel) string { return m.ID.ValueString() },
		TimeoutsFromModel: func(m ReservationResourceModel) tftimeouts.Value { return m.Timeouts },
	}
}

func (r *ReservationResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Reservation",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the reservation.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the reservation. Changing this forces a new reservation to be created.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the reservation. Changing this forces a new reservation to be created.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the reservation. Changing this forces a new reservation to be created.",
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
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region to reserve capacity in. If not specified, this defaults to the region ID configured in the provider. Changing this forces a new reservation to be created.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project the reservation is provisioned in. If not specified, this defaults to the project ID configured in the provider. Changing this forces a new reservation to be created.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"accelerator": schema.StringAttribute{
				MarkdownDescription: "The public accelerator model or family to reserve, for example `GB300`. Changing this forces a new reservation to be created.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"unit": schema.StringAttribute{
				MarkdownDescription: "The public reservation granularity to reserve, for example `NVL72`. Changing this forces a new reservation to be created.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"unit_count": schema.Int64Attribute{
				MarkdownDescription: "The number of reservation units to reserve. Must be at least 1. Changing this forces a new reservation to be created.",
				Required:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"machine_flavor_id": schema.StringAttribute{
				MarkdownDescription: "The resolved Region machine flavor used for pinned servers.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"claimed_unit_count": schema.Int64Attribute{
				MarkdownDescription: "The number of reservation units successfully claimed.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"topology_hash": schema.StringAttribute{
				MarkdownDescription: "A hash of the claimed topology projection accepted by the reservation.",
				Computed:            true,
			},
			"topology_observed_at": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the claimed topology projection was last observed.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the reservation was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the reservation.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

// setDefaultIDs fills the project and region IDs from the provider configuration
// when the plan leaves them empty. The project ID is resolved through
// Client.ResolveProjectID so a configuration that sets neither a resource nor a
// provider-level project_id fails at plan time with a clear diagnostic rather
// than producing an invalid create request.
func setDefaultIDs(client *nscale.Client, data *ReservationResourceModel) diag.Diagnostics {
	projectID, diagnostics := client.ResolveProjectID(data.ProjectID.ValueString())
	if diagnostics.HasError() {
		return diagnostics
	}
	data.ProjectID = types.StringValue(projectID)

	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(client.RegionID)
	}

	return diagnostics
}

func reservationCreate(
	ctx context.Context,
	client *nscale.Client,
	plan ReservationResourceModel,
) (*reservationapi.ReservationV2Read, diag.Diagnostics) {
	if diagnostics := setDefaultIDs(client, &plan); diagnostics.HasError() {
		return nil, diagnostics
	}

	params, diagnostics := plan.NscaleReservationCreateParams(client.OrganizationID)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	createResponse, err := client.Reservation.CreateReservation(ctx, params)
	if err != nil {
		diagnostics.AddError(
			"Failed to Create Reservation",
			fmt.Sprintf("An error occurred while creating the reservation: %s", err),
		)
		return nil, diagnostics
	}
	defer createResponse.Body.Close()

	reservation, err := nscale.ReadJSONResponsePointer[reservationapi.ReservationV2Read](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Create Reservation",
			fmt.Sprintf("An error occurred while creating the reservation: %s", err),
		)
		return nil, diagnostics
	}

	return reservation, nil
}

func reservationDelete(ctx context.Context, client *nscale.Client, id string) error {
	deleteResponse, err := client.Reservation.DeleteReservation(ctx, id)
	if err != nil {
		return err
	}
	defer deleteResponse.Body.Close()

	return nscale.ReadEmptyResponse(deleteResponse)
}
