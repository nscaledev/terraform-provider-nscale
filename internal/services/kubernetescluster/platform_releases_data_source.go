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

package kubernetescluster

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &KubernetesPlatformReleasesDataSource{}

type KubernetesPlatformReleasesDataSource struct {
	client *nscale.Client
}

func NewKubernetesPlatformReleasesDataSource() datasource.DataSource {
	return &KubernetesPlatformReleasesDataSource{}
}

func (s *KubernetesPlatformReleasesDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
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

	s.client = client
}

func (s *KubernetesPlatformReleasesDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_kubernetes_platform_releases"
}

func (s *KubernetesPlatformReleasesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Kubernetes Platform Releases",
		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "Only return releases usable by this organization. " +
					"Defaults to the organization configured on the provider.",
				Optional: true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "Only return releases available in this region.",
				Optional:            true,
			},
			"architecture": schema.StringAttribute{
				MarkdownDescription: "Only return releases supporting this CPU architecture. " +
					"One of `x86_64` or `aarch64`.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(nks.X8664),
						string(nks.Aarch64),
					),
				},
			},
			"prerelease": schema.BoolAttribute{
				MarkdownDescription: "Filter by prerelease state. Omit to return both prerelease and " +
					"non-prerelease releases.",
				Optional: true,
			},
			"deprecated": schema.BoolAttribute{
				MarkdownDescription: "Filter by deprecation state. Set to `false` to return only releases that " +
					"are still current. Omit to return both.",
				Optional: true,
			},
			"withdrawn": schema.BoolAttribute{
				MarkdownDescription: "Filter by withdrawal state. Set to `false` to exclude releases operators " +
					"have withdrawn. Omit to return both.",
				Optional: true,
			},
			"releases": schema.ListNestedAttribute{
				MarkdownDescription: "The matching platform releases, in the order returned by the API.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The unique identifier of the platform release. " +
								"Use this for `platform_release_id` on a `nscale_kubernetes_cluster`.",
							Computed: true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the platform release.",
							Computed:            true,
						},
						"kubernetes_version": schema.StringAttribute{
							MarkdownDescription: "The Kubernetes version provided by the release.",
							Computed:            true,
						},
						"prerelease": schema.BoolAttribute{
							MarkdownDescription: "Whether the release's Kubernetes version is a semver prerelease.",
							Computed:            true,
						},
						"deprecated": schema.BoolAttribute{
							MarkdownDescription: "Whether existing clusters should upgrade away from this release.",
							Computed:            true,
						},
						"withdrawn": schema.BoolAttribute{
							MarkdownDescription: "Whether operators have withdrawn the release from selection.",
							Computed:            true,
						},
						"withdrawal_reason": schema.StringAttribute{
							MarkdownDescription: "The machine-readable reason operators withdrew the release.",
							Computed:            true,
						},
						"withdrawal_message": schema.StringAttribute{
							MarkdownDescription: "The explanation of why operators withdrew the release.",
							Computed:            true,
						},
						"supported_architectures": schema.ListAttribute{
							MarkdownDescription: "The CPU architectures supported by the release's compute images.",
							ElementType:         types.StringType,
							Computed:            true,
						},
						"available_region_ids": schema.ListAttribute{
							MarkdownDescription: "The region IDs where this release is available.",
							ElementType:         types.StringType,
							Computed:            true,
						},
						"usable_organization_ids": schema.ListAttribute{
							MarkdownDescription: "The organization IDs, among your own, that may select this release.",
							ElementType:         types.StringType,
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (s *KubernetesPlatformReleasesDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var data PlatformReleasesModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	nksClient, diagnostics := s.client.RequireNKS()
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	params := data.NscalePlatformReleasesListParams(s.client.OrganizationID)

	listResponse, err := nksClient.ListPlatformReleases(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Kubernetes Platform Releases",
			fmt.Sprintf("An error occurred while listing platform releases: %s", err),
		)
		return
	}
	defer listResponse.Body.Close()

	releases, err := nscale.ReadJSONResponseValue[nks.PlatformReleasesV1Read](listResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Read Kubernetes Platform Releases",
			fmt.Sprintf("An error occurred while listing platform releases: %s", err),
		)
		return
	}

	data.Releases = make([]PlatformReleaseModel, 0, len(releases))
	for index := range releases {
		data.Releases = append(data.Releases, NewPlatformReleaseModel(&releases[index]))
	}

	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
