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

package kubernetesnodepool_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const nodePoolResourceName = "nscale_kubernetes_node_pool.test"

// captureNodePoolID records the pool's ID into target, and expectNodePoolID
// asserts against it in a later step. Together they distinguish an in-place
// update from a destroy-and-recreate.
func captureNodePoolID(target *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		res, ok := state.RootModule().Resources[nodePoolResourceName]
		if !ok {
			return fmt.Errorf("%s not found in state", nodePoolResourceName)
		}

		*target = res.Primary.ID

		return nil
	}
}

// expectNodePoolID asserts the pool still has the previously-captured ID
// (same == updated in place) or that it does not (recreated).
func expectNodePoolID(previous *string, same bool) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		res, ok := state.RootModule().Resources[nodePoolResourceName]
		if !ok {
			return fmt.Errorf("%s not found in state", nodePoolResourceName)
		}

		matches := res.Primary.ID == *previous
		if same && !matches {
			return fmt.Errorf(
				"node pool was recreated (%s -> %s) but the change should have been applied in place",
				*previous, res.Primary.ID,
			)
		}
		if !same && matches {
			return fmt.Errorf("node pool %s should have been replaced but was updated in place", *previous)
		}

		return nil
	}
}

func testAccNodePoolConfigCompute(name string, replicas int) string {
	return testAccClusterConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name              = %[1]q
  cluster_id        = data.nscale_kubernetes_cluster.test.id
  provisioning_mode = "compute"
  replicas          = %[2]d

  compute = {
    flavor_id = %[3]q
  }
}
`, name, replicas, testAccFlavorID())
}

func testAccNodePoolConfigFlavor(name, flavorID string) string {
	return testAccClusterConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name              = %[1]q
  cluster_id        = data.nscale_kubernetes_cluster.test.id
  provisioning_mode = "compute"
  replicas          = 1

  compute = {
    flavor_id = %[2]q
  }
}
`, name, flavorID)
}

// testAccNodePoolConfigTaints adds a taint to an otherwise unchanged pool. The
// edit is an in-place update at the API level and rolls every worker.
func testAccNodePoolConfigTaints(name, effect string) string {
	return testAccClusterConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name              = %[1]q
  cluster_id        = data.nscale_kubernetes_cluster.test.id
  provisioning_mode = "compute"
  replicas          = 1

  compute = {
    flavor_id = %[2]q
  }

  taints = [{
    key    = "workload"
    value  = "general"
    effect = %[3]q
  }]
}
`, name, testAccFlavorID(), effect)
}

// testAccNodePoolConfigLabels is the labels counterpart, and includes an
// empty-valued label — legal in Kubernetes and still applied, which is the
// round-trip a converter that treats "" as absent would break.
func testAccNodePoolConfigLabels(name, tier string) string {
	return testAccClusterConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name              = %[1]q
  cluster_id        = data.nscale_kubernetes_cluster.test.id
  provisioning_mode = "compute"
  replicas          = 1

  compute = {
    flavor_id = %[2]q
  }

  labels = {
    tier  = %[3]q
    blank = ""
  }
}
`, name, testAccFlavorID(), tier)
}

func testAccNodePoolConfigAltCluster(name string) string {
	return fmt.Sprintf(`
data "nscale_kubernetes_cluster" "other" {
  id = %[1]q
}

resource "nscale_kubernetes_node_pool" "test" {
  name              = %[2]q
  cluster_id        = data.nscale_kubernetes_cluster.other.id
  provisioning_mode = "compute"
  replicas          = 1

  compute = {
    flavor_id = %[3]q
  }
}
`, os.Getenv("NSCALE_TEST_NKS_CLUSTER_ID_ALT"), name, testAccFlavorID())
}

