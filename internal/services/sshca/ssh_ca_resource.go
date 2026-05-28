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

package sshca

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.ResourceWithConfigure   = &SSHCertificateAuthorityResource{}
	_ resource.ResourceWithImportState = &SSHCertificateAuthorityResource{}
)

type SSHCertificateAuthorityResourceModel struct {
	SSHCertificateAuthorityModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

type SSHCertificateAuthorityResource struct {
	client *nscale.Client
}

func NewSSHCertificateAuthorityResource() resource.Resource {
	return &SSHCertificateAuthorityResource{}
}

func (r *SSHCertificateAuthorityResource) Configure(
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

func (r *SSHCertificateAuthorityResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *SSHCertificateAuthorityResource) Metadata(
	ctx context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssh_certificate_authority"
}

func (r *SSHCertificateAuthorityResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale SSH Certificate Authority",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the SSH certificate authority.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the SSH certificate authority.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the SSH certificate authority.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "The SSH CA public key in OpenSSH format (e.g. ssh-ed25519 AAAA...).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					normalizeWhitespacePlanModifier{},
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the SSH certificate authority is provisioned. If not specified, this defaults to the project ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the SSH certificate authority was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *SSHCertificateAuthorityResource) setDefaultIDs(data *SSHCertificateAuthorityResourceModel) {
	if data.ProjectID.ValueString() == "" {
		data.ProjectID = types.StringValue(r.client.ProjectID)
	}
}

func (r *SSHCertificateAuthorityResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[SSHCertificateAuthorityResourceModel](
		ctx,
		request.Plan.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	plannedPublicKey := data.PublicKey

	params := data.NscaleSSHCACreateParams(r.client.OrganizationID)

	createResponse, err := r.client.Region.PostApiV2Sshcertificateauthorities(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create SSH Certificate Authority",
			fmt.Sprintf("An error occurred while creating the SSH certificate authority: %s", err),
		)
		return
	}
	defer createResponse.Body.Close()

	sshCA, err := nscale.ReadJSONResponsePointer[regionapi.SshCertificateAuthorityV2Read](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create SSH Certificate Authority",
			fmt.Sprintf("An error occurred while creating the SSH certificate authority: %s", err),
		)
		return
	}

	data.SSHCertificateAuthorityModel = NewSSHCertificateAuthorityModel(sshCA)
	data.PublicKey = plannedPublicKey
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[regionapi.SshCertificateAuthorityV2Read]{
		ResourceTitle: "SSH Certificate Authority",
		ResourceName:  "ssh_certificate_authority",
		GetFunc: func(ctx context.Context) (*regionapi.SshCertificateAuthorityV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getSSHCA(ctx, sshCA.Metadata.Id, r.client)
		},
	}

	sshCA, ok := stateWatcher.Wait(ctx, data.Timeouts, response)
	if !ok {
		return
	}

	data.SSHCertificateAuthorityModel = NewSSHCertificateAuthorityModel(sshCA)
	data.PublicKey = plannedPublicKey
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *SSHCertificateAuthorityResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[SSHCertificateAuthorityResourceModel](
		ctx,
		request.State.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := nscale.ResourceReader[regionapi.SshCertificateAuthorityV2Read]{
		ResourceTitle: "SSH Certificate Authority",
		ResourceName:  "ssh_certificate_authority",
		GetFunc: func(ctx context.Context, id string) (*regionapi.SshCertificateAuthorityV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getSSHCA(ctx, id, r.client)
		},
	}

	sshCA, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data.SSHCertificateAuthorityModel = NewSSHCertificateAuthorityModel(sshCA)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *SSHCertificateAuthorityResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	response.Diagnostics.AddError(
		"Update Not Supported",
		"SSH Certificate Authorities are immutable and cannot be updated in-place. All changes require resource replacement.",
	)
}

func (r *SSHCertificateAuthorityResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[SSHCertificateAuthorityResourceModel](
		ctx,
		request.State.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	deleteResponse, err := r.client.Region.DeleteApiV2SshcertificateauthoritiesSshCertificateAuthorityID(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete SSH Certificate Authority",
			fmt.Sprintf("An error occurred while deleting the SSH certificate authority: %s", err),
		)
		return
	}
	defer deleteResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(deleteResponse); err != nil {
		if e, ok := nscale.AsAPIError(err); ok && e.StatusCode != http.StatusNotFound {
			nscale.TerraformDebugLogAPIResponseBody(ctx, err)
			response.Diagnostics.AddError(
				"Failed to Delete SSH Certificate Authority",
				fmt.Sprintf("An error occurred while deleting the SSH certificate authority: %s", err),
			)
			return
		}
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "SSH Certificate Authority",
		ResourceName:  "ssh_certificate_authority",
		GetFunc: func(ctx context.Context) (any, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getSSHCA(ctx, id, r.client)
		},
	}

	stateWatcher.Wait(ctx, data.Timeouts, response)
}

// normalizeWhitespacePlanModifier sets the plan value to the state value when
// the only difference is leading/trailing whitespace. This prevents spurious
// diffs from file() including a trailing newline that the API strips.
type normalizeWhitespacePlanModifier struct{}

func (m normalizeWhitespacePlanModifier) Description(_ context.Context) string {
	return "Normalizes whitespace differences between config and state."
}

func (m normalizeWhitespacePlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m normalizeWhitespacePlanModifier) PlanModifyString(
	_ context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if strings.TrimSpace(req.PlanValue.ValueString()) == strings.TrimSpace(req.StateValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}
