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
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const nodePoolDataSourceName = "data.nscale_kubernetes_node_pool.test"

// testAccNodePoolDataSourceConfig creates a pool and reads it straight back, so
// the two can be compared attribute by attribute in one apply.
func testAccNodePoolDataSourceConfig(name string) string {
	return testAccClusterConfig() + fmt.Sprintf(`
resource "nscale_kubernetes_node_pool" "test" {
  name              = %[1]q
  description       = "read back by the data source"
  cluster_id        = data.nscale_kubernetes_cluster.test.id
  provisioning_mode = "compute"
  replicas          = 1

  compute = {
    flavor_id = %[2]q
  }

  taints = [{
    key    = "workload"
    value  = "general"
    effect = "NoSchedule"
  }]

  labels = {
    tier = "standard"
  }

  tags = {
    environment = "acc-test"
  }
}

data "nscale_kubernetes_node_pool" "test" {
  id = nscale_kubernetes_node_pool.test.id
}
`, name, testAccFlavorID())
}

// TestAccKubernetesNodePoolDataSource_basic asserts the data source agrees with
// the resource on every attribute they share. Comparing against the resource
// rather than against literals is what catches a converter the data source uses
// differently from the resource — they share one model, and this is the test
// that keeps it that way.
func TestAccKubernetesNodePoolDataSource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	shared := []string{
		"id",
		"name",
		"description",
		"cluster_id",
		"provisioning_mode",
		"replicas",
		"compute.flavor_id",
		"taints.#",
		"taints.0.key",
		"taints.0.value",
		"taints.0.effect",
		"labels.tier",
		"tags.environment",
		"project_id",
		"organization_id",
		"region_id",
		"creation_time",
		"provisioning_status",
		"health_status",
		"current_replicas",
		"ready_replicas",
		"up_to_date_replicas",
		"kubernetes_version",
		"applied_platform_release_id",
		"platform_release_kubernetes_version",
		"platform_release_deprecated",
		"platform_release_withdrawn",
	}

	checks := make([]resource.TestCheckFunc, 0, len(shared)+1)
	checks = append(checks, resource.TestCheckResourceAttr(nodePoolDataSourceName, "name", name))
	for _, attribute := range shared {
		checks = append(checks, resource.TestCheckResourceAttrPair(
			nodePoolDataSourceName, attribute,
			nodePoolResourceName, attribute,
		))
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePool(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodePoolDataSourceConfig(name),
				Check:  resource.ComposeAggregateTestCheckFunc(checks...),
			},
		},
	})
}

// TestAccKubernetesNodePoolDataSource_notFound checks the error path. Unlike the
// resource's Read, a data source pointed at a missing pool is a configuration
// error rather than drift, so it must fail the apply rather than silently
// yielding an empty result.
func TestAccKubernetesNodePoolDataSource_notFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNodePool(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "nscale_kubernetes_node_pool" "test" {
  id = "00000000-0000-0000-0000-000000000000"
}
`,
				ExpectError: regexp.MustCompile(`(?s)Failed to Read Kubernetes Node Pool`),
			},
		},
	})
}
