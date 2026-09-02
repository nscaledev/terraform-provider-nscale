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

package instance

import (
	"encoding/base64"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	computeapi "github.com/nscaledev/nscale-sdk-go/compute"
)

// The compute API types every spec identifier as a UUID, so these fixtures
// must be parseable — a placeholder like "flavor-1" is now rejected before the
// request is built.
const (
	testFlavorIDString       = "2f8a1d4e-6b3c-4f5a-9e0d-7c1b8a2f3d40"
	testImageIDString        = "5c9b2e7f-1a4d-4b8c-8f2e-3d6a9b0c1e52"
	testNetworkIDString      = "8e3f4a1b-2c5d-4e7f-9a0b-1c2d3e4f5a60"
	testProjectIDString      = "b1c2d3e4-f5a6-4b7c-8d9e-0f1a2b3c4d50"
	testOrganizationIDString = "d4e5f6a7-b8c9-4d0e-9f1a-2b3c4d5e6f70"
)

var (
	testFlavorID = uuid.MustParse(testFlavorIDString)
	testImageID  = uuid.MustParse(testImageIDString)
)

const testUserDataPlaintext = "#cloud-config\nruncmd:\n  - [ echo, hello ]\n"

func testNetworkInterfaceObject() types.Object {
	return types.ObjectValueMust(
		InstanceNetworkInterfaceModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"network_id":           types.StringValue(testNetworkIDString),
			"enable_public_ip":     types.BoolValue(true),
			"security_group_ids":   types.ListNull(types.StringType),
			"allowed_destinations": types.ListNull(types.StringType),
		},
	)
}

func testInstanceModel(userData types.String) InstanceModel {
	return InstanceModel{
		Name:             types.StringValue("test-instance"),
		UserData:         userData,
		NetworkInterface: testNetworkInterfaceObject(),
		ImageID:          types.StringValue(testImageIDString),
		FlavorID:         types.StringValue(testFlavorIDString),
		ProjectID:        types.StringValue(testProjectIDString),
		Tags:             types.MapNull(types.StringType),
	}
}

// The SDK serializes the []byte UserData field as base64 itself, so the model
// must hand it the decoded bytes to avoid sending base64(base64(plaintext)).
func TestNscaleInstanceCreateParamsDecodesUserData(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(testUserDataPlaintext))
	model := testInstanceModel(types.StringValue(encoded))

	params, diagnostics := model.NscaleInstanceCreateParams(testOrganizationIDString)
	if diagnostics.HasError() {
		t.Fatalf("NscaleInstanceCreateParams() diagnostics = %v", diagnostics)
	}

	if params.Spec.UserData == nil {
		t.Fatal("UserData = nil, want decoded bytes")
	}

	if got := string(*params.Spec.UserData); got != testUserDataPlaintext {
		t.Errorf("UserData = %q, want decoded %q", got, testUserDataPlaintext)
	}
}

func TestNscaleInstanceCreateParamsRejectsInvalidUserData(t *testing.T) {
	model := testInstanceModel(types.StringValue("not!valid!base64"))

	if _, diagnostics := model.NscaleInstanceCreateParams(testOrganizationIDString); !diagnostics.HasError() {
		t.Error("NscaleInstanceCreateParams() diagnostics = none, want an error for malformed base64")
	}
}

func TestNscaleInstanceCreateParamsOmitsEmptyUserData(t *testing.T) {
	model := testInstanceModel(types.StringNull())

	params, diagnostics := model.NscaleInstanceCreateParams(testOrganizationIDString)
	if diagnostics.HasError() {
		t.Fatalf("NscaleInstanceCreateParams() diagnostics = %v", diagnostics)
	}

	if params.Spec.UserData != nil {
		t.Errorf("UserData = %v, want nil", params.Spec.UserData)
	}
}

func TestNscaleInstanceUpdateParamsDecodesUserData(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(testUserDataPlaintext))
	model := testInstanceModel(types.StringValue(encoded))

	params, diagnostics := model.NscaleInstanceUpdateParams()
	if diagnostics.HasError() {
		t.Fatalf("NscaleInstanceUpdateParams() diagnostics = %v", diagnostics)
	}

	if params.Spec.UserData == nil {
		t.Fatal("UserData = nil, want decoded bytes")
	}

	if got := string(*params.Spec.UserData); got != testUserDataPlaintext {
		t.Errorf("UserData = %q, want decoded %q", got, testUserDataPlaintext)
	}
}

