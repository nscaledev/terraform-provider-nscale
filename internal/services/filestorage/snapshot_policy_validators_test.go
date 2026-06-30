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

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"
)

// runSetValidator drives a set validator the way the framework does and returns
// the diagnostics it produced, so each table case can assert on them.
func runSetValidator(v validator.Set, value types.Set) validator.SetResponse {
	request := validator.SetRequest{
		Path:        path.Root("snapshot_policies"),
		ConfigValue: value,
	}

	response := validator.SetResponse{}
	v.ValidateSet(context.Background(), request, &response)

	return response
}

// policySet builds a known snapshot_policies set from minimal API specs so set
// validators can be exercised against realistic element objects. Each policy
// gets a distinct retention so that same-name policies remain separate set
// elements rather than collapsing into one.
func policySet(names ...string) types.Set {
	list := make(regionapi.StorageSnapshotPolicyListV2Spec, 0, len(names))
	for i, name := range names {
		list = append(list, regionapi.StorageSnapshotPolicyV2Spec{
			Name: name,
			Schedule: regionapi.StorageSnapshotScheduleV2Spec{
				Interval: regionapi.StorageSnapshotScheduleIntervalV2Hourly,
			},
			Retention: regionapi.StorageSnapshotRetentionV2Spec{Keep: i + 1},
		})
	}
	return NewFileStorageSnapshotPolicies(&list)
}

// Two policies that share a name produce an ambiguous full-set replacement, so
// the provider must reject duplicate names before apply.
func TestUniqueSnapshotPolicyNamesRejectsDuplicates(t *testing.T) {
	response := runSetValidator(uniqueSnapshotPolicyNamesValidator{}, policySet("daily", "daily"))

	if !response.Diagnostics.HasError() {
		t.Fatal("HasError() = false, want true for a set with duplicate policy names")
	}
}

// A set whose policy names are all distinct is the normal, valid case and must
// not raise a duplicate-name error.
func TestUniqueSnapshotPolicyNamesAcceptsDistinctNames(t *testing.T) {
	response := runSetValidator(uniqueSnapshotPolicyNamesValidator{}, policySet("daily", "weekly", "monthly"))

	if response.Diagnostics.HasError() {
		t.Fatalf("HasError() = true, want false for distinct policy names (diags: %v)", response.Diagnostics)
	}
}

// A null set (omitted config) carries no policies to compare, so the validator
// must skip it rather than dereference absent elements.
func TestUniqueSnapshotPolicyNamesSkipsNull(t *testing.T) {
	response := runSetValidator(
		uniqueSnapshotPolicyNamesValidator{},
		types.SetNull(FileStorageSnapshotPolicyModelAttributeType),
	)

	if response.Diagnostics.HasError() {
		t.Fatalf("HasError() = true, want false for a null set (diags: %v)", response.Diagnostics)
	}
}

// runStringValidators drives a slice of string validators the way the framework
// does and returns the accumulated diagnostics, so a table case can assert on
// the combined result of every validator wired onto an attribute.
func runStringValidators(vs []validator.String, value types.String) validator.StringResponse {
	request := validator.StringRequest{
		Path:        path.Root("test"),
		ConfigValue: value,
	}

	response := validator.StringResponse{}
	for _, v := range vs {
		v.ValidateString(context.Background(), request, &response)
	}

	return response
}

// runInt64Validators is runStringValidators for int64 attributes.
func runInt64Validators(vs []validator.Int64, value types.Int64) validator.Int64Response {
	request := validator.Int64Request{
		Path:        path.Root("test"),
		ConfigValue: value,
	}

	response := validator.Int64Response{}
	for _, v := range vs {
		v.ValidateInt64(context.Background(), request, &response)
	}

	return response
}

// runObjectValidator drives an object validator the way the framework does and
// returns the diagnostics it produced.
func runObjectValidator(value types.Object) validator.ObjectResponse {
	request := validator.ObjectRequest{
		Path:        path.Root("schedule"),
		ConfigValue: value,
	}

	response := validator.ObjectResponse{}
	snapshotScheduleShapeValidator{}.ValidateObject(context.Background(), request, &response)

	return response
}

