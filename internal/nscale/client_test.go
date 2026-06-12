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

package nscale

import "testing"

func TestResolveProjectIDResolves(t *testing.T) {
	testCases := []struct {
		name            string
		clientProjectID string
		resourceProject string
		wantProjectID   string
	}{
		{
			name:            "resource value wins over provider default",
			clientProjectID: "provider-project",
			resourceProject: "resource-project",
			wantProjectID:   "resource-project",
		},
		{
			name:            "falls back to provider default when resource is empty",
			clientProjectID: "provider-project",
			resourceProject: "",
			wantProjectID:   "provider-project",
		},
		{
			name:            "resource value works without a provider default",
			clientProjectID: "",
			resourceProject: "resource-project",
			wantProjectID:   "resource-project",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := &Client{ProjectID: testCase.clientProjectID}

			projectID, diagnostics := client.ResolveProjectID(testCase.resourceProject)

			if diagnostics.HasError() {
				t.Fatalf("unexpected error diagnostic: %v", diagnostics.Errors())
			}
			if projectID != testCase.wantProjectID {
				t.Fatalf("project ID = %q, want %q", projectID, testCase.wantProjectID)
			}
		})
	}
}

func TestResolveProjectIDErrorsWhenUnset(t *testing.T) {
	client := &Client{ProjectID: ""}

	projectID, diagnostics := client.ResolveProjectID("")

	if !diagnostics.HasError() {
		t.Fatalf("expected an error diagnostic, got none")
	}
	if got := diagnostics.Errors()[0].Summary(); got != "Missing Project ID" {
		t.Fatalf("error summary = %q, want %q", got, "Missing Project ID")
	}
	if projectID != "" {
		t.Fatalf("project ID = %q, want empty on error", projectID)
	}
}
