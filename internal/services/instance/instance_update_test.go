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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	computeapi "github.com/nscaledev/nscale-sdk-go/compute"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

func TestInstanceUpdateSecurityGroupPreservesBootState(t *testing.T) {
	gzipUserData := []byte{0x1f, 0x8b, 0x08, 0x00, 0x9a, 0x42, 0x7c, 0x65}
	model := InstanceResourceModel{
		InstanceModel: testInstanceModel(types.StringValue(base64.StdEncoding.EncodeToString(gzipUserData))),
	}
	model.Description = types.StringValue("complete update")
	model.Tags = types.MapValueMust(types.StringType, map[string]attr.Value{
		"example": types.StringValue("tag"),
	})
	model.NetworkInterface = types.ObjectValueMust(
		InstanceNetworkInterfaceModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"network_id":       types.StringValue(testNetworkIDString),
			"enable_public_ip": types.BoolValue(true),
			"security_group_ids": types.ListValueMust(
				types.StringType,
				[]attr.Value{types.StringValue("0a2b3c4d-5e6f-4789-a0b1-c2d3e4f5a6b7")},
			),
			"allowed_destinations": types.ListValueMust(
				types.StringType,
				[]attr.Value{types.StringValue("192.0.2.0/24")},
			),
		},
	)

	var received computeapi.InstanceUpdate
	server := newInstanceUpdateServer(t, &received)
	t.Cleanup(server.Close)

	compute, err := computeapi.NewClient(server.URL)
	if err != nil {
		t.Fatalf("new compute client: %v", err)
	}

	if _, diagnostics := instanceUpdate(
		t.Context(),
		&nscale.Client{Compute: compute},
		"a1b2c3d4-e5f6-4789-a0b1-c2d3e4f5a6b7",
		model,
	); diagnostics.HasError() {
		t.Fatalf("instanceUpdate() diagnostics = %v", diagnostics)
	}

	assertCompleteInstanceUpdate(t, received, model, gzipUserData)
}

func newInstanceUpdateServer(t *testing.T, received *computeapi.InstanceUpdate) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(received); err != nil {
			t.Errorf("decode update: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
}

func assertCompleteInstanceUpdate(
	t *testing.T,
	received computeapi.InstanceUpdate,
	model InstanceResourceModel,
	gzipUserData []byte,
) {
	t.Helper()

	if received.Metadata.Name != model.Name.ValueString() {
		t.Errorf("name = %q, want %q", received.Metadata.Name, model.Name.ValueString())
	}
	if got := received.Metadata.Description; got == nil || *got != model.Description.ValueString() {
		t.Errorf("description = %v, want %q", got, model.Description.ValueString())
	}
	if !hasExampleTag(received.Metadata.Tags) {
		t.Errorf("tags = %#v, want complete desired tags", received.Metadata.Tags)
	}
	if received.Spec.FlavorId != testFlavorID || received.Spec.ImageId != testImageID {
		t.Errorf(
			"boot IDs = %s/%s, want %s/%s",
			received.Spec.FlavorId,
			received.Spec.ImageId,
			testFlavorID,
			testImageID,
		)
	}
	if received.Spec.UserData == nil || !bytes.Equal(*received.Spec.UserData, gzipUserData) {
		t.Errorf("user data = %v, want opaque gzip bytes %v", received.Spec.UserData, gzipUserData)
	}
	assertCompleteInstanceNetworking(t, received.Spec.Networking)
}

func hasExampleTag(tags *computeapi.TagList) bool {
	if tags == nil {
		return false
	}

	for _, tag := range *tags {
		if tag.Name == "example" && tag.Value == "tag" {
			return true
		}
	}

	return false
}

func assertCompleteInstanceNetworking(t *testing.T, networking *computeapi.InstanceNetworking) {
	t.Helper()

	if networking == nil || networking.SecurityGroups == nil || len(*networking.SecurityGroups) != 1 {
		t.Fatalf("networking = %#v, want complete security-group state", networking)
	}
	if got := (*networking.SecurityGroups)[0]; got != "0a2b3c4d-5e6f-4789-a0b1-c2d3e4f5a6b7" {
		t.Errorf("security group = %q, want complete desired value", got)
	}
	if networking.AllowedSourceAddresses == nil || len(*networking.AllowedSourceAddresses) != 1 ||
		(*networking.AllowedSourceAddresses)[0] != "192.0.2.0/24" {
		t.Errorf("allowed destinations = %#v, want complete desired value", networking.AllowedSourceAddresses)
	}
}
