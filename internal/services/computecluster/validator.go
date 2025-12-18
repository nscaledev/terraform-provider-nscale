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

package computecluster

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type PortsValidator struct{}

func (v PortsValidator) Description(ctx context.Context) string {
	return "Must be a valid port number (0-65535) or a port range (e.g., 80-443)"
}

func (v PortsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v PortsValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()
	ports := strings.Split(value, "-")

	if len(ports) > 2 {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid Port Format",
			fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
		)
		return
	}

	for _, port := range ports {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 0 || portNumber > 65535 {
			response.Diagnostics.AddAttributeError(
				request.Path,
				"Invalid Port Number",
				fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
			)
			return
		}
	}
}
