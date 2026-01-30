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
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func NameValidator() validator.String {
	return stringvalidator.RegexMatches(
		regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`),
		"must start with a lowercase letter, contain only lowercase letters, digits or hyphens, end with a letter or digit, and be at most 63 characters long",
	)
}
