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

func TestAccFileStorageResource_basic(t *testing.T) {
	storageClassID := os.Getenv("NSCALE_TEST_FILE_STORAGE_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFileStorageResourceConfig("tf-acc-file-storage", storageClassID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nscale_file_storage.test", "id"),
					resource.TestCheckResourceAttr("nscale_file_storage.test", "name", "tf-acc-file-storage"),
					resource.TestCheckResourceAttr("nscale_file_storage.test", "storage_class_id", storageClassID),
					resource.TestCheckResourceAttr("nscale_file_storage.test", "capacity", "20"),
					resource.TestCheckResourceAttr("nscale_file_storage.test", "root_squash", "true"),
					// Omitted: Terraform reads back the resolved platform default.
					resource.TestCheckResourceAttrSet(
						"nscale_file_storage.test", "default_snapshot_protection_enabled",
					),
					resource.TestCheckResourceAttrPair(
						"nscale_file_storage.test", "network.0.id",
						"nscale_network.test", "id",
					),
					resource.TestCheckResourceAttrSet("nscale_file_storage.test", "project_id"),
					resource.TestCheckResourceAttrSet("nscale_file_storage.test", "region_id"),
					resource.TestCheckResourceAttrSet("nscale_file_storage.test", "creation_time"),
				),
			},
			{
				// Update the description in place; storage_class_id is unchanged so
				// this must not trigger a replacement.
				Config: testAccFileStorageResourceConfigUpdated(
					"tf-acc-file-storage",
					storageClassID,
					"Updated file storage",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_file_storage.test", "description", "Updated file storage"),
				),
			},
			{
				ResourceName:            "nscale_file_storage.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts", "refresh_usage"},
			},
		},
	})
}

// TestAccFileStorageResource_defaultSnapshotProtection covers explicitly
// managing Default Snapshot Protection: creating with it disabled persists the
// configured value, and an import round-trip adopts the same remote value.
func TestAccFileStorageResource_defaultSnapshotProtection(t *testing.T) {
	storageClassID := os.Getenv("NSCALE_TEST_FILE_STORAGE_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFileStorageResourceConfigDefaultSnapshotProtection(
					"tf-acc-file-storage-dsp",
					storageClassID,
					false,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"nscale_file_storage.test", "default_snapshot_protection_enabled", "false",
					),
				),
			},
			{
				ResourceName:            "nscale_file_storage.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts", "refresh_usage"},
			},
		},
	})
}

func testAccFileStorageResourceConfigDefaultSnapshotProtection(
	name, storageClassID string,
	defaultSnapshotProtectionEnabled bool,
) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.244.0/24"
}

resource "nscale_file_storage" "test" {
  name                                = %[1]q
  storage_class_id                    = %[2]q
  capacity                            = 20
  root_squash                         = true
  default_snapshot_protection_enabled = %[3]t

  network {
    id = nscale_network.test.id
  }
}
`, name, storageClassID, defaultSnapshotProtectionEnabled)
}

func testAccFileStorageResourceConfig(name, storageClassID string) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.244.0/24"
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
`, name, storageClassID)
}

func testAccFileStorageResourceConfigUpdated(name, storageClassID, description string) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.244.0/24"
}

resource "nscale_file_storage" "test" {
  name             = %[1]q
  description      = %[3]q
  storage_class_id = %[2]q
  capacity         = 20
  root_squash      = true

  network {
    id = nscale_network.test.id
  }
}
`, name, storageClassID, description)
}
