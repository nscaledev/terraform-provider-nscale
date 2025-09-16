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

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	nscale "github.com/nscaledev/terraform-provider-nscale/internal/client"
	externalRef0 "github.com/unikorn-cloud/core/pkg/openapi"
)

var (
	_ resource.ResourceWithConfigure   = &ComputeClusterResource{}
	_ resource.ResourceWithImportState = &ComputeClusterResource{}
)

type ComputeClusterResource struct {
	client         *nscale.ClientWithResponses
	organizationID string
	projectID      string
}

func NewComputeClusterResource() resource.Resource {
	return &ComputeClusterResource{}
}

func (r *ComputeClusterResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	config, ok := request.ProviderData.(*NscaleProviderConfig)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *NscaleProviderConfig, got: %T. Please contact the Nscale team for support.", request.ProviderData),
		)
		return
	}

	r.client = config.client
	r.organizationID = config.organizationID
	r.projectID = config.projectID
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
					NameValidator(),
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
								NameValidator(),
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
								Base64Validator{},
							},
						},
						"enable_public_ip": schema.BoolAttribute{
							MarkdownDescription: "Whether to assign a public IP address to each VM in this workload pool. Default is `true`.",
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(true),
						},
						"firewall_rules": schema.ListNestedAttribute{
							MarkdownDescription: "A list of firewall rules to apply to the VMs in this workload pool.",
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
											ProtocolValidator{},
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
											setvalidator.ValueStringsAre(CIDRValidator{}),
										},
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
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
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
				MarkdownDescription: "The identifier of the region where the compute cluster is provisioned.",
				Required:            true,
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the compute cluster.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

