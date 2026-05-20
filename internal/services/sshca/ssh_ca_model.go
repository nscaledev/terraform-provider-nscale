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
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

type SSHCertificateAuthorityModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	PublicKey    types.String `tfsdk:"public_key"`
	ProjectID    types.String `tfsdk:"project_id"`
	CreationTime types.String `tfsdk:"creation_time"`
}

func NewSSHCertificateAuthorityModel(source *regionapi.SshCertificateAuthorityV2Read) SSHCertificateAuthorityModel {
	return SSHCertificateAuthorityModel{
		ID:           types.StringValue(source.Metadata.Id),
		Name:         types.StringValue(source.Metadata.Name),
		Description:  types.StringPointerValue(source.Metadata.Description),
		PublicKey:    types.StringValue(strings.TrimSpace(source.Spec.PublicKey)),
		ProjectID:    types.StringValue(source.Metadata.ProjectId),
		CreationTime: types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

func (m *SSHCertificateAuthorityModel) NscaleSSHCACreateParams(
	organizationID string,
) regionapi.SshCertificateAuthorityV2Create {
	return regionapi.SshCertificateAuthorityV2Create{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
		},
		Spec: regionapi.SshCertificateAuthorityV2CreateSpec{
			OrganizationId: organizationID,
			ProjectId:      m.ProjectID.ValueString(),
			PublicKey:      strings.TrimSpace(m.PublicKey.ValueString()),
		},
	}
}
