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
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const clusterDataSourceName = "data.nscale_kubernetes_cluster.test"

// TestAccKubernetesClusterDataSource_byID checks the data source agrees with the
// resource it reads. Comparing against the resource rather than against literals
// is what catches a converter used by only one of the two paths.
func TestAccKubernetesClusterDataSource_byID(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterConfigBasic(name) + `
data "nscale_kubernetes_cluster" "test" {
  id = nscale_kubernetes_cluster.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						clusterDataSourceName, "id", clusterResourceName, "id",
					),
					resource.TestCheckResourceAttrPair(
						clusterDataSourceName, "name", clusterResourceName, "name",
					),
					resource.TestCheckResourceAttrPair(
						clusterDataSourceName, "network_id", clusterResourceName, "network_id",
					),
					resource.TestCheckResourceAttrPair(
						clusterDataSourceName, "platform_release_id",
						clusterResourceName, "platform_release_id",
					),
					resource.TestCheckResourceAttrPair(
						clusterDataSourceName, "project_id", clusterResourceName, "project_id",
					),
					resource.TestCheckResourceAttrPair(
						clusterDataSourceName, "region_id", clusterResourceName, "region_id",
					),
					resource.TestCheckResourceAttrPair(
						clusterDataSourceName, "api_server_endpoint.private_host",
						clusterResourceName, "api_server_endpoint.private_host",
					),
					resource.TestCheckResourceAttrPair(
						clusterDataSourceName, "api_server_endpoint.certificate_authority_data",
						clusterResourceName, "api_server_endpoint.certificate_authority_data",
					),
				),
			},
		},
	})
}
