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

package reservation

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/pointer"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
)

type PlacementModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Tags               types.Map    `tfsdk:"tags"`
	ReservationID      types.String `tfsdk:"reservation_id"`
	NetworkID          types.String `tfsdk:"network_id"`
	HostCount          types.Int64  `tfsdk:"host_count"`
	Constraints        types.Object `tfsdk:"constraints"`
	ServerSpec         types.Object `tfsdk:"server_spec"`
	RegionID           types.String `tfsdk:"region_id"`
	ReadyHostCount     types.Int64  `tfsdk:"ready_host_count"`
	ProjectID          types.String `tfsdk:"project_id"`
	CreationTime       types.String `tfsdk:"creation_time"`
	ProvisioningStatus types.String `tfsdk:"provisioning_status"`
}

// PlacementConstraintsModelAttributeType describes the constraints sub-object.
var PlacementConstraintsModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"policy":             types.StringType,
		"max_skew":           types.Int64Type,
		"min_domains":        types.Int64Type,
		"when_unsatisfiable": types.StringType,
	},
}

type PlacementConstraintsModel struct {
	Policy            types.String `tfsdk:"policy"`
	MaxSkew           types.Int64  `tfsdk:"max_skew"`
	MinDomains        types.Int64  `tfsdk:"min_domains"`
	WhenUnsatisfiable types.String `tfsdk:"when_unsatisfiable"`
}

// PlacementServerNetworkingModelAttributeType describes the optional networking
// sub-object nested inside server_spec.
var PlacementServerNetworkingModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"enable_public_ip":         types.BoolType,
		"security_group_ids":       types.ListType{ElemType: types.StringType},
		"allowed_source_addresses": types.ListType{ElemType: types.StringType},
	},
}

type PlacementServerNetworkingModel struct {
	EnablePublicIP         types.Bool `tfsdk:"enable_public_ip"`
	SecurityGroupIDs       types.List `tfsdk:"security_group_ids"`
	AllowedSourceAddresses types.List `tfsdk:"allowed_source_addresses"`
}

// PlacementServerSpecModelAttributeType describes the server_spec sub-object.
var PlacementServerSpecModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"image_id":                     types.StringType,
		"ssh_certificate_authority_id": types.StringType,
		"user_data":                    types.StringType,
		"networking":                   PlacementServerNetworkingModelAttributeType,
	},
}

type PlacementServerSpecModel struct {
	ImageID                   types.String `tfsdk:"image_id"`
	SSHCertificateAuthorityID types.String `tfsdk:"ssh_certificate_authority_id"`
	UserData                  types.String `tfsdk:"user_data"`
	Networking                types.Object `tfsdk:"networking"`
}

func NewPlacementModel(source *reservationapi.PlacementV2Read) PlacementModel {
	tags := nscale.RemoveOperationTags(source.Metadata.Tags)

	readyHostCount := types.Int64Null()
	if source.Status.ReadyHostCount != nil {
		readyHostCount = types.Int64Value(int64(*source.Status.ReadyHostCount))
	}

	return PlacementModel{
		ID:                 types.StringValue(source.Metadata.Id),
		Name:               types.StringValue(source.Metadata.Name),
		Description:        types.StringPointerValue(source.Metadata.Description),
		Tags:               tftypes.TagMapValueMust(tags),
		ReservationID:      types.StringValue(source.Status.ReservationId),
		NetworkID:          types.StringValue(source.Status.NetworkId),
		HostCount:          types.Int64Value(int64(source.Spec.Count)),
		Constraints:        newPlacementConstraintsObject(source.Spec.Constraints),
		ServerSpec:         newPlacementServerSpecObject(source.Spec.ServerSpec),
		RegionID:           types.StringValue(source.Status.RegionId),
		ReadyHostCount:     readyHostCount,
		ProjectID:          types.StringValue(source.Metadata.ProjectId),
		CreationTime:       types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
		ProvisioningStatus: types.StringValue(string(source.Metadata.ProvisioningStatus)),
	}
}

