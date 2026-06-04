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

package securitygroup

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &SecurityGroupDataSource{}

// SecurityGroupDataSource embeds the generic read+map base; only Schema and the
// adapter wiring below are security-group-specific.
type SecurityGroupDataSource struct {
	*nscale.GenericDataSource[SecurityGroupModel, regionapi.SecurityGroupV2Read]
}

func NewSecurityGroupDataSource() datasource.DataSource {
	return &SecurityGroupDataSource{
		GenericDataSource: nscale.NewGenericDataSource(
			nscale.DataSourceAdapter[SecurityGroupModel, regionapi.SecurityGroupV2Read]{
				TypeNameSuffix: "_security_group",
				Title:          "Security Group",
				Name:           "security group",
				Get: func(ctx context.Context, client *nscale.Client, id string) (*regionapi.SecurityGroupV2Read, error) {
					sg, _, err := getSecurityGroup(ctx, id, client)
					return sg, err
				},
				ToModel:     NewSecurityGroupModel,
				IDFromModel: func(m SecurityGroupModel) string { return m.ID.ValueString() },
			},
		),
	}
}

func (s *SecurityGroupDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Security Group",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the security group.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the security group.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the security group.",
				Computed:            true,
			},
			"rules": schema.ListNestedAttribute{
				MarkdownDescription: "A list of rules associated with the security group.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							MarkdownDescription: "The type of the security group rule.",
							Computed:            true,
						},
						"protocol": schema.StringAttribute{
							MarkdownDescription: "The protocol for the security group rule.",
							Computed:            true,
						},
						"from_port": schema.Int32Attribute{
							MarkdownDescription: "The starting port of the port range for the security group rule.",
							Computed:            true,
						},
						"to_port": schema.Int32Attribute{
							MarkdownDescription: "The ending port of the port range for the security group rule.",
							Computed:            true,
						},
						"cidr_block": schema.StringAttribute{
							MarkdownDescription: "The CIDR block for the security group rule.",
							Computed:            true,
						},
					},
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the network to which the security group is attached.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the security group.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the security group is provisioned.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the security group was created.",
				Computed:            true,
			},
		},
	}
}
