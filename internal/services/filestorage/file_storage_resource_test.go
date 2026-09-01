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
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	terraformtypes "github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestFileStorageResourceNFSPolicySchema(t *testing.T) {
	response := &resource.SchemaResponse{}
	(&FileStorageResource{}).Schema(context.Background(), resource.SchemaRequest{}, response)

	posixACL, ok := response.Schema.Attributes["posix_acl"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("posix_acl has type %T, want schema.BoolAttribute", response.Schema.Attributes["posix_acl"])
	}
	if !posixACL.Optional || !posixACL.Computed {
		t.Fatalf("posix_acl Optional/Computed = %t/%t, want true/true", posixACL.Optional, posixACL.Computed)
	}
	if posixACL.Default != nil {
		t.Fatal("posix_acl has a Terraform default, want none")
	}
	assertBoolPlanModifiersUseStateForUnknown(t, posixACL.PlanModifiers)

	atime, ok := response.Schema.Attributes["atime_update_interval_seconds"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf(
			"atime_update_interval_seconds has type %T, want schema.Int64Attribute",
			response.Schema.Attributes["atime_update_interval_seconds"],
		)
	}
	if !atime.Optional || !atime.Computed {
		t.Fatalf(
			"atime_update_interval_seconds Optional/Computed = %t/%t, want true/true",
			atime.Optional,
			atime.Computed,
		)
	}
	if atime.Default != nil {
		t.Fatal("atime_update_interval_seconds has a Terraform default, want none")
	}
	assertInt64PlanModifiersUseStateForUnknown(t, atime.PlanModifiers)

	for _, tt := range []struct {
		name    string
		value   int64
		wantErr bool
	}{
		{name: "lower bound", value: 0},
		{name: "upper bound", value: maxAtimeUpdateIntervalSeconds},
		{name: "below lower bound", value: -1, wantErr: true},
		{name: "above upper bound", value: maxAtimeUpdateIntervalSeconds + 1, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			validation := runInt64Validators(atime.Validators, types.Int64Value(tt.value))
			if validation.Diagnostics.HasError() != tt.wantErr {
				t.Fatalf(
					"HasError() = %t, want %t (diagnostics: %v)",
					validation.Diagnostics.HasError(),
					tt.wantErr,
					validation.Diagnostics,
				)
			}
		})
	}
}

func assertBoolPlanModifiersUseStateForUnknown(t *testing.T, modifiers []planmodifier.Bool) {
	t.Helper()
	request := planmodifier.BoolRequest{
		ConfigValue: types.BoolNull(),
		PlanValue:   types.BoolUnknown(),
		StateValue:  types.BoolValue(true),
		State: tfsdk.State{
			Raw: terraformtypes.NewValue(terraformtypes.String, "present"),
		},
	}
	response := &planmodifier.BoolResponse{PlanValue: request.PlanValue}
	for _, modifier := range modifiers {
		modifier.PlanModifyBool(context.Background(), request, response)
	}
	if !response.PlanValue.Equal(request.StateValue) {
		t.Fatalf("unknown bool plan = %v, want prior state %v", response.PlanValue, request.StateValue)
	}
}

func assertInt64PlanModifiersUseStateForUnknown(t *testing.T, modifiers []planmodifier.Int64) {
	t.Helper()
	request := planmodifier.Int64Request{
		ConfigValue: types.Int64Null(),
		PlanValue:   types.Int64Unknown(),
		StateValue:  types.Int64Value(600),
		State: tfsdk.State{
			Raw: terraformtypes.NewValue(terraformtypes.String, "present"),
		},
	}
	response := &planmodifier.Int64Response{PlanValue: request.PlanValue}
	for _, modifier := range modifiers {
		modifier.PlanModifyInt64(context.Background(), request, response)
	}
	if !response.PlanValue.Equal(request.StateValue) {
		t.Fatalf("unknown int64 plan = %v, want prior state %v", response.PlanValue, request.StateValue)
	}
}

func TestFileStorageResourceModelPreserveSizeIfUsageRefreshDisabled(t *testing.T) {
	tests := []struct {
		name              string
		refreshUsage      types.Bool
		currentSize       int64
		previousSize      int64
		expectedFinalSize int64
	}{
		{
			name:              "refresh enabled keeps current size",
			refreshUsage:      types.BoolValue(true),
			currentSize:       9,
			previousSize:      3,
			expectedFinalSize: 9,
		},
		{
			name:              "refresh disabled preserves previous size",
			refreshUsage:      types.BoolValue(false),
			currentSize:       9,
			previousSize:      3,
			expectedFinalSize: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := FileStorageResourceModel{
				FileStorageModel: FileStorageModel{
					Size: types.Int64Value(tt.currentSize),
				},
				RefreshUsage: tt.refreshUsage,
			}

			model.preserveSizeIfUsageRefreshDisabled(types.Int64Value(tt.previousSize))

			if got := model.Size.ValueInt64(); got != tt.expectedFinalSize {
				t.Fatalf("Size = %d, want %d", got, tt.expectedFinalSize)
			}
		})
	}
}
