/*
Copyright 2025 Nscale

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

package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/computecluster"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/filestorage"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/instance"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/network"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/region"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/securitygroup"
	"github.com/nscaledev/terraform-provider-nscale/version"
)

const (
	DefaultNscaleRegionServiceAPIEndpoint  = "https://region.unikorn.nscale.com"
	DefaultNscaleComputeServiceAPIEndpoint = "https://compute.unikorn.nscale.com"
)

var _ provider.Provider = NscaleProvider{}

type NscaleProviderModel struct {
	RegionServiceAPIEndpoint  types.String `tfsdk:"region_service_api_endpoint"`
	ComputeServiceAPIEndpoint types.String `tfsdk:"compute_service_api_endpoint"`
	// Deprecated: use ComputeServiceAPIEndpoint instead
	Endpoint       types.String `tfsdk:"endpoint"`
	ServiceToken   types.String `tfsdk:"service_token"`
	OrganizationID types.String `tfsdk:"organization_id"`
	ProjectID      types.String `tfsdk:"project_id"`
}

type NscaleProvider struct{}

func New() provider.Provider {
	return NscaleProvider{}
}

func (p NscaleProvider) Metadata(ctx context.Context, request provider.MetadataRequest, response *provider.MetadataResponse) {
	response.TypeName = "nscale"
	response.Version = version.ProviderVersion
}

func (p NscaleProvider) Schema(ctx context.Context, request provider.SchemaRequest, response *provider.SchemaResponse) {
	response.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"region_service_api_endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the Nscale Region Service API server.",
				Optional:            true,
			},
			"compute_service_api_endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the Nscale Compute Service API server.",
				Optional:            true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the Nscale API server.",
				Optional:            true,
				DeprecationMessage:  "This attribute is deprecated. Use compute_service_api_endpoint instead.",
			},
			"service_token": schema.StringAttribute{
				MarkdownDescription: "The service token for authenticating with the Nscale API server.",
				Optional:            true,
				Sensitive:           true,
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the organization for which resources are managed.",
				Optional:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project for which resources are managed.",
				Optional:            true,
			},
		},
	}
}

func (p NscaleProvider) Configure(ctx context.Context, request provider.ConfigureRequest, response *provider.ConfigureResponse) {
	var data NscaleProviderModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	regionServiceAPIEndpoint := data.RegionServiceAPIEndpoint.ValueString()
	if value, ok := os.LookupEnv("NSCALE_REGION_SERVICE_API_ENDPOINT"); ok {
		regionServiceAPIEndpoint = value
	}
	if regionServiceAPIEndpoint == "" {
		regionServiceAPIEndpoint = DefaultNscaleRegionServiceAPIEndpoint
	}

	computeServiceAPIEndpoint := data.ComputeServiceAPIEndpoint.ValueString()
	if value, ok := os.LookupEnv("NSCALE_COMPUTE_SERVICE_API_ENDPOINT"); ok {
		computeServiceAPIEndpoint = value
	}
	if computeServiceAPIEndpoint == "" {
		// Fallback to deprecated endpoint for backwards compatibility.
		if value, ok := os.LookupEnv("NSCALE_API_ENDPOINT"); ok {
			computeServiceAPIEndpoint = value
		}
	}
	if computeServiceAPIEndpoint == "" {
		computeServiceAPIEndpoint = DefaultNscaleComputeServiceAPIEndpoint
	}

	serviceToken := data.ServiceToken.ValueString()
	if value, ok := os.LookupEnv("NSCALE_SERVICE_TOKEN"); ok {
		serviceToken = value
	}
	if serviceToken == "" {
		response.Diagnostics.AddError(
			"Missing Service Token",
			"Please provide a service token either through the configuration or the NSCALE_SERVICE_TOKEN environment variable.",
		)
		return
	}

	organizationID := data.OrganizationID.ValueString()
	if value, ok := os.LookupEnv("NSCALE_ORGANIZATION_ID"); ok {
		organizationID = value
	}
	if organizationID == "" {
		response.Diagnostics.AddError(
			"Missing Organization ID",
			"Please provide an organization ID either through the configuration or the NSCALE_ORGANIZATION_ID environment variable.",
		)
		return
	}

	projectID := data.ProjectID.ValueString()
	if value, ok := os.LookupEnv("NSCALE_PROJECT_ID"); ok {
		projectID = value
	}
	if projectID == "" {
		response.Diagnostics.AddError(
			"Missing Project ID",
			"Please provide a project ID either through the configuration or the NSCALE_PROJECT_ID environment variable.",
		)
		return
	}

	userAgent := fmt.Sprintf("Terraform/%s terraform-provider-nscale/%s", request.TerraformVersion, version.ProviderVersion)

	client, err := nscale.NewClient(regionServiceAPIEndpoint, computeServiceAPIEndpoint, serviceToken, organizationID, projectID, userAgent)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Nscale Client",
			fmt.Sprintf("An error occurred while creating the Nscale client: %s", err),
		)
		return
	}

	response.DataSourceData = client
	response.ResourceData = client
}

func (p NscaleProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		region.NewRegionDataSource,
		network.NewNetworkDataSource,
		securitygroup.NewSecurityGroupDataSource,
		filestorage.NewFileStorageClassDataSource,
		instance.NewInstanceFlavorDataSource,
		instance.NewInstanceDataSource,
		computecluster.NewComputeClusterDataSource,
	}
}

func (p NscaleProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		network.NewNetworkResource,
		securitygroup.NewSecurityGroupResource,
		instance.NewInstanceResource,
		computecluster.NewComputeClusterResource,
	}
}
