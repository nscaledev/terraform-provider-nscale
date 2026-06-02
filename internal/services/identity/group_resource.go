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
				MarkdownDescription: "The set of identity subjects that are members of this group. " +
					"This is read-only: the identity service derives it from `user_ids` " +
					"(each member user produces a subject) and any federated identities. " +
					"Manage membership through `user_ids`, not this attribute.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"issuer": schema.StringAttribute{
							MarkdownDescription: "The OIDC issuer URL that asserts this subject.",
							Computed:            true,
						},
						"id": schema.StringAttribute{
							MarkdownDescription: "The subject identifier issued by the issuer.",
							Computed:            true,
						},
						"email": schema.StringAttribute{
							MarkdownDescription: "The email address for the subject, when supplied by the issuer.",
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

	params, diagnostics := data.NscaleGroupCreateParams(ctx)
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
	stateWatcher := nscale.CreateStateWatcher[identityapi.GroupRead]{
		ResourceTitle: "Group",
		ResourceName:  "group",
		GetFunc: func(ctx context.Context) (*identityapi.GroupRead, nscale.ResourceStatus, error) {
			return getGroupStatus(ctx, group.Metadata.Id, r.client)
		},
	}

	group, ok := stateWatcher.Wait(ctx, data.Timeouts, response)
	if !ok {
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

	resourceReader := nscale.ResourceReader[identityapi.GroupRead]{
		ResourceTitle: "Group",
		ResourceName:  "group",
		GetFunc: func(ctx context.Context, id string) (*identityapi.GroupRead, nscale.ResourceStatus, error) {
			return getGroupStatus(ctx, id, r.client)
		},
	}

	group, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
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

	params, diagnostics := data.NscaleGroupUpdateParams(ctx)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	// Tag the update so the watcher can confirm the PUT has propagated through
	// the cache-backed API before reading back a terminal status.
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

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

	// Mirror the create path: wait for the operation tag to land (and a terminal
	// status) rather than reading the possibly-stale status straight after the
	// PUT, so a membership change cannot leave provisioning_status drifted.
	stateWatcher := nscale.UpdateStateWatcher[identityapi.GroupRead]{
		ResourceTitle: "Group",
		ResourceName:  "group",
		GetFunc: func(ctx context.Context) (*identityapi.GroupRead, nscale.ResourceStatus, error) {
			return getGroupStatus(ctx, id, r.client)
		},
	}

	group, ok := stateWatcher.Wait(ctx, operationTagKey, data.Timeouts, response)
	if !ok {
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
	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "Group",
		ResourceName:  "group",
		GetFunc: func(ctx context.Context) (any, nscale.ResourceStatus, error) {
			return getGroupStatus(ctx, id, r.client)
		},
	}

	stateWatcher.Wait(ctx, data.Timeouts, response)
}
