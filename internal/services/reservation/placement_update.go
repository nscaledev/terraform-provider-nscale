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
// can meet a passing 5xx. None should fail the apply. A variable so tests can
// shorten it.
//
//nolint:gochecknoglobals // tests wind it down, as nkswait does.
var placementFailureGrace = 5 * time.Minute

const (
	placementUpdatePending = "updating"
	placementUpdateDone    = "updated"
)

var (
	errPlacementUpdateErrored = errors.New("placement entered an error state")
	errPlacementUpdateStalled = errors.New("rolling update cannot complete")
)

// placementUpdateAttempts bounds the read-modify-writes an update makes when
// the service reports a conflict: another writer changed the placement between
// its read and write, and the service asks the caller to repeat from a fresh
// read.
const placementUpdateAttempts = 3

// placementUpdateAndWait changes a placement's image in place and waits for
// the change to take effect.
//
// The update endpoint ignores metadata, so the shared operation-tag watcher
// would never see the write land; the wait reads the placement's own
// convergence status instead.
//
// The update strategy comes from the plan only when the plan changes it from
// prior; otherwise the one just read is sent back, so a strategy changed
// outside Terraform since the plan is not written back over.
func placementUpdateAndWait(
	ctx context.Context,
	client *nscale.Client,
	id string,
	plan PlacementResourceModel,
	prior PlacementResourceModel,
	timeout time.Duration,
) (*reservationapi.PlacementV2Read, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if plan.UpdateStrategy.Equal(prior.UpdateStrategy) {
		plan.UpdateStrategy = newPlacementUpdateStrategyObject(nil)
	}

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

	// Nothing the API stores changed, as when only timeouts or the spelling of
	// the image UUID did, so there is nothing to wait for.
	if !sent {
		return current, diagnostics
	}

	return waitForPlacementUpdate(ctx, timeout, func(ctx context.Context) (*reservationapi.PlacementV2Read, error) {
		return getPlacement(ctx, id, client)
	})
}

// updatePlacement makes one read-modify-write of the placement's spec. It
// returns the read, and whether it sent an update: it does not when the
// planned spec matches the read.
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

	if _, err = nscale.ReadJSONResponsePointer[reservationapi.PlacementV2Read](updateResponse); err != nil {
		return nil, false, fmt.Errorf("sending the update: %w", err)
	}

	return current, true, nil
}

// placementSpecUnchanged reports whether requested asks for the spec the API
// already holds, treating two spellings of one image UUID as the same image,
// since the API stores it canonicalised.
func placementSpecUnchanged(current, requested reservationapi.PlacementV2Spec) bool {
	currentImage, currentErr := uuid.Parse(current.ServerSpec.ImageId)
	requestedImage, requestedErr := uuid.Parse(requested.ServerSpec.ImageId)

	if currentErr == nil && requestedErr == nil && currentImage == requestedImage {
		requested.ServerSpec.ImageId = current.ServerSpec.ImageId
	}

	return reflect.DeepEqual(current, requested)
}

// waitForPlacementUpdate polls until placementUpdateProgress reports the
// update done, or a failure, reported or a 5xx read, outlasts
// placementFailureGrace.
func waitForPlacementUpdate(
	ctx context.Context,
	timeout time.Duration,
	get func(ctx context.Context) (*reservationapi.PlacementV2Read, error),
) (*reservationapi.PlacementV2Read, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	grace := nscale.ErrorGrace{Period: placementFailureGrace}

	var last *reservationapi.PlacementV2Read

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

				// A nil result reads as not found, which the watcher also
				// tolerates for a while.
				if last == nil {
					return nil, "", nil
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

// placementUpdateProgress reports from one read whether a placement update has
// taken effect, or why it cannot.
//
// An error fails the update under either strategy. Until statusCurrent is
// true the service has not observed the new spec and the convergence counts
// describe the old one, so nothing else is read from them. Once it has,
// neither strategy is done until the placement is provisioned with its counts
// accounting for every server: the service also reports statusCurrent with
// every count zeroed when a reconcile stops before observing the servers.
//
//   - Manual: the update is done once every server is counted as updated or
//     drifted. Servers stay on their image, drifted, until each is reconciled
//     explicitly; that is the strategy, not a failure.
//   - RollingUpdate: the update is done when every server is updated, with
//     none drifted or in flight. A stalled server has failed provisioning; the
//     service never rebuilds a server that is not provisioned, so it holds the
//     rollout until a further image change.
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

	strategy := placementUpdateStrategyType(placement.Spec.UpdateStrategy)
	if strategy != reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate {
		return provisioned && updated+drifted == placement.Spec.Count, nil
	}

	if stalled := pointer.Dereference(status.StalledCount); stalled > 0 {
		return false, fmt.Errorf(
			"%w: %d server(s) failed provisioning and will not converge until a further image change",
			errPlacementUpdateStalled,
			stalled,
		)
	}

	converged := provisioned &&
		updated == placement.Spec.Count &&
		drifted == 0 &&
		pointer.Dereference(status.InFlightCount) == 0

	return converged, nil
}

// placementUpdateStrategyType returns the strategy's type, applying the API's
// default of Manual when it is absent.
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
