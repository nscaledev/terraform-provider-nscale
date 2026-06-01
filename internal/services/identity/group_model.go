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

package identity

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	identityapi "github.com/nscaledev/nscale-sdk-go/identity"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
)

type GroupModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Tags               types.Map    `tfsdk:"tags"`
	RoleIDs            types.Set    `tfsdk:"role_ids"`
	ServiceAccountIDs  types.Set    `tfsdk:"service_account_ids"`
	UserIDs            types.Set    `tfsdk:"user_ids"`
	Subjects           types.Set    `tfsdk:"subjects"`
	CreationTime       types.String `tfsdk:"creation_time"`
	ProvisioningStatus types.String `tfsdk:"provisioning_status"`
}

var SubjectModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"issuer": types.StringType,
		"id":     types.StringType,
		"email":  types.StringType,
	},
}

type SubjectModel struct {
	Issuer types.String `tfsdk:"issuer"`
	ID     types.String `tfsdk:"id"`
	Email  types.String `tfsdk:"email"`
}

func NewSubjectModel(source identityapi.Subject) attr.Value {
	return types.ObjectValueMust(
		SubjectModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"issuer": types.StringValue(source.Issuer),
			"id":     types.StringValue(source.Id),
			"email":  types.StringPointerValue(source.Email),
		},
	)
}

func (m *SubjectModel) NscaleSubject() identityapi.Subject {
	return identityapi.Subject{
		Issuer: m.Issuer.ValueString(),
		Id:     m.ID.ValueString(),
		Email:  m.Email.ValueStringPointer(),
	}
}

// stringSet converts a slice to a set faithfully: an empty (non-nil) slice
// becomes an empty set, not null. The identity API returns empty lists as `[]`
// (never null), so collapsing empty to null would produce "inconsistent result
// after apply" when a user configures an explicit empty set.
func stringSet(source []string) types.Set {
	values := make([]attr.Value, 0, len(source))
	for _, value := range source {
		values = append(values, types.StringValue(value))
	}
	return types.SetValueMust(types.StringType, values)
}

func NewGroupModel(source *identityapi.GroupRead) GroupModel {
	tags := nscale.RemoveOperationTags(source.Metadata.Tags)

	userIDs := types.SetNull(types.StringType)
	if source.Spec.UserIDs != nil {
		userIDs = stringSet(*source.Spec.UserIDs)
	}

	subjects := types.SetNull(SubjectModelAttributeType)
	if source.Spec.Subjects != nil {
		values := make([]attr.Value, 0, len(*source.Spec.Subjects))
		for _, subject := range *source.Spec.Subjects {
			values = append(values, NewSubjectModel(subject))
		}
		subjects = types.SetValueMust(SubjectModelAttributeType, values)
	}

	return GroupModel{
		ID:                 types.StringValue(source.Metadata.Id),
		Name:               types.StringValue(source.Metadata.Name),
		Description:        types.StringPointerValue(source.Metadata.Description),
		Tags:               tftypes.TagMapValueMust(tags),
		RoleIDs:            stringSet(source.Spec.RoleIDs),
		ServiceAccountIDs:  stringSet(source.Spec.ServiceAccountIDs),
		UserIDs:            userIDs,
		Subjects:           subjects,
		CreationTime:       types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
		ProvisioningStatus: types.StringValue(string(source.Metadata.ProvisioningStatus)),
	}
}

func setToStrings(source types.Set) ([]string, diag.Diagnostics) {
	result := []string{}
	if source.IsNull() || source.IsUnknown() {
		return result, nil
	}
	if diagnostics := source.ElementsAs(context.TODO(), &result, false); diagnostics.HasError() {
		return nil, diagnostics
	}
	if result == nil {
		result = []string{}
	}
	return result, nil
}

func (m *GroupModel) NscaleGroupSpec() (identityapi.GroupSpec, diag.Diagnostics) {
	roleIDs, diagnostics := setToStrings(m.RoleIDs)
	if diagnostics.HasError() {
		return identityapi.GroupSpec{}, diagnostics
	}

	serviceAccountIDs, diagnostics := setToStrings(m.ServiceAccountIDs)
	if diagnostics.HasError() {
		return identityapi.GroupSpec{}, diagnostics
	}

	// Subjects is intentionally not written: the identity service derives it
	// from user_ids (and any federated identities), and supplying both user_ids
	// and subjects in the same request is rejected server-side. It is a
	// read-only (computed) attribute, so membership is driven through user_ids.
	spec := identityapi.GroupSpec{
		RoleIDs:           roleIDs,
		ServiceAccountIDs: serviceAccountIDs,
		UserIDs:           nil,
		Subjects:          nil,
	}

	if !m.UserIDs.IsNull() && !m.UserIDs.IsUnknown() {
		userIDs, userDiagnostics := setToStrings(m.UserIDs)
		if userDiagnostics.HasError() {
			return identityapi.GroupSpec{}, userDiagnostics
		}
		spec.UserIDs = &userIDs
	}

	return spec, nil
}

func (m *GroupModel) NscaleGroupCreateParams() (identityapi.GroupWrite, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return identityapi.GroupWrite{}, diagnostics
	}
	tags = nscale.RemoveOperationTags(tags)

	spec, diagnostics := m.NscaleGroupSpec()
	if diagnostics.HasError() {
		return identityapi.GroupWrite{}, diagnostics
	}

	return identityapi.GroupWrite{
		Metadata: coreapi.ResourceWriteMetadata{
			Name:        m.Name.ValueString(),
			Description: m.Description.ValueStringPointer(),
			Tags:        tags,
		},
		Spec: spec,
	}, nil
}

// NscaleGroupUpdateParams produces the PUT body. The identity group API uses
// the same GroupWrite shape for create and update.
func (m *GroupModel) NscaleGroupUpdateParams() (identityapi.GroupWrite, diag.Diagnostics) {
	return m.NscaleGroupCreateParams()
}
