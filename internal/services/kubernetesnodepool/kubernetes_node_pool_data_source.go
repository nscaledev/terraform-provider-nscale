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

package kubernetesnodepool

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &KubernetesNodePoolDataSource{}

type KubernetesNodePoolDataSource struct {
	client *nscale.Client
}

func NewKubernetesNodePoolDataSource() datasource.DataSource {
	return &KubernetesNodePoolDataSource{}
}

func (s *KubernetesNodePoolDataSource) Configure(
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

func (s *KubernetesNodePoolDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_kubernetes_node_pool"
}

// Schema is id-based lookup only, per the repo convention. The API's list
// endpoint does filter by name, but that filter is exact, case-sensitive and
// explicitly non-unique — the spec says "every matching resource is returned
// because display names are not unique" — and pool names are only unique within
// a cluster, so a name lookup could not resolve to one pool without also taking
// a cluster.
func (s *KubernetesNodePoolDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Kubernetes Node Pool",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique identifier of the node pool to look up.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the node pool.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the node pool.",
				Computed:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the node pool.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the NKS cluster the node pool provides workers for.",
				Computed:            true,
			},
			"provisioning_mode": schema.StringAttribute{
				MarkdownDescription: "Where the pool's worker capacity comes from, " +
					"either `compute` or `reservation`.",
				Computed: true,
			},
			"replicas": schema.Int64Attribute{
				MarkdownDescription: "The number of workers the pool is configured to run.",
				Computed:            true,
			},
			"compute": schema.SingleNestedAttribute{
				MarkdownDescription: "Compute-backed worker capacity. Null on a `reservation` pool.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"flavor_id": schema.StringAttribute{
						MarkdownDescription: "The identifier of the compute flavor each worker runs on.",
						Computed:            true,
					},
				},
			},
			"reservation": schema.SingleNestedAttribute{
				MarkdownDescription: "Reservation-backed worker capacity. Null on a `compute` pool.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"reservation_id": schema.StringAttribute{
						MarkdownDescription: "The identifier of the reservation the pool consumes capacity from.",
						Computed:            true,
					},
				},
			},
			"taints": schema.ListNestedAttribute{
				MarkdownDescription: "Kubernetes taints applied to each worker as it joins the cluster.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							MarkdownDescription: "The taint key.",
							Computed:            true,
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "The taint value.",
							Computed:            true,
						},
						"effect": schema.StringAttribute{
							MarkdownDescription: "What the taint does to pods that do not tolerate it.",
							Computed:            true,
						},
					},
				},
			},
			"labels": schema.MapAttribute{
				MarkdownDescription: "Kubernetes labels applied to each worker as it joins the cluster.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project the node pool belongs to.",
				Computed:            true,
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the organization the node pool belongs to.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region the node pool is provisioned in.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the node pool was created.",
				Computed:            true,
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning state of the node pool.",
				Computed:            true,
			},
			"health_status": schema.StringAttribute{
				MarkdownDescription: "The health state of the node pool.",
				Computed:            true,
			},
			"current_replicas": schema.Int64Attribute{
				MarkdownDescription: "The number of workers the pool currently has.",
				Computed:            true,
			},
			"ready_replicas": schema.Int64Attribute{
				MarkdownDescription: "The number of the pool's workers that are ready.",
				Computed:            true,
			},
			"up_to_date_replicas": schema.Int64Attribute{
				MarkdownDescription: "The number of the pool's workers running its current template.",
				Computed:            true,
			},
			"kubernetes_version": schema.StringAttribute{
				MarkdownDescription: "The Kubernetes version applied to the pool's workers.",
				Computed:            true,
			},
			"placement_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the reservation placement backing the pool.",
				Computed:            true,
			},
			"applied_platform_release_id": schema.StringAttribute{
				MarkdownDescription: "The platform release the node pool is pinned to, inherited from the cluster.",
				Computed:            true,
			},
			"platform_release_kubernetes_version": schema.StringAttribute{
				MarkdownDescription: "The Kubernetes version of the pinned platform release.",
				Computed:            true,
			},
			"platform_release_deprecated": schema.BoolAttribute{
				MarkdownDescription: "Whether the pinned platform release is currently deprecated.",
				Computed:            true,
			},
			"platform_release_withdrawn": schema.BoolAttribute{
				MarkdownDescription: "Whether operators have withdrawn the pinned platform release.",
				Computed:            true,
			},
		},
	}
}

func (s *KubernetesNodePoolDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var data KubernetesNodePoolModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	pool, err := getNodePool(ctx, s.client, data.ID.ValueString())
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Kubernetes Node Pool",
			fmt.Sprintf("An error occurred while retrieving the node pool: %s", err),
		)
		return
	}

	data = NewKubernetesNodePoolModel(pool)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
