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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	identityapi "github.com/nscaledev/nscale-sdk-go/identity"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &GroupDataSource{}

// GroupDataSource embeds the generic read+map base; only Schema and the
// adapter wiring below are group-specific.
type GroupDataSource struct {
	*nscale.GenericDataSource[GroupModel, identityapi.GroupRead]
}

func NewGroupDataSource() datasource.DataSource {
	return &GroupDataSource{
		GenericDataSource: nscale.NewGenericDataSource(
			nscale.DataSourceAdapter[GroupModel, identityapi.GroupRead]{
				TypeNameSuffix: "_identity_group",
				Title:          "Group",
				Name:           "group",
				Get: func(ctx context.Context, client *nscale.Client, id string) (*identityapi.GroupRead, error) {
					return getGroup(ctx, id, client)
				},
				ToModel:     NewGroupModel,
				IDFromModel: func(m GroupModel) string { return m.ID.ValueString() },
			},
		),
	}
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
