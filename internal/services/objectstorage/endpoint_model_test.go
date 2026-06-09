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

package objectstorage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

func TestNewObjectStorageEndpointModel(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	policy := storageapi.ObjectStorageIdentityPolicySpec{
		Name: "bucket-admin",
		Document: storageapi.ObjectStorageIdentityPolicyDocument{
			"Version": "2012-10-17",
			"Statement": []any{
				map[string]any{
					"Effect":   "Allow",
					"Action":   []any{"s3:*"},
					"Resource": []any{"arn:aws:s3:::ml-artifacts"},
				},
			},
		},
	}
	policies := storageapi.ObjectStorageIdentityPolicyList{policy}
	tags := coreapi.TagList{
		{Name: "team", Value: "ingest"},
		// Operation tags must be stripped on the way into state.
		{Name: nscale.TerraformOperationTagPrefix + "abc", Value: "in-flight"},
	}
	source := &storageapi.ObjectStorageEndpointRead{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:           "ep-1",
			Name:         "ml-artifacts",
			Description:  new("ingest endpoint"),
			ProjectId:    "proj-1",
			CreationTime: created,
			Tags:         &tags,
		},
		Spec: storageapi.ObjectStorageEndpointSpec{
			ObjectStorageEndpointClassId: "class-1",
			IdentityPolicies:             &policies,
		},
		Status: storageapi.ObjectStorageEndpointStatus{
			RegionId: "region-1",
			Exposure: &storageapi.ObjectStorageEndpointExposureStatus{
				Public: &storageapi.ObjectStorageEndpointExposureDetailsStatus{
					DnsName: "ml-artifacts.s3.example",
				},
			},
		},
	}

	got, diags := NewObjectStorageEndpointModel(source)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.ID.ValueString() != "ep-1" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "ep-1")
	}
	if got.Name.ValueString() != "ml-artifacts" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "ml-artifacts")
	}
	if got.EndpointClassID.ValueString() != "class-1" {
		t.Errorf("EndpointClassID = %q, want %q", got.EndpointClassID.ValueString(), "class-1")
	}
	if got.RegionID.ValueString() != "region-1" {
		t.Errorf("RegionID = %q, want %q", got.RegionID.ValueString(), "region-1")
	}

	// Operation tag must have been stripped; user tag must remain.
	tagAttrs := got.Tags.Elements()
	if len(tagAttrs) != 1 {
		t.Fatalf("expected 1 user tag after operation-tag stripping, got %d: %v", len(tagAttrs), tagAttrs)
	}
	teamTag, ok := tagAttrs["team"].(types.String)
	if !ok || teamTag.ValueString() != "ingest" {
		t.Errorf("team tag missing or wrong: %v", tagAttrs)
	}

	// Identity policy document must round-trip as compact JSON.
	policyElems := got.IdentityPolicies.Elements()
	if len(policyElems) != 1 {
		t.Fatalf("expected 1 identity policy, got %d", len(policyElems))
	}
	policyObj, _ := policyElems[0].(types.Object)
	gotName, _ := policyObj.Attributes()["name"].(types.String)
	if gotName.ValueString() != "bucket-admin" {
		t.Errorf("policy name = %q, want %q", gotName.ValueString(), "bucket-admin")
	}
	gotDoc, _ := policyObj.Attributes()["document"].(types.String)
	// Round-trip via json.Unmarshal — the literal byte sequence depends on
	// map ordering, but the semantic value must match.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(gotDoc.ValueString()), &parsed); err != nil {
		t.Fatalf("policy document is not valid JSON: %v\n%s", err, gotDoc.ValueString())
	}
	if parsed["Version"] != "2012-10-17" {
		t.Errorf("policy Version = %v, want 2012-10-17", parsed["Version"])
	}

	exposureAttrs := got.Exposure.Attributes()
	publicObj, _ := exposureAttrs["public"].(types.Object)
	dns, _ := publicObj.Attributes()["dns_name"].(types.String)
	if dns.ValueString() != "ml-artifacts.s3.example" {
		t.Errorf("public dns_name = %q, want %q", dns.ValueString(), "ml-artifacts.s3.example")
	}
}

