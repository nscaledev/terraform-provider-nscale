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
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

var regexpReservationUnitNotFound = regexp.MustCompile(`Reservation Unit Not Found`)

// Reads a real capacity offering. Nothing is created, so this is the one
// reservation acceptance test that consumes no quota.
func TestAccReservationUnitDataSource_basic(t *testing.T) {
	accelerator := os.Getenv("NSCALE_TEST_RESERVATION_ACCELERATOR")
	unit := os.Getenv("NSCALE_TEST_RESERVATION_UNIT")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckReservation(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccReservationUnitDataSourceConfig(accelerator, unit),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.nscale_reservation_unit.test", "accelerator", accelerator,
					),
					resource.TestCheckResourceAttr(
						"data.nscale_reservation_unit.test", "unit", unit,
					),
					resource.TestCheckResourceAttr(
						"data.nscale_reservation_unit.test", "region_id", os.Getenv("NSCALE_REGION_ID"),
					),
					// The attribute the data source exists for: a unit always
					// spans at least one host, so a zero here means the mapping
					// or the filter is wrong.
					resource.TestCheckResourceAttrWith(
						"data.nscale_reservation_unit.test", "hosts_per_unit",
						func(value string) error {
							if value == "0" || value == "" {
								return fmt.Errorf("hosts_per_unit = %q, want a positive count", value)
							}
							return nil
						},
					),
					resource.TestCheckResourceAttrSet(
						"data.nscale_reservation_unit.test", "machine_flavor_id",
					),
					resource.TestCheckResourceAttrSet(
						"data.nscale_reservation_unit.test", "largest_contiguous_unit_count",
					),
				),
			},
		},
	})
}

// An accelerator the region does not offer must fail with the data source's own
// diagnostic rather than returning an empty unit.
func TestAccReservationUnitDataSource_unknownOffering(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccReservationUnitDataSourceConfig("NOTAREALACCELERATOR", "NOTAREALUNIT"),
				ExpectError: regexpReservationUnitNotFound,
			},
		},
	})
}

func testAccReservationUnitDataSourceConfig(accelerator, unit string) string {
	return fmt.Sprintf(`
data "nscale_reservation_unit" "test" {
  accelerator = %[1]q
  unit        = %[2]q
}
`, accelerator, unit)
}
