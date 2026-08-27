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
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
)

// PlatformReleasesModel is the Terraform view of a platform release query.
//
// This is the provider's first plural data source — everything else looks a
// single object up by id. The filter fields are the query, and `releases` is
// the result.
type PlatformReleasesModel struct {
	OrganizationID types.String `tfsdk:"organization_id"`
	RegionID       types.String `tfsdk:"region_id"`
	Architecture   types.String `tfsdk:"architecture"`
	Prerelease     types.Bool   `tfsdk:"prerelease"`
	Deprecated     types.Bool   `tfsdk:"deprecated"`
	Withdrawn      types.Bool   `tfsdk:"withdrawn"`

	Releases []PlatformReleaseModel `tfsdk:"releases"`
}

type PlatformReleaseModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	KubernetesVersion types.String `tfsdk:"kubernetes_version"`

	Prerelease types.Bool `tfsdk:"prerelease"`
	Deprecated types.Bool `tfsdk:"deprecated"`
	Withdrawn  types.Bool `tfsdk:"withdrawn"`

	WithdrawalReason  types.String `tfsdk:"withdrawal_reason"`
	WithdrawalMessage types.String `tfsdk:"withdrawal_message"`

	SupportedArchitectures types.List `tfsdk:"supported_architectures"`
	AvailableRegionIDs     types.List `tfsdk:"available_region_ids"`
	UsableOrganizationIDs  types.List `tfsdk:"usable_organization_ids"`
}

func NewPlatformReleaseModel(source *nks.PlatformReleaseV1Read) PlatformReleaseModel {
	architectures := make([]string, 0, len(source.Status.SupportedArchitectures))
	for _, architecture := range source.Status.SupportedArchitectures {
		architectures = append(architectures, string(architecture))
	}

	withdrawalReason := types.StringNull()
	if source.Status.WithdrawalReason != nil {
		withdrawalReason = types.StringValue(string(*source.Status.WithdrawalReason))
	}

	return PlatformReleaseModel{
		ID:                types.StringValue(source.Metadata.Id),
		Name:              types.StringValue(source.Metadata.Name),
		KubernetesVersion: types.StringValue(source.Status.KubernetesVersion),

		Prerelease: types.BoolValue(source.Status.Prerelease),
		Deprecated: types.BoolValue(source.Status.Deprecated),
		Withdrawn:  types.BoolValue(source.Status.Withdrawn),

		WithdrawalReason:  withdrawalReason,
		WithdrawalMessage: types.StringPointerValue(source.Status.WithdrawalMessage),

		SupportedArchitectures: stringSliceValue(architectures),
		AvailableRegionIDs:     stringSliceValue(source.Status.AvailableRegionIds),
		UsableOrganizationIDs:  stringSliceValue(source.Status.UsableOrganizationIds),
	}
}

// NscalePlatformReleasesListParams builds the list query.
//
// Every unset filter must be left nil so the parameter is OMITTED from the
// query string rather than sent empty — the NKS list filters are repeatable
// query params, and an empty value is not the same as an absent one. The
// generated params struct uses pointers throughout, so mapping a null Terraform
// value to nil is what achieves this; never substitute a zero value.
func (m *PlatformReleasesModel) NscalePlatformReleasesListParams(
	defaultOrganizationID string,
) *nks.ListPlatformReleasesParams {
	// Scope to the provider-configured organization unless overridden, so the
	// common case does not have to restate it.
	organizationID := m.OrganizationID.ValueString()
	if organizationID == "" {
		organizationID = defaultOrganizationID
	}

	var organizationIDs *nks.OrganizationIDQueryParameter
	if organizationID != "" {
		organizationIDs = &nks.OrganizationIDQueryParameter{organizationID}
	}

	var regionIDs *nks.RegionIDQueryParameter
	if regionID := m.RegionID.ValueString(); regionID != "" {
		regionIDs = &nks.RegionIDQueryParameter{regionID}
	}

	var architectures *nks.PlatformReleaseArchitectureQueryParameter
	if architecture := m.Architecture.ValueString(); architecture != "" {
		architectures = &nks.PlatformReleaseArchitectureQueryParameter{
			nks.PlatformReleaseArchitectureV1(architecture),
		}
	}

	return &nks.ListPlatformReleasesParams{
		OrganizationID: organizationIDs,
		RegionID:       regionIDs,
		Architecture:   architectures,
		// The three booleans are tri-state: null means "do not filter", which is
		// a distinct query from filtering on false. ValueBoolPointer preserves
		// that by yielding nil for null.
		Prerelease: m.Prerelease.ValueBoolPointer(),
		Deprecated: m.Deprecated.ValueBoolPointer(),
		Withdrawn:  m.Withdrawn.ValueBoolPointer(),
	}
}