func TestNewObjectStorageEndpointModel_NilOptionals(t *testing.T) {
	source := &storageapi.ObjectStorageEndpointRead{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:           "ep-bare",
			Name:         "bare",
			ProjectId:    "proj-1",
			CreationTime: time.Now(),
		},
		Spec: storageapi.ObjectStorageEndpointSpec{
			ObjectStorageEndpointClassId: "class-1",
			IdentityPolicies:             nil,
		},
		Status: storageapi.ObjectStorageEndpointStatus{
			RegionId: "region-1",
			Exposure: nil,
		},
	}

	got, diags := NewObjectStorageEndpointModel(source)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if !got.Description.IsNull() {
		t.Errorf("Description should be null, got %q", got.Description.ValueString())
	}
	// Nil exposure must become a null object so UseStateForUnknown can
	// preserve it without spurious diffs.
	if !got.Exposure.IsNull() {
		t.Errorf("Exposure should be null when status.exposure is nil")
	}
	// Identity policies is non-nil but empty when source has no policies.
	if got.IdentityPolicies.IsNull() {
		t.Errorf("IdentityPolicies should be an empty list, not null")
	}
	if len(got.IdentityPolicies.Elements()) != 0 {
		t.Errorf("IdentityPolicies should be empty, got %d", len(got.IdentityPolicies.Elements()))
	}
}

func TestNscaleObjectStorageEndpointCreateParams_StripsOperationTags(t *testing.T) {
	ctx := context.Background()
	tags := types.MapValueMust(types.StringType, map[string]attr.Value{
		"team": types.StringValue("ingest"),
		nscale.TerraformOperationTagPrefix + "leftover": types.StringValue("nope"),
	})
	m := &ObjectStorageEndpointModel{
		Name:             types.StringValue("ml-artifacts"),
		Description:      types.StringValue("ingest"),
		EndpointClassID:  types.StringValue("class-1"),
		ProjectID:        types.StringValue("proj-1"),
		RegionID:         types.StringValue("region-1"),
		Tags:             tags,
		IdentityPolicies: types.ListNull(ObjectStorageEndpointIdentityPolicyAttributeType),
	}

	params, diags := m.NscaleObjectStorageEndpointCreateParams(ctx, "org-1")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if params.Metadata.Tags == nil {
		t.Fatal("expected tags to survive when at least one user tag is configured")
	}
	for _, tag := range *params.Metadata.Tags {
		if tag.Name == nscale.TerraformOperationTagPrefix+"leftover" {
			t.Errorf("operation tag leaked into create params: %+v", tag)
		}
	}
	if params.Spec.OrganizationId != "org-1" {
		t.Errorf("Spec.OrganizationId = %q, want org-1", params.Spec.OrganizationId)
	}
	if params.Spec.ObjectStorageEndpointClassId != "class-1" {
		t.Errorf("Spec.ObjectStorageEndpointClassId = %q, want class-1", params.Spec.ObjectStorageEndpointClassId)
	}
}

func TestNscaleObjectStorageEndpointCreateParams_IdentityPoliciesRoundTrip(t *testing.T) {
	ctx := context.Background()
	policyObj := types.ObjectValueMust(
		ObjectStorageEndpointIdentityPolicyAttributeType.AttrTypes,
		map[string]attr.Value{
			"name": types.StringValue("bucket-admin"),
			"document": types.StringValue(
				`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:*"],"Resource":["*"]}]}`,
			),
		},
	)
	m := &ObjectStorageEndpointModel{
		Name:            types.StringValue("ml-artifacts"),
		EndpointClassID: types.StringValue("class-1"),
		ProjectID:       types.StringValue("proj-1"),
		RegionID:        types.StringValue("region-1"),
		Tags:            types.MapNull(types.StringType),
		IdentityPolicies: types.ListValueMust(
			ObjectStorageEndpointIdentityPolicyAttributeType,
			[]attr.Value{policyObj},
		),
	}

	params, diags := m.NscaleObjectStorageEndpointCreateParams(ctx, "org-1")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if params.Spec.IdentityPolicies == nil || len(*params.Spec.IdentityPolicies) != 1 {
		t.Fatalf("expected 1 identity policy in request, got %+v", params.Spec.IdentityPolicies)
	}
	gotPolicy := (*params.Spec.IdentityPolicies)[0]
	if gotPolicy.Name != "bucket-admin" {
		t.Errorf("policy name = %q, want bucket-admin", gotPolicy.Name)
	}
	if gotPolicy.Document["Version"] != "2012-10-17" {
		t.Errorf("policy Version = %v, want 2012-10-17", gotPolicy.Document["Version"])
	}
}

