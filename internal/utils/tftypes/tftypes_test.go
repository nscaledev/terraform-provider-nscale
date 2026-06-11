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

package tftypes

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
)

func TestNullableListValueMust(t *testing.T) {
	t.Run("nil elements yield a null list", func(t *testing.T) {
		got := NullableListValueMust(types.StringType, nil)
		if !got.IsNull() {
			t.Errorf("NullableListValueMust(nil) = %v, want null", got)
		}
	})

	t.Run("empty elements yield a null list", func(t *testing.T) {
		got := NullableListValueMust(types.StringType, []attr.Value{})
		if !got.IsNull() {
			t.Errorf("NullableListValueMust([]) = %v, want null", got)
		}
	})

	t.Run("non-empty elements yield a populated list", func(t *testing.T) {
		got := NullableListValueMust(types.StringType, []attr.Value{types.StringValue("x")})
		if got.IsNull() {
			t.Fatal("NullableListValueMust([x]) is null, want populated")
		}
		if n := len(got.Elements()); n != 1 {
			t.Errorf("len(elements) = %d, want 1", n)
		}
	})
}

func TestNullableSetValueMust(t *testing.T) {
	t.Run("nil elements yield a null set", func(t *testing.T) {
		got := NullableSetValueMust(types.StringType, nil)
		if !got.IsNull() {
			t.Errorf("NullableSetValueMust(nil) = %v, want null", got)
		}
	})

	t.Run("empty elements yield a null set", func(t *testing.T) {
		got := NullableSetValueMust(types.StringType, []attr.Value{})
		if !got.IsNull() {
			t.Errorf("NullableSetValueMust([]) = %v, want null", got)
		}
	})

	t.Run("non-empty elements yield a populated set", func(t *testing.T) {
		got := NullableSetValueMust(types.StringType, []attr.Value{types.StringValue("x")})
		if got.IsNull() {
			t.Fatal("NullableSetValueMust([x]) is null, want populated")
		}
		if n := len(got.Elements()); n != 1 {
			t.Errorf("len(elements) = %d, want 1", n)
		}
	})
}

func TestTagMapValueMust(t *testing.T) {
	t.Run("nil tags yield a null map", func(t *testing.T) {
		if got := TagMapValueMust(nil); !got.IsNull() {
			t.Errorf("TagMapValueMust(nil) = %v, want null", got)
		}
	})

	t.Run("empty tags yield a null map", func(t *testing.T) {
		empty := []coreapi.Tag{}
		if got := TagMapValueMust(&empty); !got.IsNull() {
			t.Errorf("TagMapValueMust(&[]) = %v, want null", got)
		}
	})

	t.Run("tags become a string map keyed by name", func(t *testing.T) {
		tags := []coreapi.Tag{
			{Name: "env", Value: "prod"},
			{Name: "team", Value: "devex"},
		}
		got := TagMapValueMust(&tags)
		if got.IsNull() {
			t.Fatal("TagMapValueMust(tags) is null, want populated")
		}

		elements := got.Elements()
		if len(elements) != 2 {
			t.Fatalf("len(elements) = %d, want 2", len(elements))
		}
		if !elements["env"].Equal(types.StringValue("prod")) {
			t.Errorf("elements[env] = %v, want \"prod\"", elements["env"])
		}
		if !elements["team"].Equal(types.StringValue("devex")) {
			t.Errorf("elements[team] = %v, want \"devex\"", elements["team"])
		}
	})
}

// TestValueTagListPointerReturnsNil covers the three inputs that should collapse
// to (nil, no diagnostics): a null, unknown, or empty tag map.
func TestValueTagListPointerReturnsNil(t *testing.T) {
	testCases := []struct {
		name string
		in   basetypes.MapValue
	}{
		{"null map", basetypes.NewMapNull(types.StringType)},
		{"unknown map", basetypes.NewMapUnknown(types.StringType)},
		{"empty map", basetypes.NewMapValueMust(types.StringType, map[string]attr.Value{})},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, diags := ValueTagListPointer(testCase.in)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got != nil {
				t.Errorf("got %v, want nil", got)
			}
		})
	}
}

func TestValueTagListPointerPopulated(t *testing.T) {
	in := basetypes.NewMapValueMust(types.StringType, map[string]attr.Value{
		"env":  types.StringValue("prod"),
		"team": types.StringValue("devex"),
	})

	got, diags := ValueTagListPointer(in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got == nil {
		t.Fatal("got nil, want tags")
	}

	// Map iteration order is unspecified, so compare via a name->value map.
	asMap := make(map[string]string, len(*got))
	for _, tag := range *got {
		asMap[tag.Name] = tag.Value
	}
	if len(asMap) != 2 || asMap["env"] != "prod" || asMap["team"] != "devex" {
		t.Errorf("tags = %v, want {env:prod, team:devex}", asMap)
	}
}

func TestValueTagListPointerSurfacesDiagnostics(t *testing.T) {
	// ElementsAs into map[string]string cannot decode int64 elements, so the
	// conversion must return the diagnostics rather than panic or drop them.
	in := basetypes.NewMapValueMust(types.Int64Type, map[string]attr.Value{
		"count": types.Int64Value(1),
	})

	got, diags := ValueTagListPointer(in)
	if !diags.HasError() {
		t.Error("expected diagnostics for a non-string element type, got none")
	}
	if got != nil {
		t.Errorf("got %v, want nil on error", got)
	}
}
