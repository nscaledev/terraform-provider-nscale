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

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.Resource                = &ComputeClusterResource{}
	_ resource.ResourceWithConfigure   = &ComputeClusterResource{}
	_ resource.ResourceWithImportState = &ComputeClusterResource{}
)

type ComputeClusterResourceModel struct {
	ComputeClusterModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// ComputeClusterResource embeds the generic CRUD base; only Schema and the
// adapter wiring below are compute-cluster-specific. The legacy unikorn-cloud
// types are confined to the adapter closures and compat.go — the generic base
// only ever sees computeapi.ComputeClusterRead.
type ComputeClusterResource struct {
	*nscale.GenericResource[ComputeClusterResourceModel, computeapi.ComputeClusterRead]
}

func NewComputeClusterResource() resource.Resource {
	return &ComputeClusterResource{
		GenericResource: nscale.NewGenericResource(computeClusterAdapter()),
	}
}

// computeClusterAdapter wires the compute-cluster-specific SDK calls and model
// mapping into the generic resource skeleton.
func computeClusterAdapter() nscale.ResourceAdapter[ComputeClusterResourceModel, computeapi.ComputeClusterRead] {
	return nscale.ResourceAdapter[ComputeClusterResourceModel, computeapi.ComputeClusterRead]{
		TypeNameSuffix: "_compute_cluster",
		Title:          "Compute Cluster",
		Name:           "compute cluster",
		Create:         computeClusterCreate,
		Update:         computeClusterUpdate,
		Delete:         computeClusterDelete,
		Get: func(
			ctx context.Context,
			client *nscale.Client,
			id string,
		) (*computeapi.ComputeClusterRead, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getComputeCluster(ctx, client.OrganizationID, id, client))
		},
		ToModel: func(api *computeapi.ComputeClusterRead, dst *ComputeClusterResourceModel) {
			dst.ComputeClusterModel = NewComputeClusterModel(api)
		},
		IDFromModel:       func(m ComputeClusterResourceModel) string { return m.ID.ValueString() },
		TimeoutsFromModel: func(m ComputeClusterResourceModel) tftimeouts.Value { return m.Timeouts },
	}
}

func (r *ComputeClusterResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		DeprecationMessage:  "The nscale_compute_cluster resource is deprecated and will be removed in a future release. Consider using the nscale_instance resource for more flexible configuration.",
		MarkdownDescription: "Nscale Compute Cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the compute cluster.",
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
						// "disk_size": schema.Int64Attribute{
						// 	MarkdownDescription: "The size of the boot disk for each VM in the workload pool, in GiB.",
						// 	Optional:            true,
						// 	Validators: []validator.Int64{
						// 		int64validator.AtLeast(10),
						// 	},
						// },
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
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the compute cluster.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the compute cluster is provisioned. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
		Blocks: map[string]schema.Block{
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func computeClusterCreate(
	ctx context.Context,
	client *nscale.Client,
	plan ComputeClusterResourceModel,
) (*computeapi.ComputeClusterRead, diag.Diagnostics) {
	// Default the region ID from the provider configuration when the plan
	// leaves it empty. This is only meaningful at create time.
	if plan.RegionID.ValueString() == "" {
		plan.RegionID = types.StringValue(client.RegionID)
	}

	requestData, diagnostics := plan.NscaleComputeCluster()
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	createResponse, err := client.LegacyCompute.PostApiV1OrganizationsOrganizationIDProjectsProjectIDClusters(
		ctx,
		client.OrganizationID,
		client.ProjectID,
		requestData,
	)
	if err != nil {
		diagnostics.AddError(
			"Failed to Create Compute Cluster",
			fmt.Sprintf("An error occurred while creating the compute cluster: %s", err),
		)
		return nil, diagnostics
	}
	defer createResponse.Body.Close()

	computeCluster, err := nscale.ReadJSONResponsePointer[computeapi.ComputeClusterRead](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Create Compute Cluster",
			fmt.Sprintf("An error occurred while creating the compute cluster: %s", err),
		)
		return nil, diagnostics
	}

	return computeCluster, nil
}

func computeClusterUpdate(
	ctx context.Context,
	client *nscale.Client,
	id string,
	plan ComputeClusterResourceModel,
) (string, diag.Diagnostics) {
	requestData, diagnostics := plan.NscaleComputeCluster()
	if diagnostics.HasError() {
		return "", diagnostics
	}

	// Tag the update so the watcher can confirm the PUT has propagated through
	// the cache-backed API before reading back a terminal status. The legacy
	// metadata shape requires the compat shim rather than nscale.WriteOperationTag.
	operationTagKey := writeOperationTagLegacy(&requestData.Metadata)

	updateResponse, err := client.LegacyCompute.PutApiV1OrganizationsOrganizationIDProjectsProjectIDClustersClusterID(
		ctx,
		client.OrganizationID,
		client.ProjectID,
		id,
		requestData,
	)
	if err != nil {
		diagnostics.AddError(
			"Failed to Update Compute Cluster",
			fmt.Sprintf("An error occurred while updating the compute cluster: %s", err),
		)
		return "", diagnostics
	}
	defer updateResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(updateResponse); err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Update Compute Cluster",
			fmt.Sprintf("An error occurred while updating the compute cluster: %s", err),
		)
		return "", diagnostics
	}

	return operationTagKey, nil
}

func computeClusterDelete(ctx context.Context, client *nscale.Client, id string) error {
	deleteResponse, err := client.LegacyCompute.DeleteApiV1OrganizationsOrganizationIDProjectsProjectIDClustersClusterID(
		ctx,
		client.OrganizationID,
		client.ProjectID,
		id,
	)
	if err != nil {
		return err
	}
	defer deleteResponse.Body.Close()

	return nscale.ReadEmptyResponse(deleteResponse)
}
