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
	"github.com/hashicorp/terraform-plugin-framework/types"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"
)

// ReservationUnitModel describes the capacity offering a reservation is claimed
// from: the unit's fixed topology, and the largest contiguous run of units
// currently reservable.
//
// Deliberately narrower than the API response, which also carries the per-unit
// CPU/GPU/memory totals and the per-host footprint the console renders. Nothing
// in the reservation or placement workflow needs those, and every attribute here
// is public API the provider then has to keep.
type ReservationUnitModel struct {
	ID                         types.String `tfsdk:"id"`
	RegionID                   types.String `tfsdk:"region_id"`
	Accelerator                types.String `tfsdk:"accelerator"`
	Unit                       types.String `tfsdk:"unit"`
	HostsPerUnit               types.Int64  `tfsdk:"hosts_per_unit"`
	MachineFlavorID            types.String `tfsdk:"machine_flavor_id"`
	LargestContiguousUnitCount types.Int64  `tfsdk:"largest_contiguous_unit_count"`
}

func NewReservationUnitModel(source *reservationapi.ReservationUnitV2) ReservationUnitModel {
	return ReservationUnitModel{
		ID:                         types.StringValue(reservationUnitID(source)),
		RegionID:                   types.StringValue(source.RegionId),
		Accelerator:                types.StringValue(source.Accelerator),
		Unit:                       types.StringValue(source.Unit),
		HostsPerUnit:               types.Int64Value(int64(source.HostsPerUnit)),
		MachineFlavorID:            types.StringValue(source.MachineFlavorId),
		LargestContiguousUnitCount: types.Int64Value(int64(source.LargestContiguousUnitCount)),
	}
}

// reservationUnitID synthesises an identifier from the triple that identifies a
// unit.
//
// The API issues no id for a reservation unit — it is a regional offering rather
// than an owned resource — but Terraform's testing framework requires every
// state entry to carry one, and a stable key is useful to users indexing units
// with for_each. The three components are exactly the data source's lookup keys,
// so the id is deterministic and carries no information the caller did not
// already supply.
func reservationUnitID(source *reservationapi.ReservationUnitV2) string {
	return source.RegionId + "/" + source.Accelerator + "/" + source.Unit
}
