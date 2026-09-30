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

// API server access: api_server.authorization (RBAC bindings) and
// api_server.authentication (webhook override + external OIDC issuers).
//
// Deployed NKS treats both as immutable, so any change plans a replacement.
// nks-core main allows removals in place; relax the plan modifiers when that
// ships (a removal returns 422 "is immutable" until then).

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"
)

// Bounds mirrored from the NKS spec.
const (
	minRoleBindings     = 1
	maxRoleBindings     = 4
	minSubjects         = 1
	maxSubjects         = 32
	maxSubjectName      = 2048
	maxExternalIssuers  = 8
	maxIssuerURL        = 2048
	minAudiences        = 1
	maxAudiences        = 8
	maxAudience         = 256
	maxClaimOrPrefix    = 256
	maxCACertificatePEM = 8192
)

// defaultUsernameClaim is the API's default for usernameClaim. The server
// stores it and reads it back, so the schema defaults to it as well.
const defaultUsernameClaim = "sub"

// reservedIdentityPrefixes may not start, or be a prefix of, an issuer's
// username or groups prefix. Mirrors the NKS spec.
func reservedIdentityPrefixes() []string {
	return []string{"system:", "kubeadm:", "nks-management:", "nks:", "nscale.com/"}
}

type authorizationModel struct {
	ClusterRoleBindings types.Set `tfsdk:"cluster_role_bindings"`
}

type clusterRoleBindingModel struct {
	ClusterRole types.String `tfsdk:"cluster_role"`
	Subjects    types.Set    `tfsdk:"subjects"`
}

type rbacSubjectModel struct {
	Kind types.String `tfsdk:"kind"`
	Name types.String `tfsdk:"name"`
}

type authenticationModel struct {
	NscaleWebhook   types.Object `tfsdk:"nscale_webhook"`
	ExternalIssuers types.List   `tfsdk:"external_issuers"`
}

type nscaleWebhookModel struct {
	Enabled types.Bool `tfsdk:"enabled"`
}

type externalIssuerModel struct {
	IssuerURL      types.String `tfsdk:"issuer_url"`
	Audiences      types.List   `tfsdk:"audiences"`
	UsernameClaim  types.String `tfsdk:"username_claim"`
	UsernamePrefix types.String `tfsdk:"username_prefix"`
	GroupsClaim    types.String `tfsdk:"groups_claim"`
	GroupsPrefix   types.String `tfsdk:"groups_prefix"`
	CACertificate  types.String `tfsdk:"ca_certificate"`
}

func rbacSubjectAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"kind": types.StringType,
		"name": types.StringType,
	}
}

func clusterRoleBindingAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"cluster_role": types.StringType,
		"subjects":     types.SetType{ElemType: types.ObjectType{AttrTypes: rbacSubjectAttrTypes()}},
	}
}

func authorizationAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"cluster_role_bindings": types.SetType{ElemType: types.ObjectType{AttrTypes: clusterRoleBindingAttrTypes()}},
	}
}

func nscaleWebhookAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enabled": types.BoolType,
	}
}

func externalIssuerAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"issuer_url":      types.StringType,
		"audiences":       types.ListType{ElemType: types.StringType},
		"username_claim":  types.StringType,
		"username_prefix": types.StringType,
		"groups_claim":    types.StringType,
		"groups_prefix":   types.StringType,
		"ca_certificate":  types.StringType,
	}
}

func authenticationAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"nscale_webhook":   types.ObjectType{AttrTypes: nscaleWebhookAttrTypes()},
		"external_issuers": types.ListType{ElemType: types.ObjectType{AttrTypes: externalIssuerAttrTypes()}},
	}
}

// --- API -> model -------------------------------------------------------------

