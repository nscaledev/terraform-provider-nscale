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

package reservation

import (
	"testing"

	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"
)

// The response carries more than the model exposes; the extra fields are set
// here so the test would catch a mapping that silently picked up the wrong one —
// HostsPerUnit and the per-unit totals are all plausible-looking counts.
func testReservationUnit() *reservationapi.ReservationUnitV2 {
	return &reservationapi.ReservationUnitV2{
		RegionId:                   "region-1",
		Accelerator:                "GB300",
		Unit:                       "NVL72",
		HostsPerUnit:               18,
		MachineFlavorId:            "flavor-1",
		LargestContiguousUnitCount: 3,
		CpusPerUnit:                1296,
		GpusPerUnit:                72,
		MemoryPerUnit:              9216,
		GpuMemoryPerUnit:           13824,
		TotalCount:                 8,
		AvailableUnitCount:         5,
		DeviceTypeResourceClass:    "gb300-nvl72",
	}
}

func TestNewReservationUnitModelMapsUnitFields(t *testing.T) {
	t.Parallel()

	model := NewReservationUnitModel(testReservationUnit())

	stringFields := map[string]struct{ got, want string }{
		// Synthetic: the API issues no id for a unit, so it is composed from the
		// lookup triple. Terraform's test framework requires state entries to
		// carry an id, which an acceptance test caught and unit tests could not.
		"id":                {model.ID.ValueString(), "region-1/GB300/NVL72"},
		"region_id":         {model.RegionID.ValueString(), "region-1"},
		"accelerator":       {model.Accelerator.ValueString(), "GB300"},
		"unit":              {model.Unit.ValueString(), "NVL72"},
		"machine_flavor_id": {model.MachineFlavorID.ValueString(), "flavor-1"},
	}
	for name, field := range stringFields {
		if field.got != field.want {
			t.Errorf("%s = %q, want %q", name, field.got, field.want)
		}
	}

	int64Fields := map[string]struct{ got, want int64 }{
		// hosts_per_unit is what removes the magic number from a placement's
		// host_count, so it must be the unit's own host count and not one of the
		// other counts in the response.
		"hosts_per_unit":                {model.HostsPerUnit.ValueInt64(), 18},
		"largest_contiguous_unit_count": {model.LargestContiguousUnitCount.ValueInt64(), 3},
	}
	for name, field := range int64Fields {
		if field.got != field.want {
			t.Errorf("%s = %d, want %d", name, field.got, field.want)
		}
	}
}
