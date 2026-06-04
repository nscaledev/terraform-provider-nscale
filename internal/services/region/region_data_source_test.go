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

package region_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccRegionDataSource_byID(t *testing.T) {
	regionID := os.Getenv("NSCALE_REGION_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRegionDataSourceConfig(regionID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.nscale_region.test", "id", regionID),
					resource.TestCheckResourceAttrSet("data.nscale_region.test", "name"),
				),
			},
		},
	})
}

// TestAccRegionDataSource_defaultsToProviderRegion exercises the data source's
// fallback to the provider-configured region (NSCALE_REGION_ID) when no id is
// supplied in the configuration.
func TestAccRegionDataSource_defaultsToProviderRegion(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRegionDataSourceConfigDefault(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.nscale_region.test", "id", os.Getenv("NSCALE_REGION_ID")),
					resource.TestCheckResourceAttrSet("data.nscale_region.test", "name"),
				),
			},
		},
	})
}

func testAccRegionDataSourceConfig(id string) string {
	return fmt.Sprintf(`
data "nscale_region" "test" {
  id = %q
}
`, id)
}

func testAccRegionDataSourceConfigDefault() string {
	return `
data "nscale_region" "test" {}
`
}
