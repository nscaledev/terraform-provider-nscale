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

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"

	"github.com/nscaledev/terraform-provider-nscale/internal/nkswait"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// The settledness rule, the health decision table and the poll timing all live
// in internal/nkswait, shared with the cluster resource. Only the two
// pool-specific things are here: how to read a pool, and how long to wait for
// one.

const (
	// Default timeouts. Create is one worker joining an existing cluster, so it
	// is quicker than a control plane. Update and delete are not: a roll is
	// serial (maxSurge 1 / maxUnavailable 0), so both cost roughly replicas x
	// per-node. Measured on staging 2026-09-17, a roll is ~1m45s per replica, so
	// 60m covers a ~30-replica pool — raise timeouts.update rather than lower
	// this.
	//
	// No default is safe against an unsatisfiable PodDisruptionBudget: nothing
	// upstream sets a nodeDrainTimeout, so the block is indefinite and this is
	// the only bound.
	//
	// delete is UNMEASURED and deliberately generous: the two attempts to time
	// it were blocked by an environment where workers came up but never reached
	// Ready, which is the undrainable case 60m exists for. Keep it until a clean
	// delete can be timed.
	defaultCreateTimeout = 30 * time.Minute
	defaultUpdateTimeout = 60 * time.Minute
	defaultDeleteTimeout = 60 * time.Minute
)

// nodePoolTarget describes a node pool to the shared waiter.
func nodePoolTarget(
	client *nscale.Client,
	id string,
	timeout time.Duration,
) nkswait.Target[kubernetesapi.NodePoolV1Read] {
	return nkswait.Target[kubernetesapi.NodePoolV1Read]{
		Kind: "node pool",
		Get: func(ctx context.Context) (*kubernetesapi.NodePoolV1Read, error) {
			return getNodePool(ctx, client, id)
		},
		Inspect: func(pool *kubernetesapi.NodePoolV1Read) nkswait.Status {
			return nkswait.Status{
				Metadata:           &pool.Metadata,
				ObservedGeneration: pool.Status.ObservedGeneration,
			}
		},
		Timeout: timeout,
	}
}

// waitNodePoolProvisioned blocks until the pool's status has caught up with its
// spec and reports provisioned. Health is not consulted — see nkswait.Classify.
// Used by both create and update: the settledness rule makes them the same
// problem.
func waitNodePoolProvisioned(
	ctx context.Context,
	client *nscale.Client,
	id string,
	timeout time.Duration,
) (*kubernetesapi.NodePoolV1Read, error) {
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
