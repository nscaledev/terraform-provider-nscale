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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &SSHCertificateAuthorityDataSource{}

// SSHCertificateAuthorityDataSource embeds the generic read+map base; only
// Schema and the adapter wiring below are SSH-certificate-authority-specific.
type SSHCertificateAuthorityDataSource struct {
	*nscale.GenericDataSource[SSHCertificateAuthorityModel, regionapi.SshCertificateAuthorityV2Read]
}

func NewSSHCertificateAuthorityDataSource() datasource.DataSource {
	return &SSHCertificateAuthorityDataSource{
		GenericDataSource: nscale.NewGenericDataSource(
			nscale.DataSourceAdapter[SSHCertificateAuthorityModel, regionapi.SshCertificateAuthorityV2Read]{
				TypeNameSuffix: "_ssh_certificate_authority",
				Title:          "SSH Certificate Authority",
				Name:           "ssh_certificate_authority",
				Get: func(ctx context.Context, client *nscale.Client, id string) (*regionapi.SshCertificateAuthorityV2Read, error) {
					sshCA, _, err := getSSHCA(ctx, id, client)
					return sshCA, err
				},
				ToModel:     NewSSHCertificateAuthorityModel,
				IDFromModel: func(m SSHCertificateAuthorityModel) string { return m.ID.ValueString() },
			},
		),
	}
}

func (s *SSHCertificateAuthorityDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale SSH Certificate Authority",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the SSH certificate authority.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the SSH certificate authority.",
				Computed:            true,
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
