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

// TestAccFileStorageResource_emptySnapshotPolicySet covers the explicit
// empty user-managed Snapshot Policy Set: creating with `snapshot_policies = []`
// persists no user-managed policies and, because the test step asserts an empty
// plan after apply, proves the explicit empty set produces no post-apply diff.
// An import round-trip then confirms the empty set is adopted from the API.
func TestAccFileStorageResource_emptySnapshotPolicySet(t *testing.T) {
	storageClassID := os.Getenv("NSCALE_TEST_FILE_STORAGE_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFileStorageResourceConfigEmptySnapshotPolicies(
					"tf-acc-file-storage-empty-policies",
					storageClassID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_file_storage.test", "snapshot_policies.#", "0"),
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

// TestAccFileStorageResource_customSnapshotPolicy covers the non-empty
// single-custom-policy path end-to-end. Creating with one user-managed daily
// policy persists its schedule and retention; the plan-only step that follows
// proves the policy round-trips with no post-apply diff, including the schedule
// fields a daily cadence leaves unset. Changing the same-named policy's
// schedule and retention updates the File Storage in place — storage_class_id
// is untouched, so the parent is not replaced — and a second plan-only step
// guards the update round-trip. Finally an import adopts the policy from the API.
func TestAccFileStorageResource_customSnapshotPolicy(t *testing.T) {
	storageClassID := os.Getenv("NSCALE_TEST_FILE_STORAGE_CLASS_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFileStorageResourceConfigCustomSnapshotPolicy(
					"tf-acc-file-storage-custom-policy", storageClassID, "02:00Z", 7,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_file_storage.test", "snapshot_policies.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs(
						"nscale_file_storage.test", "snapshot_policies.*", map[string]string{
							"name":                 "daily",
							"schedule.interval":    "daily",
							"schedule.time_of_day": "02:00Z",
							"retention.keep":       "7",
						},
					),
				),
			},
			// Plan-only no-op: proves the single custom policy round-trips with no
			// post-apply diff, including the schedule fields a daily cadence leaves
			// unset (day_of_week, day_of_month).
			{
				Config: testAccFileStorageResourceConfigCustomSnapshotPolicy(
					"tf-acc-file-storage-custom-policy", storageClassID, "02:00Z", 7,
				),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				// Change the same-named policy's schedule (time of day) and
				// retention (keep). Because storage_class_id is unchanged this is
				// an in-place update of the parent File Storage, not a replacement.
				Config: testAccFileStorageResourceConfigCustomSnapshotPolicy(
					"tf-acc-file-storage-custom-policy", storageClassID, "03:00Z", 14,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_file_storage.test", "snapshot_policies.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs(
						"nscale_file_storage.test", "snapshot_policies.*", map[string]string{
							"name":                 "daily",
							"schedule.interval":    "daily",
							"schedule.time_of_day": "03:00Z",
							"retention.keep":       "14",
						},
					),
				),
			},
			{
				Config: testAccFileStorageResourceConfigCustomSnapshotPolicy(
					"tf-acc-file-storage-custom-policy", storageClassID, "03:00Z", 14,
				),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
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

func testAccFileStorageResourceConfigCustomSnapshotPolicy(
	name, storageClassID, timeOfDay string,
	keep int,
) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.243.0/24"
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
        time_of_day = %[3]q
      }
      retention = {
        keep = %[4]d
      }
    }
  ]

  network {
    id = nscale_network.test.id
  }
}
`, name, storageClassID, timeOfDay, keep)
}

func testAccFileStorageResourceConfigEmptySnapshotPolicies(name, storageClassID string) string {
	return fmt.Sprintf(`
resource "nscale_network" "test" {
  name       = "%[1]s-net"
  cidr_block = "192.168.244.0/24"
}

resource "nscale_file_storage" "test" {
  name              = %[1]q
  storage_class_id  = %[2]q
  capacity          = 20
  root_squash       = true
  snapshot_policies = []

  network {
    id = nscale_network.test.id
  }
}
`, name, storageClassID)
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
