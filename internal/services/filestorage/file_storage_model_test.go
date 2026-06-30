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
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"
)

// A syntactically valid UUID so request-building helpers that parse the region
// ID succeed; the value is otherwise irrelevant to these tests.
const testRegionID = "11111111-1111-1111-1111-111111111111"

// unsortedSnapshotPolicySet builds a known set holding two user-managed
// policies given out of name order ("weekly" before "daily"). Request building
// must emit them sorted by name, so this fixture lets create and update tests
// assert deterministic ordering.
func unsortedSnapshotPolicySet() types.Set {
	return NewFileStorageSnapshotPolicies(&regionapi.StorageSnapshotPolicyListV2Spec{
		{
			Name: "weekly",
			Schedule: regionapi.StorageSnapshotScheduleV2Spec{
				Interval: regionapi.StorageSnapshotScheduleIntervalV2Weekly,
			},
			Retention: regionapi.StorageSnapshotRetentionV2Spec{Keep: 4},
		},
		{
			Name: "daily",
			Schedule: regionapi.StorageSnapshotScheduleV2Spec{
				Interval: regionapi.StorageSnapshotScheduleIntervalV2Daily,
			},
			Retention: regionapi.StorageSnapshotRetentionV2Spec{Keep: 7},
		},
	})
}

// singleDailySnapshotPolicySet builds a known set holding exactly one
// user-managed daily policy with a UTC time of day. It lets the
// single-custom-policy request-building test assert that schedule and retention
// detail — not just the policy name — survive into the SDK request. Building it
// through the read mapper mirrors the real round-trip: an API read populates the
// set and a subsequent apply marshals it back.
func singleDailySnapshotPolicySet() types.Set {
	return NewFileStorageSnapshotPolicies(&regionapi.StorageSnapshotPolicyListV2Spec{
		{
			Name: "daily",
			Schedule: regionapi.StorageSnapshotScheduleV2Spec{
				Interval:  regionapi.StorageSnapshotScheduleIntervalV2Daily,
				TimeOfDay: new("02:00Z"),
			},
			Retention: regionapi.StorageSnapshotRetentionV2Spec{Keep: 7},
		},
	})
}

// A Snapshot Policy Set is unordered: two sets holding the same user-managed
// policies in different orders must be equal. This is what makes reordering
// policies in configuration produce no plan diff, and is the whole reason the
// policy collection is modelled as a set rather than a list. The fixture builds
// each side through the read mapper, mirroring how an API read populates state.
func TestSnapshotPolicySetIsOrderInsensitive(t *testing.T) {
	weekly := regionapi.StorageSnapshotPolicyV2Spec{
		Name:      "weekly",
		Schedule:  regionapi.StorageSnapshotScheduleV2Spec{Interval: regionapi.StorageSnapshotScheduleIntervalV2Weekly},
		Retention: regionapi.StorageSnapshotRetentionV2Spec{Keep: 4},
	}
	daily := regionapi.StorageSnapshotPolicyV2Spec{
		Name:      "daily",
		Schedule:  regionapi.StorageSnapshotScheduleV2Spec{Interval: regionapi.StorageSnapshotScheduleIntervalV2Daily},
		Retention: regionapi.StorageSnapshotRetentionV2Spec{Keep: 7},
	}

	weeklyThenDaily := NewFileStorageSnapshotPolicies(&regionapi.StorageSnapshotPolicyListV2Spec{weekly, daily})
	dailyThenWeekly := NewFileStorageSnapshotPolicies(&regionapi.StorageSnapshotPolicyListV2Spec{daily, weekly})

	if !weeklyThenDaily.Equal(dailyThenWeekly) {
		t.Fatalf(
			"snapshot policy sets differing only in order are not equal:\n %v\n %v",
			weeklyThenDaily,
			dailyThenWeekly,
		)
	}
}

// snapshotPolicyNames returns the ordered policy names in an API request list,
// or nil when the request omits the field (null). It lets request-building
// tests distinguish "observe/preserve" (nil pointer), "enforce empty"
// (non-nil, empty), and "enforce exact set" (non-nil, named) without caring
// about per-policy detail.
func snapshotPolicyNames(list *regionapi.StorageSnapshotPolicyListV2Spec) []string {
	if list == nil {
		return nil
	}
	names := make([]string, 0, len(*list))
	for _, policy := range *list {
		names = append(names, policy.Name)
	}
	return names
}

