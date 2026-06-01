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

package identity_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccProjectDataSource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	groupName := acctest.RandomWithPrefix("tf-acc-test")
	roleID := os.Getenv("NSCALE_TEST_ROLE_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectDataSourceConfig(name, groupName, roleID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.nscale_identity_project.test", "id"),
					resource.TestCheckResourceAttr("data.nscale_identity_project.test", "name", name),
					resource.TestCheckResourceAttr("data.nscale_identity_project.test", "group_ids.#", "1"),
					resource.TestCheckResourceAttrSet("data.nscale_identity_project.test", "creation_time"),
				),
			},
		},
	})
}

func testAccProjectDataSourceConfig(name, groupName, roleID string) string {
	return fmt.Sprintf(`
resource "nscale_identity_group" "test" {
  name     = %q
  role_ids = [%q]
}

resource "nscale_identity_project" "test" {
  name      = %q
  group_ids = [nscale_identity_group.test.id]
}

data "nscale_identity_project" "test" {
  id = nscale_identity_project.test.id
}
`, groupName, roleID, name)
}
