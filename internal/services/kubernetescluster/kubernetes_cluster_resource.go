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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
	"github.com/nscaledev/terraform-provider-nscale/internal/nkswait"
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

	// WaitForProvisioned is provider behaviour rather than cluster state, so it
	// lives here and not on KubernetesClusterModel — the data source shares that
	// model and must not grow an attribute the API never returns.
	WaitForProvisioned types.Bool `tfsdk:"wait_for_provisioned"`

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
	if response.Diagnostics.HasError() {
		return
	}

	// Provider-side only, so the API cannot tell us what it was. Seed the schema
	// default rather than leaving it null, which would show as a null -> true
	// diff on the first plan after an import.
	//
	// A config that sets wait_for_provisioned = false still shows one diff after
	// import (true -> false). That is unavoidable for an attribute the API never
	// returns — the same reason `timeouts` does it — so it belongs in
	// ImportStateVerifyIgnore next to timeouts rather than being worked around.
	// Seeding the default is still the better of the two options, because it
	// makes the common case (config omits it, or sets true) import cleanly.
	response.Diagnostics.Append(
		response.State.SetAttribute(ctx, path.Root("wait_for_provisioned"), true)...,
	)
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
				MarkdownDescription: "The name of the cluster. " +
					"Immutable: changing this forces a new cluster to be created.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
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
			// The pod and service CIDRs are enforced immutable by a CEL
			// `self == oldSelf` rule on the underlying CRD, and the server returns
			// 422 "clusterNetwork.podCidr is immutable". Planning an in-place update
			// would produce an apply that always fails, so this replaces instead.
			// The modifiers sit on the nested CIDRs as well as the object so the plan
			// names the attribute that actually forced the replacement.
			"cluster_network": schema.SingleNestedAttribute{
				MarkdownDescription: "Pod and service network CIDRs for the cluster. Omit to accept the API defaults. " +
					"Immutable: changing either CIDR forces a new cluster to be created.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					// UseStateForUnknown must come before RequiresReplace, and is not
					// optional here. Configuring one CIDR and omitting the other
					// leaves the omitted one unknown (it is Computed), and
					// RequiresReplace compares the *planned* value against state —
					// so an unknown sibling would read as a change and destroy the
					// cluster for no reason. Pinning it to state is also correct on
					// the merits: the value is immutable, so state is what the
					// server will keep reporting.
					"pod_cidr": schema.StringAttribute{
						MarkdownDescription: "IPv4 CIDR used for Kubernetes pod addresses. Defaults to `10.240.0.0/12`. " +
							"Immutable: changing this forces a new cluster to be created.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							stringplanmodifier.RequiresReplace(),
						},
						Validators: []validator.String{
							validators.CIDRValidator{},
						},
					},
					"service_cidr": schema.StringAttribute{
						MarkdownDescription: "IPv4 CIDR used for Kubernetes service addresses. Defaults to `10.96.0.0/16`. " +
							"Immutable: changing this forces a new cluster to be created.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							stringplanmodifier.RequiresReplace(),
						},
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
					// A profile is an object rather than a bare bool because that is
					// how the NKS spec models it (clusterAddonProfileV1), leaving room
					// for per-profile settings beyond `enabled` without a breaking
					// change here.
					"hardware": schema.SingleNestedAttribute{
						MarkdownDescription: "Configuration for the optional hardware addon profile.",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Object{
							objectplanmodifier.UseStateForUnknown(),
						},
						Attributes: map[string]schema.Attribute{
							// No framework Default here even though the API documents one
							// (true on create). The default is the server's to own, so we
							// leave it unset and read back whatever was applied; a
							// provider-side booldefault would also only fire when the
							// profile itself is present, making omit-the-block and
							// omit-just-the-field behave differently for no good reason.
							"enabled": schema.BoolAttribute{
								MarkdownDescription: "Whether the addon profile is enabled. Defaults to `true`.",
								Optional:            true,
								Computed:            true,
							},
						},
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
			// Opt out of the readiness wait, so a node pool in the same apply is
			// created while the cluster is still provisioning rather than after
			// it. The API permits that — the console POSTs both in the same
			// breath — but Terraform orders the pool behind this resource
			// because it references the cluster ID, and there is no way to
			// depend on a resource existing without also depending on its
			// Create having finished.
			//
			// Default true, which keeps the safe contract: a finished apply
			// means a usable cluster with every status attribute populated.
			//
			// Setting this false moves the waiting elsewhere, and something must
			// pick it up. A node pool does so implicitly — workers cannot reach
			// ready before the control plane is up, so a failed control plane
			// still fails the apply. For the cluster's own observed status, read
			// it back through the nscale_kubernetes_cluster DATA SOURCE with
			// depends_on set to the pool; the resource's status attributes are
			// whatever the create response said and stay stale until the next
			// refresh. Scaleway is the only other provider that splits the wait
			// this way, and it documents the missing re-read as a footgun — the
			// data source is how this provider avoids it.
			"wait_for_provisioned": schema.BoolAttribute{
				MarkdownDescription: "Whether `terraform apply` blocks until the cluster reports " +
					"`provisioned`. Defaults to `true`. " +
					"Set to `false` to return as soon as the cluster exists, so node pools in the same apply " +
					"are created while it is still provisioning rather than afterwards. " +
					"When `false` this resource's status attributes and `api_server_endpoint` describe the " +
					"moment of creation and stay stale until the next refresh — read them back through the " +
					"`nscale_kubernetes_cluster` data source, with `depends_on` set to a node pool.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
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

	// State already holds the ID and every argument from the create response, so
	// returning here leaves Terraform tracking a real cluster — only its
	// observed status is not yet filled in.
	if !data.WaitForProvisioned.ValueBool() {
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
		if nkswait.IsNotFound(err) {
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

	cluster, readErr := nscale.ReadJSONResponsePointer[nks.ClusterV1Read](updateResponse)
	if readErr != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		response.Diagnostics.AddError(
			"Failed to Update Kubernetes Cluster",
			fmt.Sprintf("An error occurred while updating the cluster: %s", readErr),
		)
		return
	}

	// Not waiting, so the update response is the only concrete read available —
	// use it rather than the plan.
	//
	// Writing the plan here would be a bug: nine computed status attributes
	// deliberately carry no UseStateForUnknown (see the schema), so the
	// framework marks every one of them unknown in an update plan. Persisting
	// those unknowns makes Terraform reject the apply with "Provider produced
	// invalid result object after apply", on exactly the path this flag exists
	// for. The response's status describes the pre-update generation, which is
	// the trade already accepted by not waiting.
	if !data.WaitForProvisioned.ValueBool() {
		data.KubernetesClusterModel = NewKubernetesClusterModel(cluster)
		response.Diagnostics.Append(response.State.Set(ctx, &data)...)
		return
	}

	timeout, diagnostics := data.Timeouts.Update(ctx, defaultUpdateTimeout)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	// The response body above is deliberately NOT used on this path: its status
	// still describes the pre-update generation. Only the settled read from the
	// waiter is safe to write into state when we are waiting.
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

	if readErr := nscale.ReadEmptyResponse(deleteResponse); readErr != nil && !nkswait.IsNotFound(readErr) {
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
