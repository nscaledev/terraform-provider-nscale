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

package region

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &RegionDataSource{}

type RegionDataSource struct {
	client *nscale.Client
}

func NewRegionDataSource() datasource.DataSource {
	return &RegionDataSource{}
}

func (s *RegionDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (s *RegionDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_region"
}

func (s *RegionDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Network",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the region.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the region.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "A description of the region.",
				Computed:            true,
			},
		},
	}
}

func (s *RegionDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data RegionModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	regionListResponse, err := s.client.Region.GetApiV1OrganizationsOrganizationIDRegionsWithResponse(ctx, s.client.OrganizationID)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Region",
			fmt.Sprintf("An error occurred while retriving the region: %s", err),
		)
		return
	}

	if regionListResponse.StatusCode() != http.StatusOK || regionListResponse.JSON200 == nil {
		response.Diagnostics.AddError(
			"Failed to Read Region",
			fmt.Sprintf("An error occurred while retrieving the region (status %d).", regionListResponse.StatusCode()),
		)
		return
	}

	id := data.ID.ValueString()

	for _, region := range *regionListResponse.JSON200 {
		if region.Metadata.Id == id {
			data = NewRegionModel(&region)
			response.Diagnostics.Append(response.State.Set(ctx, data)...)
			return
		}
	}

	response.Diagnostics.AddError(
		"Region Not Found",
		fmt.Sprintf("The region with ID %s was not found on the server.", id),
	)
}
