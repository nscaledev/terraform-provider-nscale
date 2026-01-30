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

package securitygroup

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

type SecurityGroupModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Rules        types.List   `tfsdk:"rules"`
	NetworkID    types.String `tfsdk:"network_id"`
	Tags         types.Map    `tfsdk:"tags"`
	RegionID     types.String `tfsdk:"region_id"`
	CreationTime types.String `tfsdk:"creation_time"`
}

func NewSecurityGroupModel(source *regionapi.SecurityGroupV2Read) SecurityGroupModel {
	tags := nscale.RemoveOperationTags(source.Metadata.Tags)

	return SecurityGroupModel{
		ID:           types.StringValue(source.Metadata.Id),
		Name:         types.StringValue(source.Metadata.Name),
		Description:  types.StringPointerValue(source.Metadata.Description),
		Rules:        NewSecurityGroupRuleModels(source.Spec.Rules),
		NetworkID:    types.StringValue(source.Status.NetworkId),
		Tags:         tftypes.TagMapValueMust(tags),
		RegionID:     types.StringValue(source.Status.RegionId),
		CreationTime: types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

var SecurityGroupRuleModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"type":       types.StringType,
		"protocol":   types.StringType,
		"from_port":  types.Int32Type,
		"to_port":    types.Int32Type,
		"cidr_block": types.StringType,
	},
}

type SecurityGroupRuleModel struct {
	Type      types.String `tfsdk:"type"`
	Protocol  types.String `tfsdk:"protocol"`
	FromPort  types.Int32  `tfsdk:"from_port"`
	ToPort    types.Int32  `tfsdk:"to_port"`
	CIDRBlock types.String `tfsdk:"cidr_block"`
}

func NewSecurityGroupRuleModel(source regionapi.SecurityGroupRuleV2) attr.Value {
	fromPort := types.Int32Null()
	if source.Port != nil {
		fromPort = types.Int32Value(int32(*source.Port))
	}

	toPort := types.Int32Null()
	if source.PortMax != nil {
		toPort = types.Int32Value(int32(*source.PortMax))
	}

	cidrBlock := types.StringNull()
	if source.Prefix != nil {
		cidrBlock = types.StringValue(*source.Prefix)
	}

	return types.ObjectValueMust(
		SecurityGroupRuleModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"type":       types.StringValue(string(source.Direction)),
			"protocol":   types.StringValue(string(source.Protocol)),
			"from_port":  fromPort,
			"to_port":    toPort,
			"cidr_block": cidrBlock,
		},
	)
}

func (m *SecurityGroupModel) NscaleSecurityGroupCreateParams() (regionapi.SecurityGroupV2Create, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return regionapi.SecurityGroupV2Create{}, diagnostics
	}

	tags = nscale.RemoveOperationTags(tags)

	var sourceRules []SecurityGroupRuleModel
	if diagnostics = m.Rules.ElementsAs(context.TODO(), &sourceRules, false); diagnostics.HasError() {
		return regionapi.SecurityGroupV2Create{}, diagnostics
	}

	rules := make([]regionapi.SecurityGroupRuleV2, 0, len(sourceRules))
	for _, source := range sourceRules {
		rules = append(rules, source.NscaleSecurityGroupRule())
	}

	securityGroup := regionapi.SecurityGroupV2Create{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: regionapi.SecurityGroupV2CreateSpec{
			NetworkId: m.NetworkID.ValueString(),
			Rules:     rules,
		},
	}

	return securityGroup, nil
}

func (m *SecurityGroupModel) NscaleSecurityGroupUpdateParams() (regionapi.SecurityGroupV2Update, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return regionapi.SecurityGroupV2Update{}, diagnostics
	}

	tags = nscale.RemoveOperationTags(tags)

	var sourceRules []SecurityGroupRuleModel
	if diagnostics = m.Rules.ElementsAs(context.TODO(), &sourceRules, false); diagnostics.HasError() {
		return regionapi.SecurityGroupV2Update{}, diagnostics
	}

	rules := make([]regionapi.SecurityGroupRuleV2, 0, len(sourceRules))
	for _, source := range sourceRules {
		rules = append(rules, source.NscaleSecurityGroupRule())
	}

	securityGroup := regionapi.SecurityGroupV2Update{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: regionapi.SecurityGroupV2Spec{
			Rules: rules,
		},
	}

	return securityGroup, nil
}

func NewSecurityGroupRuleModels(source []regionapi.SecurityGroupRuleV2) types.List {
	rules := make([]attr.Value, 0, len(source))
	for _, data := range source {
		rules = append(rules, NewSecurityGroupRuleModel(data))
	}
	return types.ListValueMust(SecurityGroupRuleModelAttributeType, rules)
}

func (m *SecurityGroupRuleModel) NscaleSecurityGroupRule() regionapi.SecurityGroupRuleV2 {
	var port *int
	if value := m.FromPort.ValueInt32Pointer(); value != nil {
		temp := int(*value)
		port = &temp
	}

	var portMax *int
	if value := m.ToPort.ValueInt32Pointer(); value != nil {
		temp := int(*value)
		port = &temp
	}

	return regionapi.SecurityGroupRuleV2{
		Direction: regionapi.NetworkDirection(m.Type.ValueString()),
		Port:      port,
		PortMax:   portMax,
		Prefix:    m.CIDRBlock.ValueStringPointer(),
		Protocol:  regionapi.NetworkProtocol(m.Protocol.ValueString()),
	}
}