func newPlacementConstraintsObject(source reservationapi.PlacementConstraintsV2) types.Object {
	maxSkew := types.Int64Null()
	if source.MaxSkew != nil {
		maxSkew = types.Int64Value(int64(*source.MaxSkew))
	}

	minDomains := types.Int64Null()
	if source.MinDomains != nil {
		minDomains = types.Int64Value(int64(*source.MinDomains))
	}

	whenUnsatisfiable := types.StringNull()
	if source.WhenUnsatisfiable != nil {
		whenUnsatisfiable = types.StringValue(string(*source.WhenUnsatisfiable))
	}

	return types.ObjectValueMust(
		PlacementConstraintsModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"policy":             types.StringValue(string(source.Policy)),
			"max_skew":           maxSkew,
			"min_domains":        minDomains,
			"when_unsatisfiable": whenUnsatisfiable,
		},
	)
}

func newPlacementServerSpecObject(source reservationapi.PlacementServerSpecV2) types.Object {
	userData := types.StringNull()
	if source.UserData != nil {
		// The API returns the decoded bytes; re-encode to base64 so the value
		// round-trips against the base64-encoded string supplied in configuration.
		userData = types.StringValue(base64.StdEncoding.EncodeToString(*source.UserData))
	}

	return types.ObjectValueMust(
		PlacementServerSpecModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"image_id":                     types.StringValue(source.ImageId),
			"ssh_certificate_authority_id": types.StringPointerValue(source.SshCertificateAuthorityId),
			"user_data":                    userData,
			"networking":                   newPlacementServerNetworkingObject(source.Networking),
		},
	)
}

func newPlacementServerNetworkingObject(source *reservationapi.PlacementServerNetworkingV2) types.Object {
	if source == nil {
		return types.ObjectNull(PlacementServerNetworkingModelAttributeType.AttrTypes)
	}

	// security_group_ids and allowed_source_addresses are Optional+Computed lists
	// the user can set. Emit a known empty list (not null) when there are no
	// elements so a configured `[]` round-trips: the API echoes `[]` faithfully,
	// and collapsing it to null here would trip "inconsistent result after apply:
	// was [], now null". (This is why we avoid tftypes.NullableListValueMust,
	// which nulls empty lists, for these two fields.)
	securityGroupIDs := make([]attr.Value, 0)
	if source.SecurityGroups != nil {
		for _, securityGroupID := range *source.SecurityGroups {
			securityGroupIDs = append(securityGroupIDs, types.StringValue(securityGroupID))
		}
	}

	allowedSourceAddresses := make([]attr.Value, 0)
	if source.AllowedSourceAddresses != nil {
		for _, allowedSourceAddress := range *source.AllowedSourceAddresses {
			allowedSourceAddresses = append(allowedSourceAddresses, types.StringValue(allowedSourceAddress))
		}
	}

	return types.ObjectValueMust(
		PlacementServerNetworkingModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"enable_public_ip":         types.BoolPointerValue(source.PublicIP),
			"security_group_ids":       types.ListValueMust(types.StringType, securityGroupIDs),
			"allowed_source_addresses": types.ListValueMust(types.StringType, allowedSourceAddresses),
		},
	)
}

// NscalePlacementCreateParams builds the create request body from the plan.
func (m *PlacementModel) NscalePlacementCreateParams(
	ctx context.Context,
) (reservationapi.PlacementV2Create, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return reservationapi.PlacementV2Create{}, diagnostics
	}
	tags = nscale.RemoveOperationTags(tags)

	constraints, diagnostics := m.constraints(ctx)
	if diagnostics.HasError() {
		return reservationapi.PlacementV2Create{}, diagnostics
	}

	serverSpec, diagnostics := m.serverSpec(ctx)
	if diagnostics.HasError() {
		return reservationapi.PlacementV2Create{}, diagnostics
	}

	return reservationapi.PlacementV2Create{
		Metadata: coreapi.ResourceWriteMetadata{
			Name:        m.Name.ValueString(),
			Description: m.Description.ValueStringPointer(),
			Tags:        tags,
		},
		Spec: reservationapi.PlacementV2CreateSpec{
			ReservationId: m.ReservationID.ValueString(),
			NetworkId:     m.NetworkID.ValueString(),
			Count:         int(m.HostCount.ValueInt64()),
			Constraints:   constraints,
			// readiness_policy is a v2 field not yet exposed by this provider;
			// omitting it lets the API apply its default.
			ReadinessPolicy: nil,
			ServerSpec:      serverSpec,
		},
	}, nil
}

