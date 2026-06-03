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

var _ datasource.DataSourceWithConfigure = &ProjectDataSource{}

// ProjectDataSource embeds the generic read+map base; only Schema and the
// adapter wiring below are project-specific.
type ProjectDataSource struct {
	*nscale.GenericDataSource[ProjectModel, identityapi.ProjectRead]
}

func NewProjectDataSource() datasource.DataSource {
	return &ProjectDataSource{
		GenericDataSource: nscale.NewGenericDataSource(
			nscale.DataSourceAdapter[ProjectModel, identityapi.ProjectRead]{
				TypeNameSuffix: "_identity_project",
				Title:          "Project",
				Name:           "project",
				Get: func(ctx context.Context, client *nscale.Client, id string) (*identityapi.ProjectRead, error) {
					return getProject(ctx, id, client)
				},
				ToModel:     NewProjectModel,
				IDFromModel: func(m ProjectModel) string { return m.ID.ValueString() },
			},
		),
	}
}

func (s *ProjectDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Identity Project",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the project.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the project.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the project.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the project.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"group_ids": schema.SetAttribute{
				MarkdownDescription: "The set of group identifiers that are granted access to the project.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the project was created.",
				Computed:            true,
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the project.",
				Computed:            true,
			},
		},
	}
}
