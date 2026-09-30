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

package nkswait

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// notFoundError builds the 404 the generated client's wrapper returns, which is
// what IsNotFound keys off.
func notFoundError() error {
	return &nscale.APIError{StatusCode: http.StatusNotFound}
}

// TestIsSettled covers the observedGeneration comparison that gates every other
// status read. Getting this wrong is the difference between an apply that
// reports success with the previous generation's endpoints in state and one
// that waits properly.
func TestIsSettled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		generation         int64
		observedGeneration *int64
		want               bool
	}{
		{
			name:               "no projection yet",
			generation:         1,
			observedGeneration: nil,
			want:               false,
		},
		{
			name:               "status describes an older spec",
			generation:         5,
			observedGeneration: new(int64(4)),
			want:               false,
		},
		{
			name:               "status has caught up",
			generation:         5,
			observedGeneration: new(int64(5)),
			want:               true,
		},
		{
			// Can happen transiently if the controller races ahead of a
			// generation bump we have not read yet; treat as settled.
			name:               "status runs ahead of spec",
			generation:         5,
			observedGeneration: new(int64(6)),
			want:               true,
		},
		{
			// A zero observedGeneration is a real projection, not an absent one.
			// The nil check above is what distinguishes them.
			name:               "zero generation observed",
			generation:         0,
			observedGeneration: new(int64(0)),
			want:               true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			status := Status{
				Metadata:           &kubernetesapi.ProjectScopedResourceReadMetadataV1{Generation: test.generation},
				ObservedGeneration: test.observedGeneration,
			}

			if got := IsSettled(status); got != test.want {
				t.Errorf("IsSettled() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestIsSettledNilSafe(t *testing.T) {
	t.Parallel()

	if IsSettled(Status{ObservedGeneration: new(int64(1))}) {
		t.Error("nil metadata should not be settled")
	}
	if IsSettled(Status{Metadata: &kubernetesapi.ProjectScopedResourceReadMetadataV1{}}) {
		t.Error("absent observedGeneration should not be settled")
	}
}

// TestClassify is the waiter's decision table, shared by every NKS resource.
// The two rows that matter most are the unsettled ones: an unsettled error must
// NOT be reported as a failure, because it may describe a spec the user has
// already replaced.
func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		generation         int64
		observedGeneration *int64
		provisioning       kubernetesapi.ResourceProvisioningStatus
		health             kubernetesapi.ResourceHealthStatus
		want               string
	}{
		{
			name:               "unsettled provisioned is not ready",
			generation:         2,
			observedGeneration: new(int64(1)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusProvisioned,
			health:             kubernetesapi.ResourceHealthStatusHealthy,
			want:               StateSettling,
		},
		{
			name:               "unsettled error is not a failure",
			generation:         2,
			observedGeneration: new(int64(1)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusError,
			health:             kubernetesapi.ResourceHealthStatusError,
			want:               StateSettling,
		},
		{
			name:               "settled provisioned and healthy is ready",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusProvisioned,
			health:             kubernetesapi.ResourceHealthStatusHealthy,
			want:               StateReady,
		},
		{
			// Health does not determine convergence, so every health value on a
			// settled provisioned resource is ready. Degraded is the row that
			// matters: a single NotReady worker degrades a pool the API considers
			// done, and an earlier version of Classify polled it to the deadline.
			name:               "provisioned and degraded is ready",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusProvisioned,
			health:             kubernetesapi.ResourceHealthStatusDegraded,
			want:               StateReady,
		},
		{
			name:               "provisioned with unknown health is ready",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusProvisioned,
			health:             kubernetesapi.ResourceHealthStatusUnknown,
			want:               StateReady,
		},
		{
			name:               "provisioned with error health is still ready",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusProvisioned,
			health:             kubernetesapi.ResourceHealthStatusError,
			want:               StateReady,
		},
		{
			name:               "settled error is a failure",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusError,
			health:             kubernetesapi.ResourceHealthStatusHealthy,
			want:               StateFailed,
		},
		{
			name:               "deprovisioning is deleting",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusDeprovisioning,
			health:             kubernetesapi.ResourceHealthStatusHealthy,
			want:               StateDeleting,
		},
		{
			name:               "pending is provisioning",
			generation:         1,
			observedGeneration: new(int64(1)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusPending,
			health:             kubernetesapi.ResourceHealthStatusUnknown,
			want:               StateProvisioning,
		},
		{
			name:               "provisioning is provisioning",
			generation:         1,
			observedGeneration: new(int64(1)),
			provisioning:       kubernetesapi.ResourceProvisioningStatusProvisioning,
			health:             kubernetesapi.ResourceHealthStatusUnknown,
			want:               StateProvisioning,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			status := Status{
				Metadata: &kubernetesapi.ProjectScopedResourceReadMetadataV1{
					Generation:         test.generation,
					ProvisioningStatus: test.provisioning,
					HealthStatus:       test.health,
				},
				ObservedGeneration: test.observedGeneration,
			}

			if got := Classify(status); got != test.want {
				t.Errorf("Classify() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestFailureDetail checks that a practitioner gets the API's own explanation
// rather than a bare enum, and that it degrades gracefully when the API supplies
// no detail.
func TestFailureDetail(t *testing.T) {
	t.Parallel()

	provisioningDetail := Status{
		Metadata: &kubernetesapi.ProjectScopedResourceReadMetadataV1{
			ProvisioningStatus: kubernetesapi.ResourceProvisioningStatusError,
			ProvisioningStatusDetail: &kubernetesapi.ProvisioningStatusDetail{
				Reason:  kubernetesapi.ProvisioningStatusReasonDependencyNotFound,
				Message: "network not found",
			},
		},
	}
	if got := FailureDetail(provisioningDetail); got != "DependencyNotFound: network not found" {
		t.Errorf("FailureDetail() = %q, want the provisioning detail", got)
	}

	healthDetail := Status{
		Metadata: &kubernetesapi.ProjectScopedResourceReadMetadataV1{
			HealthStatus: kubernetesapi.ResourceHealthStatusError,
			HealthStatusDetail: &kubernetesapi.HealthStatusDetail{
				Reason:  kubernetesapi.HealthStatusReasonDegraded,
				Message: "2/12 nodes are down",
			},
		},
	}
	if got := FailureDetail(healthDetail); got != "Degraded: 2/12 nodes are down" {
		t.Errorf("FailureDetail() = %q, want the health detail", got)
	}

	// The regression that motivated the ordering rule, taken verbatim from a
	// staging pool on 2026-09-16: provisioning had finished and says so, while
	// health is the thing that is wrong. Quoting the provisioning detail here
	// tells the user the pool is fine in the middle of a failed wait.
	provisionedButDegraded := Status{
		Metadata: &kubernetesapi.ProjectScopedResourceReadMetadataV1{
			ProvisioningStatus: kubernetesapi.ResourceProvisioningStatusProvisioned,
			HealthStatus:       kubernetesapi.ResourceHealthStatusDegraded,
			ProvisioningStatusDetail: &kubernetesapi.ProvisioningStatusDetail{
				Reason:  kubernetesapi.ProvisioningStatusReasonProvisioned,
				Message: "node pool is available",
			},
			HealthStatusDetail: &kubernetesapi.HealthStatusDetail{
				Reason:  kubernetesapi.HealthStatusReasonDegraded,
				Message: "one or more node pool workers are not ready; inspect Kubernetes node conditions",
			},
		},
	}
	want := "Degraded: one or more node pool workers are not ready; inspect Kubernetes node conditions"
	if got := FailureDetail(provisionedButDegraded); got != want {
		t.Errorf("FailureDetail() = %q, want the health detail when provisioning has finished", got)
	}

	bare := Status{
		Metadata: &kubernetesapi.ProjectScopedResourceReadMetadataV1{
			ProvisioningStatus: kubernetesapi.ResourceProvisioningStatusError,
			HealthStatus:       kubernetesapi.ResourceHealthStatusUnknown,
		},
	}
	if got := FailureDetail(bare); got != `provisioning status "error", health status "unknown"` {
		t.Errorf("FailureDetail() = %q, want the bare-enum fallback", got)
	}

	if got := FailureDetail(Status{}); got != "no status was reported" {
		t.Errorf("FailureDetail() = %q, want the no-status fallback", got)
	}
}

// TestWaiterStatePartitions guards the StateChangeConf wiring. It asserts
// against the very slices the waiters pass in, not a copy of them, so dropping
// a state from provisionedPending fails here rather than silently turning a
// transient state into an unexpected-state error at runtime.
func TestWaiterStatePartitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pending []string
		target  []string
		// uncovered is the one state deliberately left out, so the waiter falls
		// out of the machine and can replace the SDK's generic error with the
		// API's own failure detail.
		uncovered string
	}{
		{"provisioning", provisionedPending(), provisionedTarget(), StateFailed},
		{"deleting", deletedPending(), deletedTarget(), StateFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertPartitionCovers(t, test.pending, test.target, test.uncovered)
		})
	}
}

// assertPartitionCovers checks that Pending and Target between them account for
// every waiter state except the one named.
func assertPartitionCovers(t *testing.T, pending, target []string, uncovered string) {
	t.Helper()

	covered := make(map[string]bool, len(pending)+len(target))
	for _, state := range pending {
		covered[state] = true
	}
	for _, state := range target {
		covered[state] = true
	}

	all := []string{
		StateSettling,
		StateProvisioning,
		StateReady,
		StateFailed,
		StateDeleting,
		StateGone,
	}

	for _, state := range all {
		want := state != uncovered
		if covered[state] != want {
			t.Errorf("state %q covered = %v, want %v", state, covered[state], want)
		}
	}
}

// TestDeletedErrorBeforeDeprovisioning is the rule that makes Deleted differ
// from the shared DeleteStateWatcher: a resource already sitting in error when
// the destroy starts must still be allowed to deprovision, so an error is only
// terminal once deprovisioning has actually been observed. Both resources share
// this function, and nothing covered it.
func TestDeletedErrorBeforeDeprovisioning(t *testing.T) {
	restoreDelay, restoreMinTimeout := pollDelay, pollMinTimeout
	pollDelay, pollMinTimeout = time.Millisecond, time.Millisecond
	t.Cleanup(func() { pollDelay, pollMinTimeout = restoreDelay, restoreMinTimeout })

	generation := int64(1)

	read := func(status kubernetesapi.ResourceProvisioningStatus) *kubernetesapi.NodePoolV1Read {
		return &kubernetesapi.NodePoolV1Read{
			Metadata: kubernetesapi.ProjectScopedResourceReadMetadataV1{
				Id:                 "pool-1",
				Generation:         generation,
				ProvisioningStatus: status,
			},
			Status: kubernetesapi.NodePoolStatusV1{ObservedGeneration: &generation},
		}
	}

	inspect := func(pool *kubernetesapi.NodePoolV1Read) Status {
		return Status{Metadata: &pool.Metadata, ObservedGeneration: pool.Status.ObservedGeneration}
	}

	t.Run("error before deprovisioning is not terminal", func(t *testing.T) {
		// Error on every poll, deprovisioning never seen. The wait must run to
		// its own deadline rather than failing fast.
		err := Deleted(t.Context(), Target[kubernetesapi.NodePoolV1Read]{
			Kind: "node pool",
			Get: func(_ context.Context) (*kubernetesapi.NodePoolV1Read, error) {
				return read(kubernetesapi.ResourceProvisioningStatusError), nil
			},
			Inspect: inspect,
			Timeout: 50 * time.Millisecond,
		})
		if err == nil {
			t.Fatal("Deleted() should have timed out")
		}
		if strings.Contains(err.Error(), "failed to deprovision") {
			t.Errorf("an error predating deprovisioning must not be terminal, got %q", err)
		}
	})

	t.Run("error after deprovisioning is terminal", func(t *testing.T) {
		// Deprovisioning first, then error: that is a failed delete, and it must
		// surface immediately rather than burning the timeout.
		var calls int
		start := time.Now()
		err := Deleted(t.Context(), Target[kubernetesapi.NodePoolV1Read]{
			Kind: "node pool",
			Get: func(_ context.Context) (*kubernetesapi.NodePoolV1Read, error) {
				calls++
				if calls == 1 {
					return read(kubernetesapi.ResourceProvisioningStatusDeprovisioning), nil
				}

				return read(kubernetesapi.ResourceProvisioningStatusError), nil
			},
			Inspect: inspect,
			Timeout: time.Minute,
		})
		if err == nil {
			t.Fatal("Deleted() should have failed")
		}
		if !strings.Contains(err.Error(), "failed to deprovision") {
			t.Errorf("want a failed-deprovision error, got %q", err)
		}
		if elapsed := time.Since(start); elapsed > 10*time.Second {
			t.Errorf("should fail fast, took %s", elapsed)
		}
	})

	t.Run("a 404 is success", func(t *testing.T) {
		err := Deleted(t.Context(), Target[kubernetesapi.NodePoolV1Read]{
			Kind: "node pool",
			Get: func(_ context.Context) (*kubernetesapi.NodePoolV1Read, error) {
				return nil, notFoundError()
			},
			Inspect: inspect,
			Timeout: time.Minute,
		})
		if err != nil {
			t.Errorf("Deleted() on a 404 = %v, want nil", err)
		}
	})
}

// TestLastReported pins the guard that keeps a never-read resource from being
// described as though we had seen its status. The zero value the refresh
// function yields for a 404 must not produce a confident-sounding
// "last reported provisioning status ..." clause in the timeout error.
func TestLastReported(t *testing.T) {
	t.Parallel()

	target := Target[kubernetesapi.NodePoolV1Read]{
		Inspect: func(pool *kubernetesapi.NodePoolV1Read) Status {
			return Status{Metadata: &pool.Metadata, ObservedGeneration: pool.Status.ObservedGeneration}
		},
	}

	if _, reported := target.lastReported(&kubernetesapi.NodePoolV1Read{}); reported {
		t.Error("a never-read resource should not report a last-observed status")
	}

	read := &kubernetesapi.NodePoolV1Read{
		Metadata: kubernetesapi.ProjectScopedResourceReadMetadataV1{
			Id:                 "pool-1",
			ProvisioningStatus: kubernetesapi.ResourceProvisioningStatusProvisioning,
			HealthStatus:       kubernetesapi.ResourceHealthStatusUnknown,
		},
	}

	detail, reported := target.lastReported(read)
	if !reported {
		t.Fatal("a successful read should report its last-observed status")
	}
	if detail != `provisioning status "provisioning", health status "unknown"` {
		t.Errorf("lastReported() = %q, want the bare-enum fallback", detail)
	}
}

// TestProvisionedTimeoutQuotesLastRead is the regression guard for the timeout
// path. WaitForStateContext throws its last result away on timeout and returns
// a bare (nil, err), so Provisioned has to remember the read itself — without
// that, the PodDisruptionBudget case this wait exists to explain degrades to an
// unhelpful "timeout while waiting for state to become 'ready'".
// Not parallel: it winds the package's poll timing down so the wait finishes in
// milliseconds rather than the 15s that gates the first real poll.
func TestProvisionedTimeoutQuotesLastRead(t *testing.T) {
	restoreDelay, restoreMinTimeout := pollDelay, pollMinTimeout
	pollDelay, pollMinTimeout = time.Millisecond, time.Millisecond
	t.Cleanup(func() { pollDelay, pollMinTimeout = restoreDelay, restoreMinTimeout })

	generation := int64(1)

	// Settled, but stuck short of provisioned — a pool mid-roll behind a drain
	// that never completes.
	target := Target[kubernetesapi.NodePoolV1Read]{
		Kind: "node pool",
		Get: func(_ context.Context) (*kubernetesapi.NodePoolV1Read, error) {
			return &kubernetesapi.NodePoolV1Read{
				Metadata: kubernetesapi.ProjectScopedResourceReadMetadataV1{
					Id:                 "pool-1",
					Generation:         generation,
					ProvisioningStatus: kubernetesapi.ResourceProvisioningStatusProvisioning,
					HealthStatus:       kubernetesapi.ResourceHealthStatusDegraded,
				},
				Status: kubernetesapi.NodePoolStatusV1{ObservedGeneration: &generation},
			}, nil
		},
		Inspect: func(pool *kubernetesapi.NodePoolV1Read) Status {
			return Status{Metadata: &pool.Metadata, ObservedGeneration: pool.Status.ObservedGeneration}
		},
		Timeout: 50 * time.Millisecond,
	}

	pool, err := Provisioned(t.Context(), target)
	if err == nil {
		t.Fatal("Provisioned() should have timed out")
	}
	if pool == nil {
		t.Error("Provisioned() should hand back the last read alongside the timeout")
	}

	for _, want := range []string{"last reported", `provisioning status "provisioning"`, "terraform untaint"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Provisioned() error = %q, want it to contain %q", err, want)
		}
	}
}

// clusterSequence returns a Get that walks through statuses, repeating the
// last one once exhausted.
func clusterSequence(
	statuses ...kubernetesapi.ResourceProvisioningStatus,
) func(context.Context) (*kubernetesapi.ClusterV1Read, error) {
	generation := int64(2)
	calls := 0

	return func(_ context.Context) (*kubernetesapi.ClusterV1Read, error) {
		status := statuses[min(calls, len(statuses)-1)]
		calls++

		return &kubernetesapi.ClusterV1Read{
			Metadata: kubernetesapi.ProjectScopedResourceReadMetadataV1{
				Id:                 "cluster-1",
				Generation:         generation,
				ProvisioningStatus: status,
				ProvisioningStatusDetail: &kubernetesapi.ProvisioningStatusDetail{
					Reason:  "Errored",
					Message: "one or more add-ons are degraded",
				},
			},
			Status: kubernetesapi.ClusterStatusV1{ObservedGeneration: &generation},
		}, nil
	}
}

func clusterTarget(
	get func(context.Context) (*kubernetesapi.ClusterV1Read, error),
	timeout time.Duration,
) Target[kubernetesapi.ClusterV1Read] {
	return Target[kubernetesapi.ClusterV1Read]{
		Kind: "cluster",
		Get:  get,
		Inspect: func(cluster *kubernetesapi.ClusterV1Read) Status {
			return Status{Metadata: &cluster.Metadata, ObservedGeneration: cluster.Status.ObservedGeneration}
		},
		Timeout: timeout,
	}
}

func windDownWaiter(t *testing.T, grace time.Duration) {
	t.Helper()

	restoreDelay, restoreMinTimeout, restoreGrace := pollDelay, pollMinTimeout, pollFailureGrace
	pollDelay, pollMinTimeout, pollFailureGrace = time.Millisecond, time.Millisecond, grace
	t.Cleanup(func() { pollDelay, pollMinTimeout, pollFailureGrace = restoreDelay, restoreMinTimeout, restoreGrace })
}

// TestProvisionedRidesOutTransientError: a settled error that clears inside
// pollFailureGrace must not fail the apply. This is the upgrade case, where an
// add-on profile is briefly Degraded while new versions roll out.
func TestProvisionedRidesOutTransientError(t *testing.T) {
	windDownWaiter(t, time.Minute)

	get := clusterSequence(
		kubernetesapi.ResourceProvisioningStatusError,
		kubernetesapi.ResourceProvisioningStatusError,
		kubernetesapi.ResourceProvisioningStatusProvisioning,
		kubernetesapi.ResourceProvisioningStatusProvisioned,
	)

	cluster, err := Provisioned(t.Context(), clusterTarget(get, 5*time.Second))
	if err != nil {
		t.Fatalf("Provisioned() = %v, want success after the error cleared", err)
	}
	if cluster.Metadata.ProvisioningStatus != kubernetesapi.ResourceProvisioningStatusProvisioned {
		t.Errorf("returned status %q, want provisioned", cluster.Metadata.ProvisioningStatus)
	}
}

// TestProvisionedFailsPersistentError: an error that outlasts pollFailureGrace is a
// real failure, reported with the API's own detail rather than as a timeout.
func TestProvisionedFailsPersistentError(t *testing.T) {
	windDownWaiter(t, 20*time.Millisecond)

	get := clusterSequence(kubernetesapi.ResourceProvisioningStatusError)

	_, err := Provisioned(t.Context(), clusterTarget(get, 5*time.Second))
	if err == nil {
		t.Fatal("Provisioned() should fail when the error persists")
	}
	for _, want := range []string{"entered a failed state", "one or more add-ons are degraded"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Provisioned() error = %q, want it to contain %q", err, want)
		}
	}
}

// limitPollServerErrors keeps the SDK's exponential backoff between skipped
// reads well inside a test's timeout.
func limitPollServerErrors(t *testing.T, limit int) {
	t.Helper()

	restore := pollMaxServerErrors
	pollMaxServerErrors = limit
	t.Cleanup(func() { pollMaxServerErrors = restore })
}

func serverError() error {
	return &nscale.APIError{StatusCode: http.StatusServiceUnavailable, Message: "unavailable"}
}

// TestProvisionedRidesOutServerErrors: a short run of 5xx reads mid-wait must
// not fail the apply, or a healthy resource ends up tainted and replaced.
func TestProvisionedRidesOutServerErrors(t *testing.T) {
	windDownWaiter(t, time.Minute)
	limitPollServerErrors(t, 2)

	ready := clusterSequence(kubernetesapi.ResourceProvisioningStatusProvisioned)
	calls := 0
	get := func(ctx context.Context) (*kubernetesapi.ClusterV1Read, error) {
		calls++
		if calls <= pollMaxServerErrors {
			return nil, serverError()
		}

		return ready(ctx)
	}

	if _, err := Provisioned(t.Context(), clusterTarget(get, time.Minute)); err != nil {
		t.Fatalf("Provisioned() = %v, want success once the 5xx cleared", err)
	}
}

// TestProvisionedFailsPersistentServerErrors: past pollMaxServerErrors in a
// row, the 5xx is surfaced rather than waited out to the timeout.
func TestProvisionedFailsPersistentServerErrors(t *testing.T) {
	windDownWaiter(t, time.Minute)
	limitPollServerErrors(t, 1)

	get := func(_ context.Context) (*kubernetesapi.ClusterV1Read, error) {
		return nil, serverError()
	}

	_, err := Provisioned(t.Context(), clusterTarget(get, 5*time.Second))
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Provisioned() = %v, want the 5xx surfaced", err)
	}
}
