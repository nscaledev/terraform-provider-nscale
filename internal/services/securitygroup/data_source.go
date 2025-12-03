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
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &SecurityGroupDataSource{}

type SecurityGroupDataSource struct {
	client *nscale.Client
}

func NewSecurityGroupDataSource() datasource.DataSource {
	return &SecurityGroupDataSource{}
}

func (s *SecurityGroupDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (s *SecurityGroupDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_security_group"
}

func (s *SecurityGroupDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Security Group",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the security group.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the security group.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "A description of the security group.",
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
				MarkdownDescription: "The identifier of the network to where the security group is attached.",
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

func (s *SecurityGroupDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data SecurityGroupModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	securityGroup, _, err := getSecurityGroup(ctx, data.ID.ValueString(), s.client)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Security Group",
			fmt.Sprintf("An error occurred while retreiving the security group: %s", err),
		)
		return
	}

	data = NewSecurityGroupModel(securityGroup)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
