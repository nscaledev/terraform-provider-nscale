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

// Package nkswait decides when an asynchronous NKS write has finished, and
// blocks until it has.
//
// Every NKS resource shares one settledness rule and one status decision table,
// so they live here once rather than per resource package. The CLI centralises
// the same logic in its statuswait package; a second copy in the provider would
// be the copy that drifts.
//
// This package deliberately does not use internal/nscale's shared
// CreateStateWatcher / UpdateStateWatcher / DeleteStateWatcher. Two reasons,
// both specific to NKS:
//
//  1. The shared watchers key off nscale.ResourceStatus, which is typed on
//     nscale-sdk-go/common's provisioning-status and tag types. NKS generates
//     its own structurally-identical enums, and Go will not bridge them.
//
//  2. The shared watchers have no concept of observedGeneration. NKS projects
//     status asynchronously and independently of the write itself, so a
//     "provisioned" read taken too early describes the PREVIOUS spec. The
//     shared create watcher would return success while writing stale endpoints
//     and versions into state. See IsSettled.
//
// Note that this package does NOT gate on healthStatus, matching both the
// shared watchers and every other resource in this provider. See Classify.
//
// If a future refactor generalises the shared watchers over observedGeneration,
// this package should collapse into that.
package nkswait

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

const (
	// Poll timing. 15s is deliberately slower than the 5s/3s the playbook
	// suggests for the control plane: an NKS build takes tens of minutes, so a
	// tighter interval only adds API load and log noise without finishing
	// sooner.
	pollDelay      = 15 * time.Second
	pollMinTimeout = 15 * time.Second
)

// Waiter states. These are internal to the state machine below and
// intentionally not the API's own enum values: the whole point is that a
// settled "provisioned" and an unsettled one are different states even though
// the API reports the same provisioningStatus for both.
const (
	StateSettling     = "settling"
	StateProvisioning = "provisioning"
	StateReady        = "ready"
	StateFailed       = "failed"
	StateDeleting     = "deleting"
	StateGone         = "gone"
)

// Status is the freshness and health view of one NKS read.
//
// NKS resources have a common read metadata type but a per-resource status
// type, and observedGeneration lives on the latter. Projecting both onto this
// struct is what lets one decision table serve every resource: a caller hands
// over the metadata pointer and the generation its own status carries, and
// nothing here needs to know which resource it is looking at.
type Status struct {
	Metadata           *nks.ProjectScopedResourceReadMetadataV1
	ObservedGeneration *int64
}

// IsSettled reports whether a resource's status has caught up with its spec.
//
// NKS writes are asynchronous AND its status projection lags independently: for
// a window after any write, metadata.provisioningStatus still describes the
// PREVIOUS generation of the spec. Trusting it during that window is how a
// create or update returns "success" while the computed attributes it wrote to
// state (endpoints, kubernetes version, applied release, replica counts) still
// describe the old resource. observedGeneration is the API's own freshness
// marker, and comparing it against metadata.generation is the only reliable way
// to know the status being read corresponds to the spec we just sent.
//
// A nil observedGeneration means no projection has completed yet — treat as
// not settled, never as settled-at-zero.
//
// This is the NKS-native equivalent of the operation-tag round-trip that
// internal/nscale's UpdateStateWatcher uses for the cache-backed region API.
// observedGeneration is strictly better: nothing is written into user-visible
// tags, so there is no strip-on-read step, and it covers create as well as
// update.
func IsSettled(status Status) bool {
	if status.Metadata == nil || status.ObservedGeneration == nil {
		return false
	}

	return *status.ObservedGeneration >= status.Metadata.Generation
}

