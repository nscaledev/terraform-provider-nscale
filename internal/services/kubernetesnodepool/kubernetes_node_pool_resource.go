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

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"

	"github.com/nscaledev/terraform-provider-nscale/internal/nkswait"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

// Bounds mirrored from the NKS spec so a bad value is rejected at plan time
// rather than by the API. The spec's patterns are enforced alongside these —
// see validators.KubernetesQualifiedNameValidator and
// validators.KubernetesLabelValueValidator.
const (
	minReplicas = 0
	maxReplicas = 2147483647

	maxTaints = 64
	maxLabels = 64

	minFlavorIDLength = 1
	maxFlavorIDLength = 128

	minTaintKeyLength   = 1
	maxTaintKeyLength   = 317
	maxTaintValueLength = 63
	maxLabelValueLength = 63
)

var (
	_ resource.Resource                   = &KubernetesNodePoolResource{}
	_ resource.ResourceWithConfigure      = &KubernetesNodePoolResource{}
	_ resource.ResourceWithImportState    = &KubernetesNodePoolResource{}
	_ resource.ResourceWithValidateConfig = &KubernetesNodePoolResource{}
)

type KubernetesNodePoolResourceModel struct {
	KubernetesNodePoolModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// KubernetesNodePoolResource implements CRUD directly rather than embedding
// nscale.GenericResource, for the same reason the cluster does: the generic
// base is built around nscale.ResourceStatus and the three shared watchers,
// none of which can express NKS's observedGeneration settledness rule — see
// internal/nkswait.
type KubernetesNodePoolResource struct {
	client *nscale.Client
}

func NewKubernetesNodePoolResource() resource.Resource {
	return &KubernetesNodePoolResource{}
}

func (r *KubernetesNodePoolResource) Configure(
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

func (r *KubernetesNodePoolResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_kubernetes_node_pool"
}

func (r *KubernetesNodePoolResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

// ValidateConfig surfaces a mode/capacity-block mismatch at plan time rather
// than letting it fail as an API error at apply.
func (r *KubernetesNodePoolResource) ValidateConfig(
	ctx context.Context,
	request resource.ValidateConfigRequest,
	response *resource.ValidateConfigResponse,
) {
	validateCapacityMode(ctx, request.Config, &response.Diagnostics)
}

func (r *KubernetesNodePoolResource) Schema(
	ctx context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Kubernetes Node Pool",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the node pool.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the node pool. " +
					"Immutable: changing this forces a new node pool to be created.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the node pool.",
				Optional:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the node pool.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the NKS cluster the node pool provides workers for. " +
					"The pool's project, organization and region are all inherited from the cluster. " +
					"Immutable: a pool cannot move between clusters, so changing this forces a new node pool " +
					"to be created.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"provisioning_mode": schema.StringAttribute{
				MarkdownDescription: "Where the pool's worker capacity comes from. " +
					"`compute` draws on a compute flavor and requires the `compute` block; " +
					"`reservation` draws on reserved capacity and requires the `reservation` block. " +
					"The mode also decides how much of the pool can be changed in place — see `replicas`. " +
					"Immutable: there is no in-place migration between modes, so changing this forces a new " +
					"node pool to be created.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(kubernetesapi.NodePoolProvisioningModeV1Compute),
						string(kubernetesapi.NodePoolProvisioningModeV1Reservation),
					),
				},
			},
			"replicas": schema.Int64Attribute{
				MarkdownDescription: "The number of workers the pool should run. " +
					"On a `compute` pool this is an in-place scale, and `0` is valid — a pool scaled to zero " +
					"keeps its configuration but runs no workers. " +
					"On a `reservation` pool it forces replacement: a placement cannot be resized, so changing " +
					"this releases and re-claims the reservation's capacity rather than scaling.",
				Required: true,
				PlanModifiers: []planmodifier.Int64{
					replicasRequiresReplaceIfReservation(),
				},
				Validators: []validator.Int64{
					int64validator.Between(minReplicas, maxReplicas),
				},
			},
			"compute": schema.SingleNestedAttribute{
				MarkdownDescription: "Compute-backed worker capacity. " +
					"Required when `provisioning_mode` is `compute`, and must be omitted otherwise.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"flavor_id": schema.StringAttribute{
						MarkdownDescription: "The identifier of the compute flavor each worker runs on. " +
							"Immutable: the API rejects a flavor change, so changing this forces a new node " +
							"pool to be created. To move a workload onto a different flavor without an " +
							"outage, add a second pool and drain the first.",
						Required: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
						Validators: []validator.String{
							stringvalidator.LengthBetween(minFlavorIDLength, maxFlavorIDLength),
						},
					},
				},
			},
			"reservation": schema.SingleNestedAttribute{
				MarkdownDescription: "Reservation-backed worker capacity. " +
					"Required when `provisioning_mode` is `reservation`, and must be omitted otherwise.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"reservation_id": schema.StringAttribute{
						MarkdownDescription: "The identifier of the reservation to consume capacity from. " +
							"Immutable: changing this forces a new node pool to be created.",
						Required: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
				},
			},
			// The roll warning has to live here, not only in the prose docs: on a
			// compute pool Terraform shows a taint edit as an ordinary in-place
			// update, because that is exactly what it is at the API level. The plan
			// cannot warn, so the attribute description carries the weight.
			"taints": schema.ListNestedAttribute{
				MarkdownDescription: "Kubernetes taints applied to each worker as it joins the cluster. " +
					"**Editing this rolls the pool.** A taint is applied during node registration and is not " +
					"reconciled onto running nodes, so the API replaces every existing worker to make the new " +
					"taints take effect — one node at a time, draining each through the Eviction API and " +
					"honouring PodDisruptionBudgets. " +
					"On a `reservation` pool this forces replacement instead, because a placement never rolls. " +
					"Ordering is preserved as given.",
				Optional: true,
				PlanModifiers: []planmodifier.List{
					taintsRequiresReplaceIfReservation(),
				},
				Validators: []validator.List{
					listvalidator.SizeAtMost(maxTaints),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							MarkdownDescription: "The taint key, as a Kubernetes qualified name.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.LengthBetween(minTaintKeyLength, maxTaintKeyLength),
								validators.KubernetesQualifiedNameValidator(),
							},
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "The taint value. Omit for a taint that carries no value.",
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.LengthAtMost(maxTaintValueLength),
								validators.KubernetesLabelValueValidator(),
							},
						},
						"effect": schema.StringAttribute{
							MarkdownDescription: "What the taint does to pods that do not tolerate it.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(
									string(kubernetesapi.NodePoolTaintV1EffectNoSchedule),
									string(kubernetesapi.NodePoolTaintV1EffectPreferNoSchedule),
									string(kubernetesapi.NodePoolTaintV1EffectNoExecute),
								),
							},
						},
					},
				},
			},
			"labels": schema.MapAttribute{
				MarkdownDescription: "Kubernetes labels applied to each worker as it joins the cluster. " +
					"**Editing this rolls the pool**, on exactly the same terms as `taints`: labels are not " +
					"reconciled onto running nodes, so every existing worker is replaced to make the new " +
					"labels take effect. " +
					"On a `reservation` pool this forces replacement instead. " +
					"An empty value is valid and still applies the label.",
				ElementType: types.StringType,
				Optional:    true,
				PlanModifiers: []planmodifier.Map{
					labelsRequiresReplaceIfReservation(),
				},
				Validators: []validator.Map{
					mapvalidator.SizeAtMost(maxLabels),
					mapvalidator.ValueStringsAre(
						stringvalidator.LengthAtMost(maxLabelValueLength),
						validators.KubernetesLabelValueValidator(),
					),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project the node pool belongs to. " +
					"Inherited from `cluster_id` and cannot be set: the NKS API derives pool scope from the " +
					"cluster.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the organization the node pool belongs to. " +
					"Inherited from `cluster_id` and cannot be set.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region the node pool is provisioned in. " +
					"Inherited from `cluster_id` and cannot be set.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the node pool was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// The status attributes below carry no UseStateForUnknown: they are
			// expected to change between plans, and pinning them to prior state
			// would report stale values as current.
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning state of the node pool.",
				Computed:            true,
			},
			"health_status": schema.StringAttribute{
				MarkdownDescription: "The health state of the node pool.",
				Computed:            true,
			},
			// desiredReplicas is deliberately not exposed: it duplicates the
			// replicas argument and would only invite confusion about which is
			// authoritative. The three observed counts are the ones that tell a
			// user something they do not already know.
			"current_replicas": schema.Int64Attribute{
				MarkdownDescription: "The number of workers the pool currently has.",
				Computed:            true,
			},
			"ready_replicas": schema.Int64Attribute{
				MarkdownDescription: "The number of the pool's workers that are ready.",
				Computed:            true,
			},
			"up_to_date_replicas": schema.Int64Attribute{
				MarkdownDescription: "The number of the pool's workers running its current template. " +
					"Drops below `replicas` while the pool is rolling.",
				Computed: true,
			},
			"kubernetes_version": schema.StringAttribute{
				MarkdownDescription: "The Kubernetes version applied to the pool's workers.",
				Computed:            true,
			},
			"placement_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the reservation placement backing the pool. " +
					"Null on a `compute` pool.",
				Computed: true,
			},
			// A node pool has no platform_release_id argument: it inherits the
			// cluster's release. These report which release it was pinned to.
			"applied_platform_release_id": schema.StringAttribute{
				MarkdownDescription: "The platform release the node pool is pinned to, inherited from the " +
					"cluster. Upgrade the cluster to move a pool onto a newer release.",
				Computed: true,
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
		Blocks: map[string]schema.Block{
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *KubernetesNodePoolResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var data KubernetesNodePoolResourceModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	nksClient, diagnostics := r.client.RequireNKS()
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	params, diagnostics := data.NscaleNodePoolCreateParams(ctx)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	createResponse, err := nksClient.CreateNodePool(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Kubernetes Node Pool",
			fmt.Sprintf("An error occurred while creating the node pool: %s", err),
		)
		return
	}
	defer createResponse.Body.Close()

	pool, err := nscale.ReadJSONResponsePointer[kubernetesapi.NodePoolV1Read](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create Kubernetes Node Pool",
			fmt.Sprintf("An error occurred while creating the node pool: %s", err),
		)
		return
	}

	// Persist the ID before waiting. Workers are real, billable compute; if the
	// wait times out or is interrupted, this is what stops Terraform from losing
	// track of a pool that exists.
	data.KubernetesNodePoolModel = NewKubernetesNodePoolModel(pool)
	if setDiagnostics := response.State.Set(ctx, &data); setDiagnostics.HasError() {
		response.Diagnostics.Append(setDiagnostics...)
		return
	}

	timeout, diagnostics := data.Timeouts.Create(ctx, defaultCreateTimeout)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	final, err := waitNodePoolProvisioned(ctx, r.client, pool.Metadata.Id, timeout)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Wait for Kubernetes Node Pool to be Created",
			fmt.Sprintf("An error occurred while waiting for the node pool to be created: %s", err),
		)
		return
	}

	data.KubernetesNodePoolModel = NewKubernetesNodePoolModel(final)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (r *KubernetesNodePoolResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var data KubernetesNodePoolResourceModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()

	pool, err := getNodePool(ctx, r.client, id)
	if err != nil {
		if nkswait.IsNotFound(err) {
			response.Diagnostics.AddWarning(
				"Kubernetes Node Pool Not Found",
				fmt.Sprintf(
					"The node pool with ID %s was not found on the server and will be removed from the "+
						"state file.",
					id,
				),
			)
			response.State.RemoveResource(ctx)
			return
		}

		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Kubernetes Node Pool",
			fmt.Sprintf("An error occurred while retrieving the node pool: %s", err),
		)
		return
	}

	data.KubernetesNodePoolModel = NewKubernetesNodePoolModel(pool)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (r *KubernetesNodePoolResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var data KubernetesNodePoolResourceModel

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

	params, diagnostics := data.NscaleNodePoolUpdateParams(ctx)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	updateResponse, err := nksClient.UpdateNodePool(ctx, id, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Kubernetes Node Pool",
			fmt.Sprintf("An error occurred while updating the node pool: %s", err),
		)
		return
	}
	defer updateResponse.Body.Close()

	if _, readErr := nscale.ReadJSONResponsePointer[kubernetesapi.NodePoolV1Read](updateResponse); readErr != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		response.Diagnostics.AddError(
			"Failed to Update Kubernetes Node Pool",
			fmt.Sprintf("An error occurred while updating the node pool: %s", readErr),
		)
		return
	}

	timeout, diagnostics := data.Timeouts.Update(ctx, defaultUpdateTimeout)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	// The response body above is discarded on purpose: its status still
	// describes the pre-update generation, so on a taint or label edit it would
	// report the pool as settled before the roll has even started. Only the
	// settled read from the waiter is safe to write into state.
	final, err := waitNodePoolProvisioned(ctx, r.client, id, timeout)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Wait for Kubernetes Node Pool to be Updated",
			fmt.Sprintf("An error occurred while waiting for the node pool to be updated: %s", err),
		)
		return
	}

	data.KubernetesNodePoolModel = NewKubernetesNodePoolModel(final)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (r *KubernetesNodePoolResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var data KubernetesNodePoolResourceModel

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

	deleteResponse, err := nksClient.DeleteNodePool(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Kubernetes Node Pool",
			fmt.Sprintf("An error occurred while deleting the node pool: %s", err),
		)
		return
	}
	defer deleteResponse.Body.Close()

	// A 404 is success: destroying the cluster cascades to its pools, so a pool
	// Terraform is about to delete may already be gone.
	if readErr := nscale.ReadEmptyResponse(deleteResponse); readErr != nil && !nkswait.IsNotFound(readErr) {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		response.Diagnostics.AddError(
			"Failed to Delete Kubernetes Node Pool",
			fmt.Sprintf("An error occurred while deleting the node pool: %s", readErr),
		)
		return
	}

	timeout, diagnostics := data.Timeouts.Delete(ctx, defaultDeleteTimeout)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	if waitErr := waitNodePoolDeleted(ctx, r.client, id, timeout); waitErr != nil {
		response.Diagnostics.AddError(
			"Failed to Wait for Kubernetes Node Pool to be Deleted",
			fmt.Sprintf("An error occurred while waiting for the node pool to be deleted: %s", waitErr),
		)
	}
}