// scheduleObject builds a schedule object with the given interval and optional
// fields. A nil pointer maps to a null attribute, letting tests express both
// "configured with X" and "X omitted" (null) shapes.
func scheduleObject(interval string, timeOfDay, dayOfWeek *string, dayOfMonth *int64) types.Object {
	str := func(p *string) attr.Value {
		if p == nil {
			return types.StringNull()
		}
		return types.StringValue(*p)
	}

	dom := types.Int64Null()
	if dayOfMonth != nil {
		dom = types.Int64Value(*dayOfMonth)
	}

	return types.ObjectValueMust(
		FileStorageSnapshotScheduleModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"interval":     types.StringValue(interval),
			"time_of_day":  str(timeOfDay),
			"day_of_week":  str(dayOfWeek),
			"day_of_month": dom,
		},
	)
}

// An hourly schedule takes snapshots every hour, so a configured time of day is
// meaningless and must be rejected before apply.
func TestSnapshotScheduleShapeHourlyRejectsTimeOfDay(t *testing.T) {
	schedule := scheduleObject("hourly", new("02:00Z"), nil, nil)

	response := runObjectValidator(schedule)

	if !response.Diagnostics.HasError() {
		t.Fatal("HasError() = false, want true for an hourly schedule with time_of_day")
	}
}

// A daily snapshot fires once per day at a chosen UTC time, so the time of day
// is mandatory and must be reported as missing before apply.
func TestSnapshotScheduleShapeDailyRequiresTimeOfDay(t *testing.T) {
	schedule := scheduleObject("daily", nil, nil, nil)

	response := runObjectValidator(schedule)

	if !response.Diagnostics.HasError() {
		t.Fatal("HasError() = false, want true for a daily schedule missing time_of_day")
	}
}

// A weekly snapshot needs a weekday in addition to a time of day; a time alone
// leaves the weekday ambiguous and must be rejected.
func TestSnapshotScheduleShapeWeeklyRequiresDayOfWeek(t *testing.T) {
	schedule := scheduleObject("weekly", new("02:00Z"), nil, nil)

	response := runObjectValidator(schedule)

	if !response.Diagnostics.HasError() {
		t.Fatal("HasError() = false, want true for a weekly schedule missing day_of_week")
	}
}

// A monthly snapshot needs a day of month in addition to a time of day; a time
// alone leaves the calendar day ambiguous and must be rejected.
func TestSnapshotScheduleShapeMonthlyRequiresDayOfMonth(t *testing.T) {
	schedule := scheduleObject("monthly", new("02:00Z"), nil, nil)

	response := runObjectValidator(schedule)

	if !response.Diagnostics.HasError() {
		t.Fatal("HasError() = false, want true for a monthly schedule missing day_of_month")
	}
}

// If the interval enum validator is extended without adding shape rules here,
// the shape validator must fail closed instead of silently accepting it.
func TestSnapshotScheduleShapeRejectsUnsupportedInterval(t *testing.T) {
	schedule := scheduleObject("yearly", nil, nil, nil)

	response := runObjectValidator(schedule)

	if !response.Diagnostics.HasError() {
		t.Fatal("HasError() = false, want true for an unsupported schedule interval")
	}
}

// Each interval has exactly one well-formed shape: hourly carries no timing
// fields, daily a time, weekly a time and weekday, monthly a time and day of
// month. The validator must accept these and never invent a diff for a correct
// configuration.
func TestSnapshotScheduleShapeAcceptsValidSchedules(t *testing.T) {
	tests := []struct {
		name     string
		schedule types.Object
	}{
		{"hourly", scheduleObject("hourly", nil, nil, nil)},
		{"daily", scheduleObject("daily", new("02:00Z"), nil, nil)},
		{"weekly", scheduleObject("weekly", new("02:00Z"), new("monday"), nil)},
		{"monthly", scheduleObject("monthly", new("02:00Z"), nil, new(int64(15)))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := runObjectValidator(tt.schedule)

			if response.Diagnostics.HasError() {
				t.Fatalf(
					"HasError() = true, want false for a valid %s schedule (diags: %v)",
					tt.name,
					response.Diagnostics,
				)
			}
		})
	}
}

