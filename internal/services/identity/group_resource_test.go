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

func TestAccGroupResource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	roleID := os.Getenv("NSCALE_TEST_ROLE_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccGroupResourceConfig(name, roleID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nscale_identity_group.test", "id"),
					resource.TestCheckResourceAttr("nscale_identity_group.test", "name", name),
					resource.TestCheckResourceAttr("nscale_identity_group.test", "role_ids.#", "1"),
					resource.TestCheckTypeSetElemAttr("nscale_identity_group.test", "role_ids.*", roleID),
					resource.TestCheckResourceAttrSet("nscale_identity_group.test", "creation_time"),
				),
			},
			{
				Config:             testAccGroupResourceConfig(name, roleID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ResourceName:      "nscale_identity_group.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccGroupResource_update(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	roleID := os.Getenv("NSCALE_TEST_ROLE_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccGroupResourceConfig(name, roleID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_identity_group.test", "name", name),
				),
			},
			{
				Config: testAccGroupResourceConfigUpdated(name, roleID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_identity_group.test", "description", "updated description"),
					resource.TestCheckResourceAttr("nscale_identity_group.test", "tags.team", "platform"),
				),
			},
			{
				Config:             testAccGroupResourceConfigUpdated(name, roleID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func testAccGroupResourceConfig(name, roleID string) string {
	// user_ids is set to an explicit empty list to guard the empty-set
	// round-trip (the API returns [] not null; the model must preserve it).
	return fmt.Sprintf(`
resource "nscale_identity_group" "test" {
  name     = %q
  role_ids = [%q]
  user_ids = []
}
`, name, roleID)
}

func testAccGroupResourceConfigUpdated(name, roleID string) string {
	return fmt.Sprintf(`
resource "nscale_identity_group" "test" {
  name        = %q
  description = "updated description"
  role_ids    = [%q]

  tags = {
    team = "platform"
  }
}
`, name, roleID)
}