func authorizationObjectValue(source *kubernetesapi.ClusterApiServerAuthorizationV1) types.Object {
	if source == nil {
		return types.ObjectNull(authorizationAttrTypes())
	}

	bindingType := types.ObjectType{AttrTypes: clusterRoleBindingAttrTypes()}
	subjectType := types.ObjectType{AttrTypes: rbacSubjectAttrTypes()}

	bindings := make([]attr.Value, 0, len(source.ClusterRoleBindings))
	for _, binding := range source.ClusterRoleBindings {
		subjects := make([]attr.Value, 0, len(binding.Subjects))
		for _, subject := range binding.Subjects {
			subjects = append(subjects, types.ObjectValueMust(rbacSubjectAttrTypes(), map[string]attr.Value{
				"kind": types.StringValue(string(subject.Kind)),
				"name": types.StringValue(subject.Name),
			}))
		}

		bindings = append(bindings, types.ObjectValueMust(clusterRoleBindingAttrTypes(), map[string]attr.Value{
			"cluster_role": types.StringValue(string(binding.ClusterRole)),
			"subjects":     types.SetValueMust(subjectType, subjects),
		}))
	}

	return types.ObjectValueMust(authorizationAttrTypes(), map[string]attr.Value{
		"cluster_role_bindings": types.SetValueMust(bindingType, bindings),
	})
}

func authenticationObjectValue(source *kubernetesapi.ClusterApiServerAuthenticationV1) types.Object {
	if source == nil {
		return types.ObjectNull(authenticationAttrTypes())
	}

	webhook := types.ObjectNull(nscaleWebhookAttrTypes())
	if source.NscaleWebhook != nil {
		webhook = types.ObjectValueMust(nscaleWebhookAttrTypes(), map[string]attr.Value{
			"enabled": types.BoolPointerValue(source.NscaleWebhook.Enabled),
		})
	}

	issuerType := types.ObjectType{AttrTypes: externalIssuerAttrTypes()}
	issuers := types.ListNull(issuerType)
	if source.ExternalIssuers != nil {
		elements := make([]attr.Value, 0, len(*source.ExternalIssuers))
		for _, issuer := range *source.ExternalIssuers {
			audiences := make([]attr.Value, 0, len(issuer.Audiences))
			for _, audience := range issuer.Audiences {
				audiences = append(audiences, types.StringValue(audience))
			}

			elements = append(elements, types.ObjectValueMust(externalIssuerAttrTypes(), map[string]attr.Value{
				"issuer_url":      types.StringValue(issuer.IssuerURL),
				"audiences":       types.ListValueMust(types.StringType, audiences),
				"username_claim":  types.StringPointerValue(issuer.UsernameClaim),
				"username_prefix": types.StringValue(issuer.UsernamePrefix),
				"groups_claim":    types.StringPointerValue(issuer.GroupsClaim),
				"groups_prefix":   types.StringPointerValue(issuer.GroupsPrefix),
				"ca_certificate":  types.StringPointerValue(issuer.CaCertificate),
			}))
		}
		issuers = types.ListValueMust(issuerType, elements)
	}

	return types.ObjectValueMust(authenticationAttrTypes(), map[string]attr.Value{
		"nscale_webhook":   webhook,
		"external_issuers": issuers,
	})
}

// --- model -> API -------------------------------------------------------------

// authorizationRequest builds apiServer.authorization. Null sends nothing,
// which on update removes every binding (the API allows removing them all).
func authorizationRequest(
	ctx context.Context,
	value types.Object,
) (*kubernetesapi.ClusterApiServerAuthorizationV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if value.IsNull() || value.IsUnknown() {
		return nil, diagnostics
	}

	var model authorizationModel
	if diagnostics = value.As(ctx, &model, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		return nil, diagnostics
	}

	var bindingModels []clusterRoleBindingModel
	if diagnostics = model.ClusterRoleBindings.ElementsAs(ctx, &bindingModels, false); diagnostics.HasError() {
		return nil, diagnostics
	}

	bindings := make([]kubernetesapi.ClusterRoleBindingV1, 0, len(bindingModels))
	for _, bindingModel := range bindingModels {
		var subjectModels []rbacSubjectModel
		if diagnostics = bindingModel.Subjects.ElementsAs(ctx, &subjectModels, false); diagnostics.HasError() {
			return nil, diagnostics
		}

		subjects := make([]kubernetesapi.RbacSubjectV1, 0, len(subjectModels))
		for _, subject := range subjectModels {
			subjects = append(subjects, kubernetesapi.RbacSubjectV1{
				Kind: kubernetesapi.RbacSubjectV1Kind(subject.Kind.ValueString()),
				Name: subject.Name.ValueString(),
			})
		}

		bindings = append(bindings, kubernetesapi.ClusterRoleBindingV1{
			ClusterRole: kubernetesapi.ClusterRoleBindingV1ClusterRole(bindingModel.ClusterRole.ValueString()),
			Subjects:    subjects,
		})
	}

	return &kubernetesapi.ClusterApiServerAuthorizationV1{ClusterRoleBindings: bindings}, diagnostics
}

