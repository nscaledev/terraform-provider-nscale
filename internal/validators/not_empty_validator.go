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

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// NotEmptyValidator rejects "" on an optional attribute, so there is one way
// to say "none": omit the attribute.
type NotEmptyValidator struct{}

func (v NotEmptyValidator) Description(ctx context.Context) string {
	return "must not be empty; omit the attribute to have none"
}

func (v NotEmptyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v NotEmptyValidator) ValidateString(
	ctx context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	if request.ConfigValue.ValueString() == "" {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Empty Value",
			fmt.Sprintf("Attribute %s must not be empty; omit %s to have none.", request.Path, request.Path),
		)
	}
}
