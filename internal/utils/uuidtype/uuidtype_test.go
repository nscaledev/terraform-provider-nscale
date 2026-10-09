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

package uuidtype_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/nscaledev/terraform-provider-nscale/internal/utils/uuidtype"
)

const canonical = "5c9b2e7f-1a4d-4b8c-8f2e-3d6a9b0c1e52"

func TestStringSemanticEquals(t *testing.T) {
	testCases := []struct {
		name  string
		prior uuidtype.Value
		next  uuidtype.Value
		want  bool
	}{
		{"identical", uuidtype.NewValue(canonical), uuidtype.NewValue(canonical), true},
		{"upper case", uuidtype.NewValue("5C9B2E7F-1A4D-4B8C-8F2E-3D6A9B0C1E52"), uuidtype.NewValue(canonical), true},
		{"braces", uuidtype.NewValue("{" + canonical + "}"), uuidtype.NewValue(canonical), true},
		{"urn", uuidtype.NewValue("urn:uuid:" + canonical), uuidtype.NewValue(canonical), true},
		{"no hyphens", uuidtype.NewValue("5c9b2e7f1a4d4b8c8f2e3d6a9b0c1e52"), uuidtype.NewValue(canonical), true},
		{
			"different UUIDs",
			uuidtype.NewValue(canonical),
			uuidtype.NewValue("2f8a1d4e-6b3c-4f5a-9e0d-7c1b8a2f3d40"),
			false,
		},
		{"same non-UUID", uuidtype.NewValue("ubuntu-24.04"), uuidtype.NewValue("ubuntu-24.04"), true},
		{"non-UUIDs differing in case", uuidtype.NewValue("Ubuntu-24.04"), uuidtype.NewValue("ubuntu-24.04"), false},
		{"UUID against non-UUID", uuidtype.NewValue(canonical), uuidtype.NewValue("ubuntu-24.04"), false},
		{"both null", uuidtype.NewNull(), uuidtype.NewNull(), true},
		{"null against value", uuidtype.NewNull(), uuidtype.NewValue(canonical), false},
		{"value against null", uuidtype.NewValue(canonical), uuidtype.NewNull(), false},
		{"both unknown", uuidtype.NewUnknown(), uuidtype.NewUnknown(), true},
		{"unknown against value", uuidtype.NewUnknown(), uuidtype.NewValue(canonical), false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, diagnostics := testCase.prior.StringSemanticEquals(context.Background(), testCase.next)
			if diagnostics.HasError() {
				t.Fatalf("StringSemanticEquals() diagnostics = %v", diagnostics)
			}

			if got != testCase.want {
				t.Errorf(
					"StringSemanticEquals(%s, %s) = %t, want %t",
					testCase.prior,
					testCase.next,
					got,
					testCase.want,
				)
			}
		})
	}
}

// Semantic equality must not leak into Equal: the framework relies on Equal to
// detect a real change, and keeps the prior spelling only through the semantic
// check.
func TestEqualIsExact(t *testing.T) {
	upper := uuidtype.NewValue("5C9B2E7F-1A4D-4B8C-8F2E-3D6A9B0C1E52")
	if upper.Equal(uuidtype.NewValue(canonical)) {
		t.Error("Equal() = true for different spellings, want false")
	}

	if uuidtype.NewValue(canonical).Equal(types.StringValue(canonical)) {
		t.Error("Equal() = true against a plain string value, want false")
	}
}

func TestStringSemanticEqualsRejectsForeignValue(t *testing.T) {
	_, diagnostics := uuidtype.NewValue(canonical).StringSemanticEquals(
		context.Background(),
		types.StringValue(canonical),
	)
	if !diagnostics.HasError() {
		t.Error("StringSemanticEquals() diagnostics = none, want an error for a plain string value")
	}
}

func TestTypeValueFromTerraform(t *testing.T) {
	value, err := uuidtype.Type{}.ValueFromTerraform(
		context.Background(),
		tftypes.NewValue(tftypes.String, canonical),
	)
	if err != nil {
		t.Fatalf("ValueFromTerraform() error = %v", err)
	}

	if !value.Equal(uuidtype.NewValue(canonical)) {
		t.Errorf("ValueFromTerraform() = %#v, want %#v", value, uuidtype.NewValue(canonical))
	}
}

func TestFromUUIDPointer(t *testing.T) {
	if got := uuidtype.FromUUIDPointer(nil); !got.IsNull() {
		t.Errorf("FromUUIDPointer(nil) = %s, want null", got)
	}

	id := uuid.MustParse(canonical)
	if got := uuidtype.FromUUIDPointer(&id); got.ValueString() != canonical {
		t.Errorf("FromUUIDPointer() = %s, want %s", got, canonical)
	}
}

func TestUseStateForSameUUID(t *testing.T) {
	upper := "5C9B2E7F-1A4D-4B8C-8F2E-3D6A9B0C1E52"
	other := "2f8a1d4e-6b3c-4f5a-9e0d-7c1b8a2f3d40"

	testCases := []struct {
		name  string
		state types.String
		plan  types.String
		want  types.String
	}{
		{"respelled", types.StringValue(canonical), types.StringValue(upper), types.StringValue(canonical)},
		{"changed", types.StringValue(canonical), types.StringValue(other), types.StringValue(other)},
		{"created", types.StringNull(), types.StringValue(upper), types.StringValue(upper)},
		{"removed", types.StringValue(canonical), types.StringNull(), types.StringNull()},
		{"unknown", types.StringValue(canonical), types.StringUnknown(), types.StringUnknown()},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := planmodifier.StringRequest{
				StateValue:  testCase.state,
				PlanValue:   testCase.plan,
				ConfigValue: testCase.plan,
			}
			response := planmodifier.StringResponse{PlanValue: testCase.plan}

			uuidtype.UseStateForSameUUID().PlanModifyString(context.Background(), request, &response)

			if response.Diagnostics.HasError() {
				t.Fatalf("PlanModifyString() diagnostics = %v", response.Diagnostics)
			}

			if !response.PlanValue.Equal(testCase.want) {
				t.Errorf("planned %s, want %s", response.PlanValue, testCase.want)
			}
		})
	}
}
