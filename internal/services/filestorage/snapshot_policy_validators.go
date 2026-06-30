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
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"
)

// snapshotTimeOfDayPattern matches the API's UTC HH:MMZ time-of-day format.
var snapshotTimeOfDayPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]Z$`)

// snapshotPolicyNamePattern matches the API's snapshot policy name pattern: a
// lowercase letter followed by lowercase letters, digits, or hyphens, ending in
// a letter or digit.
var snapshotPolicyNamePattern = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)

// Snapshot Policy field constraints mirrored from the File Storage API contract
// (region openapi.yaml). They are enforced locally so invalid configurations
// fail during planning instead of after an API request.
const (
	// snapshotRetentionKeepMin is the smallest number of snapshots a policy may
	// retain.
	snapshotRetentionKeepMin = 1

	// snapshotDayOfMonthMin and snapshotDayOfMonthMax bound a monthly schedule's
	// day of month so it never lands on an ambiguous month-end date.
	snapshotDayOfMonthMin = 1
	snapshotDayOfMonthMax = 28

	// snapshotPolicyNameMaxLength is the API's maximum snapshot policy name length.
	snapshotPolicyNameMaxLength = 19

	// snapshotPolicySetMaxSize is the API's maximum number of user-managed
	// Snapshot Policies per File Storage.
	snapshotPolicySetMaxSize = 4
)

// snapshotPoliciesSetValidators constrains the user-managed Snapshot Policy Set
// to the API's maximum size and rejects duplicate policy names.
func snapshotPoliciesSetValidators() []validator.Set {
	return []validator.Set{
		setvalidator.SizeAtMost(snapshotPolicySetMaxSize),
		uniqueSnapshotPolicyNamesValidator{},
	}
}

// snapshotPolicyNameValidators constrains a Snapshot Policy name to the API's
// name pattern and maximum length.
func snapshotPolicyNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtMost(snapshotPolicyNameMaxLength),
		stringvalidator.RegexMatches(
			snapshotPolicyNamePattern,
			"must start with a lowercase letter, contain only lowercase letters, digits or hyphens, and end with a letter or digit",
		),
	}
}

// snapshotRetentionKeepValidators constrains a Snapshot Retention keep count to
// the API minimum of one.
func snapshotRetentionKeepValidators() []validator.Int64 {
	return []validator.Int64{
		int64validator.AtLeast(snapshotRetentionKeepMin),
	}
}

// snapshotDayOfMonthValidators constrains a monthly schedule's day of month to
// the API range of 1 through 28.
func snapshotDayOfMonthValidators() []validator.Int64 {
	return []validator.Int64{
		int64validator.Between(snapshotDayOfMonthMin, snapshotDayOfMonthMax),
	}
}

// snapshotScheduleValidators enforces the per-interval schedule shape (which
// timing fields are required and which are not allowed).
func snapshotScheduleValidators() []validator.Object {
	return []validator.Object{
		snapshotScheduleShapeValidator{},
	}
}

// snapshotScheduleIntervalValidators limits a schedule's cadence to the API's
// four supported intervals.
func snapshotScheduleIntervalValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(
			string(regionapi.StorageSnapshotScheduleIntervalV2Hourly),
			string(regionapi.StorageSnapshotScheduleIntervalV2Daily),
			string(regionapi.StorageSnapshotScheduleIntervalV2Weekly),
			string(regionapi.StorageSnapshotScheduleIntervalV2Monthly),
		),
	}
}

// snapshotTimeOfDayValidators constrains a schedule's time of day to the API's
// UTC HH:MMZ format.
func snapshotTimeOfDayValidators() []validator.String {
	return []validator.String{
		stringvalidator.RegexMatches(
			snapshotTimeOfDayPattern,
			"must be a UTC time of day in HH:MMZ form, for example 02:00Z",
		),
	}
}

// snapshotDayOfWeekValidators limits a weekly schedule's day of week to the
// API's seven weekday values.
func snapshotDayOfWeekValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(
			string(regionapi.StorageSnapshotDayOfWeekV2Monday),
			string(regionapi.StorageSnapshotDayOfWeekV2Tuesday),
			string(regionapi.StorageSnapshotDayOfWeekV2Wednesday),
			string(regionapi.StorageSnapshotDayOfWeekV2Thursday),
			string(regionapi.StorageSnapshotDayOfWeekV2Friday),
			string(regionapi.StorageSnapshotDayOfWeekV2Saturday),
			string(regionapi.StorageSnapshotDayOfWeekV2Sunday),
		),
	}
}

// uniqueSnapshotPolicyNamesValidator rejects a user-managed Snapshot Policy Set
// that contains more than one policy with the same name. Names are the policies'
// stable identity keys, so duplicates would make full-set replacement ambiguous.
type uniqueSnapshotPolicyNamesValidator struct{}

func (v uniqueSnapshotPolicyNamesValidator) Description(ctx context.Context) string {
	return "snapshot policy names must be unique within the file storage"
}

func (v uniqueSnapshotPolicyNamesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v uniqueSnapshotPolicyNamesValidator) ValidateSet(
	ctx context.Context,
	request validator.SetRequest,
	response *validator.SetResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	seen := map[string]struct{}{}
	for _, element := range request.ConfigValue.Elements() {
		object, ok := element.(types.Object)
		if !ok {
			continue
		}

		name, ok := object.Attributes()["name"].(types.String)
		if !ok || name.IsNull() || name.IsUnknown() {
			continue
		}

		if _, duplicate := seen[name.ValueString()]; duplicate {
			response.Diagnostics.AddAttributeError(
				request.Path,
				"Duplicate Snapshot Policy Name",
				fmt.Sprintf(
					"Attribute %s contains more than one policy named %q; %s.",
					request.Path,
					name.ValueString(),
					v.Description(ctx),
				),
			)
			return
		}
		seen[name.ValueString()] = struct{}{}
	}
}

// snapshotScheduleShapeValidator enforces that a Snapshot Schedule carries
// exactly the timing fields its interval needs: an interval determines which of
// time_of_day, day_of_week, and day_of_month are required and which are not
// allowed. It mirrors the API's per-interval schedule contract so invalid
// combinations fail during planning instead of after an API request.
//
// Field value formats (the time_of_day pattern, the day_of_week enum, the
// day_of_month bounds) are enforced by the attribute-level validators; this
// validator only governs which fields may and must be present for an interval.
// A null optional field is treated as omitted, so it can satisfy a "not allowed"
// rule but not a "required" rule.
type snapshotScheduleShapeValidator struct{}

func (v snapshotScheduleShapeValidator) Description(ctx context.Context) string {
	return "snapshot schedule fields must match the interval: hourly takes no timing fields; " +
		"daily requires time_of_day; weekly requires time_of_day and day_of_week; " +
		"monthly requires time_of_day and day_of_month"
}

func (v snapshotScheduleShapeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v snapshotScheduleShapeValidator) ValidateObject(
	ctx context.Context,
	request validator.ObjectRequest,
	response *validator.ObjectResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	var schedule FileStorageSnapshotScheduleModel
	if diagnostics := request.ConfigValue.As(ctx, &schedule, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	// The shape depends entirely on the interval; without a known interval the
	// per-interval rules cannot be applied. The interval enum validator reports
	// a bad value separately.
	if schedule.Interval.IsNull() || schedule.Interval.IsUnknown() {
		return
	}

	interval := schedule.Interval.ValueString()
	switch interval {
	case string(regionapi.StorageSnapshotScheduleIntervalV2Hourly):
		v.notAllowed(request.Path, "hourly", "time_of_day", schedule.TimeOfDay, response)
		v.notAllowed(request.Path, "hourly", "day_of_week", schedule.DayOfWeek, response)
		v.notAllowed(request.Path, "hourly", "day_of_month", schedule.DayOfMonth, response)
	case string(regionapi.StorageSnapshotScheduleIntervalV2Daily):
		v.required(request.Path, "daily", "time_of_day", schedule.TimeOfDay, response)
		v.notAllowed(request.Path, "daily", "day_of_week", schedule.DayOfWeek, response)
		v.notAllowed(request.Path, "daily", "day_of_month", schedule.DayOfMonth, response)
	case string(regionapi.StorageSnapshotScheduleIntervalV2Weekly):
		v.required(request.Path, "weekly", "time_of_day", schedule.TimeOfDay, response)
		v.required(request.Path, "weekly", "day_of_week", schedule.DayOfWeek, response)
		v.notAllowed(request.Path, "weekly", "day_of_month", schedule.DayOfMonth, response)
	case string(regionapi.StorageSnapshotScheduleIntervalV2Monthly):
		v.required(request.Path, "monthly", "time_of_day", schedule.TimeOfDay, response)
		v.required(request.Path, "monthly", "day_of_month", schedule.DayOfMonth, response)
		v.notAllowed(request.Path, "monthly", "day_of_week", schedule.DayOfWeek, response)
	default:
		response.Diagnostics.AddAttributeError(
			request.Path.AtName("interval"),
			"Invalid Snapshot Schedule",
			fmt.Sprintf("Snapshot schedule interval %q is not supported by the provider.", interval),
		)
	}
}

// required reports an error when an interval needs field but it is omitted
// (null). An unknown value may still resolve to a concrete value, so it is not
// treated as missing. types.String and types.Int64 both satisfy attr.Value, so
// one helper serves every timing field.
func (v snapshotScheduleShapeValidator) required(
	schedulePath path.Path,
	interval, field string,
	value attr.Value,
	response *validator.ObjectResponse,
) {
	if value.IsNull() {
		response.Diagnostics.AddAttributeError(
			schedulePath.AtName(field),
			"Invalid Snapshot Schedule",
			fmt.Sprintf("A %s snapshot schedule requires %s.", interval, field),
		)
	}
}

// notAllowed reports an error when an interval forbids field but it is set to a
// known, non-null value. A null value is treated as omitted and an unknown value
// is left for re-validation once known.
func (v snapshotScheduleShapeValidator) notAllowed(
	schedulePath path.Path,
	interval, field string,
	value attr.Value,
	response *validator.ObjectResponse,
) {
	if !value.IsNull() && !value.IsUnknown() {
		response.Diagnostics.AddAttributeError(
			schedulePath.AtName(field),
			"Invalid Snapshot Schedule",
			fmt.Sprintf("A %s snapshot schedule does not allow %s.", interval, field),
		)
	}
}
