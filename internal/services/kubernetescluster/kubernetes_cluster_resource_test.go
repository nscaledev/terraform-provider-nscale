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

package kubernetescluster_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const clusterResourceName = "nscale_kubernetes_cluster.test"

// captureClusterID records the cluster's ID into target, and
// expectClusterID asserts against it in a later step. Together they distinguish
// an in-place update from a destroy-and-recreate, which the SDKv2 test harness
// has no direct plan assertion for.
func captureClusterID(target *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		res, ok := state.RootModule().Resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("%s not found in state", clusterResourceName)
		}

		*target = res.Primary.ID

		return nil
	}
}

// expectClusterID asserts the cluster still has the previously-captured ID
// (same == updated in place) or that it does not (recreated).
func expectClusterID(previous *string, same bool) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		res, ok := state.RootModule().Resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("%s not found in state", clusterResourceName)
		}

		matches := res.Primary.ID == *previous
		if same && !matches {
			return fmt.Errorf(
				"cluster was recreated (%s -> %s) but the change should have been applied in place",
				*previous, res.Primary.ID,
			)
		}
		if !same && matches {
			return fmt.Errorf("cluster %s should have been replaced but was updated in place", *previous)
		}

		return nil
	}
}

func testAccClusterConfigBasic(name string) string {
	return testAccPlatformReleaseConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_cluster" "test" {
  name                = %[1]q
  network_id          = data.nscale_network.test.id
  platform_release_id = data.nscale_kubernetes_platform_releases.test.releases[0].id
}
`, name)
}

// testAccClusterConfigBoolZeroValues is the bool-zero-value guard from playbook
// §1.6: an explicitly-configured false must survive the round trip. hardware in
// particular defaults to TRUE server-side, so a dropped field flips the value.
func testAccClusterConfigBoolZeroValues(name string) string {
	return testAccPlatformReleaseConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_cluster" "test" {
  name                = %[1]q
  network_id          = data.nscale_network.test.id
  platform_release_id = data.nscale_kubernetes_platform_releases.test.releases[0].id

  api_server = {
    public_ip = false
  }

  addons = {
    hardware = false
  }
}
`, name)
}

func testAccClusterConfigClusterNetwork(name, podCIDR string) string {
	return testAccPlatformReleaseConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_cluster" "test" {
  name                = %[1]q
  network_id          = data.nscale_network.test.id
  platform_release_id = data.nscale_kubernetes_platform_releases.test.releases[0].id

  cluster_network = {
    pod_cidr = %[2]q
  }
}
`, name, podCIDR)
}

func testAccClusterConfigRelease(name, releaseID string) string {
	return fmt.Sprintf(`
data "nscale_network" "test" {
  id = %[1]q
}

resource "nscale_kubernetes_cluster" "test" {
  name                = %[2]q
  network_id          = data.nscale_network.test.id
  platform_release_id = %[3]q
}
`, testAccNetworkID(), name, releaseID)
}

// testAccClusterConfigSecondNetwork attaches the cluster to a different
// pre-existing network, which must force replacement.
func testAccClusterConfigSecondNetwork(name string) string {
	return testAccPlatformReleaseConfig() + fmt.Sprintf(`
data "nscale_network" "other" {
  id = %[1]q
}

