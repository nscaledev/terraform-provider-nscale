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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

var _ datasource.DataSourceWithConfigure = &SSHCertificateAuthorityDataSource{}

type SSHCertificateAuthorityDataSource struct {
	client *nscale.Client
}

func NewSSHCertificateAuthorityDataSource() datasource.DataSource {
	return &SSHCertificateAuthorityDataSource{}
}

func (s *SSHCertificateAuthorityDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	client, ok := request.ProviderData.(*nscale.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configuration Type",
			fmt.Sprintf("Expected *nscale.Client, got: %T. Please contact the Nscale team for support.", request.ProviderData),
		)
		return
	}

	s.client = client
}

func (s *SSHCertificateAuthorityDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_ssh_certificate_authority"
}

func (s *SSHCertificateAuthorityDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale SSH Certificate Authority",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the SSH certificate authority.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the SSH certificate authority.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the SSH certificate authority.",
				Computed:            true,
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "The SSH CA public key in OpenSSH format.",
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the SSH certificate authority is provisioned.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the SSH certificate authority was created.",
				Computed:            true,
			},
		},
	}
}

func (s *SSHCertificateAuthorityDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[SSHCertificateAuthorityModel](ctx, request.Config.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params := &regionapi.GetApiV2SshcertificateauthoritiesParams{
		OrganizationID: &regionapi.OrganizationIDQueryParameter{
			s.client.OrganizationID,
		},
		ProjectID: &regionapi.ProjectIDQueryParameter{
			s.client.ProjectID,
		},
	}

	listResponse, err := s.client.Region.GetApiV2Sshcertificateauthorities(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to List SSH Certificate Authorities",
			fmt.Sprintf("An error occurred while listing SSH certificate authorities: %s", err),
		)
		return
	}

	sshCAs, err := nscale.ReadJSONResponseValue[[]regionapi.SshCertificateAuthorityV2Read](listResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to List SSH Certificate Authorities",
			fmt.Sprintf("An error occurred while listing SSH certificate authorities: %s", err),
		)
		return
	}

	name := data.Name.ValueString()

	for _, sshCA := range sshCAs {
		if sshCA.Metadata.Name == name {
			data = NewSSHCertificateAuthorityModel(&sshCA)
			response.Diagnostics.Append(response.State.Set(ctx, &data)...)
			return
		}
	}

	response.Diagnostics.AddError(
		"SSH Certificate Authority Not Found",
		fmt.Sprintf("An SSH certificate authority with name %q was not found.", name),
	)
}
