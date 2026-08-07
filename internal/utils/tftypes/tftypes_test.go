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

package tftypes_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
)

const userDataPlaintext = "#cloud-config\nruncmd:\n  - [ echo, hello ]\n"

// sdkSpec mirrors how the generated SDKs model these fields: a *[]byte that
// encoding/json base64-encodes on the way out.
type sdkSpec struct {
	UserData *[]byte `json:"userData,omitempty"`
}

// This is the assertion that actually pins DX-1814. The other tests check what
// the helpers return; this one checks what reaches the API, which is where the
// double-encoding was visible. Asserting on the decoded bytes alone would pass
// even if our assumption about the SDK's serialization were wrong.
func TestValueBase64BytesPointerSerializesToSinglyEncodedJSON(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(userDataPlaintext))

	userData, diagnostics := tftypes.ValueBase64BytesPointer(types.StringValue(encoded), "user_data")
	if diagnostics.HasError() {
		t.Fatalf("ValueBase64BytesPointer() diagnostics = %v", diagnostics)
	}

	body, err := json.Marshal(sdkSpec{UserData: userData})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	want := fmt.Sprintf(`{"userData":%q}`, encoded)
	if got := string(body); got != want {
		t.Errorf("wire body = %s, want %s", got, want)
	}
}

// The pre-fix code assigned []byte(encoded) directly. Pin the wire value that
// produced so the regression is recognisable if it ever reappears.
func TestDoubleEncodingIsDistinguishableOnTheWire(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(userDataPlaintext))

	doubleEncoded := []byte(encoded)
	body, err := json.Marshal(sdkSpec{UserData: &doubleEncoded})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	singlyEncoded := fmt.Sprintf(`{"userData":%q}`, encoded)
	if string(body) == singlyEncoded {
		t.Fatal(
			"double-encoded and singly-encoded payloads are indistinguishable; " +
				"this test cannot detect the regression",
		)
	}
}

// Read and write are inverses: what the API hands back must render as the
// string the practitioner configured, or every plan shows a spurious diff.
func TestBase64RoundTrip(t *testing.T) {
	for _, plaintext := range []string{
		userDataPlaintext,
		"",
		"a",
		"two\x00binary\xffbytes",
	} {
		t.Run(fmt.Sprintf("%q", plaintext), func(t *testing.T) {
			configured := types.StringValue(base64.StdEncoding.EncodeToString([]byte(plaintext)))

			decoded, diagnostics := tftypes.ValueBase64BytesPointer(configured, "user_data")
			if diagnostics.HasError() {
				t.Fatalf("ValueBase64BytesPointer() diagnostics = %v", diagnostics)
			}

			// An empty payload is omitted entirely rather than sent as "".
			if plaintext == "" {
				if decoded != nil {
					t.Errorf("ValueBase64BytesPointer() = %v, want nil for empty input", decoded)
				}

				return
			}

			if got := tftypes.Base64StringValue(decoded); got != configured {
				t.Errorf("round-trip = %v, want %v", got, configured)
			}
		})
	}
}

func TestValueBase64BytesPointerAbsentValues(t *testing.T) {
	tests := map[string]types.String{
		"null":         types.StringNull(),
		"unknown":      types.StringUnknown(),
		"empty string": types.StringValue(""),
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			userData, diagnostics := tftypes.ValueBase64BytesPointer(value, "user_data")
			if diagnostics.HasError() {
				t.Fatalf("ValueBase64BytesPointer() diagnostics = %v", diagnostics)
			}

			if userData != nil {
				t.Errorf("ValueBase64BytesPointer() = %v, want nil", userData)
			}
		})
	}
}

func TestValueBase64BytesPointerRejectsMalformedInput(t *testing.T) {
	userData, diagnostics := tftypes.ValueBase64BytesPointer(types.StringValue("not!valid!base64"), "user_data")
	if !diagnostics.HasError() {
		t.Fatal("ValueBase64BytesPointer() diagnostics = none, want an error")
	}

	if userData != nil {
		t.Errorf("ValueBase64BytesPointer() = %v, want nil on error", userData)
	}

	if summary := diagnostics.Errors()[0].Summary(); summary != "Invalid user_data" {
		t.Errorf("summary = %q, want %q", summary, "Invalid user_data")
	}
}

func TestBase64StringValueNil(t *testing.T) {
	if got := tftypes.Base64StringValue(nil); !got.IsNull() {
		t.Errorf("Base64StringValue(nil) = %v, want null", got)
	}
}