// Every interval forbids the timing fields that do not belong to it. Setting a
// disallowed field must fail before apply rather than be silently dropped.
func TestSnapshotScheduleShapeRejectsDisallowedFields(t *testing.T) {
	tests := []struct {
		name     string
		schedule types.Object
	}{
		{"hourly with day_of_week", scheduleObject("hourly", nil, new("monday"), nil)},
		{"hourly with day_of_month", scheduleObject("hourly", nil, nil, new(int64(15)))},
		{"daily with day_of_week", scheduleObject("daily", new("02:00Z"), new("monday"), nil)},
		{"daily with day_of_month", scheduleObject("daily", new("02:00Z"), nil, new(int64(15)))},
		{"weekly with day_of_month", scheduleObject("weekly", new("02:00Z"), new("monday"), new(int64(15)))},
		{"monthly with day_of_week", scheduleObject("monthly", new("02:00Z"), new("monday"), new(int64(15)))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := runObjectValidator(tt.schedule)

			if !response.Diagnostics.HasError() {
				t.Fatalf("HasError() = false, want true for %s", tt.name)
			}
		})
	}
}

// A null optional field must behave exactly like an omitted one: it satisfies a
// "not allowed" rule (an hourly schedule with all timing fields null is valid)
// and fails a "required" rule (a daily schedule with a null time of day is
// missing it).
func TestSnapshotScheduleShapeTreatsNullAsOmitted(t *testing.T) {
	validHourly := runObjectValidator(
		scheduleObject("hourly", types.StringNull().ValueStringPointer(), nil, nil),
	)
	if validHourly.Diagnostics.HasError() {
		t.Fatalf(
			"HasError() = true, want false for an hourly schedule with null timing fields (diags: %v)",
			validHourly.Diagnostics,
		)
	}

	missingDaily := runObjectValidator(
		scheduleObject("daily", types.StringNull().ValueStringPointer(), nil, nil),
	)
	if !missingDaily.Diagnostics.HasError() {
		t.Fatal("HasError() = false, want true for a daily schedule with a null time_of_day")
	}
}

// Retention must keep at least one snapshot; keep below one is meaningless and
// must be rejected before apply, while one and above are accepted.
func TestSnapshotRetentionKeepValidators(t *testing.T) {
	tests := []struct {
		name    string
		keep    types.Int64
		wantErr bool
	}{
		{"zero is rejected", types.Int64Value(0), true},
		{"negative is rejected", types.Int64Value(-1), true},
		{"one is accepted", types.Int64Value(1), false},
		{"many is accepted", types.Int64Value(30), false},
		{"null is skipped", types.Int64Null(), false},
		{"unknown is skipped", types.Int64Unknown(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := runInt64Validators(snapshotRetentionKeepValidators(), tt.keep)

			if got := response.Diagnostics.HasError(); got != tt.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tt.wantErr, response.Diagnostics)
			}
		})
	}
}

// Monthly day of month is bounded to 1 through 28 so it never lands on an
// ambiguous month-end date; values outside that range must be rejected.
func TestSnapshotDayOfMonthValidators(t *testing.T) {
	tests := []struct {
		name    string
		day     types.Int64
		wantErr bool
	}{
		{"zero is rejected", types.Int64Value(0), true},
		{"one is accepted", types.Int64Value(1), false},
		{"twenty-eight is accepted", types.Int64Value(28), false},
		{"twenty-nine is rejected", types.Int64Value(29), true},
		{"thirty-one is rejected", types.Int64Value(31), true},
		{"null is skipped", types.Int64Null(), false},
		{"unknown is skipped", types.Int64Unknown(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := runInt64Validators(snapshotDayOfMonthValidators(), tt.day)

			if got := response.Diagnostics.HasError(); got != tt.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tt.wantErr, response.Diagnostics)
			}
		})
	}
}

// Schedule cadence is limited to the four API intervals; anything else must be
// rejected before apply.
func TestSnapshotScheduleIntervalValidators(t *testing.T) {
	tests := []struct {
		name     string
		interval types.String
		wantErr  bool
	}{
		{"hourly", types.StringValue("hourly"), false},
		{"daily", types.StringValue("daily"), false},
		{"weekly", types.StringValue("weekly"), false},
		{"monthly", types.StringValue("monthly"), false},
		{"yearly is rejected", types.StringValue("yearly"), true},
		{"empty is rejected", types.StringValue(""), true},
		{"uppercase is rejected", types.StringValue("Daily"), true},
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := runStringValidators(snapshotScheduleIntervalValidators(), tt.interval)

			if got := response.Diagnostics.HasError(); got != tt.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tt.wantErr, response.Diagnostics)
			}
		})
	}
}

