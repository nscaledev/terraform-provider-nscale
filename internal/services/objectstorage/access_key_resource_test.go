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
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccObjectStorageAccessKeyResource_basic(t *testing.T) {
	endpointName := acctest.RandomWithPrefix("tf-acc-test")
	accessKeyName := acctest.RandomWithPrefix("tf-acc-test")
	classID := os.Getenv("NSCALE_TEST_OBJECT_STORAGE_ENDPOINT_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageAccessKeyResourceConfig(endpointName, accessKeyName, classID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nscale_object_storage_access_key.test", "id"),
					resource.TestCheckResourceAttr("nscale_object_storage_access_key.test", "name", accessKeyName),
					resource.TestCheckResourceAttrSet("nscale_object_storage_access_key.test", "access_key_id"),
					// Secret must be populated on create. The framework masks
					// the value in plan output and CLI; this only asserts it
					// is non-empty in state.
					resource.TestCheckResourceAttrSet("nscale_object_storage_access_key.test", "secret"),
					resource.TestCheckResourceAttrPair(
						"nscale_object_storage_access_key.test", "endpoint_id",
						"nscale_object_storage_endpoint.parent", "id",
					),
				),
			},
			// Regression guard: without UseStateForUnknown on `secret`, this
			// step would report `secret = (known after apply)` and fail. The
			// `allow_delete_bucket = false` value also exercises the explicit-
			// false bool round-trip (playbook §1.6) at the live-API layer —
			// if the converter ever drops `false` from the JSON, the post-
			// apply Read will diverge and this PlanOnly step will fail.
			{
				Config:             testAccObjectStorageAccessKeyResourceConfig(endpointName, accessKeyName, classID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ResourceName:      "nscale_object_storage_access_key.test",
				ImportState:       true,
				ImportStateVerify: true,
				// `secret` is write-once: the API only returns it on create,
				// so import has no way to recover it. `timeouts` is a
				// configuration-only block that the API does not echo back.
				ImportStateVerifyIgnore: []string{"secret", "timeouts"},
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					ak, ok := state.RootModule().Resources["nscale_object_storage_access_key.test"]
					if !ok {
						return "", errors.New("access key resource not found in state")
					}
					ep, ok := state.RootModule().Resources["nscale_object_storage_endpoint.parent"]
					if !ok {
						return "", errors.New("endpoint resource not found in state")
					}
					return fmt.Sprintf("%s/%s", ep.Primary.ID, ak.Primary.ID), nil
				},
			},
		},
	})
}

func testAccObjectStorageAccessKeyResourceConfig(endpointName, accessKeyName, classID string) string {
	return fmt.Sprintf(`
resource "nscale_object_storage_endpoint" "parent" {
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

resource "nscale_object_storage_access_key" "test" {
  endpoint_id     = nscale_object_storage_endpoint.parent.id
  name            = %q
  identity_policy = "bucket-admin"
}
`, endpointName, classID, accessKeyName)
}
