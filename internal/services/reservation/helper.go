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
	"context"

	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// listOrganizationReservationUnits reads the reservation units offered to the
// configured organization, narrowed server-side to one (region, accelerator,
// unit).
//
// Deliberately the organization-scoped endpoint, not the global
// `/api/v2/reservation-units`: capacity is calculated within the organization's
// active allocation scope, so the global endpoint both overstates capacity in
// regions the organization holds and lists regions it holds nothing in.
func listOrganizationReservationUnits(
	ctx context.Context,
	client *nscale.Client,
	regionID, accelerator, unit string,
) (reservationapi.ReservationUnitsV2, error) {
	params := &reservationapi.ListOrganizationReservationUnitsParams{
		RegionID:    &reservationapi.RegionIDQueryParameter{regionID},
		Accelerator: &reservationapi.AcceleratorQueryParameter{accelerator},
		Unit:        &reservationapi.ReservationUnitQueryParameter{unit},
	}

	unitsResponse, err := client.Reservation.ListOrganizationReservationUnits(
		ctx,
		client.OrganizationID,
		params,
	)
	if err != nil {
		return nil, err
	}
	defer unitsResponse.Body.Close()

	return nscale.ReadJSONResponseValue[reservationapi.ReservationUnitsV2](unitsResponse)
}

// statusOf projects a read's metadata onto the ResourceStatus the shared
// watchers consume.
//
// Reservations and placements are both project-scoped and carry the same
// metadata type, so the field mapping is written once here rather than repeated
// in each resource's get helper.
func statusOf(metadata *reservationapi.ProjectScopedResourceReadMetadata) nscale.ResourceStatus {
	return nscale.NewResourceStatus(
		metadata.Id,
		metadata.Name,
		metadata.ProvisioningStatus,
		metadata.Tags,
	)
}
