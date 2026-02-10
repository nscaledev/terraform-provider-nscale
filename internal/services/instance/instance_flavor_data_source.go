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

package instance

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

var _ datasource.DataSourceWithConfigure = &InstanceFlavorDataSource{}

type InstanceFlavorDataSource struct {
	client *nscale.Client
}

func NewInstanceFlavorDataSource() datasource.DataSource {
	return &InstanceFlavorDataSource{}
}

func (s *InstanceFlavorDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (s *InstanceFlavorDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_instance_flavor"
}

func (s *InstanceFlavorDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Instance Flavor",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the instance flavor.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the instance flavor.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the instance flavor.",
				Computed:            true,
			},
			"cpus": schema.Int64Attribute{
				MarkdownDescription: "The number of CPUs allocated to the instance flavor.",
				Computed:            true,
			},
			"memory_size": schema.Int64Attribute{
				// FIXME: Use a single unit consistently (GiB or GB) instead of mixing them.
				MarkdownDescription: "The memory allocated to the instance flavor, in gibibytes.",
				Computed:            true,
			},
			"disk_size": schema.Int64Attribute{
				// FIXME: Use a single unit consistently (GiB or GB) instead of mixing them.
				MarkdownDescription: "The disk storage allocated to the instance flavor, in gigabytes.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the instance flavor is available. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
			},
		},
		Blocks: map[string]schema.Block{
			"gpu": schema.SingleNestedBlock{
				MarkdownDescription: "The GPU configuration for the instance flavor, if available.",
				Attributes: map[string]schema.Attribute{
					"vendor": schema.StringAttribute{
						MarkdownDescription: "The manufacturer of the GPU.",
						Computed:            true,
					},
					"model": schema.StringAttribute{
						MarkdownDescription: "The model name of the GPU.",
						Computed:            true,
					},
					"physical_count": schema.Int64Attribute{
						MarkdownDescription: "The number of physical GPU devices available in the instance flavor.",
						Computed:            true,
					},
					"logical_count": schema.Int64Attribute{
						MarkdownDescription: "The total number of logical GPU units available for use.",
						Computed:            true,
					},
					"memory_size": schema.Int64Attribute{
						MarkdownDescription: "The memory available on the GPU, in gibibytes.",
						Computed:            true,
					},
				},
			},
		},
	}
}

func (s *InstanceFlavorDataSource) setDefaultRegionID(data *InstanceFlavorModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(s.client.RegionID)
	}
}

func (s *InstanceFlavorDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[InstanceFlavorModel](ctx, request.Config.Get, s.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	regionID := data.RegionID.ValueString()

	flavorListResponse, err := s.client.Region.GetApiV1OrganizationsOrganizationIDRegionsRegionIDFlavors(ctx, s.client.OrganizationID, regionID)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Instance Flavor",
			fmt.Sprintf("An error occurred while retrieving the instance flavor: %s", err),
		)
		return
	}

	flavors, err := nscale.ReadJSONResponseValue[[]regionapi.Flavor](flavorListResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Instance Flavor",
			fmt.Sprintf("An error occurred while retrieving the instance flavor: %s", err),
		)
		return
	}

	id := data.ID.ValueString()

	for _, region := range flavors {
		if region.Metadata.Id == id {
			data = NewInstanceFlavorModel(&region, regionID)
			response.Diagnostics.Append(response.State.Set(ctx, data)...)
			return
		}
	}

	response.Diagnostics.AddError(
		"Instance Flavor Not Found",
		fmt.Sprintf("The instance flavor with ID %s was not found in region %s on the server.", id, regionID),
	)
}
