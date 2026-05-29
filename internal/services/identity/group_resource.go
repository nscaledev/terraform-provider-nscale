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

package identity

import (
	"context"
	"fmt"
	"net/http"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	identityapi "github.com/nscaledev/nscale-sdk-go/identity"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.ResourceWithConfigure   = &GroupResource{}
	_ resource.ResourceWithImportState = &GroupResource{}
)

type GroupResourceModel struct {
	GroupModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

type GroupResource struct {
	client *nscale.Client
}

func NewGroupResource() resource.Resource {
	return &GroupResource{}
}

func (r *GroupResource) Configure(
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

func (r *GroupResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *GroupResource) Metadata(
	ctx context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_identity_group"
}

func (r *GroupResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Identity Group",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the group.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the group.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the group.",
				Optional:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the group.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"role_ids": schema.SetAttribute{
				MarkdownDescription: "The set of role identifiers granted to members of this group. Roles are pre-configured platform resources referenced by their identifier.",
				ElementType:         types.StringType,
				Required:            true,
			},
			"service_account_ids": schema.SetAttribute{
				MarkdownDescription: "The set of service account identifiers that are members of this group.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"user_ids": schema.SetAttribute{
				MarkdownDescription: "The set of user identifiers that are members of this group. Users are provisioned by the external identity platform and referenced here by their identifier.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"subjects": schema.SetNestedAttribute{
				MarkdownDescription: "The set of federated identity subjects that are members of this group.",
				Optional:            true,
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"issuer": schema.StringAttribute{
							MarkdownDescription: "The OIDC issuer URL that asserts this subject.",
							Required:            true,
						},
						"id": schema.StringAttribute{
							MarkdownDescription: "The subject identifier issued by the issuer.",
							Required:            true,
						},
						"email": schema.StringAttribute{
							MarkdownDescription: "The email address for the subject, when supplied by the issuer.",
							Optional:            true,
							Computed:            true,
						},
					},
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the group was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the group.",
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

// getGroup adapts the package-level getGroup to the generic watcher's getFunc
// signature by binding the configured client.
func (r *GroupResource) getGroup(ctx context.Context, id string) (*identityapi.GroupRead, error) {
	return getGroup(ctx, id, r.client)
}

func (r *GroupResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[GroupResourceModel](ctx, request.Plan.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleGroupCreateParams()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	createResponse, err := r.client.Identity.PostApiV1OrganizationsOrganizationIDGroups(
		ctx,
		r.client.OrganizationID,
		params,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Group",
			fmt.Sprintf("An error occurred while creating the group: %s", err),
		)
		return
	}
	defer createResponse.Body.Close()

	group, err := nscale.ReadJSONResponsePointer[identityapi.GroupRead](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create Group",
			fmt.Sprintf("An error occurred while creating the group: %s", err),
		)
		return
	}

	// Record the ID before waiting so a timeout does not orphan the resource.
	data.GroupModel = NewGroupModel(group)
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	// Groups provision synchronously today, but they expose a provisioning
	// status, so wait for a terminal state for consistency and future-proofing.
	timeout, diagnostics := data.Timeouts.Create(ctx, defaultStateTimeout)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	group, err = waitForProvisioned(ctx, group.Metadata.Id, timeout, r.getGroup, groupProvisioningStatus)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create Group",
			fmt.Sprintf("An error occurred while waiting for the group to be provisioned: %s", err),
		)
		return
	}

	data.GroupModel = NewGroupModel(group)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *GroupResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[GroupResourceModel](ctx, request.State.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	group, err := getGroup(ctx, id, r.client)
	if err != nil {
		if e, ok := nscale.AsAPIError(err); ok && e.StatusCode == http.StatusNotFound {
			response.Diagnostics.AddWarning(
				"Group Not Found",
				fmt.Sprintf(
					"The group with ID %s was not found on the server and will be removed from the state file.",
					id,
				),
			)
			response.State.RemoveResource(ctx)
			return
		}

		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Group",
			fmt.Sprintf("An error occurred while retrieving the group: %s", err),
		)
		return
	}

	data.GroupModel = NewGroupModel(group)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *GroupResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[GroupResourceModel](ctx, request.Plan.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	params, diagnostics := data.NscaleGroupUpdateParams()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	updateResponse, err := r.client.Identity.PutApiV1OrganizationsOrganizationIDGroupsGroupid(
		ctx,
		r.client.OrganizationID,
		id,
		params,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Group",
			fmt.Sprintf("An error occurred while updating the group: %s", err),
		)
		return
	}
	defer updateResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(updateResponse); err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Update Group",
			fmt.Sprintf("An error occurred while updating the group: %s", err),
		)
		return
	}

	group, err := getGroup(ctx, id, r.client)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Group After Update",
			fmt.Sprintf("An error occurred while retrieving the group: %s", err),
		)
		return
	}

	data.GroupModel = NewGroupModel(group)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *GroupResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[GroupResourceModel](ctx, request.State.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	deleteResponse, err := r.client.Identity.DeleteApiV1OrganizationsOrganizationIDGroupsGroupid(
		ctx,
		r.client.OrganizationID,
		id,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Group",
			fmt.Sprintf("An error occurred while deleting the group: %s", err),
		)
		return
	}
	defer deleteResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(deleteResponse); err != nil {
		if e, ok := nscale.AsAPIError(err); ok && e.StatusCode != http.StatusNotFound {
			nscale.TerraformDebugLogAPIResponseBody(ctx, err)
			response.Diagnostics.AddError(
				"Failed to Delete Group",
				fmt.Sprintf("An error occurred while deleting the group: %s", err),
			)
			return
		}
	}

	// Groups deprovision synchronously today, but wait for the API to report
	// the group gone for consistency with the project resource and to guard
	// against the operation becoming asynchronous.
	timeout, diagnostics := data.Timeouts.Delete(ctx, defaultStateTimeout)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	if err = waitForDeleted(ctx, id, timeout, r.getGroup); err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Delete Group",
			fmt.Sprintf("An error occurred while waiting for the group to be deleted: %s", err),
		)
		return
	}
}
