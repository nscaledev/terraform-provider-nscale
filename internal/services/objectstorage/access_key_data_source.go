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

var _ datasource.DataSourceWithConfigure = &ObjectStorageAccessKeyDataSource{}

type ObjectStorageAccessKeyDataSource struct {
	client *nscale.Client
}

func NewObjectStorageAccessKeyDataSource() datasource.DataSource {
	return &ObjectStorageAccessKeyDataSource{}
}

func (s *ObjectStorageAccessKeyDataSource) Configure(
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

func (s *ObjectStorageAccessKeyDataSource) Metadata(
	ctx context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_object_storage_access_key"
}

func (s *ObjectStorageAccessKeyDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Retrieves information about an existing object storage access key. The S3 secret is intentionally not exposed by this data source — it is only available at creation time. Use the resource (and protect Terraform state) to manage it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the access key.",
				Required:            true,
			},
			"endpoint_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the object storage endpoint this access key belongs to.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the access key.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the access key.",
				Computed:            true,
			},
			"identity_policy": schema.StringAttribute{
				MarkdownDescription: "The identity policy name the access key is bound to.",
				Computed:            true,
			},
			"access_key_id": schema.StringAttribute{
				MarkdownDescription: "The S3 access key identifier.",
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project the access key belongs to.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the access key was created.",
				Computed:            true,
			},
		},
	}
}

// dataSourceModel mirrors ObjectStorageAccessKeyModel but excludes Secret —
// the data source intentionally never exposes the secret, even null. Adding
// a separate type means the schema and config struct are tightly coupled
// (any drift produces a compile error).
type dataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	EndpointID     types.String `tfsdk:"endpoint_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	IdentityPolicy types.String `tfsdk:"identity_policy"`
	AccessKeyID    types.String `tfsdk:"access_key_id"`
	ProjectID      types.String `tfsdk:"project_id"`
	CreationTime   types.String `tfsdk:"creation_time"`
}

func (s *ObjectStorageAccessKeyDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[dataSourceModel](ctx, request.Config.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	accessKey, _, err := getObjectStorageAccessKey(ctx, data.EndpointID.ValueString(), data.ID.ValueString(), s.client)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Object Storage Access Key",
			fmt.Sprintf("An error occurred while retrieving the access key: %s", err),
		)
		return
	}

	model := NewObjectStorageAccessKeyModel(accessKey)
	out := dataSourceModel{
		ID:             model.ID,
		EndpointID:     data.EndpointID,
		Name:           model.Name,
		Description:    model.Description,
		IdentityPolicy: model.IdentityPolicy,
		AccessKeyID:    model.AccessKeyID,
		ProjectID:      model.ProjectID,
		CreationTime:   model.CreationTime,
	}

	response.Diagnostics.Append(response.State.Set(ctx, &out)...)
}
