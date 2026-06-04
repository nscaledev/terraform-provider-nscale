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

func TestAccReservationResource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	accelerator := os.Getenv("NSCALE_TEST_RESERVATION_ACCELERATOR")
	unit := os.Getenv("NSCALE_TEST_RESERVATION_UNIT")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckReservation(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create + Read.
			{
				Config: testAccReservationResourceConfig(name, accelerator, unit, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nscale_reservation.test", "id"),
					resource.TestCheckResourceAttr("nscale_reservation.test", "name", name),
					resource.TestCheckResourceAttr("nscale_reservation.test", "accelerator", accelerator),
					resource.TestCheckResourceAttr("nscale_reservation.test", "unit", unit),
					resource.TestCheckResourceAttr("nscale_reservation.test", "unit_count", "1"),
					resource.TestCheckResourceAttrSet("nscale_reservation.test", "region_id"),
					resource.TestCheckResourceAttrSet("nscale_reservation.test", "project_id"),
					resource.TestCheckResourceAttrSet("nscale_reservation.test", "machine_flavor_id"),
					resource.TestCheckResourceAttrSet("nscale_reservation.test", "claimed_unit_count"),
					resource.TestCheckResourceAttrSet("nscale_reservation.test", "provisioning_status"),
					resource.TestCheckResourceAttrSet("nscale_reservation.test", "creation_time"),
				),
			},
			// 2. Plan-only: catches spurious diffs / the inconsistent-result class.
			{
				Config:   testAccReservationResourceConfig(name, accelerator, unit, 1),
				PlanOnly: true,
			},
			// 3. Import.
			{
				ResourceName:            "nscale_reservation.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"}, // timeouts is provider-side, not returned by the API.
			},
		},
	})
}

func testAccReservationResourceConfig(name, accelerator, unit string, count int) string {
	return fmt.Sprintf(`
resource "nscale_reservation" "test" {
  name        = %[1]q
  description = "managed by terraform acceptance tests"
  accelerator = %[2]q
  unit        = %[3]q
  unit_count       = %[4]d

  tags = {
    workload = "training"
  }
}
`, name, accelerator, unit, count)
}
