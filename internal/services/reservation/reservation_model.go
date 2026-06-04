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
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
)

type ReservationModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Tags               types.Map    `tfsdk:"tags"`
	RegionID           types.String `tfsdk:"region_id"`
	ProjectID          types.String `tfsdk:"project_id"`
	Accelerator        types.String `tfsdk:"accelerator"`
	Unit               types.String `tfsdk:"unit"`
	UnitCount          types.Int64  `tfsdk:"unit_count"`
	MachineFlavorID    types.String `tfsdk:"machine_flavor_id"`
	ClaimedUnitCount   types.Int64  `tfsdk:"claimed_unit_count"`
	TopologyHash       types.String `tfsdk:"topology_hash"`
	TopologyObservedAt types.String `tfsdk:"topology_observed_at"`
	CreationTime       types.String `tfsdk:"creation_time"`
	ProvisioningStatus types.String `tfsdk:"provisioning_status"`
}

func NewReservationModel(source *reservationapi.ReservationV2Read) ReservationModel {
	tags := nscale.RemoveOperationTags(source.Metadata.Tags)

	topologyHash := types.StringNull()
	if source.Status.TopologyHash != nil {
		topologyHash = types.StringValue(*source.Status.TopologyHash)
	}

	topologyObservedAt := types.StringNull()
	if source.Status.TopologyObservedAt != nil {
		topologyObservedAt = types.StringValue(source.Status.TopologyObservedAt.Format(time.RFC3339))
	}

	return ReservationModel{
		ID:                 types.StringValue(source.Metadata.Id),
		Name:               types.StringValue(source.Metadata.Name),
		Description:        types.StringPointerValue(source.Metadata.Description),
		Tags:               tftypes.TagMapValueMust(tags),
		RegionID:           types.StringValue(source.Spec.RegionId),
		ProjectID:          types.StringValue(source.Metadata.ProjectId),
		Accelerator:        types.StringValue(source.Spec.Accelerator),
		Unit:               types.StringValue(source.Spec.Unit),
		UnitCount:          types.Int64Value(int64(source.Spec.Count)),
		MachineFlavorID:    types.StringValue(source.Status.MachineFlavorId),
		ClaimedUnitCount:   types.Int64Value(int64(source.Status.ClaimedUnitCount)),
		TopologyHash:       topologyHash,
		TopologyObservedAt: topologyObservedAt,
		CreationTime:       types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
		ProvisioningStatus: types.StringValue(string(source.Metadata.ProvisioningStatus)),
	}
}

// NscaleReservationCreateParams builds the create request body. The
// organization and project owning the reservation come from the configured
// client; the region falls back to the provider's region when the plan leaves
// it empty (resolved by the resource before this is called).
func (m *ReservationModel) NscaleReservationCreateParams(
	organizationID string,
) (reservationapi.ReservationV2Create, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return reservationapi.ReservationV2Create{}, diagnostics
	}
	tags = nscale.RemoveOperationTags(tags)

	return reservationapi.ReservationV2Create{
		Metadata: coreapi.ResourceWriteMetadata{
			Name:        m.Name.ValueString(),
			Description: m.Description.ValueStringPointer(),
			Tags:        tags,
		},
		Spec: reservationapi.ReservationV2CreateSpec{
			OrganizationId: organizationID,
			ProjectId:      m.ProjectID.ValueString(),
			RegionId:       m.RegionID.ValueString(),
			Accelerator:    m.Accelerator.ValueString(),
			Unit:           m.Unit.ValueString(),
			Count:          int(m.UnitCount.ValueInt64()),
		},
	}, nil
}
