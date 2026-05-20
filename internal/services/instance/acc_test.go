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

package instance_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/nscaledev/terraform-provider-nscale/internal/provider"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"nscale": providerserver.NewProtocol6WithError(provider.New()),
}

// testAccPreCheck skips acceptance tests unless all required environment
// variables are set. NSCALE_TEST_IMAGE_ID and NSCALE_TEST_FLAVOR_ID identify
// live catalog entries in the target project's region, since instance creation
// cannot be faked end-to-end without them.
func testAccPreCheck(t *testing.T) {
	t.Helper()

	for _, v := range []string{
		"NSCALE_SERVICE_TOKEN",
		"NSCALE_REGION_ID",
		"NSCALE_ORGANIZATION_ID",
		"NSCALE_PROJECT_ID",
		"NSCALE_TEST_IMAGE_ID",
		"NSCALE_TEST_FLAVOR_ID",
	} {
		if os.Getenv(v) == "" {
			t.Skipf("%s must be set for instance acceptance tests", v)
		}
	}
}
