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
	_ resource.Resource                = &ProjectResource{}
	_ resource.ResourceWithConfigure   = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
)

type ProjectResourceModel struct {
	ProjectModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// ProjectResource embeds the generic CRUD base; only Schema and the adapter
// wiring below are project-specific.
type ProjectResource struct {
	*nscale.GenericResource[ProjectResourceModel, identityapi.ProjectRead]
}

func NewProjectResource() resource.Resource {
	return &ProjectResource{
		GenericResource: nscale.NewGenericResource(projectAdapter()),
	}
}

// projectAdapter wires the project-specific SDK calls and model mapping into the
// generic resource skeleton.
func projectAdapter() nscale.ResourceAdapter[ProjectResourceModel, identityapi.ProjectRead] {
	return nscale.ResourceAdapter[ProjectResourceModel, identityapi.ProjectRead]{
		TypeNameSuffix: "_identity_project",
		Title:          "Project",
		Name:           "project",
		Create:         projectCreate,
		Update:         projectUpdate,
		Delete:         projectDelete,
		Get: func(
			ctx context.Context,
			client *nscale.Client,
			id string,
		) (*identityapi.ProjectRead, nscale.ResourceStatus, error) {
			return getProjectStatus(ctx, id, client)
		},
		ToModel: func(api *identityapi.ProjectRead, dst *ProjectResourceModel) {
			dst.ProjectModel = NewProjectModel(api)
		},
		IDFromModel:       func(m ProjectResourceModel) string { return m.ID.ValueString() },
		TimeoutsFromModel: func(m ProjectResourceModel) tftimeouts.Value { return m.Timeouts },
	}
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

func projectCreate(
	ctx context.Context,
	client *nscale.Client,
	plan ProjectResourceModel,
) (*identityapi.ProjectRead, diag.Diagnostics) {
	params, diagnostics := plan.NscaleProjectCreateParams(ctx)
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

	createResponse, err := client.Identity.PostApiV1OrganizationsOrganizationIDProjects(
		ctx,
		organizationID,
		params,
	)
	if err != nil {
		diagnostics.AddError(
			"Failed to Create Project",
			fmt.Sprintf("An error occurred while creating the project: %s", err),
		)
		return nil, diagnostics
	}
	defer createResponse.Body.Close()

	project, err := nscale.ReadJSONResponsePointer[identityapi.ProjectRead](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Create Project",
			fmt.Sprintf("An error occurred while creating the project: %s", err),
		)
		return nil, diagnostics
	}

	return project, nil
}

func projectUpdate(
	ctx context.Context,
	client *nscale.Client,
	id string,
	plan ProjectResourceModel,
) (string, diag.Diagnostics) {
	params, diagnostics := plan.NscaleProjectUpdateParams(ctx)
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

	projectID, err := identityids.ParseProjectID(id)
	if err != nil {
		diagnostics.AddError(
			"Invalid Project ID",
			fmt.Sprintf("Could not parse project ID %q: %s", id, err),
		)
		return "", diagnostics
	}

	// Tag the update so the watcher can confirm the PUT has propagated through
	// the cache-backed API before reading back a terminal status.
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	updateResponse, err := client.Identity.PutApiV1OrganizationsOrganizationIDProjectsProjectID(
		ctx,
		organizationID,
		projectID,
		params,
	)
	if err != nil {
		diagnostics.AddError(
			"Failed to Update Project",
			fmt.Sprintf("An error occurred while updating the project: %s", err),
		)
		return "", diagnostics
	}
	defer updateResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(updateResponse); err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Update Project",
			fmt.Sprintf("An error occurred while updating the project: %s", err),
		)
		return "", diagnostics
	}

	return operationTagKey, nil
}

func projectDelete(ctx context.Context, client *nscale.Client, id string) error {
	organizationID, err := identityids.ParseOrganizationID(client.OrganizationID)
	if err != nil {
		return err
	}

	projectID, err := identityids.ParseProjectID(id)
	if err != nil {
		return err
	}

	deleteResponse, err := client.Identity.DeleteApiV1OrganizationsOrganizationIDProjectsProjectID(
		ctx,
		organizationID,
		projectID,
	)
	if err != nil {
		return err
	}
	defer deleteResponse.Body.Close()

	return nscale.ReadEmptyResponse(deleteResponse)
}
