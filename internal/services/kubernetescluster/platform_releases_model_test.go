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
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
)

// queryFor builds the request the generated client would send and returns its
// encoded query string. Asserting on the wire format rather than the params
// struct is the point: the requirement is that unset filters produce NO query
// key at all, and only the encoder can prove that.
func queryFor(t *testing.T, params *nks.ListPlatformReleasesParams) string {
	t.Helper()

	request, err := nks.NewListPlatformReleasesRequest("https://nks.example.com", params)
	if err != nil {
		t.Fatalf("building list request: %s", err)
	}

	return request.URL.RawQuery
}

// TestListParamsOmitsUnsetFilters is the ticket's "all list filters must be
// omitted (not sent empty) when unset" requirement.
func TestListParamsOmitsUnsetFilters(t *testing.T) {
	t.Parallel()

	model := &PlatformReleasesModel{
		OrganizationID: types.StringNull(),
		RegionID:       types.StringNull(),
		Architecture:   types.StringNull(),
		Prerelease:     types.BoolNull(),
		Deprecated:     types.BoolNull(),
		Withdrawn:      types.BoolNull(),
	}

	// No provider-level organization either, so nothing at all should be sent.
	if query := queryFor(t, model.NscalePlatformReleasesListParams("")); query != "" {
		t.Errorf("query = %q, want empty when every filter is unset", query)
	}
}

// TestListParamsFalseIsDistinctFromUnset pins the tri-state behaviour: filtering
// on false is a different query from not filtering. This is what lets a config
// ask for "non-deprecated releases only".
func TestListParamsFalseIsDistinctFromUnset(t *testing.T) {
	t.Parallel()

	unset := &PlatformReleasesModel{
		OrganizationID: types.StringNull(),
		RegionID:       types.StringNull(),
		Architecture:   types.StringNull(),
		Prerelease:     types.BoolNull(),
		Deprecated:     types.BoolNull(),
		Withdrawn:      types.BoolNull(),
	}
	explicitFalse := &PlatformReleasesModel{
		OrganizationID: types.StringNull(),
		RegionID:       types.StringNull(),
		Architecture:   types.StringNull(),
		Prerelease:     types.BoolNull(),
		Deprecated:     types.BoolValue(false),
		Withdrawn:      types.BoolValue(false),
	}

	unsetQuery := queryFor(t, unset.NscalePlatformReleasesListParams(""))
	falseQuery := queryFor(t, explicitFalse.NscalePlatformReleasesListParams(""))

	if unsetQuery == falseQuery {
		t.Fatalf("unset and explicit-false produced the same query %q", falseQuery)
	}
	if falseQuery != "deprecated=false&withdrawn=false" {
		t.Errorf("query = %q, want deprecated=false&withdrawn=false", falseQuery)
	}
}

func TestListParamsSendsConfiguredFilters(t *testing.T) {
	t.Parallel()

	model := &PlatformReleasesModel{
		OrganizationID: types.StringValue("org-explicit"),
		RegionID:       types.StringValue("region-1"),
		Architecture:   types.StringValue("x86_64"),
		Prerelease:     types.BoolValue(true),
		Deprecated:     types.BoolValue(false),
		Withdrawn:      types.BoolValue(false),
	}

	query := queryFor(t, model.NscalePlatformReleasesListParams("org-default"))

	for _, want := range []string{
		"organizationID=org-explicit",
		"regionID=region-1",
		"architecture=x86_64",
		"prerelease=true",
		"deprecated=false",
		"withdrawn=false",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q is missing %q", query, want)
		}
	}
}

// TestListParamsFallsBackToProviderOrganization checks that the common case does
// not have to restate the organization, and that an explicit value wins.
func TestListParamsFallsBackToProviderOrganization(t *testing.T) {
	t.Parallel()

	implicit := &PlatformReleasesModel{
		OrganizationID: types.StringNull(),
		RegionID:       types.StringNull(),
		Architecture:   types.StringNull(),
		Prerelease:     types.BoolNull(),
		Deprecated:     types.BoolNull(),
		Withdrawn:      types.BoolNull(),
	}
	implicitQuery := queryFor(t, implicit.NscalePlatformReleasesListParams("org-default"))
	if implicitQuery != "organizationID=org-default" {
		t.Errorf("query = %q, want organizationID=org-default", implicitQuery)
	}

	explicit := &PlatformReleasesModel{
		OrganizationID: types.StringValue("org-explicit"),
		RegionID:       types.StringNull(),
		Architecture:   types.StringNull(),
		Prerelease:     types.BoolNull(),
		Deprecated:     types.BoolNull(),
		Withdrawn:      types.BoolNull(),
	}
	explicitQuery := queryFor(t, explicit.NscalePlatformReleasesListParams("org-default"))
	if explicitQuery != "organizationID=org-explicit" {
		t.Errorf("query = %q, want organizationID=org-explicit", explicitQuery)
	}
}