func authenticationRequest(
	ctx context.Context,
	value types.Object,
) (*kubernetesapi.ClusterApiServerAuthenticationV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if value.IsNull() || value.IsUnknown() {
		return nil, diagnostics
	}

	var model authenticationModel
	if diagnostics = value.As(ctx, &model, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		return nil, diagnostics
	}

	var webhook *kubernetesapi.ClusterNscaleWebhookAuthenticationV1
	if !model.NscaleWebhook.IsNull() && !model.NscaleWebhook.IsUnknown() {
		var webhookModel nscaleWebhookModel
		if diagnostics = model.NscaleWebhook.As(
			ctx,
			&webhookModel,
			basetypes.ObjectAsOptions{},
		); diagnostics.HasError() {
			return nil, diagnostics
		}
		webhook = &kubernetesapi.ClusterNscaleWebhookAuthenticationV1{Enabled: webhookModel.Enabled.ValueBoolPointer()}
	}

	var issuers *[]kubernetesapi.ClusterExternalIssuerV1
	if !model.ExternalIssuers.IsNull() && !model.ExternalIssuers.IsUnknown() {
		var issuerModels []externalIssuerModel
		if diagnostics = model.ExternalIssuers.ElementsAs(ctx, &issuerModels, false); diagnostics.HasError() {
			return nil, diagnostics
		}

		converted := make([]kubernetesapi.ClusterExternalIssuerV1, 0, len(issuerModels))
		for _, issuer := range issuerModels {
			var audiences []string
			if diagnostics = issuer.Audiences.ElementsAs(ctx, &audiences, false); diagnostics.HasError() {
				return nil, diagnostics
			}

			converted = append(converted, kubernetesapi.ClusterExternalIssuerV1{
				IssuerURL:      issuer.IssuerURL.ValueString(),
				Audiences:      audiences,
				UsernameClaim:  issuer.UsernameClaim.ValueStringPointer(),
				UsernamePrefix: issuer.UsernamePrefix.ValueString(),
				GroupsClaim:    issuer.GroupsClaim.ValueStringPointer(),
				GroupsPrefix:   issuer.GroupsPrefix.ValueStringPointer(),
				CaCertificate:  issuer.CACertificate.ValueStringPointer(),
			})
		}
		issuers = &converted
	}

	return &kubernetesapi.ClusterApiServerAuthenticationV1{
		NscaleWebhook:   webhook,
		ExternalIssuers: issuers,
	}, diagnostics
}

// --- validators ---------------------------------------------------------------

// reservedPrefixValidator rejects an identity prefix that starts with, or is
// itself a prefix of, one of the API's reserved prefixes.
type reservedPrefixValidator struct{}

var _ validator.String = reservedPrefixValidator{}

func (reservedPrefixValidator) Description(_ context.Context) string {
	return "must not start with, or be a prefix of, " + strings.Join(reservedIdentityPrefixes(), ", ")
}

