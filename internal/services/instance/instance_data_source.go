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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	computeapi "github.com/nscaledev/nscale-sdk-go/compute"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &InstanceDataSource{}

// InstanceDataSource embeds the generic read+map base; only Schema and the
// adapter wiring below are instance-specific.
type InstanceDataSource struct {
	*nscale.GenericDataSource[InstanceModel, computeapi.InstanceRead]
}

func NewInstanceDataSource() datasource.DataSource {
	return &InstanceDataSource{
		GenericDataSource: nscale.NewGenericDataSource(
			nscale.DataSourceAdapter[InstanceModel, computeapi.InstanceRead]{
				TypeNameSuffix: "_instance",
				Title:          "Instance",
				Name:           "instance",
				Get: func(ctx context.Context, client *nscale.Client, id string) (*computeapi.InstanceRead, error) {
					instance, _, err := getInstance(ctx, id, client)
					return instance, err
				},
				ToModel:     NewInstanceModel,
				IDFromModel: func(m InstanceModel) string { return m.ID.ValueString() },
			},
		),
	}
}

func (s *InstanceDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Instance",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the instance.",
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
			"ssh_certificate_authority_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the SSH certificate authority used to bootstrap login trust when the backing server is created.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the instance.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the instance is provisioned.",
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
						MarkdownDescription: "The identifier of the network where the instance is provisioned.",
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
