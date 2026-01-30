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
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type NoReservedPrefixValidator struct {
	Prefix string
}

func NoReservedPrefix(prefix string) validator.String {
	return NoReservedPrefixValidator{Prefix: prefix}
}

func (v NoReservedPrefixValidator) Description(ctx context.Context) string {
	return fmt.Sprintf("must not start with the reserved prefix %q", v.Prefix)
}

func (v NoReservedPrefixValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v NoReservedPrefixValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()

	if strings.HasPrefix(value, v.Prefix) {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Reserved Prefix Not Allowed",
			fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
		)
		return
	}
}