func (v reservedPrefixValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (reservedPrefixValidator) ValidateString(
	_ context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()
	for _, reserved := range reservedIdentityPrefixes() {
		if strings.HasPrefix(value, reserved) || strings.HasPrefix(reserved, value) {
			response.Diagnostics.AddAttributeError(
				request.Path,
				"Reserved Identity Prefix",
				fmt.Sprintf("%q collides with the reserved prefix %q. Choose a prefix that neither starts with "+
					"nor is a prefix of %s.", value, reserved, strings.Join(reservedIdentityPrefixes(), ", ")),
			)

			return
		}
	}
}

// uniqueClusterRoleValidator enforces one binding per cluster_role. The set
// already rejects identical bindings; this catches two for the same role.
type uniqueClusterRoleValidator struct{}

var _ validator.Set = uniqueClusterRoleValidator{}

func (uniqueClusterRoleValidator) Description(_ context.Context) string {
	return "each cluster_role may appear in at most one binding"
}

func (v uniqueClusterRoleValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (uniqueClusterRoleValidator) ValidateSet(
	ctx context.Context,
	request validator.SetRequest,
	response *validator.SetResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	var bindings []clusterRoleBindingModel
	if diagnostics := request.ConfigValue.ElementsAs(ctx, &bindings, true); diagnostics.HasError() {
		return
	}

	seen := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		if binding.ClusterRole.IsNull() || binding.ClusterRole.IsUnknown() {
			continue
		}

		role := binding.ClusterRole.ValueString()
		if seen[role] {
			response.Diagnostics.AddAttributeError(
				request.Path,
				"Duplicate Cluster Role Binding",
				fmt.Sprintf(
					"cluster_role %q appears in more than one binding. Put all of its subjects in one binding.",
					role,
				),
			)

			return
		}
		seen[role] = true
	}
}

// groupsClaimNeedsPrefixValidator: the API requires groups_prefix whenever
// groups_claim is set.
type groupsClaimNeedsPrefixValidator struct{}

var _ validator.Object = groupsClaimNeedsPrefixValidator{}

func (groupsClaimNeedsPrefixValidator) Description(_ context.Context) string {
	return "groups_prefix is required when groups_claim is set"
}

func (v groupsClaimNeedsPrefixValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (groupsClaimNeedsPrefixValidator) ValidateObject(
	ctx context.Context,
	request validator.ObjectRequest,
	response *validator.ObjectResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	var issuer externalIssuerModel
	if diagnostics := request.ConfigValue.As(ctx, &issuer, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	}); diagnostics.HasError() {
		return
	}

	if !issuer.GroupsClaim.IsNull() && !issuer.GroupsClaim.IsUnknown() && issuer.GroupsPrefix.IsNull() {
		response.Diagnostics.AddAttributeError(
			request.Path.AtName("groups_prefix"),
			"Missing Groups Prefix",
			"groups_prefix is required when groups_claim is set, so issuer groups cannot collide with in-cluster groups.",
		)
	}
}

// --- schema -------------------------------------------------------------------

func authorizationSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "RBAC bindings granting built-in cluster roles to users and groups. " +
			"Fixed at creation: any change, including adding or removing this block, forces a new cluster.",
		Optional: true,
		PlanModifiers: []planmodifier.Object{
			objectplanmodifier.RequiresReplace(),
		},
		Attributes: map[string]schema.Attribute{
			"cluster_role_bindings": schema.SetNestedAttribute{
				MarkdownDescription: "One binding per cluster role.",
				Required:            true,
				Validators: []validator.Set{
					setvalidator.SizeBetween(minRoleBindings, maxRoleBindings),
					uniqueClusterRoleValidator{},
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cluster_role": schema.StringAttribute{
							MarkdownDescription: "The built-in cluster role to bind: `cluster-admin`, `admin`, `edit` or `view`.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(
									string(kubernetesapi.ClusterRoleBindingV1ClusterRoleClusterAdmin),
									string(kubernetesapi.ClusterRoleBindingV1ClusterRoleAdmin),
									string(kubernetesapi.ClusterRoleBindingV1ClusterRoleEdit),
									string(kubernetesapi.ClusterRoleBindingV1ClusterRoleView),
								),
							},
						},
						"subjects": schema.SetNestedAttribute{
							MarkdownDescription: "The users and groups granted the role.",
							Required:            true,
							Validators: []validator.Set{
								setvalidator.SizeBetween(minSubjects, maxSubjects),
							},
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"kind": schema.StringAttribute{
										MarkdownDescription: "`User` or `Group`.",
										Required:            true,
										Validators: []validator.String{
											stringvalidator.OneOf(
												string(kubernetesapi.RbacSubjectV1KindUser),
												string(kubernetesapi.RbacSubjectV1KindGroup),
											),
										},
									},
									"name": schema.StringAttribute{
										MarkdownDescription: "The user or group name, as the authenticator presents it " +
											"(including any issuer prefix).",
										Required: true,
										Validators: []validator.String{
											stringvalidator.LengthBetween(1, maxSubjectName),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func authenticationSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "How the Kubernetes API server authenticates callers, beyond the Nscale defaults. " +
			"Fixed at creation: any change, including adding or removing this block, forces a new cluster.",
		Optional: true,
		PlanModifiers: []planmodifier.Object{
			objectplanmodifier.RequiresReplace(),
		},
		Attributes: map[string]schema.Attribute{
			"nscale_webhook": schema.SingleNestedAttribute{
				MarkdownDescription: "Per-cluster override of whether the Nscale authentication webhook is enabled. " +
					"Omit to inherit the cell-wide default.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						MarkdownDescription: "Whether the Nscale authentication webhook is enabled for this cluster.",
						Optional:            true,
					},
				},
			},
			// A list, not a set: the API compares issuers, audiences included, in
			// order, so a reordering set could turn a removal into a replacement.
			"external_issuers": schema.ListNestedAttribute{
				MarkdownDescription: "External JWT/OIDC issuers the API server trusts, such as a corporate identity provider.",
				Optional:            true,
				Validators: []validator.List{
					listvalidator.SizeAtMost(maxExternalIssuers),
				},
				NestedObject: schema.NestedAttributeObject{
					Validators: []validator.Object{
						groupsClaimNeedsPrefixValidator{},
					},
					Attributes: map[string]schema.Attribute{
						"issuer_url": schema.StringAttribute{
							MarkdownDescription: "The issuer URL clients present tokens from.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.LengthBetween(1, maxIssuerURL),
							},
						},
						"audiences": schema.ListAttribute{
							MarkdownDescription: "Token audiences accepted from this issuer.",
							ElementType:         types.StringType,
							Required:            true,
							Validators: []validator.List{
								listvalidator.SizeBetween(minAudiences, maxAudiences),
								listvalidator.UniqueValues(),
								listvalidator.ValueStringsAre(stringvalidator.LengthBetween(1, maxAudience)),
							},
						},
						"username_claim": schema.StringAttribute{
							MarkdownDescription: "The JWT claim mapped to the username. Defaults to `sub`.",
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString(defaultUsernameClaim),
							Validators: []validator.String{
								stringvalidator.LengthAtMost(maxClaimOrPrefix),
							},
						},
						"username_prefix": schema.StringAttribute{
							MarkdownDescription: "Prepended to the username claim, so issuer users cannot collide with " +
								"in-cluster ones. Must not start with, or be a prefix of, `system:`, `kubeadm:`, " +
								"`nks-management:`, `nks:` or `nscale.com/`.",
							Required: true,
							Validators: []validator.String{
								stringvalidator.LengthBetween(1, maxClaimOrPrefix),
								reservedPrefixValidator{},
							},
						},
						"groups_claim": schema.StringAttribute{
							MarkdownDescription: "The JWT claim mapped to the user's groups. Requires `groups_prefix`.",
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.LengthAtMost(maxClaimOrPrefix),
							},
						},
						"groups_prefix": schema.StringAttribute{
							MarkdownDescription: "Prepended to each group from `groups_claim`. Same reserved-prefix rule " +
								"as `username_prefix`.",
							Optional: true,
							Validators: []validator.String{
								stringvalidator.LengthBetween(1, maxClaimOrPrefix),
								reservedPrefixValidator{},
							},
						},
						"ca_certificate": schema.StringAttribute{
							MarkdownDescription: "PEM-encoded CA certificate for the issuer, when it is not signed by a " +
								"well-known CA.",
							Optional: true,
							Validators: []validator.String{
								stringvalidator.LengthAtMost(maxCACertificatePEM),
							},
						},
					},
				},
			},
		},
	}
}
