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
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"
)

func TestNewReservationModelFull(t *testing.T) {
	creationTime := time.Date(2026, time.May, 14, 10, 0, 0, 0, time.UTC)
	observedAt := time.Date(2026, time.May, 14, 10, 30, 0, 0, time.UTC)
	topologyHash := "sha256:abc123"

	source := &reservationapi.ReservationV2Read{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:                 "reservation-1",
			Name:               "gb300-nvl72",
			Description:        new("training capacity"),
			OrganizationId:     "org-1",
			ProjectId:          "project-1",
			CreationTime:       creationTime,
			ProvisioningStatus: coreapi.ResourceProvisioningStatusProvisioned,
			HealthStatus:       coreapi.ResourceHealthStatusHealthy,
			Tags: &[]coreapi.Tag{
				{Name: "workload", Value: "training"},
			},
		},
		Spec: reservationapi.ReservationV2Spec{
			RegionId:    "region-1",
			Accelerator: "GB300",
			Unit:        "NVL72",
			Count:       2,
		},
		Status: reservationapi.ReservationV2Status{
			MachineFlavorId:    "g.72.gb300.pinned",
			ClaimedUnitCount:   2,
			TopologyHash:       &topologyHash,
			TopologyObservedAt: &observedAt,
		},
	}

	model := NewReservationModel(source)

	if model.ID.ValueString() != "reservation-1" {
		t.Errorf("ID = %q, want %q", model.ID.ValueString(), "reservation-1")
	}
	if model.RegionID.ValueString() != "region-1" {
		t.Errorf("RegionID = %q, want %q", model.RegionID.ValueString(), "region-1")
	}
	if model.ProjectID.ValueString() != "project-1" {
		t.Errorf("ProjectID = %q, want %q", model.ProjectID.ValueString(), "project-1")
	}
	if model.UnitCount.ValueInt64() != 2 {
		t.Errorf("Count = %d, want %d", model.UnitCount.ValueInt64(), 2)
	}
	if model.ClaimedUnitCount.ValueInt64() != 2 {
		t.Errorf("ClaimedUnitCount = %d, want %d", model.ClaimedUnitCount.ValueInt64(), 2)
	}
	if model.MachineFlavorID.ValueString() != "g.72.gb300.pinned" {
		t.Errorf("MachineFlavorID = %q, want %q", model.MachineFlavorID.ValueString(), "g.72.gb300.pinned")
	}
	if !model.Description.Equal(types.StringValue("training capacity")) {
		t.Errorf("Description = %v, want %q", model.Description, "training capacity")
	}
	if model.Tags.IsNull() {
		t.Errorf("Tags null = true, want false")
	}
	if !model.TopologyHash.Equal(types.StringValue(topologyHash)) {
		t.Errorf("TopologyHash = %v, want %q", model.TopologyHash, topologyHash)
	}
	if model.TopologyObservedAt.IsNull() {
		t.Errorf("TopologyObservedAt null = true, want set")
	}
}

func TestNewReservationModelMinimal(t *testing.T) {
	creationTime := time.Date(2026, time.May, 14, 10, 0, 0, 0, time.UTC)

	source := &reservationapi.ReservationV2Read{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:                 "reservation-2",
			Name:               "bare",
			OrganizationId:     "org-1",
			ProjectId:          "project-1",
			CreationTime:       creationTime,
			ProvisioningStatus: coreapi.ResourceProvisioningStatusPending,
			HealthStatus:       coreapi.ResourceHealthStatusHealthy,
		},
		Spec: reservationapi.ReservationV2Spec{
			RegionId:    "region-1",
			Accelerator: "GB300",
			Unit:        "NVL72",
			Count:       1,
		},
		Status: reservationapi.ReservationV2Status{
			MachineFlavorId:  "g.72.gb300.pinned",
			ClaimedUnitCount: 0,
		},
	}

	model := NewReservationModel(source)

	if !model.Description.IsNull() {
		t.Errorf("Description = %v, want null", model.Description)
	}
	if !model.Tags.IsNull() {
		t.Errorf("Tags null = false, want true")
	}
	if !model.TopologyHash.IsNull() {
		t.Errorf("TopologyHash = %v, want null", model.TopologyHash)
	}
	if !model.TopologyObservedAt.IsNull() {
		t.Errorf("TopologyObservedAt = %v, want null", model.TopologyObservedAt)
	}
}

func TestNscaleReservationCreateParams(t *testing.T) {
	model := ReservationModel{
		Name:        types.StringValue("gb300-nvl72"),
		Description: types.StringValue("training capacity"),
		Tags:        types.MapNull(types.StringType),
		RegionID:    types.StringValue("region-1"),
		ProjectID:   types.StringValue("project-1"),
		Accelerator: types.StringValue("GB300"),
		Unit:        types.StringValue("NVL72"),
		UnitCount:   types.Int64Value(2),
	}

	params, diagnostics := model.NscaleReservationCreateParams("org-1")
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}

	if params.Metadata.Name != "gb300-nvl72" {
		t.Errorf("Name = %q, want %q", params.Metadata.Name, "gb300-nvl72")
	}
	if params.Spec.OrganizationId != "org-1" {
		t.Errorf("OrganizationId = %q, want %q", params.Spec.OrganizationId, "org-1")
	}
	if params.Spec.ProjectId != "project-1" {
		t.Errorf("ProjectId = %q, want %q", params.Spec.ProjectId, "project-1")
	}
	if params.Spec.RegionId != "region-1" {
		t.Errorf("RegionId = %q, want %q", params.Spec.RegionId, "region-1")
	}
	if params.Spec.Accelerator != "GB300" {
		t.Errorf("Accelerator = %q, want %q", params.Spec.Accelerator, "GB300")
	}
	if params.Spec.Unit != "NVL72" {
		t.Errorf("Unit = %q, want %q", params.Spec.Unit, "NVL72")
	}
	if params.Spec.Count != 2 {
		t.Errorf("Count = %d, want %d", params.Spec.Count, 2)
	}
}
