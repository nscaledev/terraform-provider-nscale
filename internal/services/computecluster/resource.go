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

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
)

var (
	_ resource.ResourceWithConfigure   = &ComputeClusterResource{}
	_ resource.ResourceWithImportState = &ComputeClusterResource{}
)

type ComputeClusterResource struct {
	client *nscale.Client
}

func NewComputeClusterResource() resource.Resource {
	return &ComputeClusterResource{}
}

func (r *ComputeClusterResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
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

	r.client = client
}

func (r *ComputeClusterResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *ComputeClusterResource) Metadata(ctx context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_compute_cluster"
}

func (r *ComputeClusterResource) Schema(ctx context.Context, request resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Compute Cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the compute cluster.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the compute cluster.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the compute cluster.",
				Optional:            true,
			},
			"workload_pools": schema.ListNestedAttribute{
				MarkdownDescription: "A list of pools of workload nodes in the compute cluster.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the workload pool.",
							Required:            true,
							Validators: []validator.String{
								validators.NameValidator(),
							},
						},
						"replicas": schema.Int64Attribute{
							MarkdownDescription: "The number of replicas (VMs) to provision in this workload pool.",
							Required:            true,
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
							},
						},
						"image_id": schema.StringAttribute{
							MarkdownDescription: "The identifier of the image used for initializing the boot disk of the workload pool VMs.",
							Required:            true,
						},
						"flavor_id": schema.StringAttribute{
							MarkdownDescription: "The identifier of the flavor (machine type) used for the workload pool VMs.",
							Required:            true,
						},
						//"disk_size": schema.Int64Attribute{
						//	MarkdownDescription: "The size of the boot disk for each VM in the workload pool, in GiB.",
						//	Optional:            true,
						//	Validators: []validator.Int64{
						//		int64validator.AtLeast(10),
						//	},
						//},
						"user_data": schema.StringAttribute{
							MarkdownDescription: "The data to pass to the VMs at boot time.",
							Optional:            true,
							Validators: []validator.String{
								validators.Base64Validator{},
							},
						},
						"enable_public_ip": schema.BoolAttribute{
							MarkdownDescription: "Whether to assign a public IP address to each VM in this workload pool. Default is `true`.",
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(true),
						},
						"allowed_address_pairs": schema.SetNestedAttribute{
							MarkdownDescription: "Allowed addresses that can pass through this workload pool's network ports. Each pair specifies a CIDR prefix and optionally a MAC address. Typically required when the machine is operating as a router.",
							Optional:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"cidr": schema.StringAttribute{
										MarkdownDescription: "The CIDR prefix to allow.",
										Required:            true,
										Validators: []validator.String{
											validators.CIDRValidator{},
										},
									},
									"mac_address": schema.StringAttribute{
										MarkdownDescription: "The MAC address to allow. Optional.",
										Optional:            true,
									},
								},
							},
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
							},
						},
						"firewall_rules": schema.ListNestedAttribute{
							MarkdownDescription: "A list of firewall rules for the VMs in this workload pool.",
							Optional:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"direction": schema.StringAttribute{
										MarkdownDescription: "The direction of the traffic to which this firewall rule applies. Default is `ingress`.",
										Optional:            true,
										Computed:            true,
										Default:             stringdefault.StaticString("ingress"),
										Validators: []validator.String{
											stringvalidator.OneOf("ingress", "egress"),
										},
									},
									"protocol": schema.StringAttribute{
										MarkdownDescription: "The IP protocol to which this firewall rule applies. Valid values are `tcp` or `udp`.",
										Required:            true,
										Validators: []validator.String{
											stringvalidator.OneOf("tcp", "udp"),
										},
									},
									"ports": schema.StringAttribute{
										MarkdownDescription: "The ports to which this firewall rule applies. This can be a single port, or a range of ports. For example: `22`, `80-443`.",
										Required:            true,
										Validators: []validator.String{
											PortsValidator{},
										},
									},
									"prefixes": schema.SetAttribute{
										MarkdownDescription: "A set of CIDR prefixes to which this firewall rule applies.",
										ElementType:         types.StringType,
										Required:            true,
										Validators: []validator.Set{
											setvalidator.SizeAtLeast(1),
											setvalidator.ValueStringsAre(validators.CIDRValidator{}),
										},
									},
								},
							},
							Validators: []validator.List{
								listvalidator.SizeAtLeast(1),
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the compute cluster is provisioned. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the compute cluster.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the compute cluster was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ComputeClusterResource) setDefaultRegionID(data *ComputeClusterModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(r.client.RegionID)
	}
}

