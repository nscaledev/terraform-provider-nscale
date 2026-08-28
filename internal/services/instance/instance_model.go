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
	computeapi "github.com/nscaledev/nscale-sdk-go/compute"
	identityids "github.com/unikorn-cloud/identity/pkg/ids"
	regionids "github.com/unikorn-cloud/region/pkg/ids"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/pointer"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
)

type InstanceModel struct {
	ID                        types.String `tfsdk:"id"`
	Name                      types.String `tfsdk:"name"`
	Description               types.String `tfsdk:"description"`
	NetworkInterface          types.Object `tfsdk:"network_interface"`
	UserData                  types.String `tfsdk:"user_data"`
	PublicIP                  types.String `tfsdk:"public_ip"`
	PrivateIP                 types.String `tfsdk:"private_ip"`
	PowerState                types.String `tfsdk:"power_state"`
	ImageID                   types.String `tfsdk:"image_id"`
	FlavorID                  types.String `tfsdk:"flavor_id"`
	SSHCertificateAuthorityID types.String `tfsdk:"ssh_certificate_authority_id"`
	Tags                      types.Map    `tfsdk:"tags"`
	ProjectID                 types.String `tfsdk:"project_id"`
	RegionID                  types.String `tfsdk:"region_id"`
	CreationTime              types.String `tfsdk:"creation_time"`
}

