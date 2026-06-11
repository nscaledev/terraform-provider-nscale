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
	"strings"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.Resource                = &SSHCertificateAuthorityResource{}
	_ resource.ResourceWithConfigure   = &SSHCertificateAuthorityResource{}
	_ resource.ResourceWithImportState = &SSHCertificateAuthorityResource{}
)

type SSHCertificateAuthorityResourceModel struct {
	SSHCertificateAuthorityModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// SSHCertificateAuthorityResource embeds the generic CRUD base; only Schema and
// the adapter wiring below are SSH-CA-specific.
type SSHCertificateAuthorityResource struct {
	*nscale.GenericResource[SSHCertificateAuthorityResourceModel, regionapi.SshCertificateAuthorityV2Read]
}

func NewSSHCertificateAuthorityResource() resource.Resource {
	return &SSHCertificateAuthorityResource{
		GenericResource: nscale.NewGenericResource(sshCAAdapter()),
	}
}

// sshCAAdapter wires the SSH-CA-specific SDK calls and model mapping into the
// generic resource skeleton.
func sshCAAdapter() nscale.ResourceAdapter[SSHCertificateAuthorityResourceModel, regionapi.SshCertificateAuthorityV2Read] {
	return nscale.ResourceAdapter[SSHCertificateAuthorityResourceModel, regionapi.SshCertificateAuthorityV2Read]{
		TypeNameSuffix: "_ssh_certificate_authority",
		Title:          "SSH Certificate Authority",
		Name:           "ssh_certificate_authority",
		Create:         sshCACreate,
		// SSH certificate authorities are immutable: a nil Update tells the base
		// to reject in-place updates so every change forces a replacement.
		Update: nil,
		Delete: sshCADelete,
		Get: func(
			ctx context.Context,
			client *nscale.Client,
			id string,
		) (*regionapi.SshCertificateAuthorityV2Read, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getSSHCA(ctx, id, client))
		},
		ToModel: func(api *regionapi.SshCertificateAuthorityV2Read, dst *SSHCertificateAuthorityResourceModel) {
			dst.SSHCertificateAuthorityModel = NewSSHCertificateAuthorityModel(api)
		},
		IDFromModel:       func(m SSHCertificateAuthorityResourceModel) string { return m.ID.ValueString() },
		TimeoutsFromModel: func(m SSHCertificateAuthorityResourceModel) tftimeouts.Value { return m.Timeouts },
	}
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

func sshCACreate(
	ctx context.Context,
	client *nscale.Client,
	plan SSHCertificateAuthorityResourceModel,
) (*regionapi.SshCertificateAuthorityV2Read, diag.Diagnostics) {
	// Resolve the project from the resource or the provider default, erroring
	// when the configuration omits both.
	projectID, diagnostics := client.ResolveProjectID(plan.ProjectID.ValueString())
	if diagnostics.HasError() {
		return nil, diagnostics
	}
	plan.ProjectID = types.StringValue(projectID)

	params := plan.NscaleSSHCACreateParams(client.OrganizationID)

	createResponse, err := client.Region.PostApiV2Sshcertificateauthorities(ctx, params)
	if err != nil {
		diagnostics.AddError(
			"Failed to Create SSH Certificate Authority",
			fmt.Sprintf("An error occurred while creating the SSH certificate authority: %s", err),
		)
		return nil, diagnostics
	}
	defer createResponse.Body.Close()

	sshCA, err := nscale.ReadJSONResponsePointer[regionapi.SshCertificateAuthorityV2Read](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Create SSH Certificate Authority",
			fmt.Sprintf("An error occurred while creating the SSH certificate authority: %s", err),
		)
		return nil, diagnostics
	}

	return sshCA, nil
}

func sshCADelete(ctx context.Context, client *nscale.Client, id string) error {
	deleteResponse, err := client.Region.DeleteApiV2SshcertificateauthoritiesSshCertificateAuthorityID(ctx, id)
	if err != nil {
		return err
	}
	defer deleteResponse.Body.Close()

	return nscale.ReadEmptyResponse(deleteResponse)
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
