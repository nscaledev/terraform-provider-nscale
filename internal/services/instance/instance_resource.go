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

package instance

import (
	"context"
	"fmt"
	"net/http"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.ResourceWithConfigure   = &InstanceResource{}
	_ resource.ResourceWithImportState = &InstanceResource{}
)

type InstanceResourceModel struct {
	InstanceModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

type InstanceResource struct {
	client *nscale.Client
}

func NewInstanceResource() resource.Resource {
	return &InstanceResource{}
}

func (r *InstanceResource) Configure(
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

func (r *InstanceResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *InstanceResource) Metadata(
	ctx context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_instance"
}

func (r *InstanceResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Instance",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the instance.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the instance.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the instance.",
				Optional:            true,
			},
			"user_data": schema.StringAttribute{
				MarkdownDescription: "The data to pass to the instance at boot time.",
				Optional:            true,
				Validators: []validator.String{
					validators.Base64Validator{},
				},
			},
			"public_ip": schema.StringAttribute{
				MarkdownDescription: "The public IP address assigned to the instance.",
				Computed:            true,
			},
			"private_ip": schema.StringAttribute{
				MarkdownDescription: "The private IP address assigned to the instance.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"power_state": schema.StringAttribute{
				MarkdownDescription: "The power state of the instance.",
				Computed:            true,
			},
			"image_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the image used for the instance.",
				Required:            true,
			},
			"flavor_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the flavor used for the instance.",
				Required:            true,
			},
			"ssh_certificate_authority_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the SSH certificate authority used to bootstrap login trust when the backing server is created. Changing this value forces the instance to be replaced because the CA is installed by cloud-init on first boot and cannot be rotated on a running server.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the instance.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the instance is provisioned. If not specified, this defaults to the project ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the instance is provisioned.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the instance was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"network_interface": schema.SingleNestedBlock{
				MarkdownDescription: "The network interface configuration of the instance.",
				Attributes: map[string]schema.Attribute{
					"network_id": schema.StringAttribute{
						MarkdownDescription: "The identifier of the network where the instance is provisioned.",
						Required:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"enable_public_ip": schema.BoolAttribute{
						MarkdownDescription: "Whether the instance should have a public IP.",
						Optional:            true,
					},
					"security_group_ids": schema.ListAttribute{
						MarkdownDescription: "A list of security group identifiers to associate with the instance.",
						ElementType:         types.StringType,
						Optional:            true,
						Validators: []validator.List{
							listvalidator.SizeAtLeast(1),
						},
					},
					"allowed_destinations": schema.ListAttribute{
						MarkdownDescription: "A list of CIDR blocks that are allowed to egress from the instance without SNAT.",
						ElementType:         types.StringType,
						Optional:            true,
						Validators: []validator.List{
							listvalidator.SizeAtLeast(1),
							listvalidator.ValueStringsAre(validators.CIDRValidator{}),
						},
					},
				},
				Validators: []validator.Object{
					objectvalidator.IsRequired(),
				},
			},
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *InstanceResource) setDefaultIDs(data *InstanceResourceModel) {
	if data.ProjectID.ValueString() == "" {
		data.ProjectID = types.StringValue(r.client.ProjectID)
	}
}

func (r *InstanceResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[InstanceResourceModel](ctx, request.Plan.Get, r.setDefaultIDs)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleInstanceCreateParams(r.client.OrganizationID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	instanceCreateResponse, err := r.client.Compute.PostApiV2Instances(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Instance",
			fmt.Sprintf("An error occurred while creating the instance: %s", err),
		)
		return
	}
	defer instanceCreateResponse.Body.Close()

	instance, err := nscale.ReadJSONResponsePointer[computeapi.InstanceRead](instanceCreateResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create Instance",
			fmt.Sprintf("An error occurred while creating the instance: %s", err),
		)
		return
	}

	data.InstanceModel = NewInstanceModel(instance)
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[computeapi.InstanceRead]{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context) (*computeapi.InstanceRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
			targetID := instance.Metadata.Id
			return getInstance(ctx, targetID, r.client)
		},
	}

	instance, ok := stateWatcher.Wait(ctx, data.Timeouts, response)
	if !ok {
		return
	}

	data.InstanceModel = NewInstanceModel(instance)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *InstanceResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[InstanceResourceModel](ctx, request.State.Get, r.setDefaultIDs)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := nscale.ResourceReader[computeapi.InstanceRead]{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context, id string) (*computeapi.InstanceRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getInstance(ctx, id, r.client)
		},
	}

	instance, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data.InstanceModel = NewInstanceModel(instance)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *InstanceResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[InstanceResourceModel](ctx, request.Plan.Get, r.setDefaultIDs)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleInstanceUpdateParams()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	instanceUpdateResponse, err := r.client.Compute.PutApiV2InstancesInstanceID(ctx, id, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Instance",
			fmt.Sprintf("An error occurred while updating the instance: %s", err),
		)
		return
	}
	defer instanceUpdateResponse.Body.Close()

	if _, readErr := nscale.ReadJSONResponsePointer[computeapi.InstanceRead](instanceUpdateResponse); readErr != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		response.Diagnostics.AddError(
			"Failed to Update Instance",
			fmt.Sprintf("An error occurred while updating the instance: %s", readErr),
		)
		return
	}

	stateWatcher := nscale.UpdateStateWatcher[computeapi.InstanceRead]{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context) (*computeapi.InstanceRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getInstance(ctx, id, r.client)
		},
	}

	instance, ok := stateWatcher.Wait(ctx, operationTagKey, data.Timeouts, response)
	if !ok {
		return
	}

	data.InstanceModel = NewInstanceModel(instance)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *InstanceResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[InstanceResourceModel](ctx, request.State.Get, r.setDefaultIDs)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	instanceDeleteResponse, err := r.client.Compute.DeleteApiV2InstancesInstanceID(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Instance",
			fmt.Sprintf("An error occurred while deleting the instance: %s", err),
		)
		return
	}
	defer instanceDeleteResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(instanceDeleteResponse); err != nil {
		if e, ok := nscale.AsAPIError(err); ok && e.StatusCode != http.StatusNotFound {
			nscale.TerraformDebugLogAPIResponseBody(ctx, err)
			response.Diagnostics.AddError(
				"Failed to Delete Instance",
				fmt.Sprintf("An error occurred while deleting the instance: %s", err),
			)
			return
		}
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context) (any, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getInstance(ctx, id, r.client)
		},
	}

	stateWatcher.Wait(ctx, data.Timeouts, response)
}
