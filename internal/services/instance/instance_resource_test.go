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

const testAccCAPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyForInstanceAcceptanceTests"

func TestAccInstanceResource_withSSHCertificateAuthority(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccInstanceResourceConfigWithSSHCA(
					"tf-acc-instance-ca",
					os.Getenv("NSCALE_TEST_IMAGE_ID"),
					os.Getenv("NSCALE_TEST_FLAVOR_ID"),
					testAccCAPublicKey,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nscale_instance.test", "id"),
					resource.TestCheckResourceAttr("nscale_instance.test", "name", "tf-acc-instance-ca"),
					resource.TestCheckResourceAttrSet("nscale_instance.test", "ssh_certificate_authority_id"),
					resource.TestCheckResourceAttrPair(
						"nscale_instance.test", "ssh_certificate_authority_id",
						"nscale_ssh_certificate_authority.test", "id",
					),
					resource.TestCheckResourceAttrSet("nscale_instance.test", "region_id"),
					resource.TestCheckResourceAttrSet("nscale_instance.test", "creation_time"),
				),
			},
			{
				ResourceName:            "nscale_instance.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func testAccInstanceResourceConfigWithSSHCA(name, imageID, flavorID, caPublicKey string) string {
	return fmt.Sprintf(`
resource "nscale_ssh_certificate_authority" "test" {
  name       = "%[1]s-ca"
  public_key = %[4]q
}

resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.240.0/24"
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

data "nscale_instance_flavor" "test" {
  id = %[3]q
}

resource "nscale_instance" "test" {
  name = %[1]q

  network_interface {
    network_id         = nscale_network.test.id
    security_group_ids = [nscale_security_group.test.id]
  }

  image_id                     = %[2]q
  flavor_id                    = data.nscale_instance_flavor.test.id
  ssh_certificate_authority_id = nscale_ssh_certificate_authority.test.id
}
`, name, imageID, flavorID, caPublicKey)
}
