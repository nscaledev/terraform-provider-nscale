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

package instance_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// TestAccInstanceFlavorDataSource_basic looks up the known catalog flavor
// (NSCALE_TEST_FLAVOR_ID) and asserts the computed spec fields are populated.
// This is a read-only lookup — it provisions no compute.
func TestAccInstanceFlavorDataSource_basic(t *testing.T) {
	flavorID := os.Getenv("NSCALE_TEST_FLAVOR_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccInstanceFlavorDataSourceConfig(flavorID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.nscale_instance_flavor.test", "id", flavorID),
					resource.TestCheckResourceAttrSet("data.nscale_instance_flavor.test", "name"),
					resource.TestCheckResourceAttrSet("data.nscale_instance_flavor.test", "cpus"),
					resource.TestCheckResourceAttrSet("data.nscale_instance_flavor.test", "memory_size"),
					resource.TestCheckResourceAttrSet("data.nscale_instance_flavor.test", "disk_size"),
					// region_id is Optional+Computed and falls back to the
					// provider-configured region when not set in config.
					resource.TestCheckResourceAttr(
						"data.nscale_instance_flavor.test", "region_id", os.Getenv("NSCALE_REGION_ID"),
					),
				),
			},
		},
	})
}

func testAccInstanceFlavorDataSourceConfig(id string) string {
	return fmt.Sprintf(`
data "nscale_instance_flavor" "test" {
  id = %q
}
`, id)
}