// assertSnapshotPolicyNames asserts the request list carries exactly want, in
// order. A nil want means the field must be omitted (null pointer); a non-nil
// (possibly empty) want means the field must be present with those names. The
// nil-versus-empty distinction is the whole point, so [reflect.DeepEqual] is
// used deliberately (it separates nil from an empty slice).
func assertSnapshotPolicyNames(t *testing.T, got *regionapi.StorageSnapshotPolicyListV2Spec, want []string) {
	t.Helper()

	if want == nil {
		if got != nil {
			t.Fatalf("snapshotPolicies = %v, want omitted (null)", snapshotPolicyNames(got))
		}
		return
	}

	if got == nil {
		t.Fatalf("snapshotPolicies omitted (null), want %v", want)
	}
	if names := snapshotPolicyNames(got); !reflect.DeepEqual(names, want) {
		t.Fatalf("snapshotPolicies names = %v, want %v", names, want)
	}
}

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
		{name: "enabled", resolved: new(true), want: types.BoolValue(true)},
		{name: "disabled", resolved: new(false), want: types.BoolValue(false)},
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
		{name: "explicit true enforces enabled", configured: types.BoolValue(true), want: new(true)},
		{name: "explicit false enforces disabled", configured: types.BoolValue(false), want: new(false)},
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

			params, diagnostics := model.NscaleFileStorageCreateParams(context.Background(), "org")
			if diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diagnostics)
			}

			assertBoolPointerEqual(t, params.Spec.DefaultSnapshotProtectionEnabled, tt.want)
		})
	}
}

// A read that exposes no user-managed Snapshot Policies must map to a known
// empty set, not null. The explicit-empty path depends on this: a user who
// configures `snapshot_policies = []` must see the resulting state match their
// configuration so that apply produces no post-apply diff.
func TestNewFileStorageModelMapsAbsentSnapshotPoliciesToEmptySet(t *testing.T) {
	var source regionapi.StorageV2Read
	source.Spec.SnapshotPolicies = nil

	model := NewFileStorageModel(&source)

	if model.SnapshotPolicies.IsNull() {
		t.Fatal("SnapshotPolicies = null, want empty set")
	}
	if got := len(model.SnapshotPolicies.Elements()); got != 0 {
		t.Fatalf("SnapshotPolicies has %d elements, want 0", got)
	}
}

// An API list that is present but empty represents a File Storage with no
// user-managed Snapshot Policies and must map to the same known empty set as an
// absent list.
func TestNewFileStorageModelMapsEmptySnapshotPolicyListToEmptySet(t *testing.T) {
	var source regionapi.StorageV2Read
	empty := regionapi.StorageSnapshotPolicyListV2Spec{}
	source.Spec.SnapshotPolicies = &empty

	model := NewFileStorageModel(&source)

	if model.SnapshotPolicies.IsNull() {
		t.Fatal("SnapshotPolicies = null, want empty set")
	}
	if got := len(model.SnapshotPolicies.Elements()); got != 0 {
		t.Fatalf("SnapshotPolicies has %d elements, want 0", got)
	}
}

// Reads must surface the API's user-managed Snapshot Policies so that importing
// or refreshing File Storage that already has policies does not silently drop
// them (which would otherwise clear them on the next apply).
func TestNewFileStorageModelMapsUserManagedSnapshotPolicies(t *testing.T) {
	var source regionapi.StorageV2Read
	source.Spec.SnapshotPolicies = &regionapi.StorageSnapshotPolicyListV2Spec{
		{
			Name: "daily",
			Schedule: regionapi.StorageSnapshotScheduleV2Spec{
				Interval:  regionapi.StorageSnapshotScheduleIntervalV2Daily,
				TimeOfDay: new("02:00Z"),
			},
			Retention: regionapi.StorageSnapshotRetentionV2Spec{Keep: 7},
		},
	}

	model := NewFileStorageModel(&source)

	elements := model.SnapshotPolicies.Elements()
	if len(elements) != 1 {
		t.Fatalf("SnapshotPolicies has %d elements, want 1", len(elements))
	}

	policy := elements[0].(types.Object).Attributes()
	if got := policy["name"].(types.String).ValueString(); got != "daily" {
		t.Fatalf("name = %q, want daily", got)
	}

	schedule := policy["schedule"].(types.Object).Attributes()
	if got := schedule["interval"].(types.String).ValueString(); got != "daily" {
		t.Fatalf("schedule.interval = %q, want daily", got)
	}
	if got := schedule["time_of_day"].(types.String).ValueString(); got != "02:00Z" {
		t.Fatalf("schedule.time_of_day = %q, want 02:00Z", got)
	}

	retention := policy["retention"].(types.Object).Attributes()
	if got := retention["keep"].(types.Int64).ValueInt64(); got != 7 {
		t.Fatalf("retention.keep = %d, want 7", got)
	}
}

