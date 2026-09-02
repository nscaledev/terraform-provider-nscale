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

package tags_test

import (
	"testing"

	regionapi "github.com/nscaledev/nscale-sdk-go/region"
	legacycore "github.com/unikorn-cloud/core/pkg/openapi"

	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tags"
)

// sdkTag stands in for a service package's generated Tag. Declaring one locally
// pins the shape SDKTag matches independently of whichever SDK version is
// currently vendored, so this test keeps its meaning across a bump.
type sdkTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Compile-time proof that the constraint admits the real tag types the provider
// converts: the current shared SDK type and the legacy module's, which the
// deprecated compute_cluster resource still bridges. A shape change upstream
// fails to build here rather than at the call sites.
var (
	_ = tags.FromAPI[regionapi.Tag]
	_ = tags.FromAPI[legacycore.Tag]
	_ = tags.ToAPI[regionapi.Tag]
	_ = tags.ToAPI[legacycore.Tag]
)

func TestFromAPINil(t *testing.T) {
	t.Parallel()

	if got := tags.FromAPI[sdkTag](nil); got != nil {
		t.Errorf("FromAPI(nil) = %v, want nil", got)
	}
}

func TestFromAPIEmptyIsNotNil(t *testing.T) {
	t.Parallel()

	// An empty set must survive as an empty set: "no tags configured" and "tags
	// configured to nothing" drive different behaviour on the write path.
	in := []sdkTag{}

	got := tags.FromAPI(&in)
	if got == nil {
		t.Fatal("FromAPI(&[]) = nil, want non-nil")
	}
	if len(*got) != 0 {
		t.Errorf("len = %d, want 0", len(*got))
	}
}

func TestFromAPIPreservesOrderAndValues(t *testing.T) {
	t.Parallel()

	in := []sdkTag{
		{Name: "workload", Value: "training"},
		{Name: "owner", Value: "platform"},
	}

	got := tags.FromAPI(&in)
	if got == nil {
		t.Fatal("FromAPI = nil, want non-nil")
	}

	want := tags.List{
		{Name: "workload", Value: "training"},
		{Name: "owner", Value: "platform"},
	}
	if len(*got) != len(want) {
		t.Fatalf("len = %d, want %d", len(*got), len(want))
	}
	for i := range want {
		if (*got)[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, (*got)[i], want[i])
		}
	}
}

func TestToAPINil(t *testing.T) {
	t.Parallel()

	if got := tags.ToAPI[sdkTag](nil); got != nil {
		t.Errorf("ToAPI(nil) = %v, want nil", got)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	in := []sdkTag{
		{Name: "workload", Value: "training"},
		{Name: "owner", Value: "platform"},
	}

	out := tags.ToAPI[sdkTag](tags.FromAPI(&in))
	if out == nil {
		t.Fatal("round trip = nil, want non-nil")
	}
	if len(*out) != len(in) {
		t.Fatalf("len = %d, want %d", len(*out), len(in))
	}
	for i := range in {
		if (*out)[i] != in[i] {
			t.Errorf("[%d] = %+v, want %+v", i, (*out)[i], in[i])
		}
	}
}

func TestConversionDoesNotAliasInput(t *testing.T) {
	t.Parallel()

	// The provider strips operation tags by rebuilding the slice, so a converted
	// copy must not share backing storage with the SDK response it came from.
	in := []sdkTag{{Name: "workload", Value: "training"}}

	converted := tags.FromAPI(&in)
	(*converted)[0].Value = "mutated"

	if in[0].Value != "training" {
		t.Errorf("input mutated: got %q, want %q", in[0].Value, "training")
	}
}
