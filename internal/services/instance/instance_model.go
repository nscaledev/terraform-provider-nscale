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

package instance

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/pointer"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
)

type InstanceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	NetworkInterface types.Object `tfsdk:"network_interface"`
	UserData         types.String `tfsdk:"user_data"`
	PublicIP         types.String `tfsdk:"public_ip"`
	PrivateIP        types.String `tfsdk:"private_ip"`
	PowerState       types.String `tfsdk:"power_state"`
	ImageID          types.String `tfsdk:"image_id"`
	FlavorID         types.String `tfsdk:"flavor_id"`
	RegionID         types.String `tfsdk:"region_id"`
	CreationTime     types.String `tfsdk:"creation_time"`
}

func NewInstanceModel(source *computeapi.InstanceRead) InstanceModel {
	userData := types.StringNull()
	if source.Spec.UserData != nil {
		userData = types.StringValue(string(*source.Spec.UserData))
	}

	powerState := types.StringNull()
	if source.Status.PowerState != nil {
		powerState = types.StringValue(string(*source.Status.PowerState))
	}

	return InstanceModel{
		ID:               types.StringValue(source.Metadata.Id),
		Name:             types.StringValue(source.Metadata.Name),
		Description:      types.StringPointerValue(source.Metadata.Description),
		NetworkInterface: NewInstanceNetworkInterfaceModel(source.Spec, source.Status),
		UserData:         userData,
		PublicIP:         types.StringPointerValue(source.Status.PublicIP),
		PrivateIP:        types.StringPointerValue(source.Status.PrivateIP),
		PowerState:       powerState,
		ImageID:          types.StringValue(source.Spec.ImageId),
		FlavorID:         types.StringValue(source.Spec.FlavorId),
		RegionID:         types.StringValue(source.Status.RegionId),
		CreationTime:     types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

func (m *InstanceModel) NscaleInstanceCreateParams(organizationID, projectID string) (computeapi.InstanceCreate, diag.Diagnostics) {
	var sourceNetworkInterface InstanceNetworkInterfaceModel
	if diagnostics := m.NetworkInterface.As(context.TODO(), &sourceNetworkInterface, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		return computeapi.InstanceCreate{}, diagnostics
	}
	networking, diagnostics := sourceNetworkInterface.NscaleInstanceNetworking()
	if diagnostics.HasError() {
		return computeapi.InstanceCreate{}, diagnostics
	}

	var userData *[]byte
	if value := m.UserData.ValueString(); value != "" {
		temp := []byte(value)
		userData = &temp
	}

	instance := computeapi.InstanceCreate{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			// REVIEW_ME: Not sure what the tags are for. Even the UI doesn’t provide a way to set them, so leaving it as nil for now.
			Tags: nil,
		},
		Spec: computeapi.InstanceCreateSpec{
			FlavorId:       m.FlavorID.ValueString(),
			ImageId:        m.ImageID.ValueString(),
			NetworkId:      sourceNetworkInterface.NetworkID.ValueString(),
			Networking:     &networking,
			OrganizationId: organizationID,
			ProjectId:      projectID,
			UserData:       userData,
		},
	}

	return instance, nil
}

func (m *InstanceModel) NscaleInstanceUpdateParams() (computeapi.InstanceUpdate, diag.Diagnostics) {
	var sourceNetworkInterface InstanceNetworkInterfaceModel
	if diagnostics := m.NetworkInterface.As(context.TODO(), &sourceNetworkInterface, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		return computeapi.InstanceUpdate{}, diagnostics
	}

	networking, diagnostics := sourceNetworkInterface.NscaleInstanceNetworking()
	if diagnostics.HasError() {
		return computeapi.InstanceUpdate{}, diagnostics
	}

	var userData *[]byte
	if value := m.UserData.ValueString(); value != "" {
		temp := []byte(value)
		userData = &temp
	}

	instance := computeapi.InstanceUpdate{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			// REVIEW_ME: Not sure what the tags are for. Even the UI doesn’t provide a way to set them, so leaving it as nil for now.
			Tags: nil,
		},
		Spec: computeapi.InstanceSpec{
			FlavorId:   m.FlavorID.ValueString(),
			ImageId:    m.ImageID.ValueString(),
			Networking: &networking,
			UserData:   userData,
		},
	}

	return instance, nil
}

var InstanceNetworkInterfaceModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"network_id":           types.StringType,
		"enable_public_ip":     types.BoolType,
		"security_group_ids":   types.ListType{ElemType: types.StringType},
		"allowed_destinations": types.ListType{ElemType: types.StringType},
	},
}

type InstanceNetworkInterfaceModel struct {
	NetworkID           types.String `tfsdk:"network_id"`
	EnablePublicIP      types.Bool   `tfsdk:"enable_public_ip"`
	SecurityGroupIDs    types.List   `tfsdk:"security_group_ids"`
	AllowedDestinations types.List   `tfsdk:"allowed_destinations"`
}

func NewInstanceNetworkInterfaceModel(spec computeapi.InstanceSpec, status computeapi.InstanceStatus) types.Object {
	enablePublicIP := types.BoolValue(false)
	if spec.Networking.PublicIP != nil {
		// REVIEW_ME: Should we derive the value from the status, or rely on the spec definition?
		enablePublicIP = types.BoolValue(*spec.Networking.PublicIP)
	}

	var securityGroupIDs []attr.Value
	if securityGroups := spec.Networking.SecurityGroups; securityGroups != nil {
		securityGroupIDs = make([]attr.Value, 0, len(*securityGroups))
		for _, securityGroupID := range *securityGroups {
			securityGroupIDs = append(securityGroupIDs, types.StringValue(securityGroupID))
		}
	}

	var allowedDestinations []attr.Value
	if allowedSourceAddresses := spec.Networking.AllowedSourceAddresses; allowedSourceAddresses != nil {
		allowedDestinations = make([]attr.Value, 0, len(*allowedSourceAddresses))
		for _, allowedSourceAddress := range *allowedSourceAddresses {
			allowedDestinations = append(allowedDestinations, types.StringValue(allowedSourceAddress))
		}
	}

	return types.ObjectValueMust(
		InstanceNetworkInterfaceModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"network_id":           types.StringValue(status.NetworkId),
			"enable_public_ip":     enablePublicIP,
			"security_group_ids":   tftypes.NullableListValueMust(types.StringType, securityGroupIDs),
			"allowed_destinations": tftypes.NullableListValueMust(types.StringType, allowedDestinations),
		},
	)
}

func (m *InstanceNetworkInterfaceModel) NscaleInstanceNetworking() (computeapi.InstanceNetworking, diag.Diagnostics) {
	var allowedSourceAddresses []string
	if diagnostics := m.AllowedDestinations.ElementsAs(context.TODO(), &allowedSourceAddresses, false); diagnostics.HasError() {
		return computeapi.InstanceNetworking{}, diagnostics
	}

	var securityGroupIDs []string
	if diagnostics := m.SecurityGroupIDs.ElementsAs(context.TODO(), &securityGroupIDs, false); diagnostics.HasError() {
		return computeapi.InstanceNetworking{}, diagnostics
	}

	networking := computeapi.InstanceNetworking{
		AllowedSourceAddresses: pointer.ReferenceSlice(allowedSourceAddresses),
		PublicIP:               m.EnablePublicIP.ValueBoolPointer(),
		SecurityGroups:         pointer.ReferenceSlice(securityGroupIDs),
	}

	return networking, nil
}
