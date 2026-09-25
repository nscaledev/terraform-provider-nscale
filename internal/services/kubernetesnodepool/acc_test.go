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
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/nscaledev/terraform-provider-nscale/internal/provider"
)

// COST WARNING: these tests provision real worker nodes. A pool is cheaper than
// a control plane, but it is still billable compute, and the replacement tests
// each pay for a full pool rebuild. Several tests also roll every worker in the
// pool one node at a time, so wall-clock scales with replica count — the
// configs below deliberately use the smallest pool that still proves the point.

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"nscale": providerserver.NewProtocol6WithError(provider.New()),
}

// testAccPreCheck skips unless the base credentials are set. The provider's
// Configure step needs a token plus organization, region and project before it
// will build a client.
func testAccPreCheck(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"NSCALE_SERVICE_TOKEN",
		"NSCALE_ORGANIZATION_ID",
		"NSCALE_REGION_ID",
		"NSCALE_PROJECT_ID",
	} {
		if os.Getenv(name) == "" {
			t.Skipf("%s must be set for kubernetes node pool acceptance tests", name)
		}
	}
}

// testAccPreCheckNodePool additionally requires the NKS endpoint, an existing
// cluster and a worker flavor.
//
// The endpoint is needed because NKS, unlike every other service, has no
// default URL baked into the provider.
//
// The CLUSTER is the load-bearing one. These tests attach pools to a
// pre-existing cluster rather than creating their own, because a control plane
// took a measured 32 minutes to build — creating one per test case would put
// this suite into the hours for no coverage the cluster package's own tests do
// not already provide. The cluster must be provisioned and healthy: a pool
// against a cluster that is still building will sit in the waiter.
//
// The FLAVOR must be given explicitly rather than discovered, because
// nscale_instance_flavor looks up by ID only — there is no "any small flavor in
// this region" query to write.
func testAccPreCheckNodePool(t *testing.T) {
	t.Helper()

	testAccPreCheck(t)

	for _, name := range []string{
		"NSCALE_NKS_SERVICE_API_ENDPOINT",
		"NSCALE_TEST_NKS_CLUSTER_ID",
		"NSCALE_TEST_NKS_FLAVOR_ID",
	} {
		if os.Getenv(name) == "" {
			t.Skipf("%s must be set for NKS node pool acceptance tests", name)
		}
	}
}

// testAccPreCheckNodePoolAltFlavor additionally requires a SECOND flavor, for
// the test that proves changing compute.flavor_id forces replacement.
//
// It must be available in the cluster's region. This is the assertion that
// matters most in the suite: the spec's first draft assumed a flavor change was
// an in-place update, and had that shipped, every flavour change would have
// planned clean and then failed at apply with a 422.
func testAccPreCheckNodePoolAltFlavor(t *testing.T) {
	t.Helper()

	testAccPreCheckNodePool(t)

	if os.Getenv("NSCALE_TEST_NKS_FLAVOR_ID_ALT") == "" {
		t.Skip("NSCALE_TEST_NKS_FLAVOR_ID_ALT must be set for the node pool flavor replacement test")
	}
}

// testAccPreCheckNodePoolAltCluster additionally requires a SECOND cluster, for
// the test that proves a pool cannot move between clusters. Environments often
// have only one, so this skips rather than failing.
func testAccPreCheckNodePoolAltCluster(t *testing.T) {
	t.Helper()

	testAccPreCheckNodePool(t)

	if os.Getenv("NSCALE_TEST_NKS_CLUSTER_ID_ALT") == "" {
		t.Skip("NSCALE_TEST_NKS_CLUSTER_ID_ALT must be set for the node pool cluster replacement test")
	}
}

// testAccPreCheckNodePoolReservation additionally requires a reservation with
// spare capacity. A reservation-backed pool cannot be created without one, and
// staging does not always have capacity free, so every reservation test skips
// when it is absent rather than failing the suite.
func testAccPreCheckNodePoolReservation(t *testing.T) {
	t.Helper()

	testAccPreCheckNodePool(t)

	if os.Getenv("NSCALE_TEST_NKS_RESERVATION_ID") == "" {
		t.Skip("NSCALE_TEST_NKS_RESERVATION_ID must be set for the reservation-backed node pool tests")
	}
}

func testAccClusterID() string {
	return os.Getenv("NSCALE_TEST_NKS_CLUSTER_ID")
}

func testAccFlavorID() string {
	return os.Getenv("NSCALE_TEST_NKS_FLAVOR_ID")
}

func testAccReservationID() string {
	return os.Getenv("NSCALE_TEST_NKS_RESERVATION_ID")
}

// testAccClusterConfig reads the cluster under test through the data source, so
// the pool configs below depend on a real cluster ID and the suite fails
// informatively if it has been deleted.
func testAccClusterConfig() string {
	return fmt.Sprintf(`
data "nscale_kubernetes_cluster" "test" {
  id = %[1]q
}
`, testAccClusterID())
}
