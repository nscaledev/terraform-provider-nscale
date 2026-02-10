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
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &NetworkDataSource{}

type NetworkDataSource struct {
	client *nscale.Client
}

func NewNetworkDataSource() datasource.DataSource {
	return &NetworkDataSource{}
}

func (s *NetworkDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	client, ok := request.ProviderData.(*nscale.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configuration Type",
			fmt.Sprintf("Expected *nscale.Client, got: %T. Please contact the Nscale team for support.", request.ProviderData),
		)
		return
	}

	s.client = client
}

func (s *NetworkDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_network"
}

func (s *NetworkDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Network",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the network.",
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

func (s *NetworkDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[NetworkModel](ctx, request.Config.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	network, _, err := getNetwork(ctx, data.ID.ValueString(), s.client)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Network",
			fmt.Sprintf("An error occurred while retrieving the network: %s", err),
		)
		return
	}

	data = NewNetworkModel(network)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