func NewInstanceModel(source *computeapi.InstanceRead) InstanceModel {
	powerState := types.StringNull()
	if source.Status.PowerState != nil {
		powerState = types.StringValue(string(*source.Status.PowerState))
	}

	tags := nscale.RemoveOperationTags(source.Metadata.Tags)
	sshCertificateAuthorityID := types.StringNull()
	if source.Spec.SshCertificateAuthorityId != nil {
		sshCertificateAuthorityID = types.StringValue(source.Spec.SshCertificateAuthorityId.String())
	}

	return InstanceModel{
		ID:                        types.StringValue(source.Metadata.Id),
		Name:                      types.StringValue(source.Metadata.Name),
		Description:               types.StringPointerValue(source.Metadata.Description),
		NetworkInterface:          NewInstanceNetworkInterfaceModel(source.Spec, source.Status),
		UserData:                  tftypes.Base64StringValue(source.Spec.UserData),
		PublicIP:                  types.StringPointerValue(source.Status.PublicIP),
		PrivateIP:                 types.StringPointerValue(source.Status.PrivateIP),
		PowerState:                powerState,
		ImageID:                   types.StringValue(source.Spec.ImageId.String()),
		FlavorID:                  types.StringValue(source.Spec.FlavorId.String()),
		SSHCertificateAuthorityID: sshCertificateAuthorityID,
		Tags:                      tftypes.TagMapValueMust(tags),
		ProjectID:                 types.StringValue(source.Metadata.ProjectId),
		RegionID:                  types.StringValue(source.Status.RegionId),
		CreationTime:              types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

func (m *InstanceModel) NscaleInstanceCreateParams(
	organizationID string,
) (computeapi.InstanceCreate, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer[computeapi.Tag](m.Tags)
	if diagnostics.HasError() {
		return computeapi.InstanceCreate{}, diagnostics
	}

	tags = nscale.RemoveOperationTags(tags)

	var sourceNetworkInterface InstanceNetworkInterfaceModel
	if diagnostics = m.NetworkInterface.As(
		context.TODO(),
		&sourceNetworkInterface,
		basetypes.ObjectAsOptions{},
	); diagnostics.HasError() {
		return computeapi.InstanceCreate{}, diagnostics
	}

	networking, diagnostics := sourceNetworkInterface.NscaleInstanceNetworking()
	if diagnostics.HasError() {
		return computeapi.InstanceCreate{}, diagnostics
	}

	userData, diagnostics := tftypes.ValueBase64BytesPointer(m.UserData, "user_data")
	if diagnostics.HasError() {
		return computeapi.InstanceCreate{}, diagnostics
	}

	flavorID, imageID, sshCertificateAuthorityID, ok := m.instanceSpecIDs(&diagnostics)
	if !ok {
		return computeapi.InstanceCreate{}, diagnostics
	}

	networkID, ok := nscale.ParseID(
		sourceNetworkInterface.NetworkID.ValueString(),
		"Network",
		regionids.ParseNetworkID,
		&diagnostics,
	)
	if !ok {
		return computeapi.InstanceCreate{}, diagnostics
	}

	parsedOrganizationID, ok := nscale.ParseID(
		organizationID,
		"Organization",
		identityids.ParseOrganizationID,
		&diagnostics,
	)
	if !ok {
		return computeapi.InstanceCreate{}, diagnostics
	}

	projectID, ok := nscale.ParseID(
		m.ProjectID.ValueString(),
		"Project",
		identityids.ParseProjectID,
		&diagnostics,
	)
	if !ok {
		return computeapi.InstanceCreate{}, diagnostics
	}

	instance := computeapi.InstanceCreate{
		Metadata: computeapi.ResourceMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: computeapi.InstanceCreateSpec{
			FlavorId:                  flavorID,
			ImageId:                   imageID,
			NetworkId:                 computeapi.NetworkId(networkID),
			Networking:                &networking,
			OrganizationId:            computeapi.OrganizationId(parsedOrganizationID),
			ProjectId:                 computeapi.ProjectId(projectID),
			SshCertificateAuthorityId: sshCertificateAuthorityID,
			UserData:                  userData,
		},
	}

	return instance, nil
}

func (m *InstanceModel) NscaleInstanceUpdateParams() (computeapi.InstanceUpdate, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer[computeapi.Tag](m.Tags)
	if diagnostics.HasError() {
		return computeapi.InstanceUpdate{}, diagnostics
	}

	tags = nscale.RemoveOperationTags(tags)

	var sourceNetworkInterface InstanceNetworkInterfaceModel
	if diagnostics = m.NetworkInterface.As(
		context.TODO(),
		&sourceNetworkInterface,
		basetypes.ObjectAsOptions{},
	); diagnostics.HasError() {
		return computeapi.InstanceUpdate{}, diagnostics
	}

	networking, diagnostics := sourceNetworkInterface.NscaleInstanceNetworking()
	if diagnostics.HasError() {
		return computeapi.InstanceUpdate{}, diagnostics
	}

	userData, diagnostics := tftypes.ValueBase64BytesPointer(m.UserData, "user_data")
	if diagnostics.HasError() {
		return computeapi.InstanceUpdate{}, diagnostics
	}

	flavorID, imageID, sshCertificateAuthorityID, ok := m.instanceSpecIDs(&diagnostics)
	if !ok {
		return computeapi.InstanceUpdate{}, diagnostics
	}

	instance := computeapi.InstanceUpdate{
		Metadata: computeapi.ResourceMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: computeapi.InstanceSpec{
			FlavorId:                  flavorID,
			ImageId:                   imageID,
			Networking:                &networking,
			SshCertificateAuthorityId: sshCertificateAuthorityID,
			UserData:                  userData,
		},
	}

	return instance, nil
}

func (m *InstanceModel) instanceSpecIDs(
	diagnostics *diag.Diagnostics,
) (computeapi.FlavorId, computeapi.ImageId, *computeapi.SshCertificateAuthorityId, bool) {
	flavorID, ok := nscale.ParseID(m.FlavorID.ValueString(), "Flavor", regionids.ParseFlavorID, diagnostics)
	if !ok {
		return computeapi.FlavorId{}, computeapi.ImageId{}, nil, false
	}

	imageID, ok := nscale.ParseID(m.ImageID.ValueString(), "Image", regionids.ParseImageID, diagnostics)
	if !ok {
		return computeapi.FlavorId{}, computeapi.ImageId{}, nil, false
	}

	var sshCertificateAuthorityID *computeapi.SshCertificateAuthorityId
	if value := m.SSHCertificateAuthorityID.ValueString(); value != "" {
		parsedID, parsed := nscale.ParseID(
			value,
			"SSH Certificate Authority",
			regionids.ParseSSHCertificateAuthorityID,
			diagnostics,
		)
		if !parsed {
			return computeapi.FlavorId{}, computeapi.ImageId{}, nil, false
		}

		converted := computeapi.SshCertificateAuthorityId(parsedID)
		sshCertificateAuthorityID = &converted
	}

	return computeapi.FlavorId(flavorID), computeapi.ImageId(imageID), sshCertificateAuthorityID, true
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
	enablePublicIP := types.BoolPointerValue(spec.Networking.PublicIP)

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
	if diagnostics := m.AllowedDestinations.ElementsAs(
		context.TODO(),
		&allowedSourceAddresses,
		false,
	); diagnostics.HasError() {
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
