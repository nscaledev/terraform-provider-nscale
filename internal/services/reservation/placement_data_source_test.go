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

package reservation_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccPlacementDataSource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	accelerator := os.Getenv("NSCALE_TEST_RESERVATION_ACCELERATOR")
	unit := os.Getenv("NSCALE_TEST_RESERVATION_UNIT")
	imageID := os.Getenv("NSCALE_TEST_IMAGE_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckPlacement(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPlacementDataSourceConfig(name, accelerator, unit, imageID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.nscale_placement.test", "id",
						"nscale_placement.test", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_placement.test", "name",
						"nscale_placement.test", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_placement.test", "reservation_id",
						"nscale_placement.test", "reservation_id",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_placement.test", "network_id",
						"nscale_placement.test", "network_id",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_placement.test", "server_spec.image_id",
						"nscale_placement.test", "server_spec.image_id",
					),
				),
			},
		},
	})
}

func testAccPlacementDataSourceConfig(name, accelerator, unit, imageID string) string {
	return testAccPlacementResourceConfig(name, accelerator, unit, imageID) + `
data "nscale_placement" "test" {
  id = nscale_placement.test.id
}
`
}
