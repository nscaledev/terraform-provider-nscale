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

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	computeapi "github.com/nscaledev/nscale-sdk-go/compute"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.Resource                = &InstanceResource{}
	_ resource.ResourceWithConfigure   = &InstanceResource{}
	_ resource.ResourceWithImportState = &InstanceResource{}
)

type InstanceResourceModel struct {
	InstanceModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// InstanceResource embeds the generic CRUD base; only Schema and the adapter
// wiring below are instance-specific.
type InstanceResource struct {
	*nscale.GenericResource[InstanceResourceModel, computeapi.InstanceRead]
}

func NewInstanceResource() resource.Resource {
	return &InstanceResource{
		GenericResource: nscale.NewGenericResource(instanceAdapter()),
	}
}

// instanceAdapter wires the instance-specific SDK calls and model mapping into
// the generic resource skeleton.
func instanceAdapter() nscale.ResourceAdapter[InstanceResourceModel, computeapi.InstanceRead] {
	return nscale.ResourceAdapter[InstanceResourceModel, computeapi.InstanceRead]{
		TypeNameSuffix: "_instance",
		Title:          "Instance",
		Name:           "instance",
		Create:         instanceCreate,
		Update:         instanceUpdate,
		Delete:         instanceDelete,
		Get: func(
			ctx context.Context,
			client *nscale.Client,
			id string,
		) (*computeapi.InstanceRead, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getInstance(ctx, id, client))
		},
		ToModel: func(api *computeapi.InstanceRead, dst *InstanceResourceModel) {
			dst.InstanceModel = NewInstanceModel(api)
		},
		IDFromModel:       func(m InstanceResourceModel) string { return m.ID.ValueString() },
		TimeoutsFromModel: func(m InstanceResourceModel) tftimeouts.Value { return m.Timeouts },
	}
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

func instanceCreate(
	ctx context.Context,
	client *nscale.Client,
	plan InstanceResourceModel,
) (*computeapi.InstanceRead, diag.Diagnostics) {
	// Resolve the project ID from the resource or the provider default, erroring
	// when neither is set. This is only meaningful at create time.
	projectID, diagnostics := client.ResolveProjectID(plan.ProjectID.ValueString())
	if diagnostics.HasError() {
		return nil, diagnostics
	}
	plan.ProjectID = types.StringValue(projectID)

	params, paramDiagnostics := plan.NscaleInstanceCreateParams(client.OrganizationID)
	diagnostics.Append(paramDiagnostics...)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	createResponse, err := client.Compute.PostApiV2Instances(ctx, params)
	if err != nil {
		diagnostics.AddError(
			"Failed to Create Instance",
			fmt.Sprintf("An error occurred while creating the instance: %s", err),
		)
		return nil, diagnostics
	}
	defer createResponse.Body.Close()

	instance, err := nscale.ReadJSONResponsePointer[computeapi.InstanceRead](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Create Instance",
			fmt.Sprintf("An error occurred while creating the instance: %s", err),
		)
		return nil, diagnostics
	}

	return instance, nil
}

func instanceUpdate(
	ctx context.Context,
	client *nscale.Client,
	id string,
	plan InstanceResourceModel,
) (string, diag.Diagnostics) {
	params, diagnostics := plan.NscaleInstanceUpdateParams()
	if diagnostics.HasError() {
		return "", diagnostics
	}

	// Tag the update so the watcher can confirm the PUT has propagated through
	// the cache-backed API before reading back a terminal status.
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	updateResponse, err := client.Compute.PutApiV2InstancesInstanceID(ctx, id, params)
	if err != nil {
		diagnostics.AddError(
			"Failed to Update Instance",
			fmt.Sprintf("An error occurred while updating the instance: %s", err),
		)
		return "", diagnostics
	}
	defer updateResponse.Body.Close()

	if _, readErr := nscale.ReadJSONResponsePointer[computeapi.InstanceRead](updateResponse); readErr != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		diagnostics.AddError(
			"Failed to Update Instance",
			fmt.Sprintf("An error occurred while updating the instance: %s", readErr),
		)
		return "", diagnostics
	}

	return operationTagKey, nil
}

func instanceDelete(ctx context.Context, client *nscale.Client, id string) error {
	deleteResponse, err := client.Compute.DeleteApiV2InstancesInstanceID(ctx, id)
	if err != nil {
		return err
	}
	defer deleteResponse.Body.Close()

	return nscale.ReadEmptyResponse(deleteResponse)
}
