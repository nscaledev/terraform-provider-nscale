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

package kubernetesnodepool

import (
	"context"
	"time"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
	"github.com/nscaledev/terraform-provider-nscale/internal/nkswait"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// The settledness rule, the health decision table and the poll timing all live
// in internal/nkswait, shared with the cluster resource. Only the two
// pool-specific things are here: how to read a pool, and how long to wait for
// one.
//
// One consequence of that shared rule is worth knowing. The provider must NOT
// compare replica counts itself, and does not. Upstream, a compute pool
// reaching provisioned already requires the MachineDeployment status to be
// current, none of RollingOut / ScalingUp / ScalingDown / Remediating /
// Deleting to be true, MachinesUpToDate true, and spec.replicas == desired ==
// status.replicas. A count comparison here would duplicate that check, and
// would be the copy that drifts.
//
// Note that ready_replicas can therefore still lag replicas when an apply
// finishes: provisioned means the pool converged, not that every worker passed
// its Kubernetes node checks. That is the same independence nkswait.Classify
// documents for healthStatus.

const (
	// Default timeouts.
	//
	// A single worker joining an existing cluster is faster than a control
	// plane, so create is half the cluster's 60m. Update and delete are not:
	// both walk the pool one node at a time.
	//
	// nks-core renders no rollout strategy on the underlying CAPI
	// MachineDeployment, so upstream defaults apply — RollingUpdate with
	// maxSurge 1 and maxUnavailable 0. That makes a roll serial: one extra node
	// at a time, each drained through the Eviction API before its predecessor
	// goes. Wall-clock is roughly replicas x per-node (provision + join +
	// drain), which is what update has to cover when a taint or label edit rolls
	// every worker. Delete drains every node on the way out and is bounded the
	// same way.
	//
	// Neither default can be made safe against a PodDisruptionBudget that cannot
	// be satisfied: nks-core sets no nodeDrainTimeout and upstream has no
	// default, so the block is indefinite and this timeout is the only bound.
	// That is a documentation problem rather than a tuning one — see the
	// timeouts section of the resource's website docs.
	//
	// Measured against nks-stg.europe-west2 on 2026-09-17, g.4.standard.40s:
	//
	//	create, 2 replicas, concurrent with the cluster ....... 2m01s
	//	scale 2 -> 1 (drains one worker) ...................... 30s
	//	scale 1 -> 0 ......................................... 33s
	//	scale 0 -> 1 ......................................... 16s
	//	taint edit, 1 replica (rolls the pool) ............... 1m44s
	//	label edit, 1 replica (rolls the pool) ............... 1m50s
	//
	// So a roll costs roughly 1m45s per replica on top of the scale cost, which
	// is what makes update the expensive operation. 60m covers a ~30-replica
	// roll; raise timeouts.update beyond that rather than lowering this.
	//
	// DELETE IS STILL UNMEASURED. Two attempts were blocked by an environment
	// where workers came up but never reached Ready, leaving the pool
	// deprovisioning with currentReplicas stuck above zero for more than ten
	// minutes. That is exactly the undrainable-worker case the 60m default
	// exists for, so it stays until a clean delete can be timed.
	defaultCreateTimeout = 30 * time.Minute
	defaultUpdateTimeout = 60 * time.Minute
	defaultDeleteTimeout = 60 * time.Minute
)

// nodePoolTarget describes a node pool to the shared waiter.
func nodePoolTarget(
	client *nscale.Client,
	id string,
	timeout time.Duration,
) nkswait.Target[nks.NodePoolV1Read] {
	return nkswait.Target[nks.NodePoolV1Read]{
		Kind: "node pool",
		Get: func(ctx context.Context) (*nks.NodePoolV1Read, error) {
			return getNodePool(ctx, client, id)
		},
		Inspect: func(pool *nks.NodePoolV1Read) nkswait.Status {
			return nkswait.Status{
				Metadata:           &pool.Metadata,
				ObservedGeneration: pool.Status.ObservedGeneration,
			}
		},
		Timeout: timeout,
	}
}

// waitNodePoolProvisioned blocks until the pool's status has caught up with its
// spec and reports provisioned. Used by both create and update: the settledness
// rule makes them the same problem.
//
// It does not wait for healthy — see nkswait.Classify. health_status is exposed
// as a computed attribute instead, so a degraded pool is visible without
// blocking an apply the API considers complete.
func waitNodePoolProvisioned(
	ctx context.Context,
	client *nscale.Client,
	id string,
	timeout time.Duration,
) (*nks.NodePoolV1Read, error) {
	return nkswait.Provisioned(ctx, nodePoolTarget(client, id, timeout))
}

// waitNodePoolDeleted blocks until the pool is gone.
func waitNodePoolDeleted(
	ctx context.Context,
	client *nscale.Client,
	id string,
	timeout time.Duration,
) error {
	return nkswait.Deleted(ctx, nodePoolTarget(client, id, timeout))
}
