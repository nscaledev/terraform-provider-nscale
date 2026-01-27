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

package instance

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &InstanceDataSource{}

type InstanceDataSource struct {
	client *nscale.Client
}

func NewInstanceDataSource() datasource.DataSource {
	return &InstanceDataSource{}
}

func (s *InstanceDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (s *InstanceDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_instance"
}

func (s *InstanceDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Instance",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the instance.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the instance.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the instance.",
				Computed:            true,
			},
			"user_data": schema.StringAttribute{
				MarkdownDescription: "The data to pass to the instance at boot time.",
				Computed:            true,
			},
			"public_ip": schema.StringAttribute{
				MarkdownDescription: "The public IP address assigned to the instance.",
				Computed:            true,
			},
			"private_ip": schema.StringAttribute{
				MarkdownDescription: "The private IP address assigned to the instance.",
				Computed:            true,
			},
			"power_state": schema.StringAttribute{
				MarkdownDescription: "The power state of the instance.",
				Computed:            true,
			},
			"image_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the image used for the instance.",
				Computed:            true,
			},
			"flavor_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the flavor used for the instance.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the instance is provisioned.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the instance was created.",
				Computed:            true,
			},
		},
		Blocks: map[string]schema.Block{
			"network_interface": schema.SingleNestedBlock{
				MarkdownDescription: "The network interface configuration of the instance.",
				Attributes: map[string]schema.Attribute{
					"network_id": schema.StringAttribute{
						MarkdownDescription: "The identifier of the network to where the instance is provisioned.",
						Computed:            true,
					},
					"enable_public_ip": schema.BoolAttribute{
						MarkdownDescription: "Indicates whether the instance has a public IP.",
						Computed:            true,
					},
					"security_group_ids": schema.ListAttribute{
						MarkdownDescription: "A list of security group identifiers associated with the instance.",
						ElementType:         types.StringType,
						Computed:            true,
					},
					"allowed_destinations": schema.ListAttribute{
						MarkdownDescription: "A list of CIDR blocks that are allowed to egress from the instance without SNAT.",
						ElementType:         types.StringType,
						Computed:            true,
					},
				},
			},
		},
	}
}

func (s *InstanceDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[InstanceModel](ctx, request.Config.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	instance, _, err := getInstance(ctx, data.ID.ValueString(), s.client)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Instance",
			fmt.Sprintf("An error occurred while retrieving the instance: %s", err),
		)
		return
	}

	data = NewInstanceModel(instance)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
