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
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/nscaledev/terraform-provider-nscale/internal/provider"
)

// COST WARNING: these tests provision real NKS control planes. A single run of
// the resource suite creates and destroys several Kubernetes clusters, each
// taking tens of minutes. This is by some margin the most expensive and slowest
// package in the acceptance suite — consider running it on its own rather than
// as part of a full `make testacc`.

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
			t.Skipf("%s must be set for kubernetes cluster acceptance tests", name)
		}
	}
}

// testAccPreCheckNKS additionally requires the NKS endpoint and a network.
//
// The endpoint is needed because NKS, unlike every other service, has no
// default URL baked into the provider.
//
// The network is needed because these tests attach to a pre-existing one rather
// than creating their own. That is deliberate: a cluster inherits its project,
// organization and region from its network, and on a shared environment the
// project must be one the caller can actually create clusters in. Creating a
// throwaway network per test would also add minutes to an already slow suite
// for no coverage — network creation is exercised by the network package's own
// tests.
func testAccPreCheckNKS(t *testing.T) {
	t.Helper()

	testAccPreCheck(t)

	for _, name := range []string{
		"NSCALE_NKS_SERVICE_API_ENDPOINT",
		"NSCALE_TEST_NKS_NETWORK_ID",
	} {
		if os.Getenv(name) == "" {
			t.Skipf("%s must be set for NKS acceptance tests", name)
		}
	}
}

// testAccPreCheckNKSReplace additionally requires a SECOND network, for the
// test that proves changing network_id forces replacement.
//
// It must be in the same project and region as the first: a different region
// and the platform release is unavailable (422), a different project and the
// caller likely cannot create there. Environments often have no such second
// network, so this test skips rather than failing.
func testAccPreCheckNKSReplace(t *testing.T) {
	t.Helper()

	testAccPreCheckNKS(t)

	if os.Getenv("NSCALE_TEST_NKS_NETWORK_ID_ALT") == "" {
		t.Skip("NSCALE_TEST_NKS_NETWORK_ID_ALT must be set for the NKS replacement acceptance test")
	}
}

// testAccNetworkID is the network under test.
func testAccNetworkID() string {
	return os.Getenv("NSCALE_TEST_NKS_NETWORK_ID")
}

// testAccPreCheckNKSUpgrade additionally requires two explicit platform release
// IDs for the in-place upgrade path. They are not derived from the releases data
// source because the upgrade test needs a specific, known-eligible ordered pair,
// and whether any two catalogue entries are a valid upgrade pair is a property
// of the catalogue rather than of the list order.
func testAccPreCheckNKSUpgrade(t *testing.T) {
	t.Helper()

	testAccPreCheckNKS(t)

	for _, name := range []string{
		"NSCALE_TEST_NKS_PLATFORM_RELEASE_ID",
		"NSCALE_TEST_NKS_PLATFORM_RELEASE_UPGRADE_ID",
	} {
		if os.Getenv(name) == "" {
			t.Skipf("%s must be set for the NKS upgrade acceptance test", name)
		}
	}
}

// testAccPlatformReleaseConfig selects a release eligible for creation:
// non-deprecated and non-withdrawn, mirroring the CLI's create-time convention.
//
// The region is read off the network rather than taken from NSCALE_REGION_ID,
// because the cluster's region is whatever the network says it is — reading it
// from anywhere else could select a release the cluster's actual region does
// not offer, which fails at apply with a 422.
func testAccPlatformReleaseConfig() string {
	return fmt.Sprintf(`
data "nscale_network" "test" {
  id = %[1]q
}

data "nscale_kubernetes_platform_releases" "test" {
  region_id  = data.nscale_network.test.region_id
  deprecated = false
  withdrawn  = false
}
`, testAccNetworkID())
}
