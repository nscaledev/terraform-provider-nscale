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
	"time"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
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
	regionapi "github.com/nscaledev/nscale-sdk-go/region"
	regionids "github.com/unikorn-cloud/region/pkg/ids"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

const defaultDeleteTimeout = 30 * time.Minute

var (
	_ resource.ResourceWithConfigure   = &SecurityGroupResource{}
	_ resource.ResourceWithImportState = &SecurityGroupResource{}
)

type SecurityGroupResourceModel struct {
	SecurityGroupModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

type SecurityGroupResource struct {
	client *nscale.Client
}

func NewSecurityGroupResource() resource.Resource {
	return &SecurityGroupResource{}
}

func (r *SecurityGroupResource) Configure(
	ctx context.Context,
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

func (r *SecurityGroupResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *SecurityGroupResource) Metadata(
	ctx context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_security_group"
}

func (r *SecurityGroupResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Security Group",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the security group.",
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
				MarkdownDescription: "The identifier of the network to which the security group is attached.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the security group.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the security group is provisioned.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the security group was created.",
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

func (r *SecurityGroupResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[SecurityGroupResourceModel](ctx, request.Plan.Get)
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
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create Security Group",
			fmt.Sprintf("An error occurred while creating the security group: %s", err),
		)
		return
	}

	data.SecurityGroupModel = NewSecurityGroupModel(securityGroup)
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[regionapi.SecurityGroupV2Read]{
		ResourceTitle: "Security Group",
		ResourceName:  "security group",
		GetFunc: func(ctx context.Context) (*regionapi.SecurityGroupV2Read, nscale.ResourceStatus, error) {
			targetID := securityGroup.Metadata.Id
			return nscale.AdaptProjectScoped(getSecurityGroup(ctx, targetID, r.client))
		},
	}

	securityGroup, ok := stateWatcher.Wait(ctx, data.Timeouts, response)
	if !ok {
		return
	}

	data.SecurityGroupModel = NewSecurityGroupModel(securityGroup)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *SecurityGroupResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[SecurityGroupResourceModel](ctx, request.State.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := nscale.ResourceReader[regionapi.SecurityGroupV2Read]{
		ResourceTitle: "Security Group",
		ResourceName:  "security group",
		GetFunc: func(ctx context.Context, id string) (*regionapi.SecurityGroupV2Read, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getSecurityGroup(ctx, id, r.client))
		},
	}

	securityGroup, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data.SecurityGroupModel = NewSecurityGroupModel(securityGroup)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *SecurityGroupResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[SecurityGroupResourceModel](ctx, request.Plan.Get)
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

	securityGroupID, ok := nscale.ParseID(id, "Security Group", regionids.ParseSecurityGroupID, &response.Diagnostics)
	if !ok {
		return
	}

	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	securityGroupUpdateResponse, err := r.client.Region.PutApiV2SecuritygroupsSecurityGroupID(
		ctx,
		securityGroupID,
		params,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Security Group",
			fmt.Sprintf("An error occurred while updating the security group: %s", err),
		)
		return
	}

	if _, readErr := nscale.ReadJSONResponsePointer[regionapi.SecurityGroupV2Read](
		securityGroupUpdateResponse,
	); readErr != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		response.Diagnostics.AddError(
			"Failed to Update Security Group",
			fmt.Sprintf("An error occurred while updating the security group: %s", readErr),
		)
		return
	}

	stateWatcher := nscale.UpdateStateWatcher[regionapi.SecurityGroupV2Read]{
		ResourceTitle: "Security Group",
		ResourceName:  "security group",
		GetFunc: func(ctx context.Context) (*regionapi.SecurityGroupV2Read, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getSecurityGroup(ctx, id, r.client))
		},
	}

	securityGroup, ok := stateWatcher.Wait(ctx, operationTagKey, data.Timeouts, response)
	if !ok {
		return
	}

	data.SecurityGroupModel = NewSecurityGroupModel(securityGroup)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *SecurityGroupResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[SecurityGroupResourceModel](ctx, request.State.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	securityGroupID, ok := nscale.ParseID(id, "Security Group", regionids.ParseSecurityGroupID, &response.Diagnostics)
	if !ok {
		return
	}

	deleteTimeout, diagnostics := data.Timeouts.Delete(ctx, defaultDeleteTimeout)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	// Retry while the API reports the SG is still in use — typically a parallel
	// instance update is dropping the reference. See nscale.RetryDelete.
	err := nscale.RetryDelete(ctx, deleteTimeout, func(ctx context.Context) (error, bool) {
		deleteResponse, deleteErr := r.client.Region.DeleteApiV2SecuritygroupsSecurityGroupID(ctx, securityGroupID)
		if deleteErr != nil {
			return deleteErr, false
		}
		defer deleteResponse.Body.Close()
		if readErr := nscale.ReadEmptyResponse(deleteResponse); readErr != nil {
			return readErr, nscale.IsAPIErrorInUse(readErr)
		}
		return nil, false
	})
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Delete Security Group",
			fmt.Sprintf("An error occurred while deleting the security group: %s. "+
				"If the security group is still attached to one or more instances, "+
				"remove the reference from `network_interface.security_group_ids` and re-apply.", err),
		)
		return
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "Security Group",
		ResourceName:  "security group",
		GetFunc: func(ctx context.Context) (any, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getSecurityGroup(ctx, id, r.client))
		},
	}

	stateWatcher.Wait(ctx, data.Timeouts, response)
}
