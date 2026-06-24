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

package filestorage

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"

	"github.com/nscaledev/terraform-provider-nscale/internal/utils/pointer"
)

// A syntactically valid UUID so request-building helpers that parse the region
// ID succeed; the value is otherwise irrelevant to these tests.
const testRegionID = "11111111-1111-1111-1111-111111111111"

func assertBoolPointerEqual(t *testing.T, got, want *bool) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("defaultSnapshotProtectionEnabled = %v, want %v", got, want)
	case *got != *want:
		t.Fatalf("defaultSnapshotProtectionEnabled = %v, want %v", *got, *want)
	}
}

// The API read specification always exposes the resolved Default Snapshot
// Protection setting; the model must surface it so read, refresh, and import
// store the value Terraform observed.
func TestNewFileStorageModelMapsDefaultSnapshotProtectionEnabled(t *testing.T) {
	tests := []struct {
		name     string
		resolved *bool
		want     types.Bool
	}{
		{name: "enabled", resolved: pointer.Reference(true), want: types.BoolValue(true)},
		{name: "disabled", resolved: pointer.Reference(false), want: types.BoolValue(false)},
		{name: "absent maps to null", resolved: nil, want: types.BoolNull()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var source regionapi.StorageV2Read
			source.Spec.DefaultSnapshotProtectionEnabled = tt.resolved

			model := NewFileStorageModel(&source)

			if !model.DefaultSnapshotProtectionEnabled.Equal(tt.want) {
				t.Fatalf(
					"DefaultSnapshotProtectionEnabled = %s, want %s",
					model.DefaultSnapshotProtectionEnabled,
					tt.want,
				)
			}
		})
	}
}

// Create must send the API default behavior (omit the field) when Default
// Snapshot Protection is not explicitly configured, and send the explicit
// value otherwise. CRUD populates the model field from configuration, where an
// omitted or null value is null and an explicit value is known.
func TestNscaleFileStorageCreateParamsDefaultSnapshotProtectionEnabled(t *testing.T) {
	tests := []struct {
		name       string
		configured types.Bool
		want       *bool
	}{
		{name: "omitted sends API default", configured: types.BoolNull(), want: nil},
		{name: "explicit null sends API default", configured: types.BoolNull(), want: nil},
		{name: "explicit true enforces enabled", configured: types.BoolValue(true), want: pointer.Reference(true)},
		{name: "explicit false enforces disabled", configured: types.BoolValue(false), want: pointer.Reference(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := FileStorageModel{
				Name:                             types.StringValue("fs"),
				RegionID:                         types.StringValue(testRegionID),
				Capacity:                         types.Int64Value(20),
				StorageClassID:                   types.StringValue("class"),
				RootSquash:                       types.BoolValue(true),
				Network:                          types.ListNull(FileStorageNetworkModelAttributeType),
				DefaultSnapshotProtectionEnabled: tt.configured,
			}

			params, diagnostics := model.NscaleFileStorageCreateParams("org")
			if diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diagnostics)
			}

			assertBoolPointerEqual(t, params.Spec.DefaultSnapshotProtectionEnabled, tt.want)
		})
	}
}

// Update must observe the remote value (omit the field) when Default Snapshot
// Protection is not explicitly configured, and enforce the explicit value
// otherwise.
func TestNscaleFileStorageUpdateParamsDefaultSnapshotProtectionEnabled(t *testing.T) {
	tests := []struct {
		name       string
		configured types.Bool
		want       *bool
	}{
		{name: "omitted observes remote", configured: types.BoolNull(), want: nil},
		{name: "explicit null observes remote", configured: types.BoolNull(), want: nil},
		{name: "explicit true enforces enabled", configured: types.BoolValue(true), want: pointer.Reference(true)},
		{name: "explicit false enforces disabled", configured: types.BoolValue(false), want: pointer.Reference(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := FileStorageModel{
				Name:                             types.StringValue("fs"),
				Capacity:                         types.Int64Value(20),
				RootSquash:                       types.BoolValue(true),
				Network:                          types.ListNull(FileStorageNetworkModelAttributeType),
				DefaultSnapshotProtectionEnabled: tt.configured,
			}

			params, diagnostics := model.NscaleFileStorageUpdateParams()
			if diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diagnostics)
			}

			assertBoolPointerEqual(t, params.Spec.DefaultSnapshotProtectionEnabled, tt.want)
		})
	}
}
