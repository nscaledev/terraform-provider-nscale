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
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// Both patterns are transcribed from the NKS OpenAPI spec (nodePoolTaintV1.key,
// nodePoolTaintV1.value and the nodePoolLabelsV1 additionalProperties). The
// lengths alongside them are enforced at the call site; without these the API
// accepts the plan and then rejects the apply with a 422.

const (
	// An optional DNS-subdomain prefix and a '/', then the name itself.
	kubernetesQualifiedNamePattern = `^([a-z0-9]([-a-z0-9]*[a-z0-9])?` +
		`(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?` +
		`([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$`

	// Alphanumerics, '-', '_' and '.', starting and ending alphanumeric. The
	// whole thing is optional, so the empty string matches.
	kubernetesLabelValuePattern = `^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$`
)

// KubernetesQualifiedNameValidator matches a Kubernetes qualified name. Used
// for taint keys.
func KubernetesQualifiedNameValidator() validator.String {
	return stringvalidator.RegexMatches(
		regexp.MustCompile(kubernetesQualifiedNamePattern),
		"must be a Kubernetes qualified name: an optional DNS subdomain prefix and a '/', then "+
			"alphanumerics, '-', '_' or '.' starting and ending with an alphanumeric",
	)
}

// KubernetesLabelValueValidator matches a Kubernetes label value. Shared by
// taint values and label values, which carry the same pattern in the spec. The
// empty string is valid and meaningful — it still applies the label.
func KubernetesLabelValueValidator() validator.String {
	return stringvalidator.RegexMatches(
		regexp.MustCompile(kubernetesLabelValuePattern),
		"must be empty, or alphanumerics, '-', '_' or '.' starting and ending with an alphanumeric",
	)
}
