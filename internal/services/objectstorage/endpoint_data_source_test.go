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

func TestAccObjectStorageEndpointDataSource_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")
	classID := os.Getenv("NSCALE_TEST_OBJECT_STORAGE_ENDPOINT_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageEndpointDataSourceConfig(name, classID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.nscale_object_storage_endpoint.lookup", "id",
						"nscale_object_storage_endpoint.test", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_object_storage_endpoint.lookup", "name",
						"nscale_object_storage_endpoint.test", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_object_storage_endpoint.lookup", "endpoint_class_id",
						"nscale_object_storage_endpoint.test", "endpoint_class_id",
					),
					resource.TestCheckResourceAttrPair(
						"data.nscale_object_storage_endpoint.lookup", "region_id",
						"nscale_object_storage_endpoint.test", "region_id",
					),
				),
			},
		},
	})
}

func testAccObjectStorageEndpointDataSourceConfig(name, classID string) string {
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

data "nscale_object_storage_endpoint" "lookup" {
  id = nscale_object_storage_endpoint.test.id
}
`, name, classID)
}