// Create must preserve the API's null-versus-empty distinction for the
// user-managed Snapshot Policy Set: an unconfigured (null) set is omitted so
// the API resolves it, an explicit empty set is sent as an empty list to
// enforce no user-managed policies, and a non-empty set is sent in full,
// deterministically ordered by name.
func TestNscaleFileStorageCreateParamsSnapshotPolicies(t *testing.T) {
	tests := []struct {
		name      string
		policies  types.Set
		wantNames []string // nil => field omitted (null)
	}{
		{
			name:      "null omits the field",
			policies:  types.SetNull(FileStorageSnapshotPolicyModelAttributeType),
			wantNames: nil,
		},
		{
			name:      "empty set enforces no policies",
			policies:  NewFileStorageSnapshotPolicies(&regionapi.StorageSnapshotPolicyListV2Spec{}),
			wantNames: []string{},
		},
		{
			name:      "non-empty set is sent sorted by name",
			policies:  unsortedSnapshotPolicySet(),
			wantNames: []string{"daily", "weekly"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := FileStorageModel{
				Name:             types.StringValue("fs"),
				RegionID:         types.StringValue(testRegionID),
				Capacity:         types.Int64Value(20),
				StorageClassID:   types.StringValue("class"),
				RootSquash:       types.BoolValue(true),
				Network:          types.ListNull(FileStorageNetworkModelAttributeType),
				SnapshotPolicies: tt.policies,
			}

			params, diagnostics := model.NscaleFileStorageCreateParams(context.Background(), "org")
			if diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diagnostics)
			}

			assertSnapshotPolicyNames(t, params.Spec.SnapshotPolicies, tt.wantNames)
		})
	}
}

// A single configured user-managed policy must reach the SDK request with its
// full schedule and retention detail, not just its name. The names-only
// request-building tests above prove null-versus-empty-versus-set semantics;
// this proves that for a configured policy the interval, UTC time of day, and
// retention count are marshaled, and that schedule fields irrelevant to a daily
// cadence (day of week, day of month) stay absent.
func TestNscaleFileStorageCreateParamsMarshalsSingleCustomPolicy(t *testing.T) {
	model := FileStorageModel{
		Name:             types.StringValue("fs"),
		RegionID:         types.StringValue(testRegionID),
		Capacity:         types.Int64Value(20),
		StorageClassID:   types.StringValue("class"),
		RootSquash:       types.BoolValue(true),
		Network:          types.ListNull(FileStorageNetworkModelAttributeType),
		SnapshotPolicies: singleDailySnapshotPolicySet(),
	}

	params, diagnostics := model.NscaleFileStorageCreateParams(context.Background(), "org")
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}

	policies := params.Spec.SnapshotPolicies
	if policies == nil {
		t.Fatal("snapshotPolicies omitted (null), want one policy")
	}
	if got := len(*policies); got != 1 {
		t.Fatalf("snapshotPolicies has %d entries, want 1", got)
	}

	policy := (*policies)[0]
	if policy.Name != "daily" {
		t.Fatalf("name = %q, want daily", policy.Name)
	}
	if policy.Schedule.Interval != regionapi.StorageSnapshotScheduleIntervalV2Daily {
		t.Fatalf("schedule.interval = %q, want daily", policy.Schedule.Interval)
	}
	if policy.Schedule.TimeOfDay == nil || *policy.Schedule.TimeOfDay != "02:00Z" {
		t.Fatalf("schedule.timeOfDay = %v, want 02:00Z", policy.Schedule.TimeOfDay)
	}
	if policy.Schedule.DayOfWeek != nil {
		t.Fatalf("schedule.dayOfWeek = %q, want absent for a daily policy", *policy.Schedule.DayOfWeek)
	}
	if policy.Schedule.DayOfMonth != nil {
		t.Fatalf("schedule.dayOfMonth = %d, want absent for a daily policy", *policy.Schedule.DayOfMonth)
	}
	if policy.Retention.Keep != 7 {
		t.Fatalf("retention.keep = %d, want 7", policy.Retention.Keep)
	}
}

