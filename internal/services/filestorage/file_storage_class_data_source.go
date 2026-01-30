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

package filestorage

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

var _ datasource.DataSourceWithConfigure = &FileStorageClassDataSource{}

type FileStorageClassDataSource struct {
	client *nscale.Client
}

func NewFileStorageClassDataSource() datasource.DataSource {
	return &FileStorageClassDataSource{}
}

func (s *FileStorageClassDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (s *FileStorageClassDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_file_storage_class"
}

func (s *FileStorageClassDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale File Storage Class",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the file storage class.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the file storage class.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the file storage class.",
				Computed:            true,
			},
			"protocols": schema.ListAttribute{
				MarkdownDescription: "A list of protocols supported by the file storage class.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the file storage class is available. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *FileStorageClassDataSource) setDefaultRegionID(data *FileStorageClassModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(r.client.RegionID)
	}
}

func (s *FileStorageClassDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[FileStorageClassModel](ctx, request.Config.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	regionID := data.RegionID.ValueString()

	params := &regionapi.GetApiV2FilestorageclassesParams{
		RegionID: &regionapi.RegionIDQueryParameter{
			regionID,
		},
	}

	storageClassListResponse, err := s.client.Region.GetApiV2Filestorageclasses(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read File Storage Class",
			fmt.Sprintf("An error occurred while retrieving the file storage class: %s", err),
		)
		return
	}

	storageClasses, err := nscale.ReadJSONResponseValue[[]regionapi.StorageClassV2Read](storageClassListResponse, nscale.StatusCodeAny(http.StatusOK))
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read File Storage Class",
			fmt.Sprintf("An error occurred while retrieving the file storage class: %s", err),
		)
		return
	}

	id := data.ID.ValueString()

	for _, storageClass := range storageClasses {
		if storageClass.Metadata.Id == id {
			data = NewFileStorageClassModel(&storageClass)
			response.Diagnostics.Append(response.State.Set(ctx, data)...)
			return
		}
	}

	response.Diagnostics.AddError(
		"File Storage Class Not Found",
		fmt.Sprintf("The file storage class with ID %s was not found in region %s on the server.", id, regionID),
	)
}
