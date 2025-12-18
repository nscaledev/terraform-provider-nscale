/*
Copyright 2025 Nscale

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
	"fmt"
	"net"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type CIDRValidator struct{}

func (v CIDRValidator) Description(ctx context.Context) string {
	return "Must be a valid CIDR notation"
}

func (v CIDRValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v CIDRValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()

	if _, _, err := net.ParseCIDR(value); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid CIDR Notation",
			fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
		)
		return
	}
}
