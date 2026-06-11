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

package pointer

import "testing"

// TestReferenceSlice pins the one non-trivial contract in this package: a nil
// slice must become a pointer to a non-nil *empty* slice. That distinction is
// load-bearing — a nil slice marshals to JSON `null`, an empty slice to `[]`,
// and getting it wrong produces spurious diffs or rejected request bodies.
// (Reference and Dereference are one-line generic wrappers with no logic worth
// testing; ReferenceSlice exercises Reference transitively.)
func TestReferenceSlice(t *testing.T) {
	t.Run("nil slice becomes a pointer to a non-nil empty slice", func(t *testing.T) {
		var in []int
		got := ReferenceSlice(in)
		if got == nil {
			t.Fatal("ReferenceSlice(nil) returned a nil pointer")
		}
		if *got == nil {
			t.Error("ReferenceSlice(nil) pointed at a nil slice, want a non-nil empty slice")
		}
		if len(*got) != 0 {
			t.Errorf("len(*ReferenceSlice(nil)) = %d, want 0", len(*got))
		}
	})

	t.Run("non-empty slice is preserved", func(t *testing.T) {
		in := []string{"a", "b"}
		got := ReferenceSlice(in)
		if got == nil {
			t.Fatal("ReferenceSlice returned nil")
		}
		if len(*got) != 2 || (*got)[0] != "a" || (*got)[1] != "b" {
			t.Errorf("*ReferenceSlice(%v) = %v, want [a b]", in, *got)
		}
	})
}
