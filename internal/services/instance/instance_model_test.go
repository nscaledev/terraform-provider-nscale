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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	computeapi "github.com/nscaledev/nscale-sdk-go/compute"
)

const (
	testUserDataPlaintext = "#cloud-config\nruncmd:\n  - [ echo, hello ]\n"
	testOrganizationID    = "11111111-1111-1111-1111-111111111111"
	testProjectID         = "22222222-2222-2222-2222-222222222222"
	testNetworkID         = "33333333-3333-3333-3333-333333333333"
	testImageID           = "44444444-4444-4444-4444-444444444444"
	testFlavorID          = "55555555-5555-5555-5555-555555555555"
)

func testNetworkInterfaceObject() types.Object {
	return types.ObjectValueMust(
		InstanceNetworkInterfaceModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"network_id":           types.StringValue(testNetworkID),
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
		ImageID:          types.StringValue(testImageID),
		FlavorID:         types.StringValue(testFlavorID),
		ProjectID:        types.StringValue(testProjectID),
		Tags:             types.MapNull(types.StringType),
	}
}

// The SDK serializes the []byte UserData field as base64 itself, so the model
// must hand it the decoded bytes to avoid sending base64(base64(plaintext)).
func TestNscaleInstanceCreateParamsDecodesUserData(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(testUserDataPlaintext))
	model := testInstanceModel(types.StringValue(encoded))

	params, diagnostics := model.NscaleInstanceCreateParams(testOrganizationID)
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

	if _, diagnostics := model.NscaleInstanceCreateParams(testOrganizationID); !diagnostics.HasError() {
		t.Error("NscaleInstanceCreateParams() diagnostics = none, want an error for malformed base64")
	}
}

func TestNscaleInstanceCreateParamsOmitsEmptyUserData(t *testing.T) {
	model := testInstanceModel(types.StringNull())

	params, diagnostics := model.NscaleInstanceCreateParams(testOrganizationID)
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
			FlavorId:   uuid.MustParse(testFlavorID),
			ImageId:    uuid.MustParse(testImageID),
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
			FlavorId:   uuid.MustParse(testFlavorID),
			ImageId:    uuid.MustParse(testImageID),
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
