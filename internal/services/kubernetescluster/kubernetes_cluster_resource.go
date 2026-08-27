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

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

const (
	// Bounds on api_server.allowed_cidrs, mirroring minItems/maxItems in the NKS
	// spec so a bad list is rejected at plan time rather than by the API.
	minAllowedCIDRs = 1
	maxAllowedCIDRs = 32
)

var (
	_ resource.Resource                = &KubernetesClusterResource{}
	_ resource.ResourceWithConfigure   = &KubernetesClusterResource{}
	_ resource.ResourceWithImportState = &KubernetesClusterResource{}
)

type KubernetesClusterResourceModel struct {
	KubernetesClusterModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// KubernetesClusterResource implements CRUD directly rather than embedding
// nscale.GenericResource. The generic base is built around
// nscale.ResourceStatus and the three shared watchers, none of which can
// express NKS's observedGeneration settledness rule — see
// kubernetes_cluster_wait.go.
type KubernetesClusterResource struct {
	client *nscale.Client
}

func NewKubernetesClusterResource() resource.Resource {
	return &KubernetesClusterResource{}
}

func (r *KubernetesClusterResource) Configure(
	_ context.Context,
	request resource.ConfigureRequest,
	response *resource.ConfigureResponse,
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

	r.client = client
}

func (r *KubernetesClusterResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_kubernetes_cluster"
}

func (r *KubernetesClusterResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *KubernetesClusterResource) Schema(
	ctx context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Kubernetes Cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the cluster.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the cluster.",
				Optional:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the cluster.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region network the cluster attaches to. " +
					"The cluster's project, organization and region are all inherited from this network. " +
					"Immutable: changing this forces a new cluster to be created.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"platform_release_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the NKS platform release the cluster runs. " +
					"Changing this performs an in-place cluster upgrade. Use the " +
					"`nscale_kubernetes_platform_releases` data source to select an eligible release.",
				Required: true,
			},
			"api_server": schema.SingleNestedAttribute{
				MarkdownDescription: "Network exposure for the cluster's Kubernetes API server. " +
					"Omit to accept the API defaults.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"public_ip": schema.BoolAttribute{
						MarkdownDescription: "Whether to expose the API server through a public endpoint. Defaults to `false`.",
						Optional:            true,
						Computed:            true,
					},
					"allowed_cidrs": schema.SetAttribute{
						MarkdownDescription: "Source IPv4 CIDR allowlist for the cluster API endpoint, including the " +
							"private endpoint. Defaults to `[\"0.0.0.0/0\"]`, so if `public_ip` is `true` and this is " +
							"omitted the API server is reachable from anywhere.",
						ElementType: types.StringType,
						Optional:    true,
						Computed:    true,
						Validators: []validator.Set{
							setvalidator.SizeBetween(minAllowedCIDRs, maxAllowedCIDRs),
							setvalidator.ValueStringsAre(validators.CIDRValidator{}),
						},
					},
				},
			},
			"cluster_network": schema.SingleNestedAttribute{
				MarkdownDescription: "Pod and service network CIDRs for the cluster. Omit to accept the API defaults.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"pod_cidr": schema.StringAttribute{
						MarkdownDescription: "IPv4 CIDR used for Kubernetes pod addresses. Defaults to `10.240.0.0/12`.",
						Optional:            true,
						Computed:            true,
						Validators: []validator.String{
							validators.CIDRValidator{},
						},
					},
					"service_cidr": schema.StringAttribute{
						MarkdownDescription: "IPv4 CIDR used for Kubernetes service addresses. Defaults to `10.96.0.0/16`.",
						Optional:            true,
						Computed:            true,
						Validators: []validator.String{
							validators.CIDRValidator{},
						},
					},
				},
			},
			"addons": schema.SingleNestedAttribute{
				MarkdownDescription: "Addon profiles enabled on the cluster. Omit to accept the API defaults.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					// No framework Default here even though the API documents one
					// (true on create). The default is the server's to own, so we
					// leave it unset and read back whatever was applied; a
					// provider-side booldefault would also only fire when `addons`
					// itself is present, making omit-the-block and
					// omit-just-the-field behave differently for no good reason.
					"hardware": schema.BoolAttribute{
						MarkdownDescription: "Whether the optional hardware addon profile is enabled. Defaults to `true`.",
						Optional:            true,
						Computed:            true,
					},
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project the cluster belongs to. " +
					"Inherited from `network_id` and cannot be set: the NKS API derives cluster scope from the network.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the organization the cluster belongs to. " +
					"Inherited from `network_id` and cannot be set.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region the cluster is provisioned in. " +
					"Inherited from `network_id` and cannot be set.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the cluster was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// The status attributes below carry no UseStateForUnknown: they are
			// expected to change between plans, and pinning them to prior state
			// would report stale values as current.
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
						MarkdownDescription: "The complete Kubernetes API server CA bundle, base64-encoded. " +
							"Public key material, not a secret.",
						Computed: true,
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
						MarkdownDescription: "The address of the public API server endpoint. " +
							"Null unless `api_server.public_ip` is enabled.",
						Computed: true,
					},
					"public_port": schema.Int64Attribute{
						MarkdownDescription: "The TCP port of the public API server endpoint. " +
							"Null unless `api_server.public_ip` is enabled.",
						Computed: true,
					},
				},
			},
			"applied_platform_release_id": schema.StringAttribute{
				MarkdownDescription: "The platform release last observed as applied to the cluster. " +
					"Lags `platform_release_id` while an upgrade is in progress.",
				Computed: true,
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
				MarkdownDescription: "Eligible platform release IDs, in upgrade order. " +
					"Empty when eligibility was observed and no upgrade is available.",
				ElementType: types.StringType,
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *KubernetesClusterResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var data KubernetesClusterResourceModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	nksClient, diagnostics := r.client.RequireNKS()
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	params, diagnostics := data.NscaleClusterCreateParams(ctx)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	createResponse, err := nksClient.CreateCluster(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Kubernetes Cluster",
			fmt.Sprintf("An error occurred while creating the cluster: %s", err),
		)
		return
	}
	defer createResponse.Body.Close()

	cluster, err := nscale.ReadJSONResponsePointer[nks.ClusterV1Read](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create Kubernetes Cluster",
			fmt.Sprintf("An error occurred while creating the cluster: %s", err),
		)
		return
	}

	// Persist the ID before waiting. A control plane build takes tens of minutes;
	// if the wait times out or is interrupted, this is what stops Terraform from
	// losing track of a cluster that exists and is billable.
	data.KubernetesClusterModel = NewKubernetesClusterModel(cluster)
	if setDiagnostics := response.State.Set(ctx, &data); setDiagnostics.HasError() {
		response.Diagnostics.Append(setDiagnostics...)
		return
	}

	timeout, diagnostics := data.Timeouts.Create(ctx, defaultCreateTimeout)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	final, err := waitClusterProvisioned(ctx, r.client, cluster.Metadata.Id, timeout)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Wait for Kubernetes Cluster to be Created",
			fmt.Sprintf("An error occurred while waiting for the cluster to be created: %s", err),
		)
		return
	}

	data.KubernetesClusterModel = NewKubernetesClusterModel(final)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (r *KubernetesClusterResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var data KubernetesClusterResourceModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()

	cluster, err := getCluster(ctx, r.client, id)
	if err != nil {
		if isNotFound(err) {
			response.Diagnostics.AddWarning(
				"Kubernetes Cluster Not Found",
				fmt.Sprintf(
					"The cluster with ID %s was not found on the server and will be removed from the state file.",
					id,
				),
			)
			response.State.RemoveResource(ctx)
			return
		}

		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Kubernetes Cluster",
			fmt.Sprintf("An error occurred while retrieving the cluster: %s", err),
		)
		return
	}

	data.KubernetesClusterModel = NewKubernetesClusterModel(cluster)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (r *KubernetesClusterResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var data KubernetesClusterResourceModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	nksClient, diagnostics := r.client.RequireNKS()
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()

	params, diagnostics := data.NscaleClusterUpdateParams(ctx)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	updateResponse, err := nksClient.UpdateCluster(ctx, id, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Kubernetes Cluster",
			fmt.Sprintf("An error occurred while updating the cluster: %s", err),
		)
		return
	}
	defer updateResponse.Body.Close()

	if _, readErr := nscale.ReadJSONResponsePointer[nks.ClusterV1Read](updateResponse); readErr != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		response.Diagnostics.AddError(
			"Failed to Update Kubernetes Cluster",
			fmt.Sprintf("An error occurred while updating the cluster: %s", readErr),
		)
		return
	}

	timeout, diagnostics := data.Timeouts.Update(ctx, defaultUpdateTimeout)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	// The response body above is discarded on purpose: its status still describes
	// the pre-update generation. Only the settled read from the waiter is safe to
	// write into state.
	final, err := waitClusterProvisioned(ctx, r.client, id, timeout)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Wait for Kubernetes Cluster to be Updated",
			fmt.Sprintf("An error occurred while waiting for the cluster to be updated: %s", err),
		)
		return
	}

	data.KubernetesClusterModel = NewKubernetesClusterModel(final)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (r *KubernetesClusterResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var data KubernetesClusterResourceModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	nksClient, diagnostics := r.client.RequireNKS()
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()

	deleteResponse, err := nksClient.DeleteCluster(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Kubernetes Cluster",
			fmt.Sprintf("An error occurred while deleting the cluster: %s", err),
		)
		return
	}
	defer deleteResponse.Body.Close()

	if readErr := nscale.ReadEmptyResponse(deleteResponse); readErr != nil && !isNotFound(readErr) {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		response.Diagnostics.AddError(
			"Failed to Delete Kubernetes Cluster",
			fmt.Sprintf("An error occurred while deleting the cluster: %s", readErr),
		)
		return
	}

	timeout, diagnostics := data.Timeouts.Delete(ctx, defaultDeleteTimeout)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	if waitErr := waitClusterDeleted(ctx, r.client, id, timeout); waitErr != nil {
		response.Diagnostics.AddError(
			"Failed to Wait for Kubernetes Cluster to be Deleted",
			fmt.Sprintf("An error occurred while waiting for the cluster to be deleted: %s", waitErr),
		)
	}
}
