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

package securitygroup_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccSecurityGroupDataSource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecurityGroupDataSourceConfig("tf-acc-sg-ds"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.nscale_security_group.test", "id"),
					resource.TestCheckResourceAttrPair(
						"data.nscale_security_group.test", "id",
						"nscale_security_group.test", "id",
					),
					resource.TestCheckResourceAttr("data.nscale_security_group.test", "name", "tf-acc-sg-ds"),
					resource.TestCheckResourceAttrPair(
						"data.nscale_security_group.test", "network_id",
						"nscale_network.test", "id",
					),
					resource.TestCheckResourceAttr("data.nscale_security_group.test", "rules.#", "1"),
					resource.TestCheckResourceAttrSet("data.nscale_security_group.test", "region_id"),
					resource.TestCheckResourceAttrSet("data.nscale_security_group.test", "creation_time"),
				),
			},
		},
	})
}

func testAccSecurityGroupDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.243.0/24"
}

resource "nscale_security_group" "test" {
  name = %[1]q

  rules = [
    {
      type      = "ingress"
      protocol  = "tcp"
      from_port = 22
    }
  ]

  network_id = nscale_network.test.id
}

data "nscale_security_group" "test" {
  id = nscale_security_group.test.id
}
`, name)
}
