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

package sshca_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccSSHCertificateAuthorityDataSource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHCertificateAuthorityDataSourceConfig("test-ca-ds", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyForDataSource"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.nscale_ssh_certificate_authority.test", "id"),
					resource.TestCheckResourceAttr("data.nscale_ssh_certificate_authority.test", "name", "test-ca-ds"),
					resource.TestCheckResourceAttr("data.nscale_ssh_certificate_authority.test", "public_key", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyForDataSource"),
					resource.TestCheckResourceAttrSet("data.nscale_ssh_certificate_authority.test", "project_id"),
					resource.TestCheckResourceAttrSet("data.nscale_ssh_certificate_authority.test", "creation_time"),
				),
			},
		},
	})
}

func testAccSSHCertificateAuthorityDataSourceConfig(name, publicKey string) string {
	return fmt.Sprintf(`
resource "nscale_ssh_certificate_authority" "test" {
  name       = %q
  public_key = %q
}

data "nscale_ssh_certificate_authority" "test" {
  id = nscale_ssh_certificate_authority.test.id
}
`, name, publicKey)
}
