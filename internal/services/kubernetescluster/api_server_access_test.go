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

package kubernetescluster

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"
)

func subject(kind kubernetesapi.RbacSubjectV1Kind, name string) kubernetesapi.RbacSubjectV1 {
	return kubernetesapi.RbacSubjectV1{Kind: kind, Name: name}
}

func binding(
	role kubernetesapi.ClusterRoleBindingV1ClusterRole,
	subjects ...kubernetesapi.RbacSubjectV1,
) kubernetesapi.ClusterRoleBindingV1 {
	return kubernetesapi.ClusterRoleBindingV1{ClusterRole: role, Subjects: subjects}
}

func bindings(roleBindings ...kubernetesapi.ClusterRoleBindingV1) *kubernetesapi.ClusterApiServerAuthorizationV1 {
	return &kubernetesapi.ClusterApiServerAuthorizationV1{ClusterRoleBindings: roleBindings}
}

func TestAuthorizationIsRemovalOnly(t *testing.T) {
	t.Parallel()

	alice := subject(kubernetesapi.RbacSubjectV1KindUser, "alice")
	bob := subject(kubernetesapi.RbacSubjectV1KindUser, "bob")
	ops := subject(kubernetesapi.RbacSubjectV1KindGroup, "ops")

	current := bindings(binding("admin", alice, bob), binding("view", ops))

	tests := map[string]struct {
		planned, current *kubernetesapi.ClusterApiServerAuthorizationV1
		want             bool
	}{
		"both absent":          {nil, nil, true},
		"remove all":           {nil, current, true},
		"add to none":          {bindings(binding("view", ops)), nil, false},
		"unchanged":            {current, current, true},
		"drop a subject":       {bindings(binding("admin", alice), binding("view", ops)), current, true},
		"drop a binding":       {bindings(binding("admin", bob, alice)), current, true},
		"add a subject":        {bindings(binding("admin", alice, bob, ops), binding("view", ops)), current, false},
		"add a binding":        {bindings(binding("admin", alice), binding("edit", bob)), current, false},
		"move subject to role": {bindings(binding("admin", ops)), current, false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := authorizationIsRemovalOnly(test.planned, test.current); got != test.want {
				t.Errorf("authorizationIsRemovalOnly = %t, want %t", got, test.want)
			}
		})
	}
}

func issuer(url string, audiences ...string) kubernetesapi.ClusterExternalIssuerV1 {
	return kubernetesapi.ClusterExternalIssuerV1{
		IssuerURL:      url,
		Audiences:      audiences,
		UsernameClaim:  new("sub"),
		UsernamePrefix: "oidc:",
	}
}

func authentication(
	webhook *bool,
	issuers ...kubernetesapi.ClusterExternalIssuerV1,
) *kubernetesapi.ClusterApiServerAuthenticationV1 {
	out := &kubernetesapi.ClusterApiServerAuthenticationV1{}
	if webhook != nil {
		out.NscaleWebhook = &kubernetesapi.ClusterNscaleWebhookAuthenticationV1{Enabled: webhook}
	}
	if issuers != nil {
		out.ExternalIssuers = &issuers
	}

	return out
}

func TestAuthenticationIsRemovalOnly(t *testing.T) {
	t.Parallel()

	one, two := issuer("https://one", "a", "b"), issuer("https://two", "c")
	current := authentication(new(false), one, two)

	tests := map[string]struct {
		planned, current *kubernetesapi.ClusterApiServerAuthenticationV1
		want             bool
	}{
		"both absent":           {nil, nil, true},
		"remove the block":      {nil, current, false},
		"add the block":         {current, nil, false},
		"unchanged":             {current, current, true},
		"drop an issuer":        {authentication(new(false), two), current, true},
		"drop every issuer":     {authentication(new(false)), current, true},
		"add an issuer":         {authentication(new(false), one, two, issuer("https://three", "d")), current, false},
		"edit an issuer":        {authentication(new(false), one, issuer("https://two", "changed")), current, false},
		"reorder audiences":     {authentication(new(false), issuer("https://one", "b", "a"), two), current, false},
		"change webhook":        {authentication(new(true), one, two), current, false},
		"drop webhook override": {authentication(nil, one, two), current, false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := authenticationIsRemovalOnly(test.planned, test.current); got != test.want {
				t.Errorf("authenticationIsRemovalOnly = %t, want %t", got, test.want)
			}
		})
	}
}

// runObjectModifier runs a plan modifier on an update: non-null state and plan,
// which is the only case RequiresReplaceIf acts on.
func runObjectModifier(t *testing.T, modifier planmodifier.Object, state, plan types.Object) bool {
	t.Helper()

	nonNull := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	request := planmodifier.ObjectRequest{
		Path:       path.Root("api_server").AtName("x"),
		StateValue: state,
		PlanValue:  plan,
		State:      tfsdk.State{Raw: nonNull},
		Plan:       tfsdk.Plan{Raw: nonNull},
	}

	var response planmodifier.ObjectResponse
	response.PlanValue = plan
	modifier.PlanModifyObject(context.Background(), request, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("plan modifier diagnostics: %v", response.Diagnostics)
	}

	return response.RequiresReplace
}

