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

package identity

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &GroupDataSource{}

type GroupDataSource struct {
	client *nscale.Client
}

func NewGroupDataSource() datasource.DataSource {
	return &GroupDataSource{}
}

func (s *GroupDataSource) Configure(
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

func (s *GroupDataSource) Metadata(
	ctx context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_identity_group"
}

func (s *GroupDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Identity Group",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the group.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the group.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the group.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the group.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"role_ids": schema.SetAttribute{
				MarkdownDescription: "The set of role identifiers granted to members of this group.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"service_account_ids": schema.SetAttribute{
				MarkdownDescription: "The set of service account identifiers that are members of this group.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"user_ids": schema.SetAttribute{
				MarkdownDescription: "The set of user identifiers that are members of this group.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"subjects": schema.SetNestedAttribute{
				MarkdownDescription: "The set of federated identity subjects that are members of this group.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"issuer": schema.StringAttribute{
							MarkdownDescription: "The OIDC issuer URL that asserts this subject.",
							Computed:            true,
						},
						"id": schema.StringAttribute{
							MarkdownDescription: "The subject identifier issued by the issuer.",
							Computed:            true,
						},
						"email": schema.StringAttribute{
							MarkdownDescription: "The email address for the subject, when supplied by the issuer.",
							Computed:            true,
						},
					},
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the group was created.",
				Computed:            true,
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the group.",
				Computed:            true,
			},
		},
	}
}

func (s *GroupDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[GroupModel](ctx, request.Config.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	group, err := getGroup(ctx, data.ID.ValueString(), s.client)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Group",
			fmt.Sprintf("An error occurred while retrieving the group: %s", err),
		)
		return
	}

	data = NewGroupModel(group)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
