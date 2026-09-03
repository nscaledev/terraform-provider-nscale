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

package reservation_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/nscaledev/terraform-provider-nscale/internal/provider"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"nscale": providerserver.NewProtocol6WithError(provider.New()),
}

func TestMain(m *testing.M) {
	if err := applyReservationTestEnvironment(); err != nil {
		fmt.Fprintf(os.Stderr, "configure reservation acceptance tests: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// applyReservationTestEnvironment keeps reservation-specific infrastructure
// from changing the region and image used by the other acceptance packages.
func applyReservationTestEnvironment() error {
	for _, override := range []struct {
		source string
		target string
	}{
		{source: "NSCALE_TEST_RESERVATION_REGION_ID", target: "NSCALE_REGION_ID"},
		{source: "NSCALE_TEST_RESERVATION_IMAGE_ID", target: "NSCALE_TEST_IMAGE_ID"},
	} {
		value := os.Getenv(override.source)
		if value == "" {
			continue
		}

		if err := os.Setenv(override.target, value); err != nil {
			return fmt.Errorf("set %s from %s: %w", override.target, override.source, err)
		}
	}

	return nil
}

func TestApplyReservationTestEnvironment(t *testing.T) {
	t.Setenv("NSCALE_REGION_ID", "general-region")
	t.Setenv("NSCALE_TEST_IMAGE_ID", "general-image")
	t.Setenv("NSCALE_TEST_RESERVATION_REGION_ID", "reservation-region")
	t.Setenv("NSCALE_TEST_RESERVATION_IMAGE_ID", "reservation-image")

	if err := applyReservationTestEnvironment(); err != nil {
		t.Fatalf("apply reservation test environment: %v", err)
	}

	if got := os.Getenv("NSCALE_REGION_ID"); got != "reservation-region" {
		t.Errorf("NSCALE_REGION_ID = %q, want reservation-region", got)
	}
	if got := os.Getenv("NSCALE_TEST_IMAGE_ID"); got != "reservation-image" {
		t.Errorf("NSCALE_TEST_IMAGE_ID = %q, want reservation-image", got)
	}
}

func TestApplyReservationTestEnvironmentWithoutOverrides(t *testing.T) {
	t.Setenv("NSCALE_REGION_ID", "general-region")
	t.Setenv("NSCALE_TEST_IMAGE_ID", "general-image")
	t.Setenv("NSCALE_TEST_RESERVATION_REGION_ID", "")
	t.Setenv("NSCALE_TEST_RESERVATION_IMAGE_ID", "")

	if err := applyReservationTestEnvironment(); err != nil {
		t.Fatalf("apply reservation test environment: %v", err)
	}

	if got := os.Getenv("NSCALE_REGION_ID"); got != "general-region" {
		t.Errorf("NSCALE_REGION_ID = %q, want general-region", got)
	}
	if got := os.Getenv("NSCALE_TEST_IMAGE_ID"); got != "general-image" {
		t.Errorf("NSCALE_TEST_IMAGE_ID = %q, want general-image", got)
	}
}

// testAccPreCheck skips acceptance tests unless the base credentials are set.
// The provider's Configure step requires a token plus the organization, region,
// and project identifiers before it will construct a client.
func testAccPreCheck(t *testing.T) {
	t.Helper()

	for _, v := range []string{
		"NSCALE_SERVICE_TOKEN",
		"NSCALE_ORGANIZATION_ID",
		"NSCALE_REGION_ID",
		"NSCALE_PROJECT_ID",
	} {
		if os.Getenv(v) == "" {
			t.Skipf("%s must be set for reservation acceptance tests", v)
		}
	}
}

// testAccPreCheckReservation additionally requires the capacity shape to
// reserve. Reservation units are region-specific platform capacity offerings
// that the test cannot create, so the accelerator/unit must reference a shape
// the target region actually offers.
func testAccPreCheckReservation(t *testing.T) {
	t.Helper()

	testAccPreCheck(t)

	for _, v := range []string{
		"NSCALE_TEST_RESERVATION_ACCELERATOR",
		"NSCALE_TEST_RESERVATION_UNIT",
	} {
		if os.Getenv(v) == "" {
			t.Skipf("%s must be set for reservation acceptance tests", v)
		}
	}
}

// testAccPreCheckPlacement additionally requires an image for the pinned
// servers, on top of the reservation capacity shape.
func testAccPreCheckPlacement(t *testing.T) {
	t.Helper()

	testAccPreCheckReservation(t)

	if os.Getenv("NSCALE_TEST_IMAGE_ID") == "" {
		t.Skipf("NSCALE_TEST_IMAGE_ID must be set for placement acceptance tests")
	}
}
