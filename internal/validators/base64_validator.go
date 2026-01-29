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
	"encoding/base64"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type Base64Validator struct{}

func (v Base64Validator) Description(ctx context.Context) string {
	return "must be a valid base64 encoded string"
}

func (v Base64Validator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v Base64Validator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()

	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid Base64 Encoded String",
			fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
		)
	}
}
