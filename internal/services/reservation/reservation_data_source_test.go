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

func TestAccReservationDataSource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	accelerator := os.Getenv("NSCALE_TEST_RESERVATION_ACCELERATOR")
	unit := os.Getenv("NSCALE_TEST_RESERVATION_UNIT")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckReservation(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccReservationDataSourceConfig(name, accelerator, unit),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.nscale_reservation.test", "id",
						"nscale_reservation.test", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_reservation.test", "name",
						"nscale_reservation.test", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_reservation.test", "accelerator",
						"nscale_reservation.test", "accelerator",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_reservation.test", "unit",
						"nscale_reservation.test", "unit",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_reservation.test", "machine_flavor_id",
						"nscale_reservation.test", "machine_flavor_id",
					),
				),
			},
		},
	})
}

func testAccReservationDataSourceConfig(name, accelerator, unit string) string {
	return fmt.Sprintf(`
resource "nscale_reservation" "test" {
  name        = %[1]q
  accelerator = %[2]q
  unit        = %[3]q
  unit_count       = 1
}

data "nscale_reservation" "test" {
  id = nscale_reservation.test.id
}
`, name, accelerator, unit)
}
