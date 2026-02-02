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

package securitygroup

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

var (
	_ resource.ResourceWithConfigure   = &SecurityGroupResource{}
	_ resource.ResourceWithImportState = &SecurityGroupResource{}
)

type SecurityGroupResource struct {
	client *nscale.Client
}

func NewSecurityGroupResource() resource.Resource {
	return &SecurityGroupResource{}
}

func (r *SecurityGroupResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
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

func (r *SecurityGroupResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *SecurityGroupResource) Metadata(ctx context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_security_group"
}

func (r *SecurityGroupResource) Schema(ctx context.Context, request resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Security Group",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the security group.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the security group.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the security group.",
				Optional:            true,
			},
			"rules": schema.ListNestedAttribute{
				MarkdownDescription: "A list of rules for the security group.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							MarkdownDescription: "The type of the security group rule. Valid values are `ingress` or `egress`.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("ingress", "egress"),
							},
						},
						"protocol": schema.StringAttribute{
							MarkdownDescription: "The protocol for the security group rule. Valid values are `any`, `tcp`, `udp`, `icmp`, or `vrrp`.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("any", "tcp", "udp", "icmp", "vrrp"),
							},
						},
						"from_port": schema.Int32Attribute{
							MarkdownDescription: "The starting port of the port range for the security group rule.",
							Optional:            true,
						},
						"to_port": schema.Int32Attribute{
							MarkdownDescription: "The ending port of the port range for the security group rule.",
							Optional:            true,
						},
						"cidr_block": schema.StringAttribute{
							MarkdownDescription: "The CIDR block for the security group rule. Default is `0.0.0.0/0`, which allows traffic from any IP address.",
							Optional:            true,
							Validators: []validator.String{
								validators.CIDRValidator{},
							},
						},
					},
				},
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the network to where the security group is attached.",
				Required:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the security group is provisioned. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the security group was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *SecurityGroupResource) setDefaultRegionID(data *SecurityGroupModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(r.client.RegionID)
	}
}

func (r *SecurityGroupResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	data, diagnostics := nscale.ReadTerraformState[SecurityGroupModel](ctx, request.Plan.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleSecurityGroupCreateParams()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	securityGroupCreateResponse, err := r.client.Region.PostApiV2Securitygroups(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Security Group",
			fmt.Sprintf("An error occurred while creating the security group: %s", err),
		)
		return
	}

	securityGroup, err := nscale.ReadJSONResponsePointer[regionapi.SecurityGroupV2Read](securityGroupCreateResponse)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Security Group",
			fmt.Sprintf("An error occurred while creating the security group: %s", err),
		)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[regionapi.SecurityGroupV2Read]{
		ResourceTitle: "Security Group",
		ResourceName:  "security group",
		GetFunc: func(ctx context.Context) (*regionapi.SecurityGroupV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			targetID := securityGroup.Metadata.Id
			return getSecurityGroup(ctx, targetID, r.client)
		},
	}

	securityGroup, ok := stateWatcher.Wait(ctx, response)
	if !ok {
		return
	}

	data = NewSecurityGroupModel(securityGroup)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *SecurityGroupResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[SecurityGroupModel](ctx, request.State.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := nscale.ResourceReader[regionapi.SecurityGroupV2Read]{
		ResourceTitle: "Security Group",
		ResourceName:  "security group",
		GetFunc: func(ctx context.Context, id string) (*regionapi.SecurityGroupV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getSecurityGroup(ctx, id, r.client)
		},
	}

	securityGroup, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data = NewSecurityGroupModel(securityGroup)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *SecurityGroupResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	data, diagnostics := nscale.ReadTerraformState[SecurityGroupModel](ctx, request.Plan.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleSecurityGroupUpdateParams()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	securityGroupUpdateResponse, err := r.client.Region.PutApiV2SecuritygroupsSecurityGroupID(ctx, id, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Security Group",
			fmt.Sprintf("An error occurred while updating the security group: %s", err),
		)
		return
	}

	securityGroup, err := nscale.ReadJSONResponsePointer[regionapi.SecurityGroupV2Read](securityGroupUpdateResponse)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Security Group",
			fmt.Sprintf("An error occurred while updating the security group: %s", err),
		)
		return
	}

	stateWatcher := nscale.UpdateStateWatcher[regionapi.SecurityGroupV2Read]{
		ResourceTitle: "Security Group",
		ResourceName:  "security group",
		GetFunc: func(ctx context.Context) (*regionapi.SecurityGroupV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getSecurityGroup(ctx, id, r.client)
		},
	}

	securityGroup, ok := stateWatcher.Wait(ctx, operationTagKey, response)
	if !ok {
		return
	}

	data = NewSecurityGroupModel(securityGroup)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *SecurityGroupResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	data, diagnostics := nscale.ReadTerraformState[SecurityGroupModel](ctx, request.State.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	securityGroupDeleteResponse, err := r.client.Region.DeleteApiV2SecuritygroupsSecurityGroupID(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Security Group",
			fmt.Sprintf("An error occurred while deleting the security group: %s", err),
		)
		return
	}

	if err = nscale.ReadEmptyResponse(securityGroupDeleteResponse); err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Security Group",
			fmt.Sprintf("An error occurred while deleting the security group: %s", err),
		)
		return
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "Security Group",
		ResourceName:  "security group",
		GetFunc: func(ctx context.Context) (any, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getSecurityGroup(ctx, id, r.client)
		},
	}

	stateWatcher.Wait(ctx, response)
}