resource "nscale_kubernetes_cluster" "test" {
  name                = %[2]q
  network_id          = data.nscale_network.other.id
  platform_release_id = data.nscale_kubernetes_platform_releases.test.releases[0].id
}
`, os.Getenv("NSCALE_TEST_NKS_NETWORK_ID_ALT"), name)
}

func TestAccKubernetesClusterResource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create + Read.
			{
				Config: testAccClusterConfigBasic(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(clusterResourceName, "id"),
					resource.TestCheckResourceAttr(clusterResourceName, "name", name),
					// Inherited from the network — proves the API derives scope
					// from network_id rather than needing it supplied.
					resource.TestCheckResourceAttrSet(clusterResourceName, "project_id"),
					resource.TestCheckResourceAttrSet(clusterResourceName, "organization_id"),
					resource.TestCheckResourceAttrSet(clusterResourceName, "region_id"),
					resource.TestCheckResourceAttr(clusterResourceName, "provisioning_status", "provisioned"),
					resource.TestCheckResourceAttr(clusterResourceName, "health_status", "healthy"),
					// The waiter only returns once status has settled, so the
					// endpoint data must already be populated when apply finishes.
					resource.TestCheckResourceAttrSet(
						clusterResourceName, "api_server_endpoint.certificate_authority_data",
					),
					resource.TestCheckResourceAttrSet(clusterResourceName, "api_server_endpoint.private_host"),
					resource.TestCheckResourceAttrSet(clusterResourceName, "kubernetes_version_target"),
					resource.TestCheckResourceAttrSet(clusterResourceName, "applied_platform_release_id"),
					resource.TestCheckResourceAttrSet(clusterResourceName, "creation_time"),
				),
			},
			// 2. Plan-only: catches spurious diffs, and is the only check that
			// proves the Optional+Computed defaults (api_server, cluster_network,
			// addons) actually round-trip.
			{
				Config:   testAccClusterConfigBasic(name),
				PlanOnly: true,
			},
			// 3. Import. No ImportStateVerifyIgnore beyond timeouts: NKS returns
			// no write-once secrets and every argument is readable from spec, so
			// everything must round-trip.
			{
				ResourceName:            clusterResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"}, // provider-side only, never returned by the API.
			},
		},
	})
}

// TestAccKubernetesClusterResource_boolZeroValues is the acceptance-level
// counterpart of TestCreateParamsBoolZeroValues. If either bool is dropped on
// the wire the API applies its own default, and the plan-only step fails with
// "Provider produced inconsistent result after apply".
func TestAccKubernetesClusterResource_boolZeroValues(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterConfigBoolZeroValues(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(clusterResourceName, "api_server.public_ip", "false"),
					resource.TestCheckResourceAttr(clusterResourceName, "addons.hardware", "false"),
				),
			},
			{
				Config:   testAccClusterConfigBoolZeroValues(name),
				PlanOnly: true,
			},
		},
	})
}

// TestAccKubernetesClusterResource_updateRelease exercises the in-place upgrade
// path and asserts the cluster is not recreated.
func TestAccKubernetesClusterResource_updateRelease(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	from := os.Getenv("NSCALE_TEST_NKS_PLATFORM_RELEASE_ID")
	to := os.Getenv("NSCALE_TEST_NKS_PLATFORM_RELEASE_UPGRADE_ID")

	var clusterID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKSUpgrade(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterConfigRelease(name, from),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureClusterID(&clusterID),
					resource.TestCheckResourceAttr(clusterResourceName, "applied_platform_release_id", from),
				),
			},
			{
				Config: testAccClusterConfigRelease(name, to),
				Check: resource.ComposeAggregateTestCheckFunc(
					expectClusterID(&clusterID, true),
					resource.TestCheckResourceAttr(clusterResourceName, "platform_release_id", to),
					// The settledness rule is what makes this assertion safe: an
					// unsettled read would still report the previous applied
					// release and the test would pass for the wrong reason.
					resource.TestCheckResourceAttr(clusterResourceName, "applied_platform_release_id", to),
				),
			},
			{
				Config:   testAccClusterConfigRelease(name, to),
				PlanOnly: true,
			},
		},
	})
}

// TestAccKubernetesClusterResource_updateClusterNetwork proves the mutable-CIDR
// decision. If the API rejects or silently ignores a pod CIDR change, this fails
// here rather than in a user's apply.
func TestAccKubernetesClusterResource_updateClusterNetwork(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	var clusterID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterConfigClusterNetwork(name, "10.240.0.0/12"),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureClusterID(&clusterID),
					resource.TestCheckResourceAttr(clusterResourceName, "cluster_network.pod_cidr", "10.240.0.0/12"),
				),
			},
			{
				Config: testAccClusterConfigClusterNetwork(name, "10.244.0.0/16"),
				Check: resource.ComposeAggregateTestCheckFunc(
					expectClusterID(&clusterID, true),
					resource.TestCheckResourceAttr(clusterResourceName, "cluster_network.pod_cidr", "10.244.0.0/16"),
				),
			},
			{
				Config:   testAccClusterConfigClusterNetwork(name, "10.244.0.0/16"),
				PlanOnly: true,
			},
		},
	})
}

// TestAccKubernetesClusterResource_replaceOnNetworkChange pins the one field
// that forces replacement. The API rejects a PUT carrying a different networkId,
// so if this ever plans an update instead, applies start failing with a 422.
func TestAccKubernetesClusterResource_replaceOnNetworkChange(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	var clusterID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKSReplace(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterConfigBasic(name),
				Check:  captureClusterID(&clusterID),
			},
			{
				Config: testAccClusterConfigSecondNetwork(name),
				Check:  expectClusterID(&clusterID, false),
			},
		},
	})
}

// TestAccKubernetesClusterResource_rejectsSettingScope checks that scope really
// is read-only. project_id is Computed, so the framework must reject any attempt
// to configure it — this is what stops users assuming the provider's usual
// project_id argument applies here.
func TestAccKubernetesClusterResource_rejectsSettingScope(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "nscale_kubernetes_cluster" "test" {
  name                = %[1]q
  network_id          = "00000000-0000-0000-0000-000000000000"
  platform_release_id = "rel-irrelevant"
  project_id          = "proj-should-be-rejected"
}
`, name),
				ExpectError: regexp.MustCompile(`(?s)project_id`),
				PlanOnly:    true,
			},
		},
	})
}
