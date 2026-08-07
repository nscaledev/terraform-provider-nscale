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

package computecluster

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
)

const testUserDataPlaintext = "#cloud-config\nruncmd:\n  - [ echo, hello ]\n"

func testWorkloadPoolModel(userData types.String) WorkloadPoolModel {
	return WorkloadPoolModel{
		Name:                types.StringValue("pool-1"),
		Replicas:            types.Int64Value(2),
		ImageID:             types.StringValue("image-1"),
		FlavorID:            types.StringValue("flavor-1"),
		UserData:            userData,
		EnablePublicIP:      types.BoolValue(true),
		AllowedAddressPairs: types.SetNull(AllowedAddressPairModelAttributeType),
		FirewallRules:       types.ListNull(FirewallRuleModelAttributeType),
		Machines:            types.ListNull(MachineModelAttributeType),
	}
}

// The SDK serializes the []byte UserData field as base64 itself, so the model
// must hand it the decoded bytes to avoid sending base64(base64(plaintext)).
func TestNscaleWorkloadPoolDecodesUserData(t *testing.T) {
	model := testWorkloadPoolModel(
		types.StringValue(base64.StdEncoding.EncodeToString([]byte(testUserDataPlaintext))),
	)

	workloadPool, diagnostics := model.NscaleWorkloadPool()
	if diagnostics.HasError() {
		t.Fatalf("NscaleWorkloadPool() diagnostics = %v", diagnostics)
	}

	if workloadPool.Machine.UserData == nil {
		t.Fatal("UserData = nil, want decoded bytes")
	}

	if got := string(*workloadPool.Machine.UserData); got != testUserDataPlaintext {
		t.Errorf("UserData = %q, want decoded %q", got, testUserDataPlaintext)
	}
}

func TestNscaleWorkloadPoolRejectsInvalidUserData(t *testing.T) {
	model := testWorkloadPoolModel(types.StringValue("not!valid!base64"))

	if _, diagnostics := model.NscaleWorkloadPool(); !diagnostics.HasError() {
		t.Error("NscaleWorkloadPool() diagnostics = none, want an error for malformed base64")
	}
}

func TestNscaleWorkloadPoolOmitsEmptyUserData(t *testing.T) {
	model := testWorkloadPoolModel(types.StringNull())

	workloadPool, diagnostics := model.NscaleWorkloadPool()
	if diagnostics.HasError() {
		t.Fatalf("NscaleWorkloadPool() diagnostics = %v", diagnostics)
	}

	if workloadPool.Machine.UserData != nil {
		t.Errorf("UserData = %v, want nil", workloadPool.Machine.UserData)
	}
}

// The API returns the decoded user_data bytes; the model must expose them as
// the base64 string that was originally configured, so the value round-trips.
func TestNewWorkloadPoolModelEncodesUserData(t *testing.T) {
	userData := []byte(testUserDataPlaintext)
	imageID := "image-1"
	spec := computeapi.ComputeClusterWorkloadPool{
		Name: "pool-1",
		Machine: computeapi.MachinePool{
			FlavorId: "flavor-1",
			Image:    computeapi.ComputeImage{Id: &imageID},
			Replicas: 2,
			UserData: &userData,
		},
	}

	model := workloadPoolModelFrom(t, NewWorkloadPoolModel(spec, nil))

	want := base64.StdEncoding.EncodeToString(userData)
	if got := model.UserData.ValueString(); got != want {
		t.Errorf("UserData = %q, want base64 %q", got, want)
	}
}

func TestNewWorkloadPoolModelNullUserData(t *testing.T) {
	imageID := "image-1"
	spec := computeapi.ComputeClusterWorkloadPool{
		Name: "pool-1",
		Machine: computeapi.MachinePool{
			FlavorId: "flavor-1",
			Image:    computeapi.ComputeImage{Id: &imageID},
			Replicas: 2,
		},
	}

	if model := workloadPoolModelFrom(t, NewWorkloadPoolModel(spec, nil)); !model.UserData.IsNull() {
		t.Errorf("UserData = %v, want null", model.UserData)
	}
}

func workloadPoolModelFrom(t *testing.T, value attr.Value) WorkloadPoolModel {
	t.Helper()

	object, ok := value.(types.Object)
	if !ok {
		t.Fatalf("NewWorkloadPoolModel() = %T, want types.Object", value)
	}

	var model WorkloadPoolModel
	if diagnostics := object.As(
		context.TODO(),
		&model,
		basetypes.ObjectAsOptions{},
	); diagnostics.HasError() {
		t.Fatalf("object.As() diagnostics = %v", diagnostics)
	}

	return model
}
