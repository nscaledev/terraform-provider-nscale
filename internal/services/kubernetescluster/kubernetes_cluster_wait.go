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

package kubernetescluster

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// This file deliberately does not use internal/nscale's shared
// CreateStateWatcher / UpdateStateWatcher / DeleteStateWatcher. Three reasons,
// all specific to NKS:
//
//  1. The shared watchers key off nscale.ResourceStatus, which is typed on
//     nscale-sdk-go/common's provisioning-status and tag types. NKS generates
//     its own structurally-identical enums, and Go will not bridge them.
//
//  2. The shared watchers have no concept of observedGeneration. NKS projects
//     status asynchronously and independently of the write itself, so a
//     "provisioned" read taken too early describes the PREVIOUS spec. The
//     shared create watcher would return success while writing stale endpoints
//     and versions into state. See isSettled.
//
//  3. The shared watchers only inspect provisioningStatus. For NKS,
//     provisioned + healthStatus:error is a failure, not a success.
//
// If a future refactor generalises the shared watchers over these three axes,
// this file should collapse into that.

const (
	// Poll timing. 15s is deliberately slower than the 5s/3s the playbook
	// suggests for the control plane: a Kubernetes cluster build takes tens of
	// minutes, so a tighter interval only adds API load and log noise without
	// finishing sooner.
	pollDelay      = 15 * time.Second
	pollMinTimeout = 15 * time.Second

	// Default timeouts.
	//
	// These are set from a measurement, not a guess: a cluster created against
	// uni-dev on 2026-08-26 took 32m18s to reach provisioned+healthy. The
	// original 30m create default would have timed out on that very first real
	// apply, stranding a cluster that was minutes from ready. 60m gives roughly
	// 2x observed, which is the margin a control-plane build warrants — it is
	// backed by real infrastructure whose speed varies with region and load.
	//
	// Update is longer still: changing platform_release_id is a rolling
	// control-plane upgrade rather than a configuration write.
	//
	// Raise these rather than lower them. The cost of an over-long default is a
	// slow failure on a cluster that was never coming up; the cost of a short
	// one is a failed apply on a cluster that was fine, which leaves state and
	// reality disagreeing.
	defaultCreateTimeout = 60 * time.Minute
	defaultUpdateTimeout = 90 * time.Minute
	defaultDeleteTimeout = 30 * time.Minute
)

// Waiter states. These are internal to the state machine below and intentionally
// not the API's own enum values: the whole point is that a settled "provisioned"
// and an unsettled one are different states even though the API reports the same
// provisioningStatus for both.
const (
	stateSettling     = "settling"
	stateProvisioning = "provisioning"
	stateReady        = "ready"
	stateFailed       = "failed"
	stateDeleting     = "deleting"
	stateGone         = "gone"
)

// classify maps one cluster read onto a waiter state.
//
// Ordering matters. Settledness is checked FIRST: until status has caught up
// with the spec, every other field describes the previous generation and must
// not be acted on — including an error, which may belong to a spec the user has
// already replaced.
func classify(cluster *nks.ClusterV1Read) string {
	if !isSettled(&cluster.Metadata, &cluster.Status) {
		return stateSettling
	}

	switch cluster.Metadata.ProvisioningStatus {
	case nks.ResourceProvisioningStatusError:
		return stateFailed
	case nks.ResourceProvisioningStatusDeprovisioning:
		return stateDeleting
	case nks.ResourceProvisioningStatusPending, nks.ResourceProvisioningStatusProvisioning:
		return stateProvisioning
	case nks.ResourceProvisioningStatusProvisioned:
		// A provisioned cluster that is unhealthy has not succeeded. Anything
		// short of an outright error (degraded, unknown) is still transitional —
		// addons and node registration settle after the control plane reports
		// provisioned.
		if cluster.Metadata.HealthStatus == nks.ResourceHealthStatusError {
			return stateFailed
		}
		if cluster.Metadata.HealthStatus == nks.ResourceHealthStatusHealthy {
			return stateReady
		}
		return stateProvisioning
	default:
		return stateProvisioning
	}
}

// failureDetail turns whatever the API told us about a failure into something a
// practitioner can act on, preferring the customer-safe detail messages over the
// coarse enums.
func failureDetail(cluster *nks.ClusterV1Read) string {
	if detail := cluster.Metadata.ProvisioningStatusDetail; detail != nil {
		return fmt.Sprintf("%s: %s", detail.Reason, detail.Message)
	}

	if detail := cluster.Metadata.HealthStatusDetail; detail != nil {
		return fmt.Sprintf("%s: %s", detail.Reason, detail.Message)
	}

	return fmt.Sprintf(
		"provisioning status %q, health status %q",
		cluster.Metadata.ProvisioningStatus,
		cluster.Metadata.HealthStatus,
	)
}

