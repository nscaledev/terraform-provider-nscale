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

package identity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	identityapi "github.com/nscaledev/nscale-sdk-go/identity"
)

func TestNewGroupModelFull(t *testing.T) {
	creationTime := time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC)

	source := &identityapi.GroupRead{
		Metadata: coreapi.OrganizationScopedResourceReadMetadata{
			Id:                 "group-1",
			Name:               "engineers",
			Description:        new("engineering staff"),
			OrganizationId:     "org-1",
			CreationTime:       creationTime,
			ProvisioningStatus: coreapi.ResourceProvisioningStatusProvisioned,
		},
		Spec: identityapi.GroupSpec{
			RoleIDs:           []string{"role-a"},
			ServiceAccountIDs: []string{"sa-a"},
			UserIDs:           &[]string{"user-a", "user-b"},
			Subjects: &[]identityapi.Subject{
				{Issuer: "https://accounts.google.com", Id: "subject-1", Email: new("a@example.com")},
			},
		},
	}

	model := NewGroupModel(source)

	if model.ID.ValueString() != "group-1" {
		t.Errorf("ID = %q, want %q", model.ID.ValueString(), "group-1")
	}
	if model.Name.ValueString() != "engineers" {
		t.Errorf("Name = %q, want %q", model.Name.ValueString(), "engineers")
	}
	assertStringSliceEqual(t, "RoleIDs", setValues(t, model.RoleIDs), []string{"role-a"})
	assertStringSliceEqual(t, "ServiceAccountIDs", setValues(t, model.ServiceAccountIDs), []string{"sa-a"})
	assertStringSliceEqual(t, "UserIDs", setValues(t, model.UserIDs), []string{"user-a", "user-b"})

	subjects := readSubjects(t, model)
	if len(subjects) != 1 {
		t.Fatalf("len(Subjects) = %d, want 1", len(subjects))
	}
	if subjects[0].Issuer.ValueString() != "https://accounts.google.com" {
		t.Errorf("Subject issuer = %q", subjects[0].Issuer.ValueString())
	}
	if subjects[0].Email.ValueString() != "a@example.com" {
		t.Errorf("Subject email = %q", subjects[0].Email.ValueString())
	}
}

func TestNewGroupModelNilOptionalFields(t *testing.T) {
	creationTime := time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC)

	source := &identityapi.GroupRead{
		Metadata: coreapi.OrganizationScopedResourceReadMetadata{
			Id:                 "group-2",
			Name:               "minimal",
			OrganizationId:     "org-1",
			CreationTime:       creationTime,
			ProvisioningStatus: coreapi.ResourceProvisioningStatusProvisioned,
		},
		Spec: identityapi.GroupSpec{
			RoleIDs:           []string{"role-a"},
			ServiceAccountIDs: []string{},
			UserIDs:           nil,
			Subjects:          nil,
		},
	}

	model := NewGroupModel(source)

	if !model.Description.IsNull() {
		t.Errorf("Description = %v, want null", model.Description)
	}
	// A nil pointer (omitted) becomes null...
	if !model.UserIDs.IsNull() {
		t.Errorf("UserIDs = %v, want null", model.UserIDs)
	}
	if !model.Subjects.IsNull() {
		t.Errorf("Subjects = %v, want null", model.Subjects)
	}
	// ...but an empty (non-nil) slice must round-trip faithfully as an empty
	// set, not null, so an explicit empty configured set stays consistent.
	if model.ServiceAccountIDs.IsNull() {
		t.Errorf("ServiceAccountIDs is null, want empty set")
	}
	assertStringSliceEqual(t, "ServiceAccountIDs", setValues(t, model.ServiceAccountIDs), []string{})
}

func readSubjects(t *testing.T, model GroupModel) []SubjectModel {
	t.Helper()

	if model.Subjects.IsNull() {
		t.Fatalf("Subjects is null, want elements")
	}

	var subjects []SubjectModel
	if diagnostics := model.Subjects.ElementsAs(context.Background(), &subjects, false); diagnostics.HasError() {
		t.Fatalf("failed to read subjects: %v", diagnostics)
	}

	return subjects
}

func TestGroupCreateParamsRoundTrip(t *testing.T) {
	model := GroupModel{
		Name:              types.StringValue("engineers"),
		Description:       types.StringValue("engineering staff"),
		Tags:              types.MapNull(types.StringType),
		RoleIDs:           setOf(t, "role-a", "role-b"),
		ServiceAccountIDs: setOf(t, "sa-a"),
		UserIDs:           setOf(t, "user-a"),
		Subjects:          types.SetNull(SubjectModelAttributeType),
	}

	params, diagnostics := model.NscaleGroupCreateParams(t.Context())
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}

	if params.Metadata.Name != "engineers" {
		t.Errorf("Name = %q", params.Metadata.Name)
	}
	assertStringSliceEqual(t, "RoleIDs", params.Spec.RoleIDs, []string{"role-a", "role-b"})
	assertStringSliceEqual(t, "ServiceAccountIDs", params.Spec.ServiceAccountIDs, []string{"sa-a"})
	if params.Spec.UserIDs == nil {
		t.Fatalf("UserIDs is nil, want one element")
	}
	assertStringSliceEqual(t, "UserIDs", *params.Spec.UserIDs, []string{"user-a"})
	if params.Spec.Subjects != nil {
		t.Errorf("Subjects = %v, want nil", params.Spec.Subjects)
	}
}

// TestGroupCreateParamsRequiredListsSerialized is the omitempty regression
// guard: the identity API requires roleIDs and serviceAccountIDs to be present.
// Even when the Terraform sets are null, the marshalled JSON must contain those
// keys as empty arrays rather than omitting them or sending null.
func TestGroupCreateParamsRequiredListsSerialized(t *testing.T) {
	model := GroupModel{
		Name:              types.StringValue("engineers"),
		Tags:              types.MapNull(types.StringType),
		RoleIDs:           types.SetNull(types.StringType),
		ServiceAccountIDs: types.SetNull(types.StringType),
		UserIDs:           types.SetNull(types.StringType),
		Subjects:          types.SetNull(SubjectModelAttributeType),
	}

	params, diagnostics := model.NscaleGroupCreateParams(t.Context())
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}

	encoded, err := json.Marshal(params.Spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	body := string(encoded)
	if !strings.Contains(body, `"roleIDs":[]`) {
		t.Errorf("marshalled spec missing empty roleIDs array: %s", body)
	}
	if !strings.Contains(body, `"serviceAccountIDs":[]`) {
		t.Errorf("marshalled spec missing empty serviceAccountIDs array: %s", body)
	}
}

func TestSubjectRoundTrip(t *testing.T) {
	model := SubjectModel{
		Issuer: types.StringValue("https://accounts.google.com"),
		ID:     types.StringValue("subject-1"),
		Email:  types.StringValue("a@example.com"),
	}

	subject := model.NscaleSubject()
	if subject.Issuer != "https://accounts.google.com" {
		t.Errorf("Issuer = %q", subject.Issuer)
	}
	if subject.Id != "subject-1" {
		t.Errorf("Id = %q", subject.Id)
	}
	if subject.Email == nil || *subject.Email != "a@example.com" {
		t.Errorf("Email = %v", subject.Email)
	}

	value := NewSubjectModel(subject)
	if value.IsNull() {
		t.Errorf("NewSubjectModel returned null")
	}
}
