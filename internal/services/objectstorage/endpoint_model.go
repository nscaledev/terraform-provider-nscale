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

package objectstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
)

type ObjectStorageEndpointModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	EndpointClassID  types.String `tfsdk:"endpoint_class_id"`
	IdentityPolicies types.List   `tfsdk:"identity_policies"`
	Exposure         types.Object `tfsdk:"exposure"`
	Tags             types.Map    `tfsdk:"tags"`
	ProjectID        types.String `tfsdk:"project_id"`
	RegionID         types.String `tfsdk:"region_id"`
	CreationTime     types.String `tfsdk:"creation_time"`
}

// ObjectStorageEndpointIdentityPolicyAttributeType describes the shape of a
// single identity policy entry inside the identity_policies list. The
// document is stored as a normalised JSON string so the whitespace-normalising
// plan modifier on the resource can detect equivalent values without forcing
// users to commit to a particular JSON formatting.
var ObjectStorageEndpointIdentityPolicyAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":     types.StringType,
		"document": types.StringType,
	},
}

// ObjectStorageEndpointExposureAttributeType describes the shape of the
// computed exposure block. It mirrors the OpenAPI's nested status object so
// that future exposure types (currently only `public` exists in the spec but
// `private` is enumerated as a supported endpoint type) can be added without
// a breaking schema change.
var ObjectStorageEndpointExposureAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"public": ObjectStorageEndpointExposureDetailsAttributeType,
	},
}

var ObjectStorageEndpointExposureDetailsAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"dns_name": types.StringType,
	},
}

type objectStorageEndpointIdentityPolicyModel struct {
	Name     types.String `tfsdk:"name"`
	Document types.String `tfsdk:"document"`
}

func NewObjectStorageEndpointModel(
	source *storageapi.ObjectStorageEndpointRead,
) (ObjectStorageEndpointModel, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	identityPolicies, policyDiags := newIdentityPoliciesValue(source.Spec.IdentityPolicies)
	diagnostics.Append(policyDiags...)

	exposure := newExposureValue(source.Status.Exposure)

	tags := nscale.RemoveOperationTags(source.Metadata.Tags)

	return ObjectStorageEndpointModel{
		ID:               types.StringValue(source.Metadata.Id),
		Name:             types.StringValue(source.Metadata.Name),
		Description:      types.StringPointerValue(source.Metadata.Description),
		EndpointClassID:  types.StringValue(source.Spec.ObjectStorageEndpointClassId),
		IdentityPolicies: identityPolicies,
		Exposure:         exposure,
		Tags:             tftypes.TagMapValueMust(tags),
		ProjectID:        types.StringValue(source.Metadata.ProjectId),
		RegionID:         types.StringValue(source.Status.RegionId),
		CreationTime:     types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}, diagnostics
}

func newIdentityPoliciesValue(source *storageapi.ObjectStorageIdentityPolicyList) (types.List, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if source == nil || len(*source) == 0 {
		return types.ListValueMust(ObjectStorageEndpointIdentityPolicyAttributeType, []attr.Value{}), diagnostics
	}

	values := make([]attr.Value, 0, len(*source))
	for _, policy := range *source {
		document, err := marshalIdentityPolicyDocument(policy.Document)
		if err != nil {
			diagnostics.AddError(
				"Failed to Marshal Identity Policy Document",
				fmt.Sprintf("Identity policy %q produced an unmarshalable document: %s", policy.Name, err),
			)
			continue
		}

		values = append(values, types.ObjectValueMust(
			ObjectStorageEndpointIdentityPolicyAttributeType.AttrTypes,
			map[string]attr.Value{
				"name":     types.StringValue(policy.Name),
				"document": types.StringValue(document),
			},
		))
	}

	return types.ListValueMust(ObjectStorageEndpointIdentityPolicyAttributeType, values), diagnostics
}

func newExposureValue(source *storageapi.ObjectStorageEndpointExposureStatus) types.Object {
	if source == nil {
		return types.ObjectNull(ObjectStorageEndpointExposureAttributeType.AttrTypes)
	}

	publicValue := types.ObjectNull(ObjectStorageEndpointExposureDetailsAttributeType.AttrTypes)
	if source.Public != nil {
		publicValue = types.ObjectValueMust(
			ObjectStorageEndpointExposureDetailsAttributeType.AttrTypes,
			map[string]attr.Value{
				"dns_name": types.StringValue(source.Public.DnsName),
			},
		)
	}

	return types.ObjectValueMust(
		ObjectStorageEndpointExposureAttributeType.AttrTypes,
		map[string]attr.Value{
			"public": publicValue,
		},
	)
}