func testAccNodePoolConfigReservation(name string, replicas int) string {
	return testAccClusterConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name              = %[1]q
  cluster_id        = data.nscale_kubernetes_cluster.test.id
  provisioning_mode = "reservation"
  replicas          = %[2]d

  reservation = {
    reservation_id = %[3]q
  }
}
`, name, replicas, testAccReservationID())
}

func TestAccKubernetesNodePoolResource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePool(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create + Read.
			{
				Config: testAccNodePoolConfigCompute(name, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "id"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "name", name),
					resource.TestCheckResourceAttr(nodePoolResourceName, "provisioning_mode", "compute"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "replicas", "1"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "compute.flavor_id", testAccFlavorID()),
					// Inherited from the cluster — proves the API derives pool scope
					// from cluster_id rather than needing it supplied.
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "project_id"),
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "organization_id"),
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "region_id"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "provisioning_status", "provisioned"),
					// health_status is asserted as merely SET, not as "healthy".
					// Health does not determine convergence in NKS, so a settled
					// pool can legitimately be degraded — a single worker failing
					// its Kubernetes node checks is enough. Asserting "healthy"
					// here made the test fail on a pool the API considered done.
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "health_status"),
					// up_to_date_replicas tracks the template and must have
					// converged. ready_replicas depends on node conditions, which
					// are outside the provisioning contract, so it is only
					// asserted as set.
					resource.TestCheckResourceAttr(nodePoolResourceName, "up_to_date_replicas", "1"),
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "ready_replicas"),
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "kubernetes_version"),
					// A pool has no platform release argument but inherits one.
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "applied_platform_release_id"),
					resource.TestCheckResourceAttrSet(
						nodePoolResourceName, "platform_release_kubernetes_version",
					),
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "creation_time"),
					// A compute pool has no placement.
					resource.TestCheckNoResourceAttr(nodePoolResourceName, "placement_id"),
				),
			},
			// 2. Plan-only: catches spurious diffs. This is the step that proves
			// omitted taints and labels round-trip as null rather than coming back
			// as empty collections.
			{
				Config:   testAccNodePoolConfigCompute(name, 1),
				PlanOnly: true,
			},
			// 3. Import. No ImportStateVerifyIgnore beyond timeouts: NKS returns no
			// write-once secrets and every argument is readable from spec, so
			// everything must round-trip.
			{
				ResourceName:            nodePoolResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"}, // provider-side only, never returned by the API.
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_scale is the common update: replicas
// changes in place on a compute pool and the pool keeps its ID.
func TestAccKubernetesNodePoolResource_scale(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	var poolID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePool(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolConfigCompute(name, 1),
				Check:  captureNodePoolID(&poolID),
			},
			{
				Config: testAccNodePoolConfigCompute(name, 2),
				// The plan check is the mirror image of _reservationImmutable: on a
				// compute pool the same edit must be an Update. Without it, a
				// RequiresReplaceIf predicate stuck at true would pass the
				// reservation test and quietly destroy compute pools on every
				// scale. Asserted here rather than in its own test because this
				// step already pays for the apply.
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(nodePoolResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, true),
					resource.TestCheckResourceAttr(nodePoolResourceName, "replicas", "2"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "up_to_date_replicas", "2"),
				),
			},
			// Scale back down, which is the path that exercises a drain.
			{
				Config: testAccNodePoolConfigCompute(name, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, true),
					resource.TestCheckResourceAttr(nodePoolResourceName, "replicas", "1"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "up_to_date_replicas", "1"),
				),
			},
			{
				Config:   testAccNodePoolConfigCompute(name, 1),
				PlanOnly: true,
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_scaleToZero is the acceptance-level
// counterpart of TestCreateParamsSerialisesZeroReplicas. replicas has no
// omitempty in the generated client, so 0 must reach the API; if it were ever
// dropped the pool would silently keep its workers and the plan-only step below
// would fail with "Provider produced inconsistent result after apply".
func TestAccKubernetesNodePoolResource_scaleToZero(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	var poolID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePool(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolConfigCompute(name, 1),
				Check:  captureNodePoolID(&poolID),
			},
			{
				Config: testAccNodePoolConfigCompute(name, 0),
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, true),
					resource.TestCheckResourceAttr(nodePoolResourceName, "replicas", "0"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "up_to_date_replicas", "0"),
				),
			},
			{
				Config:   testAccNodePoolConfigCompute(name, 0),
				PlanOnly: true,
			},
			// Back up again: a pool scaled to zero must still be scalable, which
			// proves nothing about the pool was torn down on the way.
			{
				Config: testAccNodePoolConfigCompute(name, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, true),
					resource.TestCheckResourceAttr(nodePoolResourceName, "up_to_date_replicas", "1"),
				),
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_taints pins the taint edit as an in-place
// update that rolls the pool.
//
// up_to_date_replicas returning to replicas is what proves the roll actually
// finished rather than the waiter returning early: it drops while workers are
// being replaced and only recovers once every node runs the new template.
func TestAccKubernetesNodePoolResource_taints(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	var poolID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePool(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolConfigCompute(name, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureNodePoolID(&poolID),
					resource.TestCheckNoResourceAttr(nodePoolResourceName, "taints"),
				),
			},
			// Adding a taint rolls the pool but must not replace it.
			{
				Config: testAccNodePoolConfigTaints(name, "NoSchedule"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(nodePoolResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, true),
					resource.TestCheckResourceAttr(nodePoolResourceName, "taints.#", "1"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "taints.0.key", "workload"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "taints.0.value", "general"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "taints.0.effect", "NoSchedule"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "up_to_date_replicas", "1"),
				),
			},
			{
				Config:   testAccNodePoolConfigTaints(name, "NoSchedule"),
				PlanOnly: true,
			},
			// Changing only the effect re-hashes the machine template, so this
			// rolls the pool a second time — still in place.
			{
				Config: testAccNodePoolConfigTaints(name, "PreferNoSchedule"),
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, true),
					resource.TestCheckResourceAttr(nodePoolResourceName, "taints.0.effect", "PreferNoSchedule"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "up_to_date_replicas", "1"),
				),
			},
			// Removing the taints entirely must return the attribute to null, not
			// to an empty list, or every subsequent plan shows a diff.
			{
				Config: testAccNodePoolConfigCompute(name, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, true),
					resource.TestCheckNoResourceAttr(nodePoolResourceName, "taints"),
				),
			},
			{
				Config:   testAccNodePoolConfigCompute(name, 1),
				PlanOnly: true,
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_labels is the labels counterpart, including
// the empty-valued label.
func TestAccKubernetesNodePoolResource_labels(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	var poolID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePool(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolConfigLabels(name, "standard"),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureNodePoolID(&poolID),
					resource.TestCheckResourceAttr(nodePoolResourceName, "labels.tier", "standard"),
					// The empty value must survive: Kubernetes still applies the
					// label, so dropping it would change what lands on the node.
					resource.TestCheckResourceAttr(nodePoolResourceName, "labels.blank", ""),
				),
			},
			{
				Config:   testAccNodePoolConfigLabels(name, "standard"),
				PlanOnly: true,
			},
			{
				Config: testAccNodePoolConfigLabels(name, "premium"),
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, true),
					resource.TestCheckResourceAttr(nodePoolResourceName, "labels.tier", "premium"),
					resource.TestCheckResourceAttr(nodePoolResourceName, "up_to_date_replicas", "1"),
				),
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_replaceOnFlavorChange is the most important
// replacement assertion in this suite.
//
// compute.flavor_id is CEL-guarded and the server returns 422 on a change. The
// spec's first draft assumed it was an in-place-but-disruptive update; had that
// shipped, every flavour change would have planned clean and then failed at
// apply with an error the user could not work around.
func TestAccKubernetesNodePoolResource_replaceOnFlavorChange(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	alt := os.Getenv("NSCALE_TEST_NKS_FLAVOR_ID_ALT")

	var poolID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePoolAltFlavor(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolConfigFlavor(name, testAccFlavorID()),
				Check:  captureNodePoolID(&poolID),
			},
			{
				Config: testAccNodePoolConfigFlavor(name, alt),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(nodePoolResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, false),
					resource.TestCheckResourceAttr(nodePoolResourceName, "compute.flavor_id", alt),
				),
			},
			{
				Config:   testAccNodePoolConfigFlavor(name, alt),
				PlanOnly: true,
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_replaceOnNameChange pins the rename as
// immutable, matching the cluster.
func TestAccKubernetesNodePoolResource_replaceOnNameChange(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	renamed := acctest.RandomWithPrefix("tf-acc-test")

	var poolID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePool(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolConfigCompute(name, 1),
				Check:  captureNodePoolID(&poolID),
			},
			{
				Config: testAccNodePoolConfigCompute(renamed, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(nodePoolResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, false),
					resource.TestCheckResourceAttr(nodePoolResourceName, "name", renamed),
				),
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_replaceOnClusterChange pins that a pool
// cannot move between clusters.
func TestAccKubernetesNodePoolResource_replaceOnClusterChange(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	var poolID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePoolAltCluster(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolConfigCompute(name, 1),
				Check:  captureNodePoolID(&poolID),
			},
			{
				Config: testAccNodePoolConfigAltCluster(name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(nodePoolResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: expectNodePoolID(&poolID, false),
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_reservation covers the other provisioning
// mode end to end, and the placement it resolves to.
func TestAccKubernetesNodePoolResource_reservation(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePoolReservation(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolConfigReservation(name, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(nodePoolResourceName, "provisioning_mode", "reservation"),
					resource.TestCheckResourceAttr(
						nodePoolResourceName, "reservation.reservation_id", testAccReservationID(),
					),
					// The placement the pool created inside the reservation. This is
					// the one piece of reservation status that is genuinely new
					// information rather than an echo of the request.
					resource.TestCheckResourceAttrSet(nodePoolResourceName, "placement_id"),
					resource.TestCheckNoResourceAttr(nodePoolResourceName, "compute"),
				),
			},
			{
				Config:   testAccNodePoolConfigReservation(name, 1),
				PlanOnly: true,
			},
			{
				ResourceName:            nodePoolResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"}, // provider-side only, never returned by the API.
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_reservationImmutable is the
// RequiresReplaceIf predicate under test.
//
// A placement never rolls, so the API rejects a replicas, taints or labels edit
// on a reservation pool. If the predicate is wrong the plan promises an
// in-place change and then fails at apply with an error the user cannot work
// around, which is why each edit is asserted on the plan AND carried through to
// a real apply.
//
// COST: this pays for three pool rebuilds, each releasing and re-claiming the
// placement. An earlier draft tried to assert on the plan alone to avoid that,
// but ConfigPlanChecks cannot be combined with PlanOnly — and "the plan is
// non-empty" would not distinguish an update from a replacement, which is the
// entire point. The test is gated behind NSCALE_TEST_NKS_RESERVATION_ID, so it
// only runs where someone has deliberately supplied reservation capacity.
//
// All three predicates are exercised rather than one standing in for the
// others: they are three separate closures (Int64, List and Map plan modifiers
// cannot share one), so a copy-paste slip in any of them is live.
func TestAccKubernetesNodePoolResource_reservationImmutable(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	expectReplace := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction(nodePoolResourceName, plancheck.ResourceActionReplace),
		},
	}

	taintedConfig := testAccClusterConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name              = %[1]q
  cluster_id        = data.nscale_kubernetes_cluster.test.id
  provisioning_mode = "reservation"
  replicas          = 2

  reservation = {
    reservation_id = %[2]q
  }

  taints = [{
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NoSchedule"
  }]
}
`, name, testAccReservationID())

	labelledConfig := testAccClusterConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name              = %[1]q
  cluster_id        = data.nscale_kubernetes_cluster.test.id
  provisioning_mode = "reservation"
  replicas          = 2

  reservation = {
    reservation_id = %[2]q
  }

  taints = [{
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NoSchedule"
  }]

  labels = {
    accelerator = "gpu"
  }
}
`, name, testAccReservationID())

	var poolID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePoolReservation(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolConfigReservation(name, 1),
				Check:  captureNodePoolID(&poolID),
			},
			// Scaling a reservation pool is a rebuild, not a scale.
			{
				Config:           testAccNodePoolConfigReservation(name, 2),
				ConfigPlanChecks: expectReplace,
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, false),
					captureNodePoolID(&poolID),
					resource.TestCheckResourceAttr(nodePoolResourceName, "replicas", "2"),
				),
			},
			// So is adding a taint...
			{
				Config:           taintedConfig,
				ConfigPlanChecks: expectReplace,
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, false),
					captureNodePoolID(&poolID),
					resource.TestCheckResourceAttr(nodePoolResourceName, "taints.0.key", "nvidia.com/gpu"),
				),
			},
			// ...and adding a label.
			{
				Config:           labelledConfig,
				ConfigPlanChecks: expectReplace,
				Check: resource.ComposeAggregateTestCheckFunc(
					expectNodePoolID(&poolID, false),
					resource.TestCheckResourceAttr(nodePoolResourceName, "labels.accelerator", "gpu"),
				),
			},
			{
				Config:   labelledConfig,
				PlanOnly: true,
			},
		},
	})
}

// TestAccKubernetesNodePoolResource_rejectsCapacityModeMismatch is the
// plan-time validation matrix, which the ticket requires fail at plan rather
// than apply. None of these steps reaches the API.
func TestAccKubernetesNodePoolResource_rejectsCapacityModeMismatch(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	tests := []struct {
		name   string
		config string
		expect *regexp.Regexp
	}{
		{
			name: "compute mode with no capacity block",
			config: `
  provisioning_mode = "compute"
  replicas          = 1
