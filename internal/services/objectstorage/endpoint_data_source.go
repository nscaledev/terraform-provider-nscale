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

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &ObjectStorageEndpointDataSource{}

type ObjectStorageEndpointDataSource struct {
	client *nscale.Client
}

func NewObjectStorageEndpointDataSource() datasource.DataSource {
	return &ObjectStorageEndpointDataSource{}
}

func (s *ObjectStorageEndpointDataSource) Configure(
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

func (s *ObjectStorageEndpointDataSource) Metadata(
	ctx context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_object_storage_endpoint"
}

func (s *ObjectStorageEndpointDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Retrieves information about an existing object storage endpoint by its unique identifier.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the object storage endpoint.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the object storage endpoint.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the object storage endpoint.",
				Computed:            true,
			},
			"endpoint_class_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the endpoint class the endpoint was created with.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the object storage endpoint.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"identity_policies": schema.ListNestedAttribute{
				MarkdownDescription: "Identity policies configured on the endpoint.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The identity policy name.",
							Computed:            true,
						},
						"document": schema.StringAttribute{
							MarkdownDescription: "The identity policy document encoded as a JSON object string.",
							Computed:            true,
						},
					},
				},
			},
			"exposure": schema.SingleNestedAttribute{
				MarkdownDescription: "The externally reachable endpoints for the object storage endpoint.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"public": schema.SingleNestedAttribute{
						MarkdownDescription: "Connection details for the public exposure mode.",
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"dns_name": schema.StringAttribute{
								MarkdownDescription: "The DNS hostname clients use to reach the endpoint over the public network.",
								Computed:            true,
							},
						},
					},
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the object storage endpoint is provisioned.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the object storage endpoint is provisioned.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the object storage endpoint was created.",
				Computed:            true,
			},
		},
	}
}

func (s *ObjectStorageEndpointDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ObjectStorageEndpointModel](ctx, request.Config.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	endpoint, _, err := getObjectStorageEndpoint(ctx, data.ID.ValueString(), s.client)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Object Storage Endpoint",
			fmt.Sprintf("An error occurred while retrieving the object storage endpoint: %s", err),
		)
		return
	}

	endpointModel, modelDiags := NewObjectStorageEndpointModel(endpoint)
	if modelDiags.HasError() {
		response.Diagnostics.Append(modelDiags...)
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &endpointModel)...)
}
