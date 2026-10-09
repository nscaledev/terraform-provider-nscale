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
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/google/uuid"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/pointer"
)

// placementFailureGrace is how long a failing read must persist before an
// update wait gives up. Under RollingUpdate the service reports error when
// Region rejects a rebuild, transport errors and 5xx included, and retries the
// rebuild shortly after; a placement already in error before the update keeps
// reading error until the service observes the new spec; and the read itself
// can meet a passing 5xx. None should fail the apply.
//
//nolint:gochecknoglobals // tests wind it down, as nkswait does.
var placementFailureGrace = 5 * time.Minute

const (
	placementUpdatePending = "updating"
	placementUpdateDone    = "updated"
)

var (
	errPlacementUpdateErrored         = errors.New("placement entered an error state")
	errPlacementUpdateStalled         = errors.New("rolling update cannot complete")
	errPlacementUpdateStrategyUnknown = errors.New("unknown update strategy")
)

const placementUpdateAttempts = 3

func placementUpdateAndWait(
	ctx context.Context,
	client *nscale.Client,
	id string,
	plan PlacementResourceModel,
	timeout time.Duration,
) (*reservationapi.PlacementV2Read, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	var (
		current *reservationapi.PlacementV2Read
		sent    bool
		err     error
	)

	for range placementUpdateAttempts {
		current, sent, err = updatePlacement(ctx, client, id, plan)
		if apiError, ok := nscale.AsAPIError(err); !ok || apiError.StatusCode != http.StatusConflict {
			break
		}
	}

	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Update Placement",
			fmt.Sprintf("An error occurred while updating the placement: %s", err),
		)

		return nil, diagnostics
	}

	if !sent {
		return current, diagnostics
	}

	return waitForPlacementUpdate(
		ctx,
		timeout,
		current,
		func(ctx context.Context) (*reservationapi.PlacementV2Read, error) {
			return getPlacement(ctx, id, client)
		},
	)
}

func updatePlacement(
	ctx context.Context,
	client *nscale.Client,
	id string,
	plan PlacementResourceModel,
) (*reservationapi.PlacementV2Read, bool, error) {
	current, err := getPlacement(ctx, id, client)
	if err != nil {
		return nil, false, fmt.Errorf("reading the placement to update: %w", err)
	}

	params, diagnostics := plan.NscalePlacementUpdateParams(ctx, current)
	if diagnostics.HasError() {
		return nil, false, fmt.Errorf("building the update: %w", nscale.DiagnosticsError(diagnostics))
	}

	if placementSpecUnchanged(current.Spec, params.Spec) {
		return current, false, nil
	}

	updateResponse, err := client.Reservation.UpdatePlacement(ctx, id, params)
	if err != nil {
		return nil, false, fmt.Errorf("sending the update: %w", err)
	}
	defer updateResponse.Body.Close()

	updated, err := nscale.ReadJSONResponsePointer[reservationapi.PlacementV2Read](updateResponse)
	if err != nil {
		return nil, false, fmt.Errorf("sending the update: %w", err)
	}

	return updated, true, nil
}

func placementSpecUnchanged(current, requested reservationapi.PlacementV2Spec) bool {
	currentImage, currentErr := uuid.Parse(current.ServerSpec.ImageId)
	requestedImage, requestedErr := uuid.Parse(requested.ServerSpec.ImageId)

	if currentErr == nil && requestedErr == nil && currentImage == requestedImage {
		requested.ServerSpec.ImageId = current.ServerSpec.ImageId
	}

	return reflect.DeepEqual(current, requested)
}

