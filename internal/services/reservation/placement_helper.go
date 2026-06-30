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

func getPlacement(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*reservationapi.PlacementV2Read, error) {
	placementResponse, err := client.Reservation.GetPlacement(ctx, id)
	if err != nil {
		return nil, err
	}
	defer placementResponse.Body.Close()

	placement, err := nscale.ReadJSONResponsePointer[reservationapi.PlacementV2Read](placementResponse)
	if err != nil {
		return nil, err
	}

	return placement, nil
}

// getPlacementStatus reads a placement and adapts it to the shared watchers'
// (resource, ResourceStatus, error) shape. Placement reads are project-scoped.
func getPlacementStatus(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*reservationapi.PlacementV2Read, nscale.ResourceStatus, error) {
	placement, err := getPlacement(ctx, id, client)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	return placement, nscale.StatusFromProjectScoped(&placement.Metadata), nil
}