func TestNscaleObjectStorageEndpointCreateParams_InvalidPolicyJSON(t *testing.T) {
	ctx := context.Background()
	policyObj := types.ObjectValueMust(
		ObjectStorageEndpointIdentityPolicyAttributeType.AttrTypes,
		map[string]attr.Value{
			"name":     types.StringValue("broken"),
			"document": types.StringValue("not-json"),
		},
	)
	m := &ObjectStorageEndpointModel{
		Name:            types.StringValue("ml-artifacts"),
		EndpointClassID: types.StringValue("class-1"),
		ProjectID:       types.StringValue("proj-1"),
		RegionID:        types.StringValue("region-1"),
		Tags:            types.MapNull(types.StringType),
		IdentityPolicies: types.ListValueMust(
			ObjectStorageEndpointIdentityPolicyAttributeType,
			[]attr.Value{policyObj},
		),
	}

	_, diags := m.NscaleObjectStorageEndpointCreateParams(ctx, "org-1")
	if !diags.HasError() {
		t.Fatal("expected error diagnostic for invalid JSON policy document")
	}
}

// TestNscaleObjectStorageEndpointUpdateParams_StripsOperationTags mirrors the
// Create-side test for the Update path so the operation-tag stripping
// invariant holds on rename / identity-policy swap flows too.
func TestNscaleObjectStorageEndpointUpdateParams_StripsOperationTags(t *testing.T) {
	ctx := context.Background()
	tags := types.MapValueMust(types.StringType, map[string]attr.Value{
		"team": types.StringValue("ingest"),
		nscale.TerraformOperationTagPrefix + "leftover": types.StringValue("nope"),
	})
	m := &ObjectStorageEndpointModel{
		Name:             types.StringValue("ml-artifacts-renamed"),
		Description:      types.StringValue("post-update"),
		EndpointClassID:  types.StringValue("class-1"),
		ProjectID:        types.StringValue("proj-1"),
		RegionID:         types.StringValue("region-1"),
		Tags:             tags,
		IdentityPolicies: types.ListNull(ObjectStorageEndpointIdentityPolicyAttributeType),
	}

	params, diags := m.NscaleObjectStorageEndpointUpdateParams(ctx)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if params.Metadata.Name != "ml-artifacts-renamed" {
		t.Errorf("Metadata.Name = %q, want ml-artifacts-renamed", params.Metadata.Name)
	}
	if params.Metadata.Tags == nil {
		t.Fatal("expected tags to survive when at least one user tag is configured")
	}
	for _, tag := range *params.Metadata.Tags {
		if tag.Name == nscale.TerraformOperationTagPrefix+"leftover" {
			t.Errorf("operation tag leaked into update params: %+v", tag)
		}
	}
	// IdentityPolicies is null on the model; the update Spec must still be
	// populated so the API call doesn't deref a nil pointer.
	if params.Spec == nil {
		t.Fatal("Spec must be non-nil even when IdentityPolicies is null")
	}
	if params.Spec.IdentityPolicies != nil {
		t.Errorf("Spec.IdentityPolicies should be nil when model has null policies")
	}
}