// A 5xx read reports the last good read as pending, never nil: the watcher
// gives up on a run of nil results inside the grace.
func waitForPlacementUpdate(
	ctx context.Context,
	timeout time.Duration,
	sent *reservationapi.PlacementV2Read,
	get func(ctx context.Context) (*reservationapi.PlacementV2Read, error),
) (*reservationapi.PlacementV2Read, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	grace := nscale.ErrorGrace{Period: placementFailureGrace}

	last := sent

	stateWatcher := retry.StateChangeConf{
		Timeout: timeout,
		Pending: []string{placementUpdatePending},
		Target:  []string{placementUpdateDone},
		Refresh: func() (any, string, error) {
			placement, err := get(ctx)
			if err != nil {
				if !nscale.IsServerError(err) || !grace.Tolerate(true) {
					return nil, "", err
				}

				return last, placementUpdatePending, nil
			}

			last = placement

			done, failure := placementUpdateProgress(placement)
			if grace.Tolerate(failure != nil) {
				return placement, placementUpdatePending, nil
			}

			if failure != nil {
				return nil, "", failure
			}

			if done {
				return placement, placementUpdateDone, nil
			}

			return placement, placementUpdatePending, nil
		},
	}

	state, err := stateWatcher.WaitForStateContext(ctx)
	if e := (*retry.TimeoutError)(nil); errors.As(err, &e) && e.LastState == placementUpdatePending {
		diagnostics.AddError(
			"Placement Still Updating at Update Timeout",
			fmt.Sprintf(
				"The placement was still converging when the %s update timeout expired. The update has been "+
					"applied and the service carries on rolling it out; updated_host_count and "+
					"drifted_host_count show its progress. To wait longer, set a longer update timeout in "+
					"the resource's timeouts block.",
				timeout,
			),
		)

		return nil, diagnostics
	}

	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Wait for Placement to be Updated",
			fmt.Sprintf("An error occurred while waiting for the placement to be updated: %s", err),
		)

		return nil, diagnostics
	}

	placement, ok := state.(*reservationapi.PlacementV2Read)
	if !ok || placement == nil {
		diagnostics.AddError(
			"Unexpected Resource Type",
			fmt.Sprintf(
				"Expected *reservation.PlacementV2Read, got: %T. Please contact the Nscale team for support.",
				state,
			),
		)

		return nil, diagnostics
	}

	return placement, diagnostics
}

func placementUpdateProgress(placement *reservationapi.PlacementV2Read) (bool, error) {
	status := placement.Status

	if placement.Metadata.ProvisioningStatus == reservationapi.ResourceProvisioningStatusError {
		return false, fmt.Errorf("%w: %s", errPlacementUpdateErrored, provisioningStatusMessage(placement))
	}

	if !status.StatusCurrent {
		return false, nil
	}

	provisioned := placement.Metadata.ProvisioningStatus == reservationapi.ResourceProvisioningStatusProvisioned
	updated := pointer.Dereference(status.UpdatedHostCount)
	drifted := pointer.Dereference(status.DriftedHostCount)

	switch strategy := placementUpdateStrategyType(placement.Spec.UpdateStrategy); strategy {
	case reservationapi.PlacementUpdateStrategyTypeV2Manual:
		return provisioned && updated+drifted == placement.Spec.Count, nil
	case reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate:
		return rollingUpdateProgress(placement, provisioned)
	default:
		return false, fmt.Errorf("%w: %q", errPlacementUpdateStrategyUnknown, strategy)
	}
}

func rollingUpdateProgress(placement *reservationapi.PlacementV2Read, provisioned bool) (bool, error) {
	status := placement.Status

	if stalled := pointer.Dereference(status.StalledCount); stalled > 0 {
		return false, fmt.Errorf(
			"%w: %d server(s) failed provisioning and will not converge until a further image change",
			errPlacementUpdateStalled,
			stalled,
		)
	}

	converged := provisioned &&
		pointer.Dereference(status.UpdatedHostCount) == placement.Spec.Count &&
		pointer.Dereference(status.DriftedHostCount) == 0 &&
		pointer.Dereference(status.InFlightCount) == 0

	return converged, nil
}

func placementUpdateStrategyType(
	strategy *reservationapi.PlacementUpdateStrategyV2,
) reservationapi.PlacementUpdateStrategyTypeV2 {
	if strategy == nil || strategy.Type == "" {
		return reservationapi.PlacementUpdateStrategyTypeV2Manual
	}

	return strategy.Type
}

func provisioningStatusMessage(placement *reservationapi.PlacementV2Read) string {
	if detail := placement.Metadata.ProvisioningStatusDetail; detail != nil && detail.Message != "" {
		return detail.Message
	}

	return "no detail reported"
}
