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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

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