// refreshCluster is the shared StateRefreshFunc. notFoundState decides how a 404
// is interpreted: terminal success when deleting, a transient not-yet-visible
// state when creating.
func refreshCluster(
	ctx context.Context,
	client *nscale.Client,
	id string,
	notFoundState string,
) retry.StateRefreshFunc {
	return func() (any, string, error) {
		cluster, err := getCluster(ctx, client, id)
		if err != nil {
			if isNotFound(err) {
				return &nks.ClusterV1Read{}, notFoundState, nil
			}
			return nil, "", err
		}

		return cluster, classify(cluster), nil
	}
}

// waitClusterProvisioned blocks until the cluster's status has caught up with
// its spec AND reports provisioned + healthy. Used by both create and update:
// the settledness rule makes them the same problem.
func waitClusterProvisioned(
	ctx context.Context,
	client *nscale.Client,
	id string,
	timeout time.Duration,
) (*nks.ClusterV1Read, error) {
	stateChange := &retry.StateChangeConf{
		// stateGone is pending, not an error: immediately after create the
		// cluster may not yet be readable through the API's cache.
		Pending:    []string{stateSettling, stateProvisioning, stateDeleting, stateGone},
		Target:     []string{stateReady},
		Refresh:    refreshCluster(ctx, client, id, stateGone),
		Timeout:    timeout,
		Delay:      pollDelay,
		MinTimeout: pollMinTimeout,
	}

	raw, err := stateChange.WaitForStateContext(ctx)

	cluster, ok := raw.(*nks.ClusterV1Read)
	if !ok {
		if err != nil {
			return nil, fmt.Errorf("waiting for cluster to be provisioned: %w", err)
		}
		// Unreachable in practice: the refresh function only ever yields a
		// *ClusterV1Read or an error. Fail loudly rather than hand back a nil
		// cluster the caller would dereference.
		return nil, fmt.Errorf("waiting for cluster to be provisioned: unexpected refresh result of type %T", raw)
	}

	// StateChangeConf reports an unexpected state as a generic error; replace it
	// with the API's own explanation of what went wrong.
	if err != nil && classify(cluster) == stateFailed {
		return cluster, fmt.Errorf("cluster entered a failed state — %s", failureDetail(cluster))
	}

	if err != nil {
		return cluster, fmt.Errorf("waiting for cluster to be provisioned: %w", err)
	}

	return cluster, nil
}

// waitClusterDeleted blocks until the cluster is gone.
//
// The error handling is deliberately narrower than the shared
// DeleteStateWatcher, which treats provisioningStatus:error as terminal from
// the first poll. A cluster that was already sitting in error before the
// destroy must still be allowed to deprovision, so an error only becomes
// terminal once deprovisioning has actually been observed.
func waitClusterDeleted(
	ctx context.Context,
	client *nscale.Client,
	id string,
	timeout time.Duration,
) error {
	var deprovisioningObserved bool

	stateChange := &retry.StateChangeConf{
		Pending: []string{stateSettling, stateProvisioning, stateDeleting, stateReady},
		Target:  []string{stateGone},
		Refresh: func() (any, string, error) {
			cluster, err := getCluster(ctx, client, id)
			if err != nil {
				if isNotFound(err) {
					return &nks.ClusterV1Read{}, stateGone, nil
				}
				return nil, "", err
			}

			state := classify(cluster)

			if state == stateDeleting {
				deprovisioningObserved = true
			}

			// A flip to error after deprovisioning started is a failed delete.
			// Surface it now rather than burning the rest of the timeout waiting
			// for a 404 that is never coming.
			if state == stateFailed && deprovisioningObserved {
				return cluster, "", fmt.Errorf(
					"cluster failed to deprovision — %s",
					failureDetail(cluster),
				)
			}

			// An error that predates deprovisioning is not itself terminal; keep
			// polling and let the timeout decide.
			if state == stateFailed {
				return cluster, stateDeleting, nil
			}

			return cluster, state, nil
		},
		Timeout:    timeout,
		Delay:      pollDelay,
		MinTimeout: pollMinTimeout,
	}

	if _, err := stateChange.WaitForStateContext(ctx); err != nil {
		return fmt.Errorf("waiting for cluster to be deleted: %w", err)
	}

	return nil
}
