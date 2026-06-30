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
	"github.com/nscaledev/terraform-provider-nscale/internal/services/identity"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/instance"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/network"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/objectstorage"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/region"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/reservation"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/securitygroup"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/sshca"
	"github.com/nscaledev/terraform-provider-nscale/version"
)

const (
	DefaultNscaleRegionServiceAPIEndpoint      = "https://region.unikorn.nscale.com"
	DefaultNscaleComputeServiceAPIEndpoint     = "https://compute.unikorn.nscale.com"
	DefaultNscaleIdentityServiceAPIEndpoint    = "https://identity.unikorn.nscale.com"
	DefaultNscaleReservationServiceAPIEndpoint = "https://reservation.unikorn.nscale.com"
	DefaultNscaleStorageServiceAPIEndpoint     = "https://storage.unikorn.nscale.com"
)

var _ provider.Provider = NscaleProvider{}

type NscaleProviderModel struct {
	RegionServiceAPIEndpoint      types.String `tfsdk:"region_service_api_endpoint"`
	ComputeServiceAPIEndpoint     types.String `tfsdk:"compute_service_api_endpoint"`
	IdentityServiceAPIEndpoint    types.String `tfsdk:"identity_service_api_endpoint"`
	ReservationServiceAPIEndpoint types.String `tfsdk:"reservation_service_api_endpoint"`
	StorageServiceAPIEndpoint     types.String `tfsdk:"storage_service_api_endpoint"`
	ServiceToken                  types.String `tfsdk:"service_token"`
	RegionID                      types.String `tfsdk:"region_id"`
	OrganizationID                types.String `tfsdk:"organization_id"`
	ProjectID                     types.String `tfsdk:"project_id"`
}

type NscaleProvider struct{}

func New() provider.Provider {
	return NscaleProvider{}
}

func (p NscaleProvider) Metadata(
	ctx context.Context,
	request provider.MetadataRequest,
	response *provider.MetadataResponse,
) {
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
			"identity_service_api_endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the Nscale Identity Service API server.",
				Optional:            true,
			},
			"reservation_service_api_endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the Nscale Reservation Service API server.",
				Optional:            true,
			},
			"storage_service_api_endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the Nscale Storage Service API server.",
				Optional:            true,
			},
			"service_token": schema.StringAttribute{
				MarkdownDescription: "The service token for authenticating with the Nscale API server.",
				Optional:            true,
				Sensitive:           true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region for which resources are managed. Regional resources include a top-level region_id field, allowing the region to be explicitly specified and to override the default region when provided.",
				Optional:            true,
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the organization for which resources are managed.",
				Optional:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The default project identifier for project-scoped resources that do not set their own project_id. Optional: org-level workflows and configurations that set project_id on every resource do not need it.",
				Optional:            true,
			},
		},
	}
}

// resolveValue picks a provider setting from, in order of precedence: the named
// environment variable, the provider configuration value, then the supplied
// fallback (which may be empty for required values that have no default).
func resolveValue(configValue, envVar, fallback string) string {
	value := configValue
	if envValue, ok := os.LookupEnv(envVar); ok {
		value = envValue
	}
	if value == "" {
		value = fallback
	}
	return value
}