// Classify maps one read onto a waiter state.
//
// Ordering matters. Settledness is checked FIRST: until status has caught up
// with the spec, every other field describes the previous generation and must
// not be acted on — including an error, which may belong to a spec the user has
// already replaced.
//
// healthStatus is deliberately NOT consulted. NKS defines convergence as
// observedGeneration catching up plus provisioningStatus reaching provisioned,
// and says so explicitly: "Health is an independent signal and does not
// determine convergence… a resource can make provisioning progress while
// reporting healthy, degraded, or unknown health" (nks-core
// docs/api-commentary.yaml). Health is a runtime property of a Kubernetes
// cluster — a single NotReady node makes a perfectly well-provisioned pool
// degraded — so waiting for healthy hangs on resources the API considers done.
// Observed on staging 2026-09-16: a pool sat at provisioned/degraded with "one
// or more node pool workers are not ready" and an earlier version of this
// function polled it to the deadline.
//
// This also matches every other resource in the provider: internal/nscale's
// shared watchers target {provisioned, error} on provisioningStatus alone and
// never read health. Callers expose health_status as a computed attribute so a
// practitioner can see and act on it.
func Classify(status Status) string {
	if !IsSettled(status) {
		return StateSettling
	}

	switch status.Metadata.ProvisioningStatus {
	case nks.ResourceProvisioningStatusError:
		return StateFailed
	case nks.ResourceProvisioningStatusDeprovisioning:
		return StateDeleting
	case nks.ResourceProvisioningStatusPending, nks.ResourceProvisioningStatusProvisioning:
		return StateProvisioning
	case nks.ResourceProvisioningStatusProvisioned:
		return StateReady
	default:
		return StateProvisioning
	}
}

// FailureDetail turns whatever the API told us about a failure into something a
// practitioner can act on, preferring the customer-safe detail messages over
// the coarse enums.
func FailureDetail(status Status) string {
	if status.Metadata == nil {
		return "no status was reported"
	}

	// Quote whichever detail describes what is actually wrong. The provisioning
	// detail wins only when provisioning is itself the problem: a pool can sit
	// at provisioned with the cheerful "node pool is available" while health is
	// degraded with "one or more node pool workers are not ready", and quoting
	// the former in a failed or timed-out wait states the opposite of the truth.
	// Observed against staging on 2026-09-16, where a worker came up but never
	// went Ready.
	if status.Metadata.ProvisioningStatus == nks.ResourceProvisioningStatusProvisioned {
		if detail := status.Metadata.HealthStatusDetail; detail != nil {
			return fmt.Sprintf("%s: %s", detail.Reason, detail.Message)
		}
	}

	if detail := status.Metadata.ProvisioningStatusDetail; detail != nil {
		return fmt.Sprintf("%s: %s", detail.Reason, detail.Message)
	}

	if detail := status.Metadata.HealthStatusDetail; detail != nil {
		return fmt.Sprintf("%s: %s", detail.Reason, detail.Message)
	}

	return fmt.Sprintf(
		"provisioning status %q, health status %q",
		status.Metadata.ProvisioningStatus,
		status.Metadata.HealthStatus,
	)
}

// IsNotFound reports whether err is an API 404. Delete tolerates it (the
// resource is already gone) and Read treats it as "remove from state".
func IsNotFound(err error) bool {
	e, ok := nscale.AsAPIError(err)

	return ok && e.StatusCode == http.StatusNotFound
}

// Target describes the resource being waited on. T is the resource's own read
// type — ClusterV1Read, NodePoolV1Read — so callers get their concrete type
// back without a cast.
type Target[T any] struct {
	// Kind names the resource in error messages, lower case and unqualified:
	// "cluster", "node pool".
	Kind string

	// Get reads the resource. A 404 must come back as an error IsNotFound
	// reports true for, which is what the generated client's wrapper does.
	Get func(ctx context.Context) (*T, error)

	// Inspect projects one read onto its freshness and health inputs.
	Inspect func(*T) Status

	// Timeout bounds the whole wait.
	Timeout time.Duration
}

