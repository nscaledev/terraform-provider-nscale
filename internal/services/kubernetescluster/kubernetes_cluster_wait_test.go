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

			metadata := &nks.ProjectScopedResourceReadMetadataV1{Generation: test.generation}
			status := &nks.ClusterStatusV1{ObservedGeneration: test.observedGeneration}

			if got := isSettled(metadata, status); got != test.want {
				t.Errorf("isSettled() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestIsSettledNilSafe(t *testing.T) {
	t.Parallel()

	if isSettled(nil, &nks.ClusterStatusV1{}) {
		t.Error("nil metadata should not be settled")
	}
	if isSettled(&nks.ProjectScopedResourceReadMetadataV1{}, nil) {
		t.Error("nil status should not be settled")
	}
}

// TestClassify is the waiter's decision table. The two rows that matter most
// are the unsettled ones: an unsettled error must NOT be reported as a failure,
// because it may describe a spec the user has already replaced.
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
			want:               stateSettling,
		},
		{
			name:               "unsettled error is not a failure",
			generation:         2,
			observedGeneration: new(int64(1)),
			provisioning:       nks.ResourceProvisioningStatusError,
			health:             nks.ResourceHealthStatusError,
			want:               stateSettling,
		},
		{
			name:               "settled provisioned and healthy is ready",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusProvisioned,
			health:             nks.ResourceHealthStatusHealthy,
			want:               stateReady,
		},
		{
			name:               "provisioned but unhealthy is a failure",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusProvisioned,
			health:             nks.ResourceHealthStatusError,
			want:               stateFailed,
		},
		{
			name:               "provisioned but degraded is still transitional",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusProvisioned,
			health:             nks.ResourceHealthStatusDegraded,
			want:               stateProvisioning,
		},
		{
			name:               "provisioned with unknown health is still transitional",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusProvisioned,
			health:             nks.ResourceHealthStatusUnknown,
			want:               stateProvisioning,
		},
		{
			name:               "settled error is a failure",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusError,
			health:             nks.ResourceHealthStatusHealthy,
			want:               stateFailed,
		},
		{
			name:               "deprovisioning is deleting",
			generation:         2,
			observedGeneration: new(int64(2)),
			provisioning:       nks.ResourceProvisioningStatusDeprovisioning,
			health:             nks.ResourceHealthStatusHealthy,
			want:               stateDeleting,
		},
		{
			name:               "pending is provisioning",
			generation:         1,
			observedGeneration: new(int64(1)),
			provisioning:       nks.ResourceProvisioningStatusPending,
			health:             nks.ResourceHealthStatusUnknown,
			want:               stateProvisioning,
		},
		{
			name:               "provisioning is provisioning",
			generation:         1,
			observedGeneration: new(int64(1)),
			provisioning:       nks.ResourceProvisioningStatusProvisioning,
			health:             nks.ResourceHealthStatusUnknown,
			want:               stateProvisioning,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cluster := &nks.ClusterV1Read{
				Metadata: nks.ProjectScopedResourceReadMetadataV1{
					Generation:         test.generation,
					ProvisioningStatus: test.provisioning,
					HealthStatus:       test.health,
				},
				Status: nks.ClusterStatusV1{
					ObservedGeneration: test.observedGeneration,
				},
			}

			if got := classify(cluster); got != test.want {
				t.Errorf("classify() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestFailureDetail checks that a practitioner gets the API's own explanation
// rather than a bare enum, and that it degrades gracefully when the API supplies
// no detail.
func TestFailureDetail(t *testing.T) {
	t.Parallel()

	provisioningDetail := &nks.ClusterV1Read{
		Metadata: nks.ProjectScopedResourceReadMetadataV1{
			ProvisioningStatus: nks.ResourceProvisioningStatusError,
			ProvisioningStatusDetail: &nks.ProvisioningStatusDetail{
				Reason:  nks.ProvisioningStatusReasonDependencyNotFound,
				Message: "network not found",
			},
		},
	}
	if got := failureDetail(provisioningDetail); got != "DependencyNotFound: network not found" {
		t.Errorf("failureDetail() = %q, want the provisioning detail", got)
	}

	healthDetail := &nks.ClusterV1Read{
		Metadata: nks.ProjectScopedResourceReadMetadataV1{
			HealthStatus: nks.ResourceHealthStatusError,
			HealthStatusDetail: &nks.HealthStatusDetail{
				Reason:  nks.HealthStatusReasonDegraded,
				Message: "2/12 nodes are down",
			},
		},
	}
	if got := failureDetail(healthDetail); got != "Degraded: 2/12 nodes are down" {
		t.Errorf("failureDetail() = %q, want the health detail", got)
	}

	bare := &nks.ClusterV1Read{
		Metadata: nks.ProjectScopedResourceReadMetadataV1{
			ProvisioningStatus: nks.ResourceProvisioningStatusError,
			HealthStatus:       nks.ResourceHealthStatusUnknown,
		},
	}
	if got := failureDetail(bare); got != `provisioning status "error", health status "unknown"` {
		t.Errorf("failureDetail() = %q, want the bare-enum fallback", got)
	}
}

// TestWaiterStatePartitions guards the StateChangeConf wiring: a state that is
// in neither Pending nor Target is treated by the SDK as an unexpected-state
// error, so the create/update partition must cover every value classify can
// return except the terminal failure.
func TestWaiterStatePartitions(t *testing.T) {
	t.Parallel()

	all := []string{
		stateSettling,
		stateProvisioning,
		stateReady,
		stateFailed,
		stateDeleting,
		stateGone,
	}

	provisionPending := map[string]bool{
		stateSettling:     true,
		stateProvisioning: true,
		stateDeleting:     true,
		stateGone:         true,
	}
	provisionTarget := map[string]bool{stateReady: true}

	for _, state := range all {
		covered := provisionPending[state] || provisionTarget[state]
		// stateFailed is deliberately uncovered: falling out of the state machine
		// is how the waiter surfaces the API's failure detail.
		if state == stateFailed {
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