// Order-only differences in the configured Snapshot Policy Set must not produce
// unstable API requests: the same user-managed policies marshal to the same
// deterministic list — sorted by name — regardless of the order they appear in
// the set. This is what lets reordering policies in configuration round-trip
// without churning the API request. A three-policy set exercises ordering beyond
// a single swapped pair.
func TestNscaleFileStorageUpdateParamsSnapshotPolicyOrderIsDeterministic(t *testing.T) {
	makeModel := func(policies types.Set) FileStorageModel {
		return FileStorageModel{
			Name:             types.StringValue("fs"),
			Capacity:         types.Int64Value(20),
			RootSquash:       types.BoolValue(true),
			Network:          types.ListNull(FileStorageNetworkModelAttributeType),
			SnapshotPolicies: policies,
		}
	}

	policy := func(name string) regionapi.StorageSnapshotPolicyV2Spec {
		return regionapi.StorageSnapshotPolicyV2Spec{
			Name: name,
			Schedule: regionapi.StorageSnapshotScheduleV2Spec{
				Interval: regionapi.StorageSnapshotScheduleIntervalV2Hourly,
			},
			Retention: regionapi.StorageSnapshotRetentionV2Spec{Keep: 3},
		}
	}

	scrambled := NewFileStorageSnapshotPolicies(&regionapi.StorageSnapshotPolicyListV2Spec{
		policy("charlie"), policy("alpha"), policy("bravo"),
	})
	reordered := NewFileStorageSnapshotPolicies(&regionapi.StorageSnapshotPolicyListV2Spec{
		policy("bravo"), policy("charlie"), policy("alpha"),
	})

	want := []string{"alpha", "bravo", "charlie"}

	for _, tt := range []struct {
		name     string
		policies types.Set
	}{
		{name: "scrambled", policies: scrambled},
		{name: "reordered", policies: reordered},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := makeModel(tt.policies)
			params, diagnostics := model.NscaleFileStorageUpdateParams(context.Background())
			if diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diagnostics)
			}
			assertSnapshotPolicyNames(t, params.Spec.SnapshotPolicies, want)
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
		{name: "explicit true enforces enabled", configured: types.BoolValue(true), want: new(true)},
		{name: "explicit false enforces disabled", configured: types.BoolValue(false), want: new(false)},
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

			params, diagnostics := model.NscaleFileStorageUpdateParams(context.Background())
			if diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diagnostics)
			}

			assertBoolPointerEqual(t, params.Spec.DefaultSnapshotProtectionEnabled, tt.want)
		})
	}
}

// Update must preserve the API's null-versus-empty distinction: a null set is
// omitted so existing user-managed policies are preserved, an explicit empty
// set is sent as an empty list to clear them, and a non-empty set replaces the
// full list, deterministically ordered by name.
func TestNscaleFileStorageUpdateParamsSnapshotPolicies(t *testing.T) {
	tests := []struct {
		name      string
		policies  types.Set
		wantNames []string // nil => field omitted (null)
	}{
		{
			name:      "null preserves existing policies",
			policies:  types.SetNull(FileStorageSnapshotPolicyModelAttributeType),
			wantNames: nil,
		},
		{
			name:      "empty set clears policies",
			policies:  NewFileStorageSnapshotPolicies(&regionapi.StorageSnapshotPolicyListV2Spec{}),
			wantNames: []string{},
		},
		{
			name:      "non-empty set replaces the full list sorted by name",
			policies:  unsortedSnapshotPolicySet(),
			wantNames: []string{"daily", "weekly"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := FileStorageModel{
				Name:             types.StringValue("fs"),
				Capacity:         types.Int64Value(20),
				RootSquash:       types.BoolValue(true),
				Network:          types.ListNull(FileStorageNetworkModelAttributeType),
				SnapshotPolicies: tt.policies,
			}

			params, diagnostics := model.NscaleFileStorageUpdateParams(context.Background())
			if diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diagnostics)
			}

			assertSnapshotPolicyNames(t, params.Spec.SnapshotPolicies, tt.wantNames)
		})
	}
}