func (r *ComputeClusterResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var data ComputeClusterModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	requestData, diagnostics := data.NscaleComputeCluster()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	// REVIEW_ME: Should we retrieve the organization ID and project ID using the service token, or is that even possible?
	clusterCreateResponse, err := r.client.PostApiV1OrganizationsOrganizationIDProjectsProjectIDClustersWithResponse(ctx, r.organizationID, r.projectID, requestData)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Compute Cluster",
			fmt.Sprintf("An error occurred while creating the compute cluster: %s", err),
		)
		return
	}
	if clusterCreateResponse.StatusCode() != http.StatusAccepted || clusterCreateResponse.JSON202 == nil {
		response.Diagnostics.AddError(
			"Failed to Create Compute Cluster",
			fmt.Sprintf("The compute cluster creation failed with status code %d.", clusterCreateResponse.StatusCode()),
		)
		return
	}

	targetID := clusterCreateResponse.JSON202.Metadata.Id

	stateWatcher := retry.StateChangeConf{
		Timeout: 30 * time.Minute,
		Pending: []string{
			string(externalRef0.ResourceProvisioningStatusProvisioning),
			string(externalRef0.ResourceProvisioningStatusUnknown),
		},
		Target: []string{
			string(externalRef0.ResourceProvisioningStatusProvisioned),
		},
		Refresh: func() (interface{}, string, error) {
			clusterListResponse, err := r.client.GetApiV1OrganizationsOrganizationIDClustersWithResponse(ctx, r.organizationID, nil)
			if err != nil {
				return nil, "", err
			}
			if clusterListResponse.StatusCode() != http.StatusOK || clusterListResponse.JSON200 == nil {
				err = fmt.Errorf("compute cluster read operation failed with status code %d", clusterListResponse.StatusCode())
				return nil, "", err
			}

			for _, cluster := range *clusterListResponse.JSON200 {
				if cluster.Metadata.Id == targetID {
					return &cluster, string(cluster.Metadata.ProvisioningStatus), nil
				}
			}

			return nil, string(externalRef0.ResourceProvisioningStatusUnknown), nil
		},
	}

	state, err := stateWatcher.WaitForStateContext(ctx)
	if err != nil {
		e := errors.Unwrap(err)
		if e == nil {
			e = err
		}
		response.Diagnostics.AddError(
			"Failed to Wait for Compute Cluster to be Provisioned",
			fmt.Sprintf("An error occurred while waiting for the compute cluster to be provisioned: %s", e),
		)
		return
	}

	computeCluster, ok := state.(*nscale.ComputeClusterRead)
	if !ok || computeCluster == nil {
		response.Diagnostics.AddError(
			"Unexpected Resource Type",
			fmt.Sprintf("Expected *nscale.ComputeClusterRead, got: %T. Please contact the Nscale team for support.", computeCluster),
		)
		return
	}

	data = NewComputeClusterModel(computeCluster)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ComputeClusterResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var data ComputeClusterModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	clusterListResponse, err := r.client.GetApiV1OrganizationsOrganizationIDClustersWithResponse(ctx, r.organizationID, nil)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Compute Cluster",
			fmt.Sprintf("An error occurred while reading the compute cluster: %s", err),
		)
		return
	}
	if clusterListResponse.StatusCode() != http.StatusOK || clusterListResponse.JSON200 == nil {
		response.Diagnostics.AddError(
			"Failed to Read Compute Cluster",
			fmt.Sprintf("The compute cluster read operation failed with status code %d.", clusterListResponse.StatusCode()),
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

	response.State.RemoveResource(ctx)
}

func (r *ComputeClusterResource) setUniqueOperationTag(cluster *nscale.ComputeClusterWrite) {
	tagList := []externalRef0.Tag{
		{
			Name:  fmt.Sprintf("terraform.nscale.com/%s", uuid.New().String()),
			Value: "0",
		},
	}
	cluster.Metadata.Tags = &tagList
}

func (r *ComputeClusterResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var data ComputeClusterModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	requestData, diagnostics := data.NscaleComputeCluster()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	r.setUniqueOperationTag(&requestData)
	operationKey := (*requestData.Metadata.Tags)[0].Name

	clusterUpdateResponse, err := r.client.PutApiV1OrganizationsOrganizationIDProjectsProjectIDClustersClusterIDWithResponse(ctx, r.organizationID, r.projectID, data.ID.ValueString(), requestData)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Compute Cluster",
			fmt.Sprintf("An error occurred while updating the compute cluster: %s", err),
		)
		return
	}
	if clusterUpdateResponse.StatusCode() != http.StatusAccepted {
		response.Diagnostics.AddError(
			"Failed to Update Compute Cluster",
			fmt.Sprintf("The compute cluster update operation failed with status code %d.", clusterUpdateResponse.StatusCode()),
		)
		return
	}

	stateWatcher := retry.StateChangeConf{
		Timeout: 30 * time.Minute,
		Pending: []string{"updating"},
		Target:  []string{"completed"},
		Refresh: func() (interface{}, string, error) {
			clusterListResponse, err := r.client.GetApiV1OrganizationsOrganizationIDClustersWithResponse(ctx, r.organizationID, nil)
			if err != nil {
				return nil, "", err
			}
			if clusterListResponse.StatusCode() != http.StatusOK || clusterListResponse.JSON200 == nil {
				err = fmt.Errorf("compute cluster read operation failed with status code %d", clusterListResponse.StatusCode())
				return nil, "", err
			}

			for _, cluster := range *clusterListResponse.JSON200 {
				if cluster.Metadata.Id == data.ID.ValueString() && cluster.Metadata.Tags != nil {
					tagList := *cluster.Metadata.Tags
					for _, tag := range tagList {
						if tag.Name == operationKey {
							return &cluster, "completed", nil
						}
					}
				}
			}

			return nil, "updating", nil
		},
	}

	state, err := stateWatcher.WaitForStateContext(ctx)
	if err != nil {
		e := errors.Unwrap(err)
		if e == nil {
			e = err
		}
		response.Diagnostics.AddError(
			"Failed to Wait for Compute Cluster to be Updated",
			fmt.Sprintf("An error occurred while waiting for the compute cluster to be updated: %s", e),
		)
		return
	}

	computeCluster, ok := state.(*nscale.ComputeClusterRead)
	if !ok || computeCluster == nil {
		response.Diagnostics.AddError(
			"Unexpected Resource Type",
			fmt.Sprintf("Expected *nscale.ComputeClusterRead, got: %T. Please contact the Nscale team for support.", computeCluster),
		)
		return
	}

	data = NewComputeClusterModel(computeCluster)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ComputeClusterResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var data ComputeClusterModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	clusterDeleteResponse, err := r.client.DeleteApiV1OrganizationsOrganizationIDProjectsProjectIDClustersClusterIDWithResponse(ctx, r.organizationID, r.projectID, data.ID.ValueString())
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Compute Cluster",
			fmt.Sprintf("An error occurred while deleting the compute cluster: %s", err),
		)
		return
	}
	if clusterDeleteResponse.StatusCode() != http.StatusAccepted {
		response.Diagnostics.AddError(
			"Failed to Delete Compute Cluster",
			fmt.Sprintf("The compute cluster deletion failed with status code %d.", clusterDeleteResponse.StatusCode()),
		)
		return
	}

	stateWatcher := retry.StateChangeConf{
		Timeout: 30 * time.Minute,
		Pending: []string{"deleting"},
		Target:  []string{"completed"},
		Refresh: func() (interface{}, string, error) {
			clusterListResponse, err := r.client.GetApiV1OrganizationsOrganizationIDClustersWithResponse(ctx, r.organizationID, nil)
			if err != nil {
				return nil, "", err
			}
			if clusterListResponse.StatusCode() != http.StatusOK || clusterListResponse.JSON200 == nil {
				err = fmt.Errorf("compute cluster read operation failed with status code %d", clusterListResponse.StatusCode())
				return nil, "", err
			}

			for _, cluster := range *clusterListResponse.JSON200 {
				if cluster.Metadata.Id == data.ID.ValueString() {
					return nil, "deleting", nil
				}
			}

			return &struct{}{}, "completed", nil
		},
	}

	if _, err = stateWatcher.WaitForStateContext(ctx); err != nil {
		e := errors.Unwrap(err)
		if e == nil {
			e = err
		}
		response.Diagnostics.AddError(
			"Failed to Wait for Compute Cluster to be Deleted",
			fmt.Sprintf("An error occurred while waiting for the compute cluster to be deleted: %s", e),
		)
		return
	}
}
