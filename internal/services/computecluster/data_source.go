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

package computecluster

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &ComputeClusterDataSource{}

type ComputeClusterDataSource struct {
	client *nscale.Client
}

func NewComputeClusterDataSource() datasource.DataSource {
	return &ComputeClusterDataSource{}
}

func (s *ComputeClusterDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (s *ComputeClusterDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_compute_cluster"
}

func (s *ComputeClusterDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Compute Cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the compute cluster.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the compute cluster.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the compute cluster.",
				Computed:            true,
			},
			"workload_pools": schema.ListNestedAttribute{
				MarkdownDescription: "A list of pools of workload nodes in the compute cluster.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the workload pool.",
							Computed:            true,
						},
						"replicas": schema.Int64Attribute{
							MarkdownDescription: "The number of replicas (VMs) to provision in this workload pool.",
							Computed:            true,
						},
						"image_id": schema.StringAttribute{
							MarkdownDescription: "The identifier of the image used for initializing the boot disk of the workload pool VMs.",
							Computed:            true,
						},
						"flavor_id": schema.StringAttribute{
							MarkdownDescription: "The identifier of the flavor (machine type) used for the workload pool VMs.",
							Computed:            true,
						},
						//"disk_size": schema.Int64Attribute{
						//	MarkdownDescription: "The size of the boot disk for each VM in the workload pool, in GiB.",
						//	Computed:            true,
						//},
						"user_data": schema.StringAttribute{
							MarkdownDescription: "The data to pass to the VMs at boot time.",
							Computed:            true,
						},
						"enable_public_ip": schema.BoolAttribute{
							MarkdownDescription: "Whether to assign a public IP address to each VM in this workload pool.",
							Computed:            true,
						},
						"firewall_rules": schema.ListNestedAttribute{
							MarkdownDescription: "A list of firewall rules applied to the VMs in this workload pool.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"direction": schema.StringAttribute{
										MarkdownDescription: "The direction of the traffic to which this firewall rule applies.",
										Computed:            true,
									},
									"protocol": schema.StringAttribute{
										MarkdownDescription: "The IP protocol to which this firewall rule applies.",
										Computed:            true,
									},
									"ports": schema.StringAttribute{
										MarkdownDescription: "The ports to which this firewall rule applies. This can be a single port, or a range of ports.",
										Computed:            true,
									},
									"prefixes": schema.SetAttribute{
										MarkdownDescription: "A set of CIDR prefixes to which this firewall rule applies.",
										ElementType:         types.StringType,
										Computed:            true,
									},
								},
							},
						},
						"machines": schema.ListNestedAttribute{
							MarkdownDescription: "A list of machines in this workload pool.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"hostname": schema.StringAttribute{
										MarkdownDescription: "The hostname of the machine.",
										Computed:            true,
									},
									"private_ip": schema.StringAttribute{
										MarkdownDescription: "The private IP address of the machine.",
										Computed:            true,
									},
									"public_ip": schema.StringAttribute{
										MarkdownDescription: "The public IP address of the machine, if assigned.",
										Computed:            true,
									},
								},
							},
						},
					},
				},
			},
			"ssh_private_key": schema.StringAttribute{
				MarkdownDescription: "The SSH private key for accessing the compute cluster.",
				Computed:            true,
				Sensitive:           true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the compute cluster is provisioned.",
				Computed:            true,
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the compute cluster.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the compute cluster was created.",
				Computed:            true,
			},
		},
	}
}

func (s *ComputeClusterDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data ComputeClusterModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	clusterListResponse, err := s.client.Compute.GetApiV1OrganizationsOrganizationIDClustersWithResponse(ctx, s.client.OrganizationID, nil)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Compute Cluster",
			fmt.Sprintf("An error occurred while retrieving the compute cluster: %s", err),
		)
		return
	}

	id := data.ID.ValueString()

	if clusterListResponse.StatusCode() != http.StatusOK || clusterListResponse.JSON200 == nil {
		response.Diagnostics.AddError(
			"Failed to Read Compute Cluster",
			fmt.Sprintf("An error occurred while retrieving the compute cluster (status %d).", clusterListResponse.StatusCode()),
		)
		return
	}

	for _, cluster := range *clusterListResponse.JSON200 {
		if cluster.Metadata.Id == data.ID.ValueString() {
			data = NewComputeClusterModel(&cluster)
			response.Diagnostics.Append(response.State.Set(ctx, data)...)
			return
		}
	}

	response.Diagnostics.AddError(
		"Compute Cluster Not Found",
		fmt.Sprintf("The compute cluster with ID %s was not found on the server.", id),
	)
}
