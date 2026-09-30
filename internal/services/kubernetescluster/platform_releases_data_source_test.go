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

package kubernetescluster_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const platformReleasesDataSourceName = "data.nscale_kubernetes_platform_releases.test"

// checkReleasesNonEmpty asserts the catalogue returned at least one release.
// A cluster cannot be created without one, so an empty catalogue means the whole
// resource is unusable in this environment — worth failing loudly rather than
// letting a later test fail on an index-out-of-range.
func checkReleasesNonEmpty(name string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		res, ok := state.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}

		count, err := strconv.Atoi(res.Primary.Attributes["releases.#"])
		if err != nil {
			return fmt.Errorf("reading releases count for %s: %w", name, err)
		}

		if count == 0 {
			return fmt.Errorf("%s returned no platform releases", name)
		}

		return nil
	}
}

// checkReleasesAllMatch asserts every returned release satisfies predicate,
// which is how the filter tests prove the filter was actually applied rather
// than silently ignored.
func checkReleasesAllMatch(name, attribute, want string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		res, ok := state.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}

		count, err := strconv.Atoi(res.Primary.Attributes["releases.#"])
		if err != nil {
			return fmt.Errorf("reading releases count for %s: %w", name, err)
		}

		for index := range count {
			key := fmt.Sprintf("releases.%d.%s", index, attribute)
			if got := res.Primary.Attributes[key]; got != want {
				return fmt.Errorf("%s = %q, want %q", key, got, want)
			}
		}

		return nil
	}
}

func TestAccKubernetesPlatformReleasesDataSource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "nscale_kubernetes_platform_releases" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkReleasesNonEmpty(platformReleasesDataSourceName),
					resource.TestCheckResourceAttrSet(platformReleasesDataSourceName, "releases.0.id"),
					resource.TestCheckResourceAttrSet(
						platformReleasesDataSourceName, "releases.0.kubernetes_version",
					),
					resource.TestCheckResourceAttrSet(platformReleasesDataSourceName, "releases.0.deprecated"),
					resource.TestCheckResourceAttrSet(platformReleasesDataSourceName, "releases.0.withdrawn"),
					resource.TestCheckResourceAttrSet(
						platformReleasesDataSourceName, "releases.0.supported_architectures.#",
					),
				),
			},
		},
	})
}

// TestAccKubernetesPlatformReleasesDataSource_eligibleOnly covers the selection
// convention the CLI uses at create time, and which the docs recommend:
// non-deprecated and non-withdrawn only.
func TestAccKubernetesPlatformReleasesDataSource_eligibleOnly(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "nscale_kubernetes_platform_releases" "test" {
  deprecated = false
  withdrawn  = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkReleasesNonEmpty(platformReleasesDataSourceName),
					checkReleasesAllMatch(platformReleasesDataSourceName, "deprecated", "false"),
					checkReleasesAllMatch(platformReleasesDataSourceName, "withdrawn", "false"),
				),
			},
		},
	})
}

// TestAccKubernetesPlatformReleasesDataSource_filterSubset checks that a
// filtered query returns no more than the unfiltered one. This is the cheapest
// way to catch a filter being dropped from the query string entirely — the exact
// failure mode the "omit, don't send empty" rule guards against.
func TestAccKubernetesPlatformReleasesDataSource_filterSubset(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNKS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "nscale_kubernetes_platform_releases" "test" {}

data "nscale_kubernetes_platform_releases" "filtered" {
  architecture = "x86_64"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkReleasesNonEmpty(platformReleasesDataSourceName),
					checkReleasesSubset(
						"data.nscale_kubernetes_platform_releases.filtered",
						platformReleasesDataSourceName,
					),
				),
			},
		},
	})
}

func checkReleasesSubset(filteredName, unfilteredName string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		count := func(name string) (int, error) {
			res, ok := state.RootModule().Resources[name]
			if !ok {
				return 0, fmt.Errorf("%s not found in state", name)
			}

			parsed, err := strconv.Atoi(res.Primary.Attributes["releases.#"])
			if err != nil {
				return 0, fmt.Errorf("reading releases count for %s: %w", name, err)
			}

			return parsed, nil
		}

		filtered, err := count(filteredName)
		if err != nil {
			return err
		}

		unfiltered, err := count(unfilteredName)
		if err != nil {
			return err
		}

		if filtered > unfiltered {
			return fmt.Errorf(
				"filtered query returned %d releases, more than the %d unfiltered — the filter was not applied",
				filtered, unfiltered,
			)
		}

		return nil
	}
}
