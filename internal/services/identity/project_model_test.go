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
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	identityapi "github.com/nscaledev/nscale-sdk-go/identity"
)

func TestNewProjectModel(t *testing.T) {
	creationTime := time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		name                string
		source              *identityapi.ProjectRead
		expectedDescription types.String
		expectedTagsNull    bool
		expectedGroupIDs    []string
	}{
		{
			name: "full",
			source: &identityapi.ProjectRead{
				Metadata: coreapi.OrganizationScopedResourceReadMetadata{
					Id:                 "project-1",
					Name:               "demo-project",
					Description:        new("a description"),
					OrganizationId:     "org-1",
					CreationTime:       creationTime,
					ProvisioningStatus: coreapi.ResourceProvisioningStatusProvisioned,
					Tags: &[]coreapi.Tag{
						{Name: "team", Value: "platform"},
					},
				},
				Spec: identityapi.ProjectSpec{
					GroupIDs: []string{"group-a", "group-b"},
				},
			},
			expectedDescription: types.StringValue("a description"),
			expectedTagsNull:    false,
			expectedGroupIDs:    []string{"group-a", "group-b"},
		},
		{
			name: "nil description and tags and empty groups",
			source: &identityapi.ProjectRead{
				Metadata: coreapi.OrganizationScopedResourceReadMetadata{
					Id:                 "project-2",
					Name:               "bare-project",
					OrganizationId:     "org-1",
					CreationTime:       creationTime,
					ProvisioningStatus: coreapi.ResourceProvisioningStatusProvisioned,
				},
				Spec: identityapi.ProjectSpec{
					GroupIDs: []string{},
				},
			},
			expectedDescription: types.StringNull(),
			expectedTagsNull:    true,
			expectedGroupIDs:    []string{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			model := NewProjectModel(testCase.source)

			if model.ID.ValueString() != testCase.source.Metadata.Id {
				t.Errorf("ID = %q, want %q", model.ID.ValueString(), testCase.source.Metadata.Id)
			}
			if model.Name.ValueString() != testCase.source.Metadata.Name {
				t.Errorf("Name = %q, want %q", model.Name.ValueString(), testCase.source.Metadata.Name)
			}
			if !model.Description.Equal(testCase.expectedDescription) {
				t.Errorf("Description = %v, want %v", model.Description, testCase.expectedDescription)
			}
			if model.Tags.IsNull() != testCase.expectedTagsNull {
				t.Errorf("Tags null = %v, want %v", model.Tags.IsNull(), testCase.expectedTagsNull)
			}
			wantCreationTime := creationTime.Format(time.RFC3339)
			if model.CreationTime.ValueString() != wantCreationTime {
				t.Errorf("CreationTime = %q, want %q", model.CreationTime.ValueString(), wantCreationTime)
			}

			groupIDs := setValues(t, model.GroupIDs)
			assertStringSliceEqual(t, "GroupIDs", groupIDs, testCase.expectedGroupIDs)
		})
	}
}

func TestProjectCreateParamsRoundTrip(t *testing.T) {
	model := ProjectModel{
		ID:          types.StringValue("project-1"),
		Name:        types.StringValue("demo-project"),
		Description: types.StringValue("a description"),
		Tags:        types.MapNull(types.StringType),
		GroupIDs:    setOf(t, "group-a", "group-b"),
	}

	params, diagnostics := model.NscaleProjectCreateParams(t.Context())
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}

	if params.Metadata.Name != "demo-project" {
		t.Errorf("Name = %q, want %q", params.Metadata.Name, "demo-project")
	}
	if params.Metadata.Description == nil || *params.Metadata.Description != "a description" {
		t.Errorf("Description = %v, want %q", params.Metadata.Description, "a description")
	}
	assertStringSliceEqual(t, "GroupIDs", params.Spec.GroupIDs, []string{"group-a", "group-b"})
}

func TestProjectCreateParamsNullGroupIDs(t *testing.T) {
	model := ProjectModel{
		Name:     types.StringValue("demo-project"),
		Tags:     types.MapNull(types.StringType),
		GroupIDs: types.SetNull(types.StringType),
	}

	params, diagnostics := model.NscaleProjectCreateParams(t.Context())
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}

	// A null set must serialize to an empty (non-nil) slice so the required
	// groupIDs field is sent as [] rather than omitted.
	if params.Spec.GroupIDs == nil {
		t.Fatalf("GroupIDs is nil, want empty slice")
	}
	if len(params.Spec.GroupIDs) != 0 {
		t.Errorf("GroupIDs = %v, want empty", params.Spec.GroupIDs)
	}
}
