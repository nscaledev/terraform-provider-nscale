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

package network_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccNetworkResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNetworkResourceConfig("tf-acc-network", "192.168.240.0/24"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nscale_network.test", "id"),
					resource.TestCheckResourceAttr("nscale_network.test", "name", "tf-acc-network"),
					resource.TestCheckResourceAttr("nscale_network.test", "cidr_block", "192.168.240.0/24"),
					resource.TestCheckResourceAttrSet("nscale_network.test", "project_id"),
					resource.TestCheckResourceAttrSet("nscale_network.test", "region_id"),
					resource.TestCheckResourceAttrSet("nscale_network.test", "creation_time"),
				),
			},
			{
				// Update the description and DNS nameservers in place; cidr_block is
				// unchanged so this must not trigger a replacement.
				Config: testAccNetworkResourceConfigUpdated("tf-acc-network", "192.168.240.0/24", "Updated network"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_network.test", "description", "Updated network"),
					resource.TestCheckResourceAttr("nscale_network.test", "dns_nameservers.#", "2"),
					resource.TestCheckResourceAttr("nscale_network.test", "dns_nameservers.0", "8.8.8.8"),
					resource.TestCheckResourceAttr("nscale_network.test", "dns_nameservers.1", "8.8.4.4"),
				),
			},
			{
				ResourceName:            "nscale_network.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func testAccNetworkResourceConfig(name, cidr string) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name       = %q
  cidr_block = %q
}
`, name, cidr)
}

func testAccNetworkResourceConfigUpdated(name, cidr, description string) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name            = %q
  cidr_block      = %q
  description     = %q
  dns_nameservers = ["8.8.8.8", "8.8.4.4"]
}
`, name, cidr, description)
}
