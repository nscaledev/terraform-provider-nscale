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
	"testing"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
)

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
				Metadata:           &nks.ProjectScopedResourceReadMetadataV1{Generation: test.generation},
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
	if IsSettled(Status{Metadata: &nks.ProjectScopedResourceReadMetadataV1{}}) {
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
		provisioning       nks.ResourceProvisioningStatus
		health             nks.ResourceHealthStatus
		want               string
	}{
		{
			name:               "unsettled provisioned is not ready",
			generation:         2,
			observedGeneration: new(int64(1)),
			provisioning:       nks.ResourceProvisioningStatusProvisioned,
			health:             nks.ResourceHealthStatusHealthy,
			want:               StateSettling,
		},
		{
			name:               "unsettled error is not a failure",
			generation:         2,
			observedGeneration: new(int64(1)),
			provisioning:       nks.ResourceProvisioningStatusError,
			health:             nks.ResourceHealthStatusError,
			want:               StateSettling,
		},
		{
			name:               "settled provisioned and healthy is ready",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusProvisioned,
			health:             nks.ResourceHealthStatusHealthy,
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
			provisioning:       nks.ResourceProvisioningStatusProvisioned,
			health:             nks.ResourceHealthStatusDegraded,
			want:               StateReady,
		},
		{
			name:               "provisioned with unknown health is ready",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusProvisioned,
			health:             nks.ResourceHealthStatusUnknown,
			want:               StateReady,
		},
		{
			name:               "provisioned with error health is still ready",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusProvisioned,
			health:             nks.ResourceHealthStatusError,
			want:               StateReady,
		},
		{
			name:               "settled error is a failure",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusError,
			health:             nks.ResourceHealthStatusHealthy,
			want:               StateFailed,
		},
		{
			name:               "deprovisioning is deleting",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusDeprovisioning,
			health:             nks.ResourceHealthStatusHealthy,
			want:               StateDeleting,
		},
		{
			name:               "pending is provisioning",
			generation:         1,
			observedGeneration: new(int64(1)),
			provisioning:       nks.ResourceProvisioningStatusPending,
			health:             nks.ResourceHealthStatusUnknown,
			want:               StateProvisioning,
		},
		{
			name:               "provisioning is provisioning",
			generation:         1,
			observedGeneration: new(int64(1)),
			provisioning:       nks.ResourceProvisioningStatusProvisioning,
			health:             nks.ResourceHealthStatusUnknown,
			want:               StateProvisioning,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			status := Status{
				Metadata: &nks.ProjectScopedResourceReadMetadataV1{
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
		Metadata: &nks.ProjectScopedResourceReadMetadataV1{
			ProvisioningStatus: nks.ResourceProvisioningStatusError,
			ProvisioningStatusDetail: &nks.ProvisioningStatusDetail{
				Reason:  nks.ProvisioningStatusReasonDependencyNotFound,
				Message: "network not found",
			},
		},
	}
	if got := FailureDetail(provisioningDetail); got != "DependencyNotFound: network not found" {
		t.Errorf("FailureDetail() = %q, want the provisioning detail", got)
	}

	healthDetail := Status{
		Metadata: &nks.ProjectScopedResourceReadMetadataV1{
			HealthStatus: nks.ResourceHealthStatusError,
			HealthStatusDetail: &nks.HealthStatusDetail{
				Reason:  nks.HealthStatusReasonDegraded,
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
		Metadata: &nks.ProjectScopedResourceReadMetadataV1{
			ProvisioningStatus: nks.ResourceProvisioningStatusProvisioned,
			HealthStatus:       nks.ResourceHealthStatusDegraded,
			ProvisioningStatusDetail: &nks.ProvisioningStatusDetail{
				Reason:  nks.ProvisioningStatusReasonProvisioned,
				Message: "node pool is available",
			},
			HealthStatusDetail: &nks.HealthStatusDetail{
				Reason:  nks.HealthStatusReasonDegraded,
				Message: "one or more node pool workers are not ready; inspect Kubernetes node conditions",
			},
		},
	}
	want := "Degraded: one or more node pool workers are not ready; inspect Kubernetes node conditions"
	if got := FailureDetail(provisionedButDegraded); got != want {
		t.Errorf("FailureDetail() = %q, want the health detail when provisioning has finished", got)
	}

	bare := Status{
		Metadata: &nks.ProjectScopedResourceReadMetadataV1{
			ProvisioningStatus: nks.ResourceProvisioningStatusError,
			HealthStatus:       nks.ResourceHealthStatusUnknown,
		},
	}
	if got := FailureDetail(bare); got != `provisioning status "error", health status "unknown"` {
		t.Errorf("FailureDetail() = %q, want the bare-enum fallback", got)
	}

	if got := FailureDetail(Status{}); got != "no status was reported" {
		t.Errorf("FailureDetail() = %q, want the no-status fallback", got)
	}
}

// TestWaiterStatePartitions guards the StateChangeConf wiring: a state that is
// in neither Pending nor Target is treated by the SDK as an unexpected-state
// error, so the create/update partition must cover every value Classify can
// return except the terminal failure.
func TestWaiterStatePartitions(t *testing.T) {
	t.Parallel()

	all := []string{
		StateSettling,
		StateProvisioning,
		StateReady,
		StateFailed,
		StateDeleting,
		StateGone,
	}

	provisionPending := map[string]bool{
		StateSettling:     true,
		StateProvisioning: true,
		StateDeleting:     true,
		StateGone:         true,
	}
	provisionTarget := map[string]bool{StateReady: true}

	for _, state := range all {
		covered := provisionPending[state] || provisionTarget[state]
		// StateFailed is deliberately uncovered: falling out of the state machine
		// is how the waiter surfaces the API's failure detail.
		if state == StateFailed {
			if covered {
				t.Errorf("%q should not be in the provisioning partition", state)
			}
			continue
		}
		if !covered {
			t.Errorf("%q is in neither Pending nor Target for provisioning", state)
		}
	}
}

// TestLastReported pins the guard that keeps a never-read resource from being
// described as though we had seen its status. The zero value the refresh
// function yields for a 404 must not produce a confident-sounding
// "last reported provisioning status ..." clause in the timeout error.
func TestLastReported(t *testing.T) {
	t.Parallel()

	target := Target[nks.NodePoolV1Read]{
		Inspect: func(pool *nks.NodePoolV1Read) Status {
			return Status{Metadata: &pool.Metadata, ObservedGeneration: pool.Status.ObservedGeneration}
		},
	}

	if _, reported := target.lastReported(&nks.NodePoolV1Read{}); reported {
		t.Error("a never-read resource should not report a last-observed status")
	}

	read := &nks.NodePoolV1Read{
		Metadata: nks.ProjectScopedResourceReadMetadataV1{
			Id:                 "pool-1",
			ProvisioningStatus: nks.ResourceProvisioningStatusProvisioning,
			HealthStatus:       nks.ResourceHealthStatusUnknown,
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
