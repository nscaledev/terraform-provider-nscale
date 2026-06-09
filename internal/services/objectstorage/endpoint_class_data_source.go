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

package objectstorage

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &ObjectStorageEndpointClassDataSource{}

type ObjectStorageEndpointClassDataSource struct {
	client *nscale.Client
}

func NewObjectStorageEndpointClassDataSource() datasource.DataSource {
	return &ObjectStorageEndpointClassDataSource{}
}

func (s *ObjectStorageEndpointClassDataSource) Configure(
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

func (s *ObjectStorageEndpointClassDataSource) Metadata(
	ctx context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_object_storage_endpoint_class"
}

func (s *ObjectStorageEndpointClassDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Retrieves an object storage endpoint class by its unique identifier. Endpoint classes determine an endpoint's exposure type (public/private) and regional availability.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the endpoint class.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the endpoint class.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the endpoint class.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the endpoint class is available. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
			},
			"supported_endpoint_types": schema.ListAttribute{
				MarkdownDescription: "Endpoint exposure types supported by this class. Possible values are `public` and `private`.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the endpoint class was created.",
				Computed:            true,
			},
		},
	}
}

func (s *ObjectStorageEndpointClassDataSource) setDefaultRegionID(data *ObjectStorageEndpointClassModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(s.client.RegionID)
	}
}

func (s *ObjectStorageEndpointClassDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ObjectStorageEndpointClassModel](
		ctx,
		request.Config.Get,
		s.setDefaultRegionID,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	regionID := data.RegionID.ValueString()
	if regionID == "" {
		response.Diagnostics.AddError(
			"Missing Region ID",
			"A region ID is required to look up an object storage endpoint class. Either set `region_id` on the data source or configure `region_id` on the provider.",
		)
		return
	}

	params := &storageapi.GetApiV1ObjectstorageendpointclassesParams{
		RegionID: &storageapi.RegionIDQueryParameter{regionID},
	}

	listResponse, err := s.client.Storage.GetApiV1Objectstorageendpointclasses(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Object Storage Endpoint Class",
			fmt.Sprintf("An error occurred while listing object storage endpoint classes: %s", err),
		)
		return
	}
	defer listResponse.Body.Close()

	classes, err := nscale.ReadJSONResponseValue[[]storageapi.ObjectStorageEndpointClassRead](listResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Object Storage Endpoint Class",
			fmt.Sprintf("An error occurred while listing object storage endpoint classes: %s", err),
		)
		return
	}

	id := data.ID.ValueString()
	for _, class := range classes {
		if class.Metadata.Id == id {
			data = NewObjectStorageEndpointClassModel(&class)
			response.Diagnostics.Append(response.State.Set(ctx, data)...)
			return
		}
	}

	response.Diagnostics.AddError(
		"Object Storage Endpoint Class Not Found",
		fmt.Sprintf("The endpoint class with ID %s was not found in region %s.", id, regionID),
	)
}
