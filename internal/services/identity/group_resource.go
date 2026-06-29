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

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	identityapi "github.com/nscaledev/nscale-sdk-go/identity"
	identityids "github.com/unikorn-cloud/identity/pkg/ids"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.Resource                = &GroupResource{}
	_ resource.ResourceWithConfigure   = &GroupResource{}
	_ resource.ResourceWithImportState = &GroupResource{}
)

type GroupResourceModel struct {
	GroupModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// GroupResource embeds the generic CRUD base; only Schema and the adapter
// wiring below are group-specific.
type GroupResource struct {
	*nscale.GenericResource[GroupResourceModel, identityapi.GroupRead]
}

func NewGroupResource() resource.Resource {
	return &GroupResource{
		GenericResource: nscale.NewGenericResource(groupAdapter()),
	}
}

// groupAdapter wires the group-specific SDK calls and model mapping into the
// generic resource skeleton.
func groupAdapter() nscale.ResourceAdapter[GroupResourceModel, identityapi.GroupRead] {
	return nscale.ResourceAdapter[GroupResourceModel, identityapi.GroupRead]{
		TypeNameSuffix: "_identity_group",
		Title:          "Group",
		Name:           "group",
		Create:         groupCreate,
		Update:         groupUpdate,
		Delete:         groupDelete,
		Get: func(
			ctx context.Context,
			client *nscale.Client,
			id string,
		) (*identityapi.GroupRead, nscale.ResourceStatus, error) {
			return getGroupStatus(ctx, id, client)
		},
		ToModel: func(api *identityapi.GroupRead, dst *GroupResourceModel) {
			dst.GroupModel = NewGroupModel(api)
		},
		IDFromModel:       func(m GroupResourceModel) string { return m.ID.ValueString() },
		TimeoutsFromModel: func(m GroupResourceModel) tftimeouts.Value { return m.Timeouts },
	}
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

func groupCreate(
	ctx context.Context,
	client *nscale.Client,
	plan GroupResourceModel,
) (*identityapi.GroupRead, diag.Diagnostics) {
	params, diagnostics := plan.NscaleGroupCreateParams(ctx)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	organizationID, err := identityids.ParseOrganizationID(client.OrganizationID)
	if err != nil {
		diagnostics.AddError(
			"Invalid Organization ID",
			fmt.Sprintf("Could not parse organization ID %q: %s", client.OrganizationID, err),
		)
		return nil, diagnostics
	}

	createResponse, err := client.Identity.PostApiV1OrganizationsOrganizationIDGroups(
		ctx,
		organizationID,
		params,
	)
	if err != nil {
		diagnostics.AddError(
			"Failed to Create Group",
			fmt.Sprintf("An error occurred while creating the group: %s", err),
		)
		return nil, diagnostics
	}
	defer createResponse.Body.Close()

	group, err := nscale.ReadJSONResponsePointer[identityapi.GroupRead](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Create Group",
			fmt.Sprintf("An error occurred while creating the group: %s", err),
		)
		return nil, diagnostics
	}

	return group, nil
}

func groupUpdate(
	ctx context.Context,
	client *nscale.Client,
	id string,
	plan GroupResourceModel,
) (string, diag.Diagnostics) {
	params, diagnostics := plan.NscaleGroupUpdateParams(ctx)
	if diagnostics.HasError() {
		return "", diagnostics
	}

	organizationID, err := identityids.ParseOrganizationID(client.OrganizationID)
	if err != nil {
		diagnostics.AddError(
			"Invalid Organization ID",
			fmt.Sprintf("Could not parse organization ID %q: %s", client.OrganizationID, err),
		)
		return "", diagnostics
	}

	groupID, err := identityids.ParseGroupID(id)
	if err != nil {
		diagnostics.AddError(
			"Invalid Group ID",
			fmt.Sprintf("Could not parse group ID %q: %s", id, err),
		)
		return "", diagnostics
	}

	// Tag the update so the watcher can confirm the PUT has propagated through
	// the cache-backed API before reading back a terminal status.
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	updateResponse, err := client.Identity.PutApiV1OrganizationsOrganizationIDGroupsGroupid(
		ctx,
		organizationID,
		groupID,
		params,
	)
	if err != nil {
		diagnostics.AddError(
			"Failed to Update Group",
			fmt.Sprintf("An error occurred while updating the group: %s", err),
		)
		return "", diagnostics
	}
	defer updateResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(updateResponse); err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Update Group",
			fmt.Sprintf("An error occurred while updating the group: %s", err),
		)
		return "", diagnostics
	}

	return operationTagKey, nil
}

func groupDelete(ctx context.Context, client *nscale.Client, id string) error {
	organizationID, err := identityids.ParseOrganizationID(client.OrganizationID)
	if err != nil {
		return err
	}

	groupID, err := identityids.ParseGroupID(id)
	if err != nil {
		return err
	}

	deleteResponse, err := client.Identity.DeleteApiV1OrganizationsOrganizationIDGroupsGroupid(
		ctx,
		organizationID,
		groupID,
	)
	if err != nil {
		return err
	}
	defer deleteResponse.Body.Close()

	return nscale.ReadEmptyResponse(deleteResponse)
}
