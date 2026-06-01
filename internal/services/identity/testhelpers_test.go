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
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// setOf builds a types.Set of strings for use in unit tests.
func setOf(t *testing.T, values ...string) types.Set {
	t.Helper()

	set, diagnostics := types.SetValueFrom(context.Background(), types.StringType, values)
	if diagnostics.HasError() {
		t.Fatalf("failed to build set: %v", diagnostics)
	}

	return set
}

// setValues extracts the string elements from a types.Set for assertions.
func setValues(t *testing.T, set types.Set) []string {
	t.Helper()

	if set.IsNull() || set.IsUnknown() {
		return []string{}
	}

	values := []string{}
	if diagnostics := set.ElementsAs(context.Background(), &values, false); diagnostics.HasError() {
		t.Fatalf("failed to read set: %v", diagnostics)
	}

	return values
}

// assertStringSliceEqual compares two string slices irrespective of ordering,
// which matches set semantics.
func assertStringSliceEqual(t *testing.T, field string, got, want []string) {
	t.Helper()

	gotCopy := append([]string(nil), got...)
	wantCopy := append([]string(nil), want...)
	sort.Strings(gotCopy)
	sort.Strings(wantCopy)

	if len(gotCopy) != len(wantCopy) {
		t.Errorf("%s = %v, want %v", field, got, want)
		return
	}

	for i := range gotCopy {
		if gotCopy[i] != wantCopy[i] {
			t.Errorf("%s = %v, want %v", field, got, want)
			return
		}
	}
}