// TestNscaleObjectStorageEndpointUpdateParams_IdentityPoliciesRoundTrip
// verifies the update payload carries the configured policies through.
func TestNscaleObjectStorageEndpointUpdateParams_IdentityPoliciesRoundTrip(t *testing.T) {
	ctx := context.Background()
	policyObj := types.ObjectValueMust(
		ObjectStorageEndpointIdentityPolicyAttributeType.AttrTypes,
		map[string]attr.Value{
			"name": types.StringValue("readers"),
			"document": types.StringValue(
				`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			),
		},
	)
	m := &ObjectStorageEndpointModel{
		Name:            types.StringValue("ml-artifacts"),
		EndpointClassID: types.StringValue("class-1"),
		ProjectID:       types.StringValue("proj-1"),
		RegionID:        types.StringValue("region-1"),
		Tags:            types.MapNull(types.StringType),
		IdentityPolicies: types.ListValueMust(
			ObjectStorageEndpointIdentityPolicyAttributeType,
			[]attr.Value{policyObj},
		),
	}

	params, diags := m.NscaleObjectStorageEndpointUpdateParams(ctx)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if params.Spec == nil || params.Spec.IdentityPolicies == nil {
		t.Fatalf("expected policies to survive into Update.Spec, got %+v", params.Spec)
	}
	if (*params.Spec.IdentityPolicies)[0].Name != "readers" {
		t.Errorf("policy name = %q, want readers", (*params.Spec.IdentityPolicies)[0].Name)
	}
}

// TestNscaleObjectStorageEndpointUpdateParams_InvalidPolicyJSON guarantees the
// Update path errors out the same way Create does when a policy document is
// not valid JSON, rather than silently shipping it to the API.
func TestNscaleObjectStorageEndpointUpdateParams_InvalidPolicyJSON(t *testing.T) {
	ctx := context.Background()
	policyObj := types.ObjectValueMust(
		ObjectStorageEndpointIdentityPolicyAttributeType.AttrTypes,
		map[string]attr.Value{
			"name":     types.StringValue("broken"),
			"document": types.StringValue("not-json"),
		},
	)
	m := &ObjectStorageEndpointModel{
		Name:            types.StringValue("ml-artifacts"),
		EndpointClassID: types.StringValue("class-1"),
		ProjectID:       types.StringValue("proj-1"),
		RegionID:        types.StringValue("region-1"),
		Tags:            types.MapNull(types.StringType),
		IdentityPolicies: types.ListValueMust(
			ObjectStorageEndpointIdentityPolicyAttributeType,
			[]attr.Value{policyObj},
		),
	}

	_, diags := m.NscaleObjectStorageEndpointUpdateParams(ctx)
	if !diags.HasError() {
		t.Fatal("expected error diagnostic for invalid JSON policy document")
	}
}

// TestNewExposureValue_NilPublic covers the half-populated branch: status.Exposure
// is non-nil but no Public block — e.g. a private-only endpoint class. The
// outer object must be set, the public attribute must be null.
func TestNewExposureValue_NilPublic(t *testing.T) {
	got := newExposureValue(&storageapi.ObjectStorageEndpointExposureStatus{Public: nil})
	if got.IsNull() {
		t.Fatal("outer exposure object should be non-null when status.Exposure is set")
	}
	publicObj, ok := got.Attributes()["public"].(types.Object)
	if !ok {
		t.Fatalf("public attribute should be a types.Object, got %T", got.Attributes()["public"])
	}
	if !publicObj.IsNull() {
		t.Errorf("public attribute should be null when status.Public is nil")
	}
}

// TestIdentityPoliciesAPI_NullList covers the early-return branch in
// identityPoliciesAPI when the IdentityPolicies attribute is null.
func TestIdentityPoliciesAPI_NullList(t *testing.T) {
	m := &ObjectStorageEndpointModel{
		IdentityPolicies: types.ListNull(ObjectStorageEndpointIdentityPolicyAttributeType),
	}
	policies, diags := m.identityPoliciesAPI(context.Background())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if policies != nil {
		t.Errorf("expected nil policies for null list, got %d", len(policies))
	}
}

// TestIdentityPoliciesAPI_EmptyList covers the explicit-empty path — a
// configured-but-empty list is also "no policies".
func TestIdentityPoliciesAPI_EmptyList(t *testing.T) {
	m := &ObjectStorageEndpointModel{
		IdentityPolicies: types.ListValueMust(ObjectStorageEndpointIdentityPolicyAttributeType, []attr.Value{}),
	}
	policies, diags := m.identityPoliciesAPI(context.Background())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if policies != nil {
		t.Errorf("expected nil policies for empty list, got %d", len(policies))
	}
}

// TestIdentityPoliciesAPI_UnknownList covers the early-return branch when the
// attribute is unknown (e.g. during plan-time with computed values).
func TestIdentityPoliciesAPI_UnknownList(t *testing.T) {
	m := &ObjectStorageEndpointModel{
		IdentityPolicies: types.ListUnknown(ObjectStorageEndpointIdentityPolicyAttributeType),
	}
	policies, diags := m.identityPoliciesAPI(context.Background())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if policies != nil {
		t.Errorf("expected nil policies for unknown list, got %d", len(policies))
	}
}
