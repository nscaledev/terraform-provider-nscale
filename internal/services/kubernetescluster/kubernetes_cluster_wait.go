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
	"time"

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"

	"github.com/nscaledev/terraform-provider-nscale/internal/nkswait"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// The settledness rule, the health decision table and the poll timing all live
// in internal/nkswait, shared with the node pool resource. Only the two
// cluster-specific things are here: how to read a cluster, and how long to wait
// for one.

const (
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
	// Delete was measured at 2m11s, but on a cluster with no node pools. With
	// pools it drains every worker first, and nothing upstream bounds a drain
	// blocked by a PodDisruptionBudget. See the spec's "Why delete goes to 60m".
	//
	// Raise these rather than lower them. The cost of an over-long default is a
	// slow failure on a cluster that was never coming up; the cost of a short
	// one is a failed apply on a cluster that was fine, which leaves state and
	// reality disagreeing.
	defaultCreateTimeout = 60 * time.Minute
	defaultUpdateTimeout = 90 * time.Minute
	defaultDeleteTimeout = 60 * time.Minute
)

// clusterTarget describes a cluster to the shared waiter.
func clusterTarget(
	client *nscale.Client,
	id string,
	timeout time.Duration,
) nkswait.Target[kubernetesapi.ClusterV1Read] {
	return nkswait.Target[kubernetesapi.ClusterV1Read]{
		Kind: "cluster",
		Get: func(ctx context.Context) (*kubernetesapi.ClusterV1Read, error) {
			return getCluster(ctx, client, id)
		},
		Inspect: func(cluster *kubernetesapi.ClusterV1Read) nkswait.Status {
			return nkswait.Status{
				Metadata:           &cluster.Metadata,
				ObservedGeneration: cluster.Status.ObservedGeneration,
			}
		},
		Timeout: timeout,
	}
}

// waitClusterProvisioned blocks until the cluster's status has caught up with
// its spec and reports provisioned. Health is not consulted — see
// nkswait.Classify. Used by both create and update: the settledness rule makes
// them the same problem.
func waitClusterProvisioned(
	ctx context.Context,
	client *nscale.Client,
	id string,
	timeout time.Duration,
) (*kubernetesapi.ClusterV1Read, error) {
	return nkswait.Provisioned(ctx, clusterTarget(client, id, timeout))
}

// waitClusterDeleted blocks until the cluster is gone.
func waitClusterDeleted(
	ctx context.Context,
	client *nscale.Client,
	id string,
	timeout time.Duration,
) error {
	return nkswait.Deleted(ctx, clusterTarget(client, id, timeout))
}