`,
			expect: regexp.MustCompile(`(?s)Missing Node Pool Capacity Block`),
		},
		{
			name: "compute mode with a reservation block",
			config: `
  provisioning_mode = "compute"
  replicas          = 1

  reservation = {
    reservation_id = "res-1"
  }
`,
			expect: regexp.MustCompile(`(?s)Unexpected Node Pool Capacity Block`),
		},
		{
			name: "reservation mode with a compute block",
			config: `
  provisioning_mode = "reservation"
  replicas          = 1

  compute = {
    flavor_id = "flavor-1"
  }
`,
			expect: regexp.MustCompile(`(?s)Unexpected Node Pool Capacity Block`),
		},
		{
			name: "both capacity blocks",
			config: `
  provisioning_mode = "compute"
  replicas          = 1

  compute = {
    flavor_id = "flavor-1"
  }

  reservation = {
    reservation_id = "res-1"
  }
`,
			expect: regexp.MustCompile(`(?s)Unexpected Node Pool Capacity Block`),
		},
		{
			name: "an unknown provisioning mode",
			config: `
  provisioning_mode = "spot"
  replicas          = 1

  compute = {
    flavor_id = "flavor-1"
  }
`,
			expect: regexp.MustCompile(`(?s)provisioning_mode`),
		},
		{
			name: "a negative replica count",
			config: `
  provisioning_mode = "compute"
  replicas          = -1

  compute = {
    flavor_id = "flavor-1"
  }
