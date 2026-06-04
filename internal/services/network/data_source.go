/*
Copyright 2025 Nscale

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

package network

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &NetworkDataSource{}

// NetworkDataSource embeds the generic read+map base; only Schema and the
// adapter wiring below are network-specific.
type NetworkDataSource struct {
	*nscale.GenericDataSource[NetworkModel, regionapi.NetworkV2Read]
}

func NewNetworkDataSource() datasource.DataSource {
	return &NetworkDataSource{
		GenericDataSource: nscale.NewGenericDataSource(
			nscale.DataSourceAdapter[NetworkModel, regionapi.NetworkV2Read]{
				TypeNameSuffix: "_network",
				Title:          "Network",
				Name:           "network",
				Get: func(ctx context.Context, client *nscale.Client, id string) (*regionapi.NetworkV2Read, error) {
					network, _, err := getNetwork(ctx, id, client)
					return network, err
				},
				ToModel:     NewNetworkModel,
				IDFromModel: func(m NetworkModel) string { return m.ID.ValueString() },
			},
		),
	}
}

func (s *NetworkDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Network",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the network.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the network.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the network.",
				Computed:            true,
			},
			"dns_nameservers": schema.ListAttribute{
				MarkdownDescription: "A list of DNS nameservers associated with the network.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"routes": schema.ListNestedAttribute{
				MarkdownDescription: "A list of routes associated with the network.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"destination": schema.StringAttribute{
							MarkdownDescription: "The destination CIDR block for the route.",
							Computed:            true,
						},
						"nexthop": schema.StringAttribute{
							MarkdownDescription: "The next-hop address for the route.",
							Computed:            true,
						},
					},
				},
			},
			"cidr_block": schema.StringAttribute{
				MarkdownDescription: "The CIDR block assigned to the network.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the network.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the network is provisioned.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the network is provisioned.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the network was created.",
				Computed:            true,
			},
		},
	}
}