// marshalIdentityPolicyDocument produces a canonical JSON representation of an
// identity policy document. The output is compact (no extra whitespace) so
// that round-tripping a `jsonencode({...})` value through the API does not
// produce spurious diffs — the resource's normalizeWhitespacePlanModifier
// uses semantic JSON equality, but a deterministic stored form keeps state
// readable.
func marshalIdentityPolicyDocument(document storageapi.ObjectStorageIdentityPolicyDocument) (string, error) {
	bytes, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal identity policy document: %w", err)
	}
	return string(bytes), nil
}

// identityPoliciesAPI extracts the configured identity policies as upstream
// API structs. Returns nil + nil diagnostics when the list is null or empty
// so callers can treat "no policies" uniformly.
func (m *ObjectStorageEndpointModel) identityPoliciesAPI(
	ctx context.Context,
) ([]storageapi.ObjectStorageIdentityPolicySpec, diag.Diagnostics) {
	if m.IdentityPolicies.IsNull() || m.IdentityPolicies.IsUnknown() {
		return nil, nil
	}

	var policies []objectStorageEndpointIdentityPolicyModel
	if diagnostics := m.IdentityPolicies.ElementsAs(ctx, &policies, false); diagnostics.HasError() {
		return nil, diagnostics
	}

	if len(policies) == 0 {
		return nil, nil
	}

	out := make([]storageapi.ObjectStorageIdentityPolicySpec, 0, len(policies))
	var diagnostics diag.Diagnostics
	for _, policy := range policies {
		var document storageapi.ObjectStorageIdentityPolicyDocument
		if err := json.Unmarshal([]byte(policy.Document.ValueString()), &document); err != nil {
			diagnostics.AddError(
				"Invalid Identity Policy Document",
				fmt.Sprintf("Identity policy %q has an invalid JSON document: %s", policy.Name.ValueString(), err),
			)
			continue
		}
		out = append(out, storageapi.ObjectStorageIdentityPolicySpec{
			Name:     policy.Name.ValueString(),
			Document: document,
		})
	}

	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return out, nil
}

func (m *ObjectStorageEndpointModel) NscaleObjectStorageEndpointCreateParams(
	ctx context.Context,
	organizationID string,
) (storageapi.ObjectStorageEndpointCreate, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return storageapi.ObjectStorageEndpointCreate{}, diagnostics
	}
	tags = nscale.RemoveOperationTags(tags)

	policies, policyDiags := m.identityPoliciesAPI(ctx)
	if policyDiags.HasError() {
		return storageapi.ObjectStorageEndpointCreate{}, policyDiags
	}

	endpoint := storageapi.ObjectStorageEndpointCreate{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: struct {
			IdentityPolicies             *storageapi.ObjectStorageIdentityPolicyList `json:"identityPolicies,omitempty"`
			ObjectStorageEndpointClassId string                                      `json:"objectStorageEndpointClassId"`
			OrganizationId               string                                      `json:"organizationId"`
			ProjectId                    string                                      `json:"projectId"`
			RegionId                     string                                      `json:"regionId"`
		}{
			IdentityPolicies:             nil,
			ObjectStorageEndpointClassId: m.EndpointClassID.ValueString(),
			OrganizationId:               organizationID,
			ProjectId:                    m.ProjectID.ValueString(),
			RegionId:                     m.RegionID.ValueString(),
		},
	}

	if policies != nil {
		policyList := policies
		endpoint.Spec.IdentityPolicies = &policyList
	}

	return endpoint, nil
}

func (m *ObjectStorageEndpointModel) NscaleObjectStorageEndpointUpdateParams(
	ctx context.Context,
) (storageapi.ObjectStorageEndpointUpdate, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return storageapi.ObjectStorageEndpointUpdate{}, diagnostics
	}
	tags = nscale.RemoveOperationTags(tags)

	policies, policyDiags := m.identityPoliciesAPI(ctx)
	if policyDiags.HasError() {
		return storageapi.ObjectStorageEndpointUpdate{}, policyDiags
	}

	update := storageapi.ObjectStorageEndpointUpdate{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: &storageapi.ObjectStorageEndpointUpdateSpec{IdentityPolicies: nil},
	}

	if policies != nil {
		policyList := policies
		update.Spec.IdentityPolicies = &policyList
	}

	return update, nil
}
