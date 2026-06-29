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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &ReservationDataSource{}

// ReservationDataSource embeds the generic read+map base; only Schema and the
// adapter wiring below are reservation-specific.
type ReservationDataSource struct {
	*nscale.GenericDataSource[ReservationModel, reservationapi.ReservationV2Read]
}

func NewReservationDataSource() datasource.DataSource {
	return &ReservationDataSource{
		GenericDataSource: nscale.NewGenericDataSource(
			nscale.DataSourceAdapter[ReservationModel, reservationapi.ReservationV2Read]{
				TypeNameSuffix: "_reservation",
				Title:          "Reservation",
				Name:           "reservation",
				Get: func(ctx context.Context, client *nscale.Client, id string) (*reservationapi.ReservationV2Read, error) {
					return getReservation(ctx, id, client)
				},
				ToModel:     NewReservationModel,
				IDFromModel: func(m ReservationModel) string { return m.ID.ValueString() },
			},
		),
	}
}

func (s *ReservationDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Reservation",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the reservation.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the reservation.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the reservation.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the reservation.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region the capacity is reserved in.",
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project the reservation is provisioned in.",
				Computed:            true,
			},
			"accelerator": schema.StringAttribute{
				MarkdownDescription: "The public accelerator model or family reserved.",
				Computed:            true,
			},
			"unit": schema.StringAttribute{
				MarkdownDescription: "The public reservation granularity reserved.",
				Computed:            true,
			},
			"unit_count": schema.Int64Attribute{
				MarkdownDescription: "The number of reservation units reserved.",
				Computed:            true,
			},
			"machine_flavor_id": schema.StringAttribute{
				MarkdownDescription: "The resolved Region machine flavor used for pinned servers.",
				Computed:            true,
			},
			"claimed_unit_count": schema.Int64Attribute{
				MarkdownDescription: "The number of reservation units successfully claimed.",
				Computed:            true,
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
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the reservation.",
				Computed:            true,
			},
		},
	}
}