func TestNewPlatformReleaseModel(t *testing.T) {
	t.Parallel()

	created, err := time.Parse(time.RFC3339, "2026-08-01T00:00:00Z")
	if err != nil {
		t.Fatalf("parsing fixture time: %s", err)
	}

	source := &nks.PlatformReleaseV1Read{
		Metadata: nks.StaticResourceMetadata{
			Id:           "rel-1",
			Name:         "nks-1.33",
			CreationTime: created,
		},
		Status: nks.PlatformReleaseStatusV1{
			KubernetesVersion:      "v1.33.1",
			AvailableRegionIds:     []string{"region-1", "region-2"},
			UsableOrganizationIds:  []string{"org-1"},
			SupportedArchitectures: []nks.PlatformReleaseArchitectureV1{nks.X8664, nks.Aarch64},
			Prerelease:             false,
			Deprecated:             false,
			Withdrawn:              false,
		},
	}

	model := NewPlatformReleaseModel(source)

	if got := model.ID.ValueString(); got != "rel-1" {
		t.Errorf("ID = %q, want rel-1", got)
	}
	if got := model.KubernetesVersion.ValueString(); got != "v1.33.1" {
		t.Errorf("KubernetesVersion = %q, want v1.33.1", got)
	}
	if model.Deprecated.ValueBool() || model.Withdrawn.ValueBool() || model.Prerelease.ValueBool() {
		t.Error("expected a current, non-prerelease, non-withdrawn release")
	}
	if !model.WithdrawalReason.IsNull() {
		t.Error("WithdrawalReason should be null for a release that was never withdrawn")
	}
	if !model.WithdrawalMessage.IsNull() {
		t.Error("WithdrawalMessage should be null for a release that was never withdrawn")
	}

	var architectures []string
	if diagnostics := model.SupportedArchitectures.ElementsAs(
		context.Background(), &architectures, false,
	); diagnostics.HasError() {
		t.Fatalf("reading architectures: %v", diagnostics)
	}
	if len(architectures) != 2 || architectures[0] != "x86_64" || architectures[1] != "aarch64" {
		t.Errorf("SupportedArchitectures = %v, want [x86_64 aarch64]", architectures)
	}

	var organizations []string
	if diagnostics := model.UsableOrganizationIDs.ElementsAs(
		context.Background(), &organizations, false,
	); diagnostics.HasError() {
		t.Fatalf("reading usable organizations: %v", diagnostics)
	}
	if len(organizations) != 1 || organizations[0] != "org-1" {
		t.Errorf("UsableOrganizationIDs = %v, want [org-1]", organizations)
	}
}

func TestNewPlatformReleaseModelWithdrawn(t *testing.T) {
	t.Parallel()

	source := &nks.PlatformReleaseV1Read{
		Metadata: nks.StaticResourceMetadata{Id: "rel-old", Name: "nks-1.30"},
		Status: nks.PlatformReleaseStatusV1{
			KubernetesVersion:      "v1.30.0",
			AvailableRegionIds:     []string{},
			UsableOrganizationIds:  []string{},
			SupportedArchitectures: []nks.PlatformReleaseArchitectureV1{},
			Deprecated:             true,
			Withdrawn:              true,
			WithdrawalReason:       new(nks.SecurityIssue),
			WithdrawalMessage:      new("CVE-2026-0001"),
		},
	}

	model := NewPlatformReleaseModel(source)

	if !model.Withdrawn.ValueBool() || !model.Deprecated.ValueBool() {
		t.Error("expected a deprecated, withdrawn release")
	}
	if got := model.WithdrawalReason.ValueString(); got != "SecurityIssue" {
		t.Errorf("WithdrawalReason = %q, want SecurityIssue", got)
	}
	if got := model.WithdrawalMessage.ValueString(); got != "CVE-2026-0001" {
		t.Errorf("WithdrawalMessage = %q, want CVE-2026-0001", got)
	}

	// An empty API array should surface as an empty list, not null: the release
	// exists but is available nowhere.
	if model.AvailableRegionIDs.IsNull() {
		t.Error("AvailableRegionIDs should be an empty list, not null")
	}
	if got := len(model.AvailableRegionIDs.Elements()); got != 0 {
		t.Errorf("AvailableRegionIDs length = %d, want 0", got)
	}
}
