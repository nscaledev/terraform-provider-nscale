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

package validators

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// runStringValidator drives a string validator the way the framework does and
// returns the diagnostics it produced, so each table case can assert on them.
func runStringValidator(v validator.String, value types.String) validator.StringResponse {
	request := validator.StringRequest{
		Path:        path.Root("test"),
		ConfigValue: value,
	}

	response := validator.StringResponse{}
	v.ValidateString(context.Background(), request, &response)

	return response
}

func TestBase64Validator(t *testing.T) {
	testCases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"valid", types.StringValue(base64.StdEncoding.EncodeToString([]byte("hello world"))), false},
		{"empty string is valid base64", types.StringValue(""), false},
		{"invalid characters", types.StringValue("not valid base64!!!"), true},
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response := runStringValidator(Base64Validator{}, testCase.value)

			if got := response.Diagnostics.HasError(); got != testCase.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, testCase.wantErr, response.Diagnostics)
			}

			if testCase.wantErr && response.Diagnostics[0].Summary() != "Invalid Base64 Encoded String" {
				t.Errorf("summary = %q, want %q", response.Diagnostics[0].Summary(), "Invalid Base64 Encoded String")
			}
		})
	}
}

func TestCIDRValidator(t *testing.T) {
	testCases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"valid ipv4 cidr", types.StringValue("10.0.0.0/24"), false},
		{"valid ipv6 cidr", types.StringValue("2001:db8::/32"), false},
		{"bare ip is not cidr", types.StringValue("10.0.0.1"), true},
		{"garbage", types.StringValue("not-a-cidr"), true},
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response := runStringValidator(CIDRValidator{}, testCase.value)

			if got := response.Diagnostics.HasError(); got != testCase.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, testCase.wantErr, response.Diagnostics)
			}

			if testCase.wantErr && response.Diagnostics[0].Summary() != "Invalid CIDR Notation" {
				t.Errorf("summary = %q, want %q", response.Diagnostics[0].Summary(), "Invalid CIDR Notation")
			}
		})
	}
}

func TestIPAddressValidator(t *testing.T) {
	testCases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"valid ipv4", types.StringValue("192.168.1.1"), false},
		{"valid ipv6", types.StringValue("2001:db8::1"), false},
		{"cidr is not a bare ip", types.StringValue("10.0.0.0/24"), true},
		{"garbage", types.StringValue("999.999.999.999"), true},
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response := runStringValidator(IPAddressValidator{}, testCase.value)

			if got := response.Diagnostics.HasError(); got != testCase.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, testCase.wantErr, response.Diagnostics)
			}

			if testCase.wantErr && response.Diagnostics[0].Summary() != "Invalid IP Address" {
				t.Errorf("summary = %q, want %q", response.Diagnostics[0].Summary(), "Invalid IP Address")
			}
		})
	}
}

func TestNameValidator(t *testing.T) {
	testCases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"simple lowercase", types.StringValue("name"), false},
		{"with digits and hyphens", types.StringValue("my-name-01"), false},
		{"single character", types.StringValue("a"), false},
		{"max length 63", types.StringValue("a" + strings.Repeat("b", 62)), false},
		{"too long 64", types.StringValue("a" + strings.Repeat("b", 63)), true},
		{"leading digit", types.StringValue("1name"), true},
		{"uppercase", types.StringValue("Name"), true},
		{"underscore", types.StringValue("my_name"), true},
		{"trailing hyphen", types.StringValue("name-"), true},
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response := runStringValidator(NameValidator(), testCase.value)

			if got := response.Diagnostics.HasError(); got != testCase.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, testCase.wantErr, response.Diagnostics)
			}
		})
	}
}

func TestNoReservedPrefixValidator(t *testing.T) {
	const prefix = "nscale-"

	testCases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"no prefix", types.StringValue("my-resource"), false},
		{"contains but not prefix", types.StringValue("my-nscale-resource"), false},
		{"exact prefix", types.StringValue("nscale-resource"), true},
		{"prefix only", types.StringValue("nscale-"), true},
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response := runStringValidator(NoReservedPrefix(prefix), testCase.value)

			if got := response.Diagnostics.HasError(); got != testCase.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, testCase.wantErr, response.Diagnostics)
			}

			if testCase.wantErr && response.Diagnostics[0].Summary() != "Reserved Prefix Not Allowed" {
				t.Errorf("summary = %q, want %q", response.Diagnostics[0].Summary(), "Reserved Prefix Not Allowed")
			}
		})
	}
}

// TestDescriptions exercises the Description / MarkdownDescription methods on
// the hand-written validators so the human-facing copy stays covered.
func TestDescriptions(t *testing.T) {
	ctx := context.Background()

	describables := []struct {
		name string
		v    interface {
			Description(context.Context) string
			MarkdownDescription(context.Context) string
		}
	}{
		{"base64", Base64Validator{}},
		{"cidr", CIDRValidator{}},
		{"ip", IPAddressValidator{}},
		{"no_reserved_prefix", NoReservedPrefixValidator{Prefix: "nscale-"}},
	}

	for _, describable := range describables {
		t.Run(describable.name, func(t *testing.T) {
			if describable.v.Description(ctx) == "" {
				t.Error("Description() is empty")
			}

			if describable.v.MarkdownDescription(ctx) != describable.v.Description(ctx) {
				t.Error("MarkdownDescription() should match Description()")
			}
		})
	}
}
