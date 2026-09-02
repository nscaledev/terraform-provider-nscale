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

import (
	"strings"
	"testing"
)

// sdkTag stands in for a service package's generated tag type, so these tests
// exercise the same generic instantiation the service packages do without
// pinning to whichever SDK version is vendored.
type sdkTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func TestAppendOperationTagOnNilTags(t *testing.T) {
	t.Parallel()

	got, key := AppendOperationTag[sdkTag](nil)

	if got == nil {
		t.Fatal("AppendOperationTag(nil) returned nil tags, want a one-element list")
	}
	if len(*got) != 1 {
		t.Fatalf("len = %d, want 1", len(*got))
	}
	if (*got)[0].Name != key {
		t.Errorf("tag name = %q, want the returned key %q", (*got)[0].Name, key)
	}
	if !strings.HasPrefix(key, TerraformOperationTagPrefix) {
		t.Errorf("key = %q, want prefix %q", key, TerraformOperationTagPrefix)
	}
}

func TestAppendOperationTagPreservesExistingTags(t *testing.T) {
	t.Parallel()

	existing := []sdkTag{{Name: "workload", Value: "training"}}

	got, key := AppendOperationTag(&existing)
	if got == nil {
		t.Fatal("AppendOperationTag returned nil tags")
	}
	if len(*got) != 2 {
		t.Fatalf("len = %d, want 2", len(*got))
	}
	if (*got)[0] != existing[0] {
		t.Errorf("[0] = %+v, want the caller's tag %+v", (*got)[0], existing[0])
	}
	if (*got)[1].Name != key {
		t.Errorf("[1].Name = %q, want %q", (*got)[1].Name, key)
	}

	// The helper returns a new list rather than mutating in place, so the
	// caller's own slice must be untouched.
	if len(existing) != 1 {
		t.Errorf("input grew to %d elements, want 1", len(existing))
	}
}

func TestAppendOperationTagKeysAreUnique(t *testing.T) {
	t.Parallel()

	// Two updates in flight must not collide, or one watcher would accept the
	// other's write as confirmation of its own.
	_, first := AppendOperationTag[sdkTag](nil)
	_, second := AppendOperationTag[sdkTag](nil)

	if first == second {
		t.Errorf("keys collided: both %q", first)
	}
}

func TestHasOperationTag(t *testing.T) {
	t.Parallel()

	tagged, key := AppendOperationTag[sdkTag](nil)
	converted := TagsFromAPI(tagged)

	if !HasOperationTag(converted, key) {
		t.Errorf("HasOperationTag(tags, %q) = false, want true", key)
	}
	if HasOperationTag(converted, TerraformOperationTagPrefix+"absent") {
		t.Error("HasOperationTag(tags, absent) = true, want false")
	}
	if HasOperationTag(nil, key) {
		t.Error("HasOperationTag(nil, key) = true, want false")
	}
}

func TestRemoveOperationTagsStripsOnlyOperationTags(t *testing.T) {
	t.Parallel()

	in := []sdkTag{
		{Name: "workload", Value: "training"},
		{Name: TerraformOperationTagPrefix + "abc", Value: "2026-08-24T00:00:00Z"},
		{Name: "owner", Value: "platform"},
	}

	got := RemoveOperationTags(&in)
	if got == nil {
		t.Fatal("RemoveOperationTags returned nil, want the user tags")
	}
	if len(*got) != 2 {
		t.Fatalf("len = %d, want 2", len(*got))
	}
	for _, tag := range *got {
		if strings.HasPrefix(tag.Name, TerraformOperationTagPrefix) {
			t.Errorf("operation tag %q survived", tag.Name)
		}
	}
	if (*got)[0].Name != "workload" || (*got)[1].Name != "owner" {
		t.Errorf("order not preserved: got %+v", *got)
	}
}

func TestRemoveOperationTagsNilIn(t *testing.T) {
	t.Parallel()

	if got := RemoveOperationTags[sdkTag](nil); got != nil {
		t.Errorf("RemoveOperationTags(nil) = %v, want nil", got)
	}
}

func TestAppendThenRemoveRoundTrips(t *testing.T) {
	t.Parallel()

	// The invariant Terraform state depends on: an update writes an operation
	// tag, and the tags read back into state must match what the user
	// configured. Any leak here surfaces as "inconsistent result after apply".
	configured := []sdkTag{
		{Name: "workload", Value: "training"},
		{Name: "owner", Value: "platform"},
	}

	tagged, _ := AppendOperationTag(&configured)

	got := RemoveOperationTags(tagged)
	if got == nil {
		t.Fatal("round trip returned nil tags")
	}
	if len(*got) != len(configured) {
		t.Fatalf("len = %d, want %d", len(*got), len(configured))
	}
	for i, want := range configured {
		if (*got)[i] != Tag(want) {
			t.Errorf("[%d] = %+v, want %+v", i, (*got)[i], want)
		}
	}
}
