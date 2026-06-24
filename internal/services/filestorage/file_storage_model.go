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
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"
	regionids "github.com/unikorn-cloud/region/pkg/ids"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
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
	Tags           types.Map    `tfsdk:"tags"`
	ProjectID      types.String `tfsdk:"project_id"`
	RegionID       types.String `tfsdk:"region_id"`
	CreationTime   types.String `tfsdk:"creation_time"`

	// DefaultSnapshotProtectionEnabled mirrors the API-resolved platform-managed
	// Default Snapshot Protection setting. It is separate from any user-managed
	// Snapshot Policy Set.
	DefaultSnapshotProtectionEnabled types.Bool `tfsdk:"default_snapshot_protection_enabled"`
}

// bytesToGiBShift converts a byte count to whole gibibytes (1 GiB = 2^30 bytes).
const bytesToGiBShift = 30

// defaultSnapshotProtectionPointer maps the configured Default Snapshot
// Protection value to the API request field. A null or unknown value means the
// user did not explicitly configure the setting, so it is omitted from the
// request and the API resolves it (observe/adopt). An explicit true or false is
// sent so the API manages and drift-corrects it (enforce).
func defaultSnapshotProtectionPointer(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	return value.ValueBoolPointer()
}

func NewFileStorageModel(source *regionapi.StorageV2Read) FileStorageModel {
	size := types.Int64Value(0)
	if source.Status.Usage != nil && source.Status.Usage.UsedBytes != nil {
		size = types.Int64Value(*source.Status.Usage.UsedBytes >> bytesToGiBShift)
	}

	rootSquash := types.BoolNull()
	if source.Spec.StorageType.NFS != nil {
		rootSquash = types.BoolValue(source.Spec.StorageType.NFS.RootSquash)
	}

	networks := types.ListNull(FileStorageNetworkModelAttributeType)
	if source.Status.Attachments != nil {
		networks = NewFileStorageNetworkModels(*source.Status.Attachments)
	}

	tags := nscale.RemoveOperationTags(source.Metadata.Tags)

	return FileStorageModel{
		ID:             types.StringValue(source.Metadata.Id),
		Name:           types.StringValue(source.Metadata.Name),
		Description:    types.StringPointerValue(source.Metadata.Description),
		StorageClassID: types.StringValue(source.Status.StorageClassId),
		Size:           size,
		Capacity:       types.Int64Value(source.Spec.SizeGiB),
		RootSquash:     rootSquash,
		Network:        networks,
		Tags:           tftypes.TagMapValueMust(tags),
		ProjectID:      types.StringValue(source.Metadata.ProjectId),
		RegionID:       types.StringValue(source.Status.RegionId),
		CreationTime:   types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),

		DefaultSnapshotProtectionEnabled: types.BoolPointerValue(source.Spec.DefaultSnapshotProtectionEnabled),
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

func (m *FileStorageModel) NscaleFileStorageCreateParams(
	organizationID string,
) (regionapi.StorageV2Create, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return regionapi.StorageV2Create{}, diagnostics
	}

	tags = nscale.RemoveOperationTags(tags)

	networkIDs, diagnostics := m.networkIDs()
	if diagnostics.HasError() {
		return regionapi.StorageV2Create{}, diagnostics
	}

	regionID, err := regionids.ParseRegionID(m.RegionID.ValueString())
	if err != nil {
		diagnostics.AddError(
			"Invalid Region ID",
			fmt.Sprintf("Could not parse region ID %q: %s", m.RegionID.ValueString(), err),
		)
		return regionapi.StorageV2Create{}, diagnostics
	}

	fileStorage := regionapi.StorageV2Create{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
	}
	fileStorage.Spec.Attachments = &regionapi.StorageAttachmentV2Spec{NetworkIds: networkIDs}
	fileStorage.Spec.DefaultSnapshotProtectionEnabled = defaultSnapshotProtectionPointer(m.DefaultSnapshotProtectionEnabled)
	fileStorage.Spec.OrganizationId = organizationID
	fileStorage.Spec.ProjectId = m.ProjectID.ValueString()
	fileStorage.Spec.RegionId = regionID
	fileStorage.Spec.SizeGiB = m.Capacity.ValueInt64()
	fileStorage.Spec.StorageClassId = m.StorageClassID.ValueString()
	fileStorage.Spec.StorageType = regionapi.StorageTypeV2Spec{
		NFS: &regionapi.NFSV2Spec{
			RootSquash: m.RootSquash.ValueBool(),
		},
	}

	return fileStorage, nil
}

func (m *FileStorageModel) NscaleFileStorageUpdateParams() (regionapi.StorageV2Update, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return regionapi.StorageV2Update{}, diagnostics
	}

	tags = nscale.RemoveOperationTags(tags)

	networkIDs, diagnostics := m.networkIDs()
	if diagnostics.HasError() {
		return regionapi.StorageV2Update{}, diagnostics
	}

	fileStorage := regionapi.StorageV2Update{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: regionapi.StorageV2Spec{
			Attachments: &regionapi.StorageAttachmentV2Spec{
				NetworkIds: networkIDs,
			},
			DefaultSnapshotProtectionEnabled: defaultSnapshotProtectionPointer(m.DefaultSnapshotProtectionEnabled),
			SizeGiB:                          m.Capacity.ValueInt64(),
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
