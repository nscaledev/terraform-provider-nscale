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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &ReservationUnitDataSource{}

// ReservationUnitDataSource reads one reservation unit, identified by the
// (region, accelerator, unit) triple rather than by an id — reservation units
// are a regional offering rather than an owned resource, and the API has no id
// for them.
type ReservationUnitDataSource struct {
	client *nscale.Client
}

func NewReservationUnitDataSource() datasource.DataSource {
	return &ReservationUnitDataSource{}
}

func (s *ReservationUnitDataSource) Configure(
	ctx context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}

	client, ok := request.ProviderData.(*nscale.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configuration Type",
			fmt.Sprintf(
				"Expected *nscale.Client, got: %T. Please contact the Nscale team for support.",
				request.ProviderData,
			),
		)
		return
	}

	s.client = client
}

func (s *ReservationUnitDataSource) Metadata(
	ctx context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_reservation_unit"
}

func (s *ReservationUnitDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Reservation Unit",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A synthetic identifier for the reservation unit, formed as " +
					"`<region_id>/<accelerator>/<unit>`. The API issues no identifier for a " +
					"reservation unit, since it is a regional offering rather than an owned resource.",
				Computed: true,
			},
			"accelerator": schema.StringAttribute{
				MarkdownDescription: "The public accelerator model or family to look up, for example GB300.",
				Required:            true,
			},
			"unit": schema.StringAttribute{
				MarkdownDescription: "The public reservation granularity to look up, for example NVL72.",
				Required:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region to look the unit up in. If not specified, " +
					"this defaults to the region ID configured in the provider.",
				Optional: true,
				Computed: true,
			},
			"hosts_per_unit": schema.Int64Attribute{
				MarkdownDescription: "The number of hosts claimed by one reservation unit. Multiply by a " +
					"reservation's `claimed_unit_count` to size a placement's `host_count`.",
				Computed: true,
			},
			"machine_flavor_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the Region machine flavor resolved for this reservation unit.",
				Computed:            true,
			},
			"largest_contiguous_unit_count": schema.Int64Attribute{
				MarkdownDescription: "The largest contiguous reservation-unit count currently satisfiable, " +
					"which is the effective upper bound on a new reservation's `unit_count`. This reflects " +
					"capacity shared with every other consumer in the organization, so it can change " +
					"between a plan and an apply — treat it as advisory rather than a guarantee.",
				Computed: true,
			},
		},
	}
}

func (s *ReservationUnitDataSource) setDefaultRegionID(data *ReservationUnitModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(s.client.RegionID)
	}
}

func (s *ReservationUnitDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ReservationUnitModel](
		ctx,
		request.Config.Get,
		s.setDefaultRegionID,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	regionID := data.RegionID.ValueString()
	accelerator := data.Accelerator.ValueString()
	unit := data.Unit.ValueString()

	units, err := listOrganizationReservationUnits(ctx, s.client, regionID, accelerator, unit)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Reservation Unit",
			fmt.Sprintf("An error occurred while retrieving the reservation unit: %s", err),
		)
		return
	}

	// The request is already narrowed to one (region, accelerator, unit), but the
	// filters are advisory on the wire, so match exactly rather than taking the
	// first row: silently reporting another region's capacity would be worse than
	// reporting none.
	for _, candidate := range units {
		if candidate.RegionId == regionID &&
			candidate.Accelerator == accelerator &&
			candidate.Unit == unit {
			response.Diagnostics.Append(
				response.State.Set(ctx, NewReservationUnitModel(&candidate))...,
			)
			return
		}
	}

	response.Diagnostics.AddError(
		"Reservation Unit Not Found",
		fmt.Sprintf(
			"No %s %s reservation unit is offered to this organization in region %s. Accelerator and "+
				"unit combinations are region-specific, and an organization only sees regions it holds "+
				"an allocation in.",
			accelerator,
			unit,
			regionID,
		),
	)
}
