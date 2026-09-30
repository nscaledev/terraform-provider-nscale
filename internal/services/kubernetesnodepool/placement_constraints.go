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

package kubernetesnodepool

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"
)

// spreadOnlyAttributes are the constraints fields the API accepts only with
// the spread policy.
func spreadOnlyAttributes() []string {
	return []string{"max_skew", "min_domains", "when_unsatisfiable"}
}

// spreadOnlyConstraintsFindings names the spread-only fields set under a policy
// other than spread. Empty when the policy is unresolved: it cannot be checked
// against yet, and OneOf on the policy already rejects a bad literal.
func spreadOnlyConstraintsFindings(policy types.String, set map[string]bool) []string {
	if policy.IsNull() || policy.IsUnknown() ||
		policy.ValueString() == string(kubernetesapi.NodePoolPlacementConstraintsV1PolicySpread) {
		return nil
	}

	var findings []string
	for _, attribute := range spreadOnlyAttributes() {
		if set[attribute] {
			findings = append(findings, attribute)
		}
	}

	return findings
}

// spreadOnlyConstraintsValidator rejects spread-only fields under pack at plan
// time; the API returns a 422 for the same thing.
type spreadOnlyConstraintsValidator struct{}

var _ validator.Object = spreadOnlyConstraintsValidator{}

func (spreadOnlyConstraintsValidator) Description(_ context.Context) string {
	return "max_skew, min_domains and when_unsatisfiable are only valid when policy is spread"
}

func (v spreadOnlyConstraintsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (spreadOnlyConstraintsValidator) ValidateObject(
	ctx context.Context,
	request validator.ObjectRequest,
	response *validator.ObjectResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	var model constraintsModel
	objectDiagnostics := request.ConfigValue.As(ctx, &model, basetypes.ObjectAsOptions{})
	if response.Diagnostics.Append(objectDiagnostics...); response.Diagnostics.HasError() {
		return
	}

	set := map[string]bool{
		"max_skew":           !model.MaxSkew.IsNull(),
		"min_domains":        !model.MinDomains.IsNull(),
		"when_unsatisfiable": !model.WhenUnsatisfiable.IsNull(),
	}

	for _, attribute := range spreadOnlyConstraintsFindings(model.Policy, set) {
		response.Diagnostics.AddAttributeError(
			request.Path.AtName(attribute),
			"Invalid Placement Constraint",
			fmt.Sprintf("`%s` is only valid when `policy` is %q, but `policy` is %q.",
				attribute, kubernetesapi.NodePoolPlacementConstraintsV1PolicySpread, model.Policy.ValueString()),
		)
	}
}

// validateMinDomainsWithinReplicas rejects min_domains above replicas, which
// the API refuses because there are not enough hosts to cover the domains.
// Skipped while either value is unresolved.
func validateMinDomainsWithinReplicas(ctx context.Context, config tfsdk.Config, diagnostics *diag.Diagnostics) {
	minDomainsPath := path.Root(reservationAttribute).AtName("constraints").AtName("min_domains")

	var minDomains, replicas types.Int64
	diagnostics.Append(config.GetAttribute(ctx, minDomainsPath, &minDomains)...)
	diagnostics.Append(config.GetAttribute(ctx, path.Root("replicas"), &replicas)...)
	if diagnostics.HasError() {
		return
	}

	if minDomains.IsNull() || minDomains.IsUnknown() || replicas.IsNull() || replicas.IsUnknown() {
		return
	}

	if minDomains.ValueInt64() > replicas.ValueInt64() {
		diagnostics.AddAttributeError(
			minDomainsPath,
			"Invalid Placement Constraint",
			fmt.Sprintf("`min_domains` (%d) cannot exceed `replicas` (%d): each domain needs at least one host.",
				minDomains.ValueInt64(), replicas.ValueInt64()),
		)
	}
}
