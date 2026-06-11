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

func TestReference(t *testing.T) {
	t.Run("preserves a non-zero value", func(t *testing.T) {
		got := Reference(42) //nolint:modernize // exercising Reference itself, not new().
		if got == nil {
			t.Fatal("Reference returned nil")
		}
		if *got != 42 {
			t.Errorf("*Reference(42) = %d, want 42", *got)
		}
	})

	t.Run("zero value still yields a non-nil pointer", func(t *testing.T) {
		// This is the whole reason Reference exists over new(T): it keeps the
		// supplied value, including a zero, rather than zero-valuing.
		got := Reference(0) //nolint:modernize // exercising Reference itself, not new().
		if got == nil {
			t.Fatal("Reference(0) returned nil")
		}
		if *got != 0 {
			t.Errorf("*Reference(0) = %d, want 0", *got)
		}
	})

	t.Run("string", func(t *testing.T) {
		got := Reference("hello") //nolint:modernize // exercising Reference itself, not new().
		if got == nil || *got != "hello" {
			t.Errorf("*Reference(\"hello\") = %v, want \"hello\"", got)
		}
	})
}

func TestDereference(t *testing.T) {
	t.Run("non-nil returns the pointed-to value", func(t *testing.T) {
		value := 7
		if got := Dereference(&value); got != 7 {
			t.Errorf("Dereference(&7) = %d, want 7", got)
		}
	})

	t.Run("nil int returns the zero value", func(t *testing.T) {
		var p *int
		if got := Dereference(p); got != 0 {
			t.Errorf("Dereference((*int)(nil)) = %d, want 0", got)
		}
	})

	t.Run("nil string returns the empty string", func(t *testing.T) {
		var p *string
		if got := Dereference(p); got != "" {
			t.Errorf("Dereference((*string)(nil)) = %q, want \"\"", got)
		}
	})
}

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
