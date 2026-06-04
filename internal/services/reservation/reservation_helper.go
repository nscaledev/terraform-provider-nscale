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

func getReservation(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*reservationapi.ReservationV2Read, error) {
	reservationResponse, err := client.Reservation.GetReservation(ctx, id)
	if err != nil {
		return nil, err
	}
	defer reservationResponse.Body.Close()

	reservation, err := nscale.ReadJSONResponsePointer[reservationapi.ReservationV2Read](reservationResponse)
	if err != nil {
		return nil, err
	}

	return reservation, nil
}

// getReservationStatus reads a reservation and adapts it to the shared watchers'
// (resource, ResourceStatus, error) shape. Reservation reads are project-scoped.
func getReservationStatus(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*reservationapi.ReservationV2Read, nscale.ResourceStatus, error) {
	reservation, err := getReservation(ctx, id, client)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	return reservation, nscale.StatusFromProjectScoped(&reservation.Metadata), nil
}
