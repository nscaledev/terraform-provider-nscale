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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &FileStorageDataSource{}

type FileStorageDataSource struct {
	client *nscale.Client
}

func NewFileStorageDataSource() datasource.DataSource {
	return &FileStorageDataSource{}
}

func (s *FileStorageDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (s *FileStorageDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_file_storage"
}

func (s *FileStorageDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale File Storage",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the file storage.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the file storage.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the file storage.",
				Computed:            true,
			},
			"storage_class_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the storage class assigned to the file storage.",
				Computed:            true,
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "The amount of storage currently used, in gibibytes.",
				Computed:            true,
			},
			"capacity": schema.Int64Attribute{
				MarkdownDescription: "The total capacity of the file storage, in gibibytes.",
				Computed:            true,
			},
			"root_squash": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether root squashing is enabled for the file storage.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the file storage is provisioned.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the file storage was created.",
				Computed:            true,
			},
		},
		Blocks: map[string]schema.Block{
			"network": schema.ListNestedBlock{
				MarkdownDescription: "The network to which the file storage is attached.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The identifier of the network to which the file storage is attached.",
							Computed:            true,
						},
						"mount_source": schema.StringAttribute{
							MarkdownDescription: "The network path for mounting the file storage.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (s *FileStorageDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data FileStorageModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	fileStorage, _, err := getFileStorage(ctx, data.ID.ValueString(), s.client)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read File Storage",
			fmt.Sprintf("An error occurred while retrieving the file storage: %s", err),
		)
		return
	}

	data = NewFileStorageModel(fileStorage)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
