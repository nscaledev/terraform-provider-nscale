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

package instance

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

var _ datasource.DataSourceWithConfigure = &InstanceFlavorDataSource{}

type InstanceSSHKeyDataSource struct {
	client *nscale.Client
}

func NewInstanceSSHKeyDataSource() datasource.DataSource {
	return &InstanceSSHKeyDataSource{}
}

func (s *InstanceSSHKeyDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (s *InstanceSSHKeyDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_instance_ssh_key"
}

func (s *InstanceSSHKeyDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Instance SSH Key",
		Attributes: map[string]schema.Attribute{
			"instance_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the instance associated with the SSH key.",
				Required:            true,
			},
			"private_key": schema.StringAttribute{
				MarkdownDescription: "The private SSH key for accessing the instance.",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}

func (s *InstanceSSHKeyDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[InstanceSSHKeyModel](ctx, request.Config.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	instanceID := data.InstanceID.ValueString()

	sshKeyResponse, err := s.client.Compute.GetApiV2InstancesInstanceIDSshkey(ctx, instanceID)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Instance SSH Key",
			fmt.Sprintf("An error occurred while retrieving the instance SSH key: %s", err),
		)
		return
	}

	sshKey, err := nscale.ReadJSONResponsePointer[regionapi.SshKey](sshKeyResponse)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Instance SSH Key",
			fmt.Sprintf("An error occurred while retrieving the instance SSH key: %s", err),
		)
		return
	}

	data = NewInstanceSSHKeyModel(instanceID, sshKey)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}
