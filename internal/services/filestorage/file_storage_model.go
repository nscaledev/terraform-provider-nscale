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

package filestorage

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

type FileStorageModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	StorageClassID types.String `tfsdk:"storage_class_id"`
	Size           types.Int64  `tfsdk:"size"`
	Capacity       types.Int64  `tfsdk:"capacity"`
	RootSquash     types.Bool   `tfsdk:"root_squash"`
	Network        types.List   `tfsdk:"network"`
	RegionID       types.String `tfsdk:"region_id"`
	CreationTime   types.String `tfsdk:"creation_time"`
}

func NewFileStorageModel(source *regionapi.StorageV2Read) FileStorageModel {
	size := types.Int64Value(0)
	if source.Status.Usage != nil && source.Status.Usage.UsedBytes != nil {
		size = types.Int64Value(*source.Status.Usage.UsedBytes >> 30)
	}

	rootSquash := types.BoolNull()
	if source.Spec.StorageType.NFS != nil {
		rootSquash = types.BoolValue(source.Spec.StorageType.NFS.RootSquash)
	}

	networks := types.ListNull(FileStorageNetworkModelAttributeType)
	if source.Status.Attachments != nil {
		networks = NewFileStorageNetworkModels(*source.Status.Attachments)
	}

	return FileStorageModel{
		ID:             types.StringValue(source.Metadata.Id),
		Name:           types.StringValue(source.Metadata.Name),
		Description:    types.StringPointerValue(source.Metadata.Description),
		StorageClassID: types.StringValue(source.Status.StorageClassId),
		Size:           size,
		Capacity:       types.Int64Value(source.Spec.SizeGiB),
		RootSquash:     rootSquash,
		Network:        networks,
		RegionID:       types.StringValue(source.Status.RegionId),
		CreationTime:   types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

func (m *FileStorageModel) networkIDs() ([]string, diag.Diagnostics) {
	var networks []FileStorageNetworkModel
	if diagnostics := m.Network.ElementsAs(context.TODO(), &networks, false); diagnostics.HasError() {
		return nil, diagnostics
	}

	networkIDs := make([]string, 0, len(networks))
	for _, network := range networks {
		networkIDs = append(networkIDs, network.ID.ValueString())
	}

	return networkIDs, nil
}

func (m *FileStorageModel) NscaleFileStorageCreateParams(organizationID, projectID string) (regionapi.StorageV2Create, diag.Diagnostics) {
	networkIDs, diagnostics := m.networkIDs()
	if diagnostics.HasError() {
		return regionapi.StorageV2Create{}, diagnostics
	}

	fileStorage := regionapi.StorageV2Create{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			// REVIEW_ME: Not sure what the tags are for. Even the UI doesn’t provide a way to set them, so leaving it as nil for now.
			Tags: nil,
		},
		Spec: struct {
			Attachments    *regionapi.StorageAttachmentV2Spec `json:"attachments,omitempty"`
			OrganizationId string                             `json:"organizationId"`
			ProjectId      string                             `json:"projectId"`
			RegionId       string                             `json:"regionId"`
			SizeGiB        int64                              `json:"sizeGiB"`
			StorageClassId string                             `json:"storageClassId"`
			StorageType    regionapi.StorageTypeV2Spec        `json:"storageType"`
		}{
			Attachments: &regionapi.StorageAttachmentV2Spec{
				NetworkIds: networkIDs,
			},
			OrganizationId: organizationID,
			ProjectId:      projectID,
			RegionId:       m.RegionID.ValueString(),
			SizeGiB:        m.Capacity.ValueInt64(),
			StorageClassId: m.StorageClassID.ValueString(),
			StorageType: regionapi.StorageTypeV2Spec{
				NFS: &regionapi.NFSV2Spec{
					RootSquash: m.RootSquash.ValueBool(),
				},
			},
		},
	}

	return fileStorage, nil
}

func (m *FileStorageModel) NscaleFileStorageUpdateParams() (regionapi.StorageV2Update, diag.Diagnostics) {
	networkIDs, diagnostics := m.networkIDs()
	if diagnostics.HasError() {
		return regionapi.StorageV2Update{}, diagnostics
	}

	fileStorage := regionapi.StorageV2Update{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			// REVIEW_ME: Not sure what the tags are for. Even the UI doesn’t provide a way to set them, so leaving it as nil for now.
			Tags: nil,
		},
		Spec: regionapi.StorageV2Spec{
			Attachments: &regionapi.StorageAttachmentV2Spec{
				NetworkIds: networkIDs,
			},
			SizeGiB: m.Capacity.ValueInt64(),
			StorageType: regionapi.StorageTypeV2Spec{
				NFS: &regionapi.NFSV2Spec{
					RootSquash: m.RootSquash.ValueBool(),
				},
			},
		},
	}

	return fileStorage, nil
}

var FileStorageNetworkModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":           types.StringType,
		"mount_source": types.StringType,
	},
}

type FileStorageNetworkModel struct {
	ID          types.String `tfsdk:"id"`
	MountSource types.String `tfsdk:"mount_source"`
}

func NewFileStorageNetworkModel(source regionapi.StorageAttachmentV2Status) attr.Value {
	return types.ObjectValueMust(
		FileStorageNetworkModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"id":           types.StringValue(source.NetworkId),
			"mount_source": types.StringPointerValue(source.MountSource),
		},
	)
}

func NewFileStorageNetworkModels(source []regionapi.StorageAttachmentV2Status) types.List {
	networks := make([]attr.Value, 0, len(source))
	for _, data := range source {
		networks = append(networks, NewFileStorageNetworkModel(data))
	}
	return types.ListValueMust(FileStorageNetworkModelAttributeType, networks)
}