`,
			expect: regexp.MustCompile(`(?s)replicas`),
		},
		{
			name: "an invalid taint effect",
			config: `
  provisioning_mode = "compute"
  replicas          = 1

  compute = {
    flavor_id = "flavor-1"
  }

  taints = [{
    key    = "workload"
    effect = "NoBueno"
  }]
`,
			expect: regexp.MustCompile(`(?s)effect`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheckNodePool(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name       = %[1]q
  cluster_id = %[2]q
%[3]s
}
`, name, testAccClusterID(), test.config),
						ExpectError: test.expect,
						PlanOnly:    true,
					},
				},
			})
		})
	}
}

// TestAccKubernetesNodePoolResource_rejectsSettingScope checks that scope is
// read-only. project_id is Computed, so the framework must reject any attempt
// to configure it — this is what stops users assuming the provider's usual
// project_id argument applies here.
func TestAccKubernetesNodePoolResource_rejectsSettingScope(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePool(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name              = %[1]q
  cluster_id        = "00000000-0000-0000-0000-000000000000"
  provisioning_mode = "compute"
  replicas          = 1
  project_id        = "proj-should-be-rejected"

  compute = {
    flavor_id = "flavor-1"
  }
}
`, name),
				ExpectError: regexp.MustCompile(`(?s)project_id`),
				PlanOnly:    true,
			},
		},
	})
}
