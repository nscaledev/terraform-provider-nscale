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
	nscale "github.com/nscaledev/terraform-provider-nscale/internal/client"
	"github.com/nscaledev/terraform-provider-nscale/version"
)

const DefaultNscaleEndpoint = "https://compute.unikorn.nscale.com"

var _ provider.Provider = NscaleProvider{}

type NscaleProviderConfig struct {
	client         *nscale.ClientWithResponses
	organizationID string
	projectID      string
}

type NscaleProviderModel struct {
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
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the Nscale API server.",
				Optional:            true,
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

	endpoint := data.Endpoint.ValueString()
	if value, ok := os.LookupEnv("NSCALE_API_ENDPOINT"); ok {
		endpoint = value
	}
	if endpoint == "" {
		endpoint = DefaultNscaleEndpoint
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

	userAgent := fmt.Sprintf("Terraform/%s terraform-provider-nscale/%s", version.ProviderVersion, version.ProviderVersion)

	httpClient := nscale.NewHTTPClient(userAgent, serviceToken)

	client, err := nscale.NewClientWithResponses(endpoint, nscale.WithHTTPClient(httpClient))
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Nscale Client",
			fmt.Sprintf("An error occurred while creating the Nscale client: %s.", err),
		)
		return
	}

	config := &NscaleProviderConfig{
		client:         client,
		organizationID: organizationID,
		projectID:      projectID,
	}

	response.DataSourceData = config
	response.ResourceData = config
}

func (p NscaleProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewComputeClusterDataSource,
	}
}

func (p NscaleProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewComputeClusterResource,
	}
}
