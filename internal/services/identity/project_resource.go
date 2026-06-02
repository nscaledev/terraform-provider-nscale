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
	_ resource.ResourceWithConfigure   = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
)

type ProjectResourceModel struct {
	ProjectModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

type ProjectResource struct {
	client *nscale.Client
}

func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

func (r *ProjectResource) Configure(
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

func (r *ProjectResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *ProjectResource) Metadata(
	ctx context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_identity_project"
}

func (r *ProjectResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Identity Project",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the project.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the project.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the project.",
				Optional:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the project.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"group_ids": schema.SetAttribute{
				MarkdownDescription: "The set of group identifiers that are granted access to the project.",
				ElementType:         types.StringType,
				Required:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the project was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the project.",
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

func (r *ProjectResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ProjectResourceModel](ctx, request.Plan.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleProjectCreateParams(ctx)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	createResponse, err := r.client.Identity.PostApiV1OrganizationsOrganizationIDProjects(
		ctx,
		r.client.OrganizationID,
		params,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Project",
			fmt.Sprintf("An error occurred while creating the project: %s", err),
		)
		return
	}
	defer createResponse.Body.Close()

	project, err := nscale.ReadJSONResponsePointer[identityapi.ProjectRead](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create Project",
			fmt.Sprintf("An error occurred while creating the project: %s", err),
		)
		return
	}

	// Record the ID before waiting so a timeout does not orphan the resource.
	data.ProjectModel = NewProjectModel(project)
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	// Project creation is asynchronous (the create response returns "pending").
	stateWatcher := nscale.CreateStateWatcher[identityapi.ProjectRead]{
		ResourceTitle: "Project",
		ResourceName:  "project",
		GetFunc: func(ctx context.Context) (*identityapi.ProjectRead, nscale.ResourceStatus, error) {
			return getProjectStatus(ctx, project.Metadata.Id, r.client)
		},
	}

	project, ok := stateWatcher.Wait(ctx, data.Timeouts, response)
	if !ok {
		return
	}

	data.ProjectModel = NewProjectModel(project)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ProjectResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ProjectResourceModel](ctx, request.State.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := nscale.ResourceReader[identityapi.ProjectRead]{
		ResourceTitle: "Project",
		ResourceName:  "project",
		GetFunc: func(ctx context.Context, id string) (*identityapi.ProjectRead, nscale.ResourceStatus, error) {
			return getProjectStatus(ctx, id, r.client)
		},
	}

	project, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data.ProjectModel = NewProjectModel(project)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ProjectResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ProjectResourceModel](ctx, request.Plan.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	params, diagnostics := data.NscaleProjectUpdateParams(ctx)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	// Tag the update so the watcher can confirm the PUT has propagated through
	// the cache-backed API before reading back a terminal status.
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	updateResponse, err := r.client.Identity.PutApiV1OrganizationsOrganizationIDProjectsProjectID(
		ctx,
		r.client.OrganizationID,
		id,
		params,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Project",
			fmt.Sprintf("An error occurred while updating the project: %s", err),
		)
		return
	}
	defer updateResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(updateResponse); err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Update Project",
			fmt.Sprintf("An error occurred while updating the project: %s", err),
		)
		return
	}

	// Updating group_ids can put the project back into a provisioning state, so
	// wait for the operation tag to land (and a terminal status) rather than
	// reading the possibly-stale status straight after the PUT.
	stateWatcher := nscale.UpdateStateWatcher[identityapi.ProjectRead]{
		ResourceTitle: "Project",
		ResourceName:  "project",
		GetFunc: func(ctx context.Context) (*identityapi.ProjectRead, nscale.ResourceStatus, error) {
			return getProjectStatus(ctx, id, r.client)
		},
	}

	project, ok := stateWatcher.Wait(ctx, operationTagKey, data.Timeouts, response)
	if !ok {
		return
	}

	data.ProjectModel = NewProjectModel(project)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ProjectResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ProjectResourceModel](ctx, request.State.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	deleteResponse, err := r.client.Identity.DeleteApiV1OrganizationsOrganizationIDProjectsProjectID(
		ctx,
		r.client.OrganizationID,
		id,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Project",
			fmt.Sprintf("An error occurred while deleting the project: %s", err),
		)
		return
	}
	defer deleteResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(deleteResponse); err != nil {
		if e, ok := nscale.AsAPIError(err); ok && e.StatusCode != http.StatusNotFound {
			nscale.TerraformDebugLogAPIResponseBody(ctx, err)
			response.Diagnostics.AddError(
				"Failed to Delete Project",
				fmt.Sprintf("An error occurred while deleting the project: %s", err),
			)
			return
		}
	}

	// Project deletion is asynchronous (DELETE returns 202 and the project
	// lingers in "deprovisioning"). Wait until it is actually gone so the
	// resource does not leak and a same-name recreate does not race.
	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "Project",
		ResourceName:  "project",
		GetFunc: func(ctx context.Context) (any, nscale.ResourceStatus, error) {
			return getProjectStatus(ctx, id, r.client)
		},
	}

	stateWatcher.Wait(ctx, data.Timeouts, response)
}