// Weekly day of week is limited to the seven API weekday values; anything else
// must be rejected before apply.
func TestSnapshotDayOfWeekValidators(t *testing.T) {
	tests := []struct {
		name    string
		day     types.String
		wantErr bool
	}{
		{"monday", types.StringValue("monday"), false},
		{"sunday", types.StringValue("sunday"), false},
		{"abbreviation is rejected", types.StringValue("mon"), true},
		{"uppercase is rejected", types.StringValue("Monday"), true},
		{"garbage is rejected", types.StringValue("someday"), true},
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := runStringValidators(snapshotDayOfWeekValidators(), tt.day)

			if got := response.Diagnostics.HasError(); got != tt.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tt.wantErr, response.Diagnostics)
			}
		})
	}
}

// Time of day must be a UTC HH:MMZ value; malformed or non-UTC times must be
// rejected before apply.
func TestSnapshotTimeOfDayValidators(t *testing.T) {
	tests := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"midnight", types.StringValue("00:00Z"), false},
		{"end of day", types.StringValue("23:59Z"), false},
		{"afternoon", types.StringValue("14:30Z"), false},
		{"hour out of range", types.StringValue("24:00Z"), true},
		{"minute out of range", types.StringValue("12:60Z"), true},
		{"missing zulu", types.StringValue("02:00"), true},
		{"with seconds", types.StringValue("02:00:00Z"), true},
		{"single digit hour", types.StringValue("2:00Z"), true},
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := runStringValidators(snapshotTimeOfDayValidators(), tt.value)

			if got := response.Diagnostics.HasError(); got != tt.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tt.wantErr, response.Diagnostics)
			}
		})
	}
}

// Policy names are the stable identity key and must follow the API name pattern
// and 19-character maximum; violations must be reported before apply.
func TestSnapshotPolicyNameValidators(t *testing.T) {
	tests := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"simple lowercase", types.StringValue("daily"), false},
		{"with digits and hyphens", types.StringValue("daily-02"), false},
		{"single character", types.StringValue("a"), false},
		{"max length 19", types.StringValue("a234567890123456789"), false},
		{"too long 20", types.StringValue("a2345678901234567890"), true},
		{"leading digit", types.StringValue("1daily"), true},
		{"uppercase", types.StringValue("Daily"), true},
		{"underscore", types.StringValue("my_policy"), true},
		{"trailing hyphen", types.StringValue("daily-"), true},
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := runStringValidators(snapshotPolicyNameValidators(), tt.value)

			if got := response.Diagnostics.HasError(); got != tt.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tt.wantErr, response.Diagnostics)
			}
		})
	}
}

// runSetValidators drives a slice of set validators the way the framework does
// and returns the accumulated diagnostics.
func runSetValidators(vs []validator.Set, value types.Set) validator.SetResponse {
	request := validator.SetRequest{
		Path:        path.Root("snapshot_policies"),
		ConfigValue: value,
	}

	response := validator.SetResponse{}
	for _, v := range vs {
		v.ValidateSet(context.Background(), request, &response)
	}

	return response
}

// The user-managed Snapshot Policy Set caps at four policies, and a fifth must
// be rejected before apply.
func TestSnapshotPoliciesSetValidatorsRejectsMoreThanFour(t *testing.T) {
	response := runSetValidators(
		snapshotPoliciesSetValidators(),
		policySet("one", "two", "three", "four", "five"),
	)

	if !response.Diagnostics.HasError() {
		t.Fatal("HasError() = false, want true for a set of five policies")
	}
}

// Four distinct policies sit at the cap and must be accepted.
func TestSnapshotPoliciesSetValidatorsAcceptsFour(t *testing.T) {
	response := runSetValidators(
		snapshotPoliciesSetValidators(),
		policySet("one", "two", "three", "four"),
	)

	if response.Diagnostics.HasError() {
		t.Fatalf("HasError() = true, want false for a set of four policies (diags: %v)", response.Diagnostics)
	}
}

// The set-level validators also carry the duplicate-name rule, so a duplicate
// within the cap is still rejected.
func TestSnapshotPoliciesSetValidatorsRejectsDuplicateNames(t *testing.T) {
	response := runSetValidators(snapshotPoliciesSetValidators(), policySet("daily", "daily"))

	if !response.Diagnostics.HasError() {
		t.Fatal("HasError() = false, want true for a set with duplicate policy names")
	}
}
