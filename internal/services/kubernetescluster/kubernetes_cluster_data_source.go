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

package kubernetescluster

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &KubernetesClusterDataSource{}

type KubernetesClusterDataSource struct {
	client *nscale.Client
}

func NewKubernetesClusterDataSource() datasource.DataSource {
	return &KubernetesClusterDataSource{}
}

func (s *KubernetesClusterDataSource) Configure(
	_ context.Context,
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

func (s *KubernetesClusterDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_kubernetes_cluster"
}

func (s *KubernetesClusterDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Kubernetes Cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique identifier of the cluster to look up.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the cluster.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the cluster.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region network the cluster attaches to.",
				Computed:            true,
			},
			"platform_release_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the NKS platform release the cluster runs.",
				Computed:            true,
			},
			"api_server": schema.SingleNestedAttribute{
				MarkdownDescription: "Network exposure configured for the cluster's Kubernetes API server.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"public_ip": schema.BoolAttribute{
						MarkdownDescription: "Whether the API server is exposed through a public endpoint.",
						Computed:            true,
					},
					"allowed_cidrs": schema.SetAttribute{
						MarkdownDescription: "Source IPv4 CIDR allowlist for the cluster API endpoint.",
						ElementType:         types.StringType,
						Computed:            true,
					},
				},
			},
			"cluster_network": schema.SingleNestedAttribute{
				MarkdownDescription: "Pod and service network CIDRs for the cluster.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"pod_cidr": schema.StringAttribute{
						MarkdownDescription: "IPv4 CIDR used for Kubernetes pod addresses.",
						Computed:            true,
					},
					"service_cidr": schema.StringAttribute{
						MarkdownDescription: "IPv4 CIDR used for Kubernetes service addresses.",
						Computed:            true,
					},
				},
			},
			"addons": schema.SingleNestedAttribute{
				MarkdownDescription: "Addon profiles enabled on the cluster.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"hardware": schema.BoolAttribute{
						MarkdownDescription: "Whether the optional hardware addon profile is enabled.",
						Computed:            true,
					},
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project the cluster belongs to.",
				Computed:            true,
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the organization the cluster belongs to.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region the cluster is provisioned in.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the cluster was created.",
				Computed:            true,
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning state of the cluster.",
				Computed:            true,
			},
			"health_status": schema.StringAttribute{
				MarkdownDescription: "The health state of the cluster.",
				Computed:            true,
			},
			"kubernetes_version_target": schema.StringAttribute{
				MarkdownDescription: "The Kubernetes version applied to the cluster's control plane.",
				Computed:            true,
			},
			"kubernetes_version_observed": schema.StringAttribute{
				MarkdownDescription: "The Kubernetes version reported by the managed control plane.",
				Computed:            true,
			},
			"api_server_endpoint": schema.SingleNestedAttribute{
				MarkdownDescription: "Credential-free connection data for the cluster's Kubernetes API server. " +
					"Null until the control plane is reachable.",
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"certificate_authority_data": schema.StringAttribute{
						MarkdownDescription: "The complete Kubernetes API server CA bundle, base64-encoded.",
						Computed:            true,
					},
					"private_host": schema.StringAttribute{
						MarkdownDescription: "The address of the private API server endpoint.",
						Computed:            true,
					},
					"private_port": schema.Int64Attribute{
						MarkdownDescription: "The TCP port of the private API server endpoint.",
						Computed:            true,
					},
					"public_host": schema.StringAttribute{
						MarkdownDescription: "The address of the public API server endpoint.",
						Computed:            true,
					},
					"public_port": schema.Int64Attribute{
						MarkdownDescription: "The TCP port of the public API server endpoint.",
						Computed:            true,
					},
				},
			},
			"applied_platform_release_id": schema.StringAttribute{
				MarkdownDescription: "The platform release last observed as applied to the cluster.",
				Computed:            true,
			},
			"platform_release_deprecated": schema.BoolAttribute{
				MarkdownDescription: "Whether the applied platform release is currently deprecated.",
				Computed:            true,
			},
			"platform_release_withdrawn": schema.BoolAttribute{
				MarkdownDescription: "Whether operators have withdrawn the applied platform release.",
				Computed:            true,
			},
			"upgrade_available": schema.BoolAttribute{
				MarkdownDescription: "Whether at least one eligible platform release upgrade target was observed.",
				Computed:            true,
			},
			"eligible_upgrade_target_ids": schema.ListAttribute{
				MarkdownDescription: "Eligible platform release IDs, in upgrade order.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (s *KubernetesClusterDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var data KubernetesClusterModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	cluster, err := getCluster(ctx, s.client, data.ID.ValueString())
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Kubernetes Cluster",
			fmt.Sprintf("An error occurred while retrieving the cluster: %s", err),
		)
		return
	}

	data = NewKubernetesClusterModel(cluster)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