func (p NscaleProvider) Configure(
	ctx context.Context,
	request provider.ConfigureRequest,
	response *provider.ConfigureResponse,
) {
	var data NscaleProviderModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	regionServiceAPIEndpoint := resolveValue(
		data.RegionServiceAPIEndpoint.ValueString(),
		"NSCALE_REGION_SERVICE_API_ENDPOINT",
		DefaultNscaleRegionServiceAPIEndpoint,
	)
	computeServiceAPIEndpoint := resolveValue(
		data.ComputeServiceAPIEndpoint.ValueString(),
		"NSCALE_COMPUTE_SERVICE_API_ENDPOINT",
		DefaultNscaleComputeServiceAPIEndpoint,
	)
	identityServiceAPIEndpoint := resolveValue(
		data.IdentityServiceAPIEndpoint.ValueString(),
		"NSCALE_IDENTITY_SERVICE_API_ENDPOINT",
		DefaultNscaleIdentityServiceAPIEndpoint,
	)
	reservationServiceAPIEndpoint := resolveValue(
		data.ReservationServiceAPIEndpoint.ValueString(),
		"NSCALE_RESERVATION_SERVICE_API_ENDPOINT",
		DefaultNscaleReservationServiceAPIEndpoint,
	)

	storageServiceAPIEndpoint := resolveValue(
		data.StorageServiceAPIEndpoint.ValueString(),
		"NSCALE_STORAGE_SERVICE_API_ENDPOINT",
		DefaultNscaleStorageServiceAPIEndpoint,
	)

	serviceToken := resolveValue(data.ServiceToken.ValueString(), "NSCALE_SERVICE_TOKEN", "")
	if serviceToken == "" {
		response.Diagnostics.AddError(
			"Missing Service Token",
			"Please provide a service token either through the configuration or the NSCALE_SERVICE_TOKEN environment variable.",
		)
		return
	}

	regionID := resolveValue(data.RegionID.ValueString(), "NSCALE_REGION_ID", "")
	if regionID == "" {
		response.Diagnostics.AddWarning(
			"Missing Region ID",
			"Please provide a region ID either through the configuration or the NSCALE_REGION_ID environment variable.",
		)
		return
	}

	organizationID := resolveValue(data.OrganizationID.ValueString(), "NSCALE_ORGANIZATION_ID", "")
	if organizationID == "" {
		response.Diagnostics.AddError(
			"Missing Organization ID",
			"Please provide an organization ID either through the configuration or the NSCALE_ORGANIZATION_ID environment variable.",
		)
		return
	}

	// project_id is an optional default: it is only consumed as a fallback by
	// project-scoped resources that omit their own project_id. Resources enforce
	// the requirement at point of use (via Client.ResolveProjectID), so an empty
	// value here is valid and keeps org-level and fully-explicit workflows working.
	projectID := resolveValue(data.ProjectID.ValueString(), "NSCALE_PROJECT_ID", "")

	userAgent := fmt.Sprintf(
		"Terraform/%s terraform-provider-nscale/%s",
		request.TerraformVersion,
		version.ProviderVersion,
	)

	client, err := nscale.NewClient(
		regionServiceAPIEndpoint,
		computeServiceAPIEndpoint,
		identityServiceAPIEndpoint,
		reservationServiceAPIEndpoint,
		storageServiceAPIEndpoint,
		serviceToken,
		organizationID,
		projectID,
		regionID,
		userAgent,
	)
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
		filestorage.NewFileStorageDataSource,
		instance.NewInstanceFlavorDataSource,
		instance.NewInstanceDataSource,
		instance.NewInstanceSSHKeyDataSource,
		sshca.NewSSHCertificateAuthorityDataSource,
		computecluster.NewComputeClusterDataSource,
		objectstorage.NewObjectStorageEndpointClassDataSource,
		objectstorage.NewObjectStorageEndpointDataSource,
		objectstorage.NewObjectStorageAccessKeyDataSource,
		identity.NewProjectDataSource,
		identity.NewGroupDataSource,
		reservation.NewReservationDataSource,
		reservation.NewPlacementDataSource,
	}
}

func (p NscaleProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		network.NewNetworkResource,
		securitygroup.NewSecurityGroupResource,
		filestorage.NewFileStorageResource,
		instance.NewInstanceResource,
		sshca.NewSSHCertificateAuthorityResource,
		computecluster.NewComputeClusterResource,
		objectstorage.NewObjectStorageEndpointResource,
		objectstorage.NewObjectStorageAccessKeyResource,
		identity.NewProjectResource,
		identity.NewGroupResource,
		reservation.NewReservationResource,
		reservation.NewPlacementResource,
	}
}