func (m *PlacementModel) constraints(ctx context.Context) (reservationapi.PlacementConstraintsV2, diag.Diagnostics) {
	var model PlacementConstraintsModel
	if diagnostics := m.Constraints.As(ctx, &model, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		return reservationapi.PlacementConstraintsV2{}, diagnostics
	}

	var maxSkew *int
	if !model.MaxSkew.IsNull() && !model.MaxSkew.IsUnknown() {
		value := int(model.MaxSkew.ValueInt64())
		maxSkew = &value
	}

	var minDomains *int
	if !model.MinDomains.IsNull() && !model.MinDomains.IsUnknown() {
		value := int(model.MinDomains.ValueInt64())
		minDomains = &value
	}

	var whenUnsatisfiable *reservationapi.WhenUnsatisfiableV2
	if value := model.WhenUnsatisfiable.ValueString(); value != "" {
		unsatisfiable := reservationapi.WhenUnsatisfiableV2(value)
		whenUnsatisfiable = &unsatisfiable
	}

	return reservationapi.PlacementConstraintsV2{
		Policy:            reservationapi.PlacementPolicyV2(model.Policy.ValueString()),
		MaxSkew:           maxSkew,
		MinDomains:        minDomains,
		WhenUnsatisfiable: whenUnsatisfiable,
	}, nil
}

func (m *PlacementModel) serverSpec(ctx context.Context) (reservationapi.PlacementServerSpecV2, diag.Diagnostics) {
	var model PlacementServerSpecModel
	if diagnostics := m.ServerSpec.As(ctx, &model, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		return reservationapi.PlacementServerSpecV2{}, diagnostics
	}

	var userData *[]byte
	if value := model.UserData.ValueString(); value != "" {
		// user_data is supplied as a base64-encoded string. The SDK serializes the
		// []byte field as base64 itself, so decode here to avoid double-encoding;
		// the Base64Validator on the attribute guarantees the value is well-formed.
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			var diagnostics diag.Diagnostics
			diagnostics.AddError(
				"Invalid user_data",
				fmt.Sprintf("Failed to decode base64 user_data: %s", err),
			)
			return reservationapi.PlacementServerSpecV2{}, diagnostics
		}
		userData = &decoded
	}

	networking, diagnostics := model.networking(ctx)
	if diagnostics.HasError() {
		return reservationapi.PlacementServerSpecV2{}, diagnostics
	}

	return reservationapi.PlacementServerSpecV2{
		ImageId:                   model.ImageID.ValueString(),
		SshCertificateAuthorityId: model.SSHCertificateAuthorityID.ValueStringPointer(),
		UserData:                  userData,
		Networking:                networking,
	}, nil
}

func (m *PlacementServerSpecModel) networking(
	ctx context.Context,
) (*reservationapi.PlacementServerNetworkingV2, diag.Diagnostics) {
	if m.Networking.IsNull() || m.Networking.IsUnknown() {
		return nil, nil
	}

	var model PlacementServerNetworkingModel
	if diagnostics := m.Networking.As(ctx, &model, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		return nil, diagnostics
	}

	// security_group_ids and allowed_source_addresses are Optional+Computed, so
	// Terraform passes them as unknown when the user omits them. ElementsAs into
	// a plain []string cannot represent unknown, so guard each list and treat
	// null/unknown as "omit".
	var securityGroupIDs []string
	if !model.SecurityGroupIDs.IsNull() && !model.SecurityGroupIDs.IsUnknown() {
		if diagnostics := model.SecurityGroupIDs.ElementsAs(ctx, &securityGroupIDs, false); diagnostics.HasError() {
			return nil, diagnostics
		}
	}

	var allowedSourceAddresses []string
	if !model.AllowedSourceAddresses.IsNull() && !model.AllowedSourceAddresses.IsUnknown() {
		diagnostics := model.AllowedSourceAddresses.ElementsAs(ctx, &allowedSourceAddresses, false)
		if diagnostics.HasError() {
			return nil, diagnostics
		}
	}

	return &reservationapi.PlacementServerNetworkingV2{
		PublicIP:               model.EnablePublicIP.ValueBoolPointer(),
		SecurityGroups:         pointer.ReferenceSlice(securityGroupIDs),
		AllowedSourceAddresses: pointer.ReferenceSlice(allowedSourceAddresses),
	}, nil
}
