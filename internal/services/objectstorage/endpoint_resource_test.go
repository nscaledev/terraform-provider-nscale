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

package objectstorage_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccObjectStorageEndpointResource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	classID := os.Getenv("NSCALE_TEST_OBJECT_STORAGE_ENDPOINT_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageEndpointResourceConfig(name, classID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nscale_object_storage_endpoint.test", "id"),
					resource.TestCheckResourceAttr("nscale_object_storage_endpoint.test", "name", name),
					resource.TestCheckResourceAttr("nscale_object_storage_endpoint.test", "endpoint_class_id", classID),
					resource.TestCheckResourceAttrSet("nscale_object_storage_endpoint.test", "region_id"),
					resource.TestCheckResourceAttrSet("nscale_object_storage_endpoint.test", "project_id"),
					resource.TestCheckResourceAttrSet("nscale_object_storage_endpoint.test", "creation_time"),
					resource.TestCheckResourceAttr("nscale_object_storage_endpoint.test", "identity_policies.#", "1"),
					resource.TestCheckResourceAttr(
						"nscale_object_storage_endpoint.test",
						"identity_policies.0.name",
						"bucket-admin",
					),
				),
			},
			// Plan-only step verifies that running plan immediately after apply
			// is a no-op. This catches drift in the JSON-document plan modifier
			// or in computed-but-unknown attributes losing their state value.
			{
				Config:             testAccObjectStorageEndpointResourceConfig(name, classID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ResourceName:      "nscale_object_storage_endpoint.test",
				ImportState:       true,
				ImportStateVerify: true,
				// `timeouts` is a configuration-only block; the API does not
				// echo it back so import cannot reconstruct it.
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func TestAccObjectStorageEndpointResource_update(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	renamed := acctest.RandomWithPrefix("tf-acc-test")
	classID := os.Getenv("NSCALE_TEST_OBJECT_STORAGE_ENDPOINT_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageEndpointResourceConfig(name, classID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_object_storage_endpoint.test", "name", name),
					resource.TestCheckResourceAttr(
						"nscale_object_storage_endpoint.test",
						"identity_policies.0.name",
						"bucket-admin",
					),
				),
			},
			{
				Config: testAccObjectStorageEndpointResourceConfigUpdated(renamed, classID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_object_storage_endpoint.test", "name", renamed),
					resource.TestCheckResourceAttr(
						"nscale_object_storage_endpoint.test",
						"identity_policies.0.name",
						"bucket-readonly",
					),
				),
			},
			// Plan-only step after update: the post-update plan must be a
			// no-op. This is the regression guard for in-place mutation paths
			// (identity policy replacement, name/description updates) — if any
			// converter or plan modifier mishandles the round-trip we'll see
			// a non-empty plan here.
			{
				Config:             testAccObjectStorageEndpointResourceConfigUpdated(renamed, classID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func testAccObjectStorageEndpointResourceConfig(name, classID string) string {
	return fmt.Sprintf(`
resource "nscale_object_storage_endpoint" "test" {
  name              = %q
  endpoint_class_id = %q

  identity_policies = [
    {
      name = "bucket-admin"
      document = jsonencode({
        Version = "2012-10-17"
        Statement = [{
          Effect   = "Allow"
          Action   = ["s3:*"]
          Resource = ["*"]
        }]
      })
    }
  ]
}
`, name, classID)
}

func testAccObjectStorageEndpointResourceConfigUpdated(name, classID string) string {
	return fmt.Sprintf(`
resource "nscale_object_storage_endpoint" "test" {
  name              = %q
  endpoint_class_id = %q

  identity_policies = [
    {
      name = "bucket-readonly"
      document = jsonencode({
        Version = "2012-10-17"
        Statement = [{
          Effect   = "Allow"
          Action   = ["s3:GetObject", "s3:ListBucket"]
          Resource = ["*"]
        }]
      })
    }
  ]
}
`, name, classID)
}