func (r *ComputeClusterResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	data, diagnostics := nscale.ReadTerraformState[ComputeClusterModel](ctx, request.Plan.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	requestData, diagnostics := data.NscaleComputeCluster()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	// REVIEW_ME: Should we retrieve the organization ID and project ID using the service token, or is that even possible?
	computeClusterCreateResponse, err := r.client.Compute.PostApiV1OrganizationsOrganizationIDProjectsProjectIDClusters(ctx, r.client.OrganizationID, r.client.ProjectID, requestData)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Compute Cluster",
			fmt.Sprintf("An error occurred while creating the compute cluster: %s", err),
		)
		return
	}

	computeCluster, err := nscale.ReadJSONResponsePointer[computeapi.ComputeClusterRead](computeClusterCreateResponse)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Compute Cluster",
			fmt.Sprintf("An error occurred while creating the compute cluster: %s", err),
		)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[computeapi.ComputeClusterRead]{
		ResourceTitle: "Compute Cluster",
		ResourceName:  "compute cluster",
		GetFunc: func(ctx context.Context) (*computeapi.ComputeClusterRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
			targetID := computeCluster.Metadata.Id
			return getComputeCluster(ctx, r.client.OrganizationID, targetID, r.client)
		},
	}

	computeCluster, ok := stateWatcher.Wait(ctx, response)
	if !ok {
		return
	}

	data = NewComputeClusterModel(computeCluster)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ComputeClusterResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[ComputeClusterModel](ctx, request.State.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := nscale.ResourceReader[computeapi.ComputeClusterRead]{
		ResourceTitle: "Compute Cluster",
		ResourceName:  "compute cluster",
		GetFunc: func(ctx context.Context, id string) (*computeapi.ComputeClusterRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getComputeCluster(ctx, r.client.OrganizationID, id, r.client)
		},
	}

	computeCluster, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data = NewComputeClusterModel(computeCluster)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ComputeClusterResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	data, diagnostics := nscale.ReadTerraformState[ComputeClusterModel](ctx, request.Plan.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	requestData, diagnostics := data.NscaleComputeCluster()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()
	operationTagKey := nscale.WriteOperationTag(&requestData.Metadata)

	computeClusterUpdateResponse, err := r.client.Compute.PutApiV1OrganizationsOrganizationIDProjectsProjectIDClustersClusterID(ctx, r.client.OrganizationID, r.client.ProjectID, id, requestData)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Compute Cluster",
			fmt.Sprintf("An error occurred while updating the compute cluster: %s", err),
		)
		return
	}

	if err = nscale.ReadEmptyResponse(computeClusterUpdateResponse); err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Compute Cluster",
			fmt.Sprintf("An error occurred while updating the compute cluster: %s", err),
		)
		return
	}

	stateWatcher := nscale.UpdateStateWatcher[computeapi.ComputeClusterRead]{
		ResourceTitle: "Compute Cluster",
		ResourceName:  "compute cluster",
		GetFunc: func(ctx context.Context) (*computeapi.ComputeClusterRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getComputeCluster(ctx, r.client.OrganizationID, id, r.client)
		},
	}

	computeCluster, ok := stateWatcher.Wait(ctx, operationTagKey, response)
	if !ok {
		return
	}

	data = NewComputeClusterModel(computeCluster)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ComputeClusterResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	data, diagnostics := nscale.ReadTerraformState[ComputeClusterModel](ctx, request.State.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	computeClusterDeleteResponse, err := r.client.Compute.DeleteApiV1OrganizationsOrganizationIDProjectsProjectIDClustersClusterID(ctx, r.client.OrganizationID, r.client.ProjectID, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Compute Cluster",
			fmt.Sprintf("An error occurred while deleting the compute cluster: %s", err),
		)
		return
	}

	if err = nscale.ReadEmptyResponse(computeClusterDeleteResponse); err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Compute Cluster",
			fmt.Sprintf("An error occurred while deleting the compute cluster: %s", err),
		)
		return
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "Compute Cluster",
		ResourceName:  "compute cluster",
		GetFunc: func(ctx context.Context) (any, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getComputeCluster(ctx, r.client.OrganizationID, id, r.client)
		},
	}

	stateWatcher.Wait(ctx, response)
}