// The compute API types identifiers as UUIDs, so a malformed one is reported as
// a diagnostic instead of being sent to the API as an opaque string. The summary
// names the resource the identifier refers to, matching nscale.ParseID's
// convention across every other service. All of them are checked in one pass, so
// a config with several bad identifiers reports them together rather than one
// per apply.
func TestNscaleInstanceCreateParamsRejectsNonUUIDIdentifiers(t *testing.T) {
	model := testInstanceModel(types.StringNull())
	model.FlavorID = types.StringValue("flavor-1")
	model.ImageID = types.StringValue("image-1")

	_, diagnostics := model.NscaleInstanceCreateParams(testOrganizationIDString)
	if !diagnostics.HasError() {
		t.Fatal("diagnostics = none, want errors for the malformed identifiers")
	}

	var summaries []string
	for _, diagnostic := range diagnostics.Errors() {
		summaries = append(summaries, diagnostic.Summary())
	}

	for _, want := range []string{"Invalid Flavor ID", "Invalid Image ID"} {
		if !slices.Contains(summaries, want) {
			t.Errorf("diagnostics %v missing %q", summaries, want)
		}
	}
}

// An optional identifier left unset must stay unset rather than becoming the
// zero UUID, which the API would read as a real reference.
func TestNscaleInstanceCreateParamsOmitsUnsetSSHCertificateAuthority(t *testing.T) {
	model := testInstanceModel(types.StringNull())

	params, diagnostics := model.NscaleInstanceCreateParams(testOrganizationIDString)
	if diagnostics.HasError() {
		t.Fatalf("diagnostics = %v", diagnostics)
	}

	if params.Spec.SshCertificateAuthorityId != nil {
		t.Errorf("SshCertificateAuthorityId = %v, want nil", params.Spec.SshCertificateAuthorityId)
	}
}

func TestNscaleInstanceUpdateParamsRejectsInvalidUserData(t *testing.T) {
	model := testInstanceModel(types.StringValue("not!valid!base64"))

	if _, diagnostics := model.NscaleInstanceUpdateParams(); !diagnostics.HasError() {
		t.Error("NscaleInstanceUpdateParams() diagnostics = none, want an error for malformed base64")
	}
}

// The API returns the decoded user_data bytes; the model must expose them as
// the base64 string that was originally configured, so the value round-trips.
func TestNewInstanceModelEncodesUserData(t *testing.T) {
	userData := []byte(testUserDataPlaintext)
	source := &computeapi.InstanceRead{
		Metadata: computeapi.ProjectScopedResourceReadMetadata{
			Id:           "instance-1",
			Name:         "test-instance",
			ProjectId:    "project-1",
			CreationTime: time.Date(2026, time.April, 28, 11, 3, 12, 0, time.UTC),
		},
		Spec: computeapi.InstanceSpec{
			FlavorId:   testFlavorID,
			ImageId:    testImageID,
			Networking: &computeapi.InstanceNetworking{},
			UserData:   &userData,
		},
		Status: computeapi.InstanceStatus{
			NetworkId: "network-1",
			RegionId:  "region-1",
		},
	}

	model := NewInstanceModel(source)

	want := base64.StdEncoding.EncodeToString(userData)
	if got := model.UserData.ValueString(); got != want {
		t.Errorf("UserData = %q, want base64 %q", got, want)
	}
}

func TestNewInstanceModelNullUserData(t *testing.T) {
	source := &computeapi.InstanceRead{
		Metadata: computeapi.ProjectScopedResourceReadMetadata{
			Id:           "instance-1",
			Name:         "test-instance",
			ProjectId:    "project-1",
			CreationTime: time.Date(2026, time.April, 28, 11, 3, 12, 0, time.UTC),
		},
		Spec: computeapi.InstanceSpec{
			FlavorId:   testFlavorID,
			ImageId:    testImageID,
			Networking: &computeapi.InstanceNetworking{},
		},
		Status: computeapi.InstanceStatus{
			NetworkId: "network-1",
			RegionId:  "region-1",
		},
	}

	if model := NewInstanceModel(source); !model.UserData.IsNull() {
		t.Errorf("UserData = %v, want null", model.UserData)
	}
}
