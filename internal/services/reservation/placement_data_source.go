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

var _ datasource.DataSourceWithConfigure = &PlacementDataSource{}

// PlacementDataSource embeds the generic read+map base; only Schema and the
// adapter wiring below are placement-specific.
type PlacementDataSource struct {
	*nscale.GenericDataSource[PlacementModel, reservationapi.PlacementV2Read]
}

func NewPlacementDataSource() datasource.DataSource {
	return &PlacementDataSource{
		GenericDataSource: nscale.NewGenericDataSource(
			nscale.DataSourceAdapter[PlacementModel, reservationapi.PlacementV2Read]{
				TypeNameSuffix: "_placement",
				Title:          "Placement",
				Name:           "placement",
				Get: func(ctx context.Context, client *nscale.Client, id string) (*reservationapi.PlacementV2Read, error) {
					return getPlacement(ctx, id, client)
				},
				ToModel:     NewPlacementModel,
				IDFromModel: func(m PlacementModel) string { return m.ID.ValueString() },
			},
		),
	}
}

func (s *PlacementDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Placement",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the placement.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the placement.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the placement.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the placement.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"reservation_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the reservation the placement allocates from.",
				Computed:            true,
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the network the placement belongs to.",
				Computed:            true,
			},
			"host_count": schema.Int64Attribute{
				MarkdownDescription: "The number of hosts allocated from the reservation.",
				Computed:            true,
			},
			"constraints": schema.SingleNestedAttribute{
				MarkdownDescription: "The scheduling policy applied when selecting hosts from the reservation.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"policy": schema.StringAttribute{
						MarkdownDescription: "The placement policy.",
						Computed:            true,
					},
					"max_skew": schema.Int64Attribute{
						MarkdownDescription: "The maximum difference in host count between any two domains.",
						Computed:            true,
					},
					"min_domains": schema.Int64Attribute{
						MarkdownDescription: "The minimum number of domains that must receive at least one selected host.",
						Computed:            true,
					},
					"when_unsatisfiable": schema.StringAttribute{
						MarkdownDescription: "The behaviour when the constraint cannot be fully satisfied.",
						Computed:            true,
					},
				},
			},
			"server_spec": schema.SingleNestedAttribute{
				MarkdownDescription: "Region server options applied to each pinned server.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"image_id": schema.StringAttribute{
						MarkdownDescription: "The image used for each pinned server.",
						Computed:            true,
					},
					"ssh_certificate_authority_id": schema.StringAttribute{
						MarkdownDescription: "The SSH certificate authority ID.",
						Computed:            true,
					},
					"user_data": schema.StringAttribute{
						MarkdownDescription: "Base64-encoded configuration information or scripts used upon launch.",
						Computed:            true,
					},
					"networking": schema.SingleNestedAttribute{
						MarkdownDescription: "Region server networking options applied to each pinned server.",
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"enable_public_ip": schema.BoolAttribute{
								MarkdownDescription: "Whether or not a public IP is provisioned for each server.",
								Computed:            true,
							},
							"security_group_ids": schema.ListAttribute{
								MarkdownDescription: "A list of security group IDs applied to each server.",
								ElementType:         types.StringType,
								Computed:            true,
							},
							"allowed_source_addresses": schema.ListAttribute{
								MarkdownDescription: "A list of network prefixes that are allowed to egress from each server.",
								ElementType:         types.StringType,
								Computed:            true,
							},
						},
					},
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region the placement belongs to.",
				Computed:            true,
			},
			"ready_host_count": schema.Int64Attribute{
				MarkdownDescription: "The number of hosts whose Region server resources are ready.",
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project the placement is provisioned in.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the placement was created.",
				Computed:            true,
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the placement.",
				Computed:            true,
			},
		},
	}
}
