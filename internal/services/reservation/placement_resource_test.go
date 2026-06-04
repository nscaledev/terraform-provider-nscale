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
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccPlacementResource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	accelerator := os.Getenv("NSCALE_TEST_RESERVATION_ACCELERATOR")
	unit := os.Getenv("NSCALE_TEST_RESERVATION_UNIT")
	imageID := os.Getenv("NSCALE_TEST_IMAGE_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckPlacement(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create + Read.
			{
				Config: testAccPlacementResourceConfig(name, accelerator, unit, imageID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nscale_placement.test", "id"),
					resource.TestCheckResourceAttr("nscale_placement.test", "name", name),
					resource.TestCheckResourceAttr("nscale_placement.test", "host_count", "1"),
					resource.TestCheckResourceAttr("nscale_placement.test", "constraints.policy", "pack"),
					resource.TestCheckResourceAttr("nscale_placement.test", "server_spec.image_id", imageID),
					resource.TestCheckResourceAttrPair(
						"nscale_placement.test", "reservation_id",
						"nscale_reservation.test", "id",
					),
					resource.TestCheckResourceAttrPair(
						"nscale_placement.test", "network_id",
						"nscale_network.test", "id",
					),
					resource.TestCheckResourceAttrSet("nscale_placement.test", "region_id"),
					resource.TestCheckResourceAttrSet("nscale_placement.test", "project_id"),
					resource.TestCheckResourceAttrSet("nscale_placement.test", "provisioning_status"),
					resource.TestCheckResourceAttrSet("nscale_placement.test", "creation_time"),
				),
			},
			// 2. Plan-only: catches spurious diffs across the nested attributes.
			{
				Config:   testAccPlacementResourceConfig(name, accelerator, unit, imageID),
				PlanOnly: true,
			},
			// 3. Import.
			{
				ResourceName:            "nscale_placement.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"}, // timeouts is provider-side, not returned by the API.
			},
		},
	})
}

func testAccPlacementResourceConfig(name, accelerator, unit, imageID string) string {
	return fmt.Sprintf(`
resource "nscale_reservation" "test" {
  name        = "%[1]s-res"
  accelerator = %[2]q
  unit        = %[3]q
  unit_count  = 1
}

resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.241.0/24"
}

resource "nscale_security_group" "test" {
  name = "%[1]s-sg"

  rules = [
    {
      type      = "ingress"
      protocol  = "tcp"
      from_port = 22
    }
  ]

  network_id = nscale_network.test.id
}

resource "nscale_placement" "test" {
  name           = %[1]q
  reservation_id = nscale_reservation.test.id
  network_id     = nscale_network.test.id
  host_count          = 1

  constraints = {
    policy = "pack"
  }

  server_spec = {
    image_id = %[4]q

    networking = {
      security_group_ids = [nscale_security_group.test.id]
    }
  }
}
`, name, accelerator, unit, imageID)
}