// Provisioned blocks until the resource's status has caught up with its spec
// AND reports provisioned. Health is not consulted — see Classify. Used by both
// create and update: the settledness rule makes them the same problem.
func Provisioned[T any](ctx context.Context, target Target[T]) (*T, error) {
	stateChange := &retry.StateChangeConf{
		// StateGone is pending, not an error: immediately after create the
		// resource may not yet be readable through the API's cache.
		Pending:    []string{StateSettling, StateProvisioning, StateDeleting, StateGone},
		Target:     []string{StateReady},
		Refresh:    target.refresh(ctx, StateGone),
		Timeout:    target.Timeout,
		Delay:      pollDelay,
		MinTimeout: pollMinTimeout,
	}

	raw, err := stateChange.WaitForStateContext(ctx)

	value, ok := raw.(*T)
	if !ok {
		if err != nil {
			return nil, fmt.Errorf("waiting for %s to be provisioned: %w", target.Kind, err)
		}
		// Unreachable in practice: the refresh function only ever yields a *T or
		// an error. Fail loudly rather than hand back a nil value the caller
		// would dereference.
		return nil, fmt.Errorf(
			"waiting for %s to be provisioned: unexpected refresh result of type %T",
			target.Kind, raw,
		)
	}

	if err == nil {
		return value, nil
	}

	status := target.Inspect(value)

	// StateChangeConf reports an unexpected state as a generic error; replace it
	// with the API's own explanation of what went wrong.
	if Classify(status) == StateFailed {
		return value, fmt.Errorf("%s entered a failed state — %s", target.Kind, FailureDetail(status))
	}

	// Not a failure, so the operation is most likely still running: nothing
	// upstream sets a drain timeout, which means an unsatisfiable
	// PodDisruptionBudget blocks a roll indefinitely and this timeout is the
	// only bound. The work resumes on the next apply, so say so rather than
	// leaving a bare deadline that reads like a broken resource.
	if detail, reported := target.lastReported(value); reported {
		return value, fmt.Errorf(
			"waiting for %s to be provisioned: %w — last reported %s; "+
				"the operation may still be in progress remotely, in which case the next apply resumes it",
			target.Kind, err, detail,
		)
	}

	return value, fmt.Errorf("waiting for %s to be provisioned: %w", target.Kind, err)
}

// Deleted blocks until the resource is gone.
//
// The error handling is deliberately narrower than the shared
// DeleteStateWatcher, which treats provisioningStatus:error as terminal from
// the first poll. A resource that was already sitting in error before the
// destroy must still be allowed to deprovision, so an error only becomes
// terminal once deprovisioning has actually been observed.
func Deleted[T any](ctx context.Context, target Target[T]) error {
	var deprovisioningObserved bool

	stateChange := &retry.StateChangeConf{
		Pending: []string{StateSettling, StateProvisioning, StateDeleting, StateReady},
		Target:  []string{StateGone},
		Refresh: func() (any, string, error) {
			value, err := target.Get(ctx)
			if err != nil {
				if IsNotFound(err) {
					var zero T

					return &zero, StateGone, nil
				}

				return nil, "", err
			}

			status := target.Inspect(value)
			state := Classify(status)

			if state == StateDeleting {
				deprovisioningObserved = true
			}

			// A flip to error after deprovisioning started is a failed delete.
			// Surface it now rather than burning the rest of the timeout waiting
			// for a 404 that is never coming.
			if state == StateFailed && deprovisioningObserved {
				return value, "", fmt.Errorf(
					"%s failed to deprovision — %s",
					target.Kind, FailureDetail(status),
				)
			}

			// An error that predates deprovisioning is not itself terminal; keep
			// polling and let the timeout decide.
			if state == StateFailed {
				return value, StateDeleting, nil
			}

			return value, state, nil
		},
		Timeout:    target.Timeout,
		Delay:      pollDelay,
		MinTimeout: pollMinTimeout,
	}

	if _, err := stateChange.WaitForStateContext(ctx); err != nil {
		return fmt.Errorf("waiting for %s to be deleted: %w", target.Kind, err)
	}

	return nil
}

// refresh is the shared StateRefreshFunc. notFoundState decides how a 404 is
// interpreted: terminal success when deleting, a transient not-yet-visible
// state when creating.
func (t Target[T]) refresh(ctx context.Context, notFoundState string) retry.StateRefreshFunc {
	return func() (any, string, error) {
		value, err := t.Get(ctx)
		if err != nil {
			if IsNotFound(err) {
				var zero T

				return &zero, notFoundState, nil
			}

			return nil, "", err
		}

		return value, Classify(t.Inspect(value)), nil
	}
}

// lastReported describes the final observed status, or reports false when the
// wait never managed a successful read. The zero value the refresh function
// hands back for a 404 has no ID, which is what distinguishes "we read this and
// it said X" from "we never saw it".
func (t Target[T]) lastReported(value *T) (string, bool) {
	status := t.Inspect(value)
	if status.Metadata == nil || status.Metadata.Id == "" {
		return "", false
	}

	return FailureDetail(status), true
}