// TestAccessPlanModifiers drives the modifiers through the framework, so the
// model <-> API conversion they rely on is covered too. While
// removalsApplyInPlace is false every change, removals included, replaces.
func TestAccessPlanModifiers(t *testing.T) {
	t.Parallel()

	alice := subject(kubernetesapi.RbacSubjectV1KindUser, "alice")
	bob := subject(kubernetesapi.RbacSubjectV1KindUser, "bob")
	both := authorizationObjectValue(bindings(binding("admin", alice, bob)))
	justAlice := authorizationObjectValue(bindings(binding("admin", alice)))
	withTwo := authenticationObjectValue(authentication(nil, issuer("https://one", "a"), issuer("https://two", "b")))
	withOne := authenticationObjectValue(authentication(nil, issuer("https://one", "a")))

	tests := map[string]struct {
		modifier    planmodifier.Object
		state, plan types.Object
		inPlace     bool // expected when removalsApplyInPlace is true
	}{
		"remove a subject": {authorizationRequiresReplace(), both, justAlice, true},
		"add a subject":    {authorizationRequiresReplace(), justAlice, both, false},
		"remove authorization": {
			authorizationRequiresReplace(),
			both,
			types.ObjectNull(authorizationAttrTypes()),
			true,
		},
		"remove an issuer": {authenticationRequiresReplace(), withTwo, withOne, true},
		"add an issuer":    {authenticationRequiresReplace(), withOne, withTwo, false},
		"remove authentication": {
			authenticationRequiresReplace(),
			withOne,
			types.ObjectNull(authenticationAttrTypes()),
			false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			want := !removalsApplyInPlace || !test.inPlace
			if got := runObjectModifier(t, test.modifier, test.state, test.plan); got != want {
				t.Errorf("RequiresReplace = %t, want %t", got, want)
			}
		})
	}

	if runObjectModifier(t, authorizationRequiresReplace(), both, both) {
		t.Error("an unchanged value must never replace")
	}
}

func TestReservedPrefixValidator(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"oidc:":            false,
		"corp-":            false,
		"system:oidc:":     true,
		"kubeadm:":         true,
		"nks-management:x": true,
		"nks:":             true,
		"nscale.com/team:": true,
		"sys":              true, // a prefix of "system:"
		"nks":              true, // a prefix of "nks:"
		"nscale":           true, // a prefix of "nscale.com/"
	}

	for value, wantError := range tests {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			var response validator.StringResponse
			reservedPrefixValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("username_prefix"),
				ConfigValue: types.StringValue(value),
			}, &response)

			if got := response.Diagnostics.HasError(); got != wantError {
				t.Errorf("%q: error = %t, want %t", value, got, wantError)
			}
		})
	}
}

func TestUniqueClusterRoleValidator(t *testing.T) {
	t.Parallel()

	alice := subject(kubernetesapi.RbacSubjectV1KindUser, "alice")
	bob := subject(kubernetesapi.RbacSubjectV1KindUser, "bob")
	valid := authorizationObjectValue(bindings(binding("admin", alice), binding("view", bob)))
	duplicate := authorizationObjectValue(&kubernetesapi.ClusterApiServerAuthorizationV1{
		ClusterRoleBindings: []kubernetesapi.ClusterRoleBindingV1{
			{ClusterRole: "admin", Subjects: []kubernetesapi.RbacSubjectV1{alice}},
			{ClusterRole: "admin", Subjects: []kubernetesapi.RbacSubjectV1{bob}},
		},
	})

	for name, test := range map[string]struct {
		value     types.Object
		wantError bool
	}{"one per role": {valid, false}, "two for admin": {duplicate, true}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var response validator.SetResponse
			uniqueClusterRoleValidator{}.ValidateSet(context.Background(), validator.SetRequest{
				Path:        path.Root("cluster_role_bindings"),
				ConfigValue: test.value.Attributes()["cluster_role_bindings"].(types.Set), //nolint:forcetypeassert // fixture is a set
			}, &response)

			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Errorf("error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
		})
	}
}

func TestGroupsClaimNeedsPrefixValidator(t *testing.T) {
	t.Parallel()

	object := func(groupsClaim, groupsPrefix types.String) types.Object {
		return types.ObjectValueMust(externalIssuerAttrTypes(), map[string]attr.Value{
			"issuer_url":      types.StringValue("https://issuer"),
			"audiences":       types.ListValueMust(types.StringType, []attr.Value{types.StringValue("k8s")}),
			"username_claim":  types.StringValue("sub"),
			"username_prefix": types.StringValue("oidc:"),
			"groups_claim":    groupsClaim,
			"groups_prefix":   groupsPrefix,
			"ca_certificate":  types.StringNull(),
		})
	}

	for name, test := range map[string]struct {
		value     types.Object
		wantError bool
	}{
		"neither":              {object(types.StringNull(), types.StringNull()), false},
		"claim and prefix":     {object(types.StringValue("groups"), types.StringValue("oidc:")), false},
		"claim without prefix": {object(types.StringValue("groups"), types.StringNull()), true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var response validator.ObjectResponse
			groupsClaimNeedsPrefixValidator{}.ValidateObject(context.Background(), validator.ObjectRequest{
				Path:        path.Root("external_issuers").AtListIndex(0),
				ConfigValue: test.value,
			}, &response)

			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Errorf("error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
		})
	}
}
