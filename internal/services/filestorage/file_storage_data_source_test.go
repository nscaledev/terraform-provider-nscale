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

package filestorage_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccFileStorageDataSource_basic(t *testing.T) {
	storageClassID := os.Getenv("NSCALE_TEST_FILE_STORAGE_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFileStorageDataSourceConfig("tf-acc-file-storage-ds", storageClassID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.nscale_file_storage.test", "id"),
					resource.TestCheckResourceAttrPair(
						"data.nscale_file_storage.test", "id",
						"nscale_file_storage.test", "id",
					),
					resource.TestCheckResourceAttr("data.nscale_file_storage.test", "name", "tf-acc-file-storage-ds"),
					resource.TestCheckResourceAttr("data.nscale_file_storage.test", "storage_class_id", storageClassID),
					resource.TestCheckResourceAttr("data.nscale_file_storage.test", "capacity", "20"),
					resource.TestCheckResourceAttr("data.nscale_file_storage.test", "root_squash", "true"),
					resource.TestCheckResourceAttrSet(
						"data.nscale_file_storage.test", "default_snapshot_protection_enabled",
					),
					// The storage is created without user-managed policies, so the
					// data source exposes an empty user-managed Snapshot Policy Set.
					resource.TestCheckResourceAttr("data.nscale_file_storage.test", "snapshot_policies.#", "0"),
					resource.TestCheckResourceAttrSet("data.nscale_file_storage.test", "project_id"),
					resource.TestCheckResourceAttrSet("data.nscale_file_storage.test", "region_id"),
					resource.TestCheckResourceAttrSet("data.nscale_file_storage.test", "creation_time"),
				),
			},
		},
	})
}

// TestAccFileStorageDataSource_customSnapshotPolicy verifies the data source
// reads and exposes a single user-managed snapshot policy, with its schedule
// and retention detail, as computed values.
func TestAccFileStorageDataSource_customSnapshotPolicy(t *testing.T) {
	storageClassID := os.Getenv("NSCALE_TEST_FILE_STORAGE_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFileStorageDataSourceConfigCustomSnapshotPolicy(
					"tf-acc-file-storage-ds-policy", storageClassID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.nscale_file_storage.test", "snapshot_policies.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.nscale_file_storage.test", "snapshot_policies.*", map[string]string{
							"name":                 "daily",
							"schedule.interval":    "daily",
							"schedule.time_of_day": "02:00Z",
							"retention.keep":       "7",
						},
					),
				),
			},
		},
	})
}

func testAccFileStorageDataSourceConfigCustomSnapshotPolicy(name, storageClassID string) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.246.0/24"
}

resource "nscale_file_storage" "test" {
  name             = %[1]q
  storage_class_id = %[2]q
  capacity         = 20
  root_squash      = true

  snapshot_policies = [
    {
      name = "daily"
      schedule = {
        interval    = "daily"
        time_of_day = "02:00Z"
      }
      retention = {
        keep = 7
      }
    }
  ]

  network {
    id = nscale_network.test.id
  }
}

data "nscale_file_storage" "test" {
  id = nscale_file_storage.test.id
}
`, name, storageClassID)
}

func testAccFileStorageDataSourceConfig(name, storageClassID string) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.245.0/24"
}

resource "nscale_file_storage" "test" {
  name             = %[1]q
  storage_class_id = %[2]q
  capacity         = 20
  root_squash      = true

  network {
    id = nscale_network.test.id
  }
}

data "nscale_file_storage" "test" {
  id = nscale_file_storage.test.id
}
`, name, storageClassID)
}
