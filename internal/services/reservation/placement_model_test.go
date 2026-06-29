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
	"encoding/base64"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"
)

func objectAsOptions() basetypes.ObjectAsOptions { return basetypes.ObjectAsOptions{} }

func TestNewPlacementModelFull(t *testing.T) {
	creationTime := time.Date(2026, time.April, 28, 11, 3, 12, 0, time.UTC)
	// The API returns the decoded user_data bytes; the model must expose them as
	// the base64 string that was originally configured.
	userData := []byte("#!/bin/sh\n")
	encodedUserData := base64.StdEncoding.EncodeToString(userData)
	whenUnsatisfiable := reservationapi.Fail
	publicIP := true

	source := &reservationapi.PlacementV2Read{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:                 "placement-1",
			Name:               "training-workers",
			Description:        new("host placement"),
			OrganizationId:     "org-1",
			ProjectId:          "project-1",
			CreationTime:       creationTime,
			ProvisioningStatus: coreapi.ResourceProvisioningStatusProvisioned,
			HealthStatus:       coreapi.ResourceHealthStatusHealthy,
			Tags: &[]coreapi.Tag{
				{Name: "workload", Value: "training"},
			},
		},
		Spec: reservationapi.PlacementV2Spec{
			Count: 8,
			Constraints: reservationapi.PlacementConstraintsV2{
				Policy:            reservationapi.Spread,
				MaxSkew:           new(1),
				MinDomains:        new(3),
				WhenUnsatisfiable: &whenUnsatisfiable,
			},
			ServerSpec: reservationapi.PlacementServerSpecV2{
				ImageId:                   "ubuntu-24.04",
				SshCertificateAuthorityId: new("ca-1"),
				UserData:                  &userData,
				Networking: &reservationapi.PlacementServerNetworkingV2{
					PublicIP:               &publicIP,
					SecurityGroups:         &[]string{"sg-1", "sg-2"},
					AllowedSourceAddresses: &[]string{"10.0.0.0/8"},
				},
			},
		},
		Status: reservationapi.PlacementV2Status{
			RegionId:       "region-1",
			ReservationId:  "reservation-1",
			NetworkId:      "network-1",
			ReadyHostCount: new(8),
		},
	}

	model := NewPlacementModel(source)

	if model.ID.ValueString() != "placement-1" {
		t.Errorf("ID = %q, want %q", model.ID.ValueString(), "placement-1")
	}
	if model.ReservationID.ValueString() != "reservation-1" {
		t.Errorf("ReservationID = %q, want %q", model.ReservationID.ValueString(), "reservation-1")
	}
	if model.NetworkID.ValueString() != "network-1" {
		t.Errorf("NetworkID = %q, want %q", model.NetworkID.ValueString(), "network-1")
	}
	if model.RegionID.ValueString() != "region-1" {
		t.Errorf("RegionID = %q, want %q", model.RegionID.ValueString(), "region-1")
	}
	if model.HostCount.ValueInt64() != 8 {
		t.Errorf("Count = %d, want %d", model.HostCount.ValueInt64(), 8)
	}
	if model.ReadyHostCount.ValueInt64() != 8 {
		t.Errorf("ReadyHostCount = %d, want %d", model.ReadyHostCount.ValueInt64(), 8)
	}

	var constraints PlacementConstraintsModel
	if diagnostics := model.Constraints.As(t.Context(), &constraints, objectAsOptions()); diagnostics.HasError() {
		t.Fatalf("constraints As: %v", diagnostics)
	}
	if constraints.Policy.ValueString() != "spread" {
		t.Errorf("Policy = %q, want %q", constraints.Policy.ValueString(), "spread")
	}
	if constraints.MaxSkew.ValueInt64() != 1 {
		t.Errorf("MaxSkew = %d, want %d", constraints.MaxSkew.ValueInt64(), 1)
	}
	if constraints.MinDomains.ValueInt64() != 3 {
		t.Errorf("MinDomains = %d, want %d", constraints.MinDomains.ValueInt64(), 3)
	}
	if constraints.WhenUnsatisfiable.ValueString() != "fail" {
		t.Errorf("WhenUnsatisfiable = %q, want %q", constraints.WhenUnsatisfiable.ValueString(), "fail")
	}

	var serverSpec PlacementServerSpecModel
	if diagnostics := model.ServerSpec.As(t.Context(), &serverSpec, objectAsOptions()); diagnostics.HasError() {
		t.Fatalf("server_spec As: %v", diagnostics)
	}
	if serverSpec.ImageID.ValueString() != "ubuntu-24.04" {
		t.Errorf("ImageID = %q, want %q", serverSpec.ImageID.ValueString(), "ubuntu-24.04")
	}
	if serverSpec.SSHCertificateAuthorityID.ValueString() != "ca-1" {
		t.Errorf("SSHCertificateAuthorityID = %q, want %q", serverSpec.SSHCertificateAuthorityID.ValueString(), "ca-1")
	}
	if serverSpec.UserData.ValueString() != encodedUserData {
		t.Errorf("UserData = %q, want %q", serverSpec.UserData.ValueString(), encodedUserData)
	}
	if serverSpec.Networking.IsNull() {
		t.Fatalf("Networking is null, want populated")
	}
}

func TestNewPlacementModelMinimal(t *testing.T) {
	creationTime := time.Date(2026, time.April, 28, 11, 3, 12, 0, time.UTC)

	source := &reservationapi.PlacementV2Read{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:                 "placement-2",
			Name:               "bare",
			OrganizationId:     "org-1",
			ProjectId:          "project-1",
			CreationTime:       creationTime,
			ProvisioningStatus: coreapi.ResourceProvisioningStatusPending,
			HealthStatus:       coreapi.ResourceHealthStatusHealthy,
		},
		Spec: reservationapi.PlacementV2Spec{
			Count: 1,
			Constraints: reservationapi.PlacementConstraintsV2{
				Policy: reservationapi.Pack,
			},
			ServerSpec: reservationapi.PlacementServerSpecV2{
				ImageId: "ubuntu-24.04",
			},
		},
		Status: reservationapi.PlacementV2Status{
			RegionId:      "region-1",
			ReservationId: "reservation-1",
			NetworkId:     "network-1",
		},
	}

	model := NewPlacementModel(source)

	if model.Description.IsNull() != true {
		t.Errorf("Description null = %v, want true", model.Description.IsNull())
	}
	if model.Tags.IsNull() != true {
		t.Errorf("Tags null = %v, want true", model.Tags.IsNull())
	}
	if model.ReadyHostCount.IsNull() != true {
		t.Errorf("ReadyHostCount null = %v, want true", model.ReadyHostCount.IsNull())
	}

	var constraints PlacementConstraintsModel
	if diagnostics := model.Constraints.As(t.Context(), &constraints, objectAsOptions()); diagnostics.HasError() {
		t.Fatalf("constraints As: %v", diagnostics)
	}
	if constraints.Policy.ValueString() != "pack" {
		t.Errorf("Policy = %q, want %q", constraints.Policy.ValueString(), "pack")
	}
	if !constraints.MaxSkew.IsNull() {
		t.Errorf("MaxSkew = %v, want null", constraints.MaxSkew)
	}
	if !constraints.WhenUnsatisfiable.IsNull() {
		t.Errorf("WhenUnsatisfiable = %v, want null", constraints.WhenUnsatisfiable)
	}

	var serverSpec PlacementServerSpecModel
	if diagnostics := model.ServerSpec.As(t.Context(), &serverSpec, objectAsOptions()); diagnostics.HasError() {
		t.Fatalf("server_spec As: %v", diagnostics)
	}
	if !serverSpec.Networking.IsNull() {
		t.Errorf("Networking = %v, want null", serverSpec.Networking)
	}
}

func TestNscalePlacementCreateParams(t *testing.T) {
	model := PlacementModel{
		Name:          types.StringValue("training-workers"),
		Description:   types.StringValue("host placement"),
		Tags:          types.MapNull(types.StringType),
		ReservationID: types.StringValue("reservation-1"),
		NetworkID:     types.StringValue("network-1"),
		HostCount:     types.Int64Value(8),
		Constraints: types.ObjectValueMust(
			PlacementConstraintsModelAttributeType.AttrTypes,
			map[string]attr.Value{
				"policy":             types.StringValue("spread"),
				"max_skew":           types.Int64Value(1),
				"min_domains":        types.Int64Value(3),
				"when_unsatisfiable": types.StringValue("fail"),
			},
		),
		ServerSpec: types.ObjectValueMust(
			PlacementServerSpecModelAttributeType.AttrTypes,
			map[string]attr.Value{
				"image_id":                     types.StringValue("ubuntu-24.04"),
				"ssh_certificate_authority_id": types.StringValue("ca-1"),
				// Supplied as base64; the create params must carry the decoded bytes
				// so the SDK does not base64-encode the value a second time.
				"user_data": types.StringValue(base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\n"))),
				"networking": types.ObjectValueMust(
					PlacementServerNetworkingModelAttributeType.AttrTypes,
					map[string]attr.Value{
						"enable_public_ip": types.BoolValue(true),
						"security_group_ids": types.ListValueMust(
							types.StringType,
							[]attr.Value{types.StringValue("sg-1")},
						),
						"allowed_source_addresses": types.ListNull(types.StringType),
					},
				),
			},
		),
	}

	params, diagnostics := model.NscalePlacementCreateParams(t.Context())
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}

	if params.Spec.ReservationId != "reservation-1" {
		t.Errorf("ReservationId = %q, want %q", params.Spec.ReservationId, "reservation-1")
	}
	if params.Spec.NetworkId != "network-1" {
		t.Errorf("NetworkId = %q, want %q", params.Spec.NetworkId, "network-1")
	}
	if params.Spec.Count != 8 {
		t.Errorf("Count = %d, want %d", params.Spec.Count, 8)
	}
	if params.Spec.Constraints.Policy != reservationapi.Spread {
		t.Errorf("Policy = %q, want %q", params.Spec.Constraints.Policy, reservationapi.Spread)
	}
	if params.Spec.Constraints.MaxSkew == nil || *params.Spec.Constraints.MaxSkew != 1 {
		t.Errorf("MaxSkew = %v, want 1", params.Spec.Constraints.MaxSkew)
	}
	if params.Spec.Constraints.MinDomains == nil || *params.Spec.Constraints.MinDomains != 3 {
		t.Errorf("MinDomains = %v, want 3", params.Spec.Constraints.MinDomains)
	}
	if params.Spec.Constraints.WhenUnsatisfiable == nil ||
		*params.Spec.Constraints.WhenUnsatisfiable != reservationapi.Fail {
		t.Errorf("WhenUnsatisfiable = %v, want fail", params.Spec.Constraints.WhenUnsatisfiable)
	}
	if params.Spec.ServerSpec.ImageId != "ubuntu-24.04" {
		t.Errorf("ImageId = %q, want %q", params.Spec.ServerSpec.ImageId, "ubuntu-24.04")
	}
	if params.Spec.ServerSpec.Networking == nil {
		t.Fatalf("Networking is nil, want populated")
	}
	if params.Spec.ServerSpec.Networking.PublicIP == nil || !*params.Spec.ServerSpec.Networking.PublicIP {
		t.Errorf("PublicIP = %v, want true", params.Spec.ServerSpec.Networking.PublicIP)
	}
	if params.Spec.ServerSpec.Networking.SecurityGroups == nil ||
		len(*params.Spec.ServerSpec.Networking.SecurityGroups) != 1 {
		t.Errorf("SecurityGroups = %v, want one element", params.Spec.ServerSpec.Networking.SecurityGroups)
	}
	if params.Spec.ServerSpec.UserData == nil || string(*params.Spec.ServerSpec.UserData) != "#!/bin/sh\n" {
		t.Errorf("UserData = %q, want decoded %q", params.Spec.ServerSpec.UserData, "#!/bin/sh\n")
	}
}

// TestNscalePlacementCreateParamsNoNetworking confirms that omitting the
// optional networking block produces a nil networking pointer (not an empty
// object) so the API sees the field as absent.
func TestNscalePlacementCreateParamsNoNetworking(t *testing.T) {
	model := PlacementModel{
		Name:          types.StringValue("bare"),
		Tags:          types.MapNull(types.StringType),
		ReservationID: types.StringValue("reservation-1"),
		NetworkID:     types.StringValue("network-1"),
		HostCount:     types.Int64Value(1),
		Constraints: types.ObjectValueMust(
			PlacementConstraintsModelAttributeType.AttrTypes,
			map[string]attr.Value{
				"policy":             types.StringValue("pack"),
				"max_skew":           types.Int64Null(),
				"min_domains":        types.Int64Null(),
				"when_unsatisfiable": types.StringNull(),
			},
		),
		ServerSpec: types.ObjectValueMust(
			PlacementServerSpecModelAttributeType.AttrTypes,
			map[string]attr.Value{
				"image_id":                     types.StringValue("ubuntu-24.04"),
				"ssh_certificate_authority_id": types.StringNull(),
				"user_data":                    types.StringNull(),
				"networking":                   types.ObjectNull(PlacementServerNetworkingModelAttributeType.AttrTypes),
			},
		),
	}

	params, diagnostics := model.NscalePlacementCreateParams(t.Context())
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}

	if params.Spec.Constraints.MaxSkew != nil {
		t.Errorf("MaxSkew = %v, want nil", params.Spec.Constraints.MaxSkew)
	}
	if params.Spec.Constraints.WhenUnsatisfiable != nil {
		t.Errorf("WhenUnsatisfiable = %v, want nil", params.Spec.Constraints.WhenUnsatisfiable)
	}
	if params.Spec.ServerSpec.Networking != nil {
		t.Errorf("Networking = %v, want nil", params.Spec.ServerSpec.Networking)
	}
	if params.Spec.ServerSpec.UserData != nil {
		t.Errorf("UserData = %v, want nil", params.Spec.ServerSpec.UserData)
	}
}

// TestNscalePlacementCreateParamsUnknownNetworkingLists reproduces the crash
// that occurred when a networking list was left unset. Because the list
// attributes are Optional+Computed, Terraform passes an omitted value as
// unknown ("known after apply") at plan time; the expand must skip those rather
// than failing the ElementsAs conversion into []string.
func TestNscalePlacementCreateParamsUnknownNetworkingLists(t *testing.T) {
	model := PlacementModel{
		Name:          types.StringValue("workers"),
		Tags:          types.MapNull(types.StringType),
		ReservationID: types.StringValue("reservation-1"),
		NetworkID:     types.StringValue("network-1"),
		HostCount:     types.Int64Value(1),
		Constraints: types.ObjectValueMust(
			PlacementConstraintsModelAttributeType.AttrTypes,
			map[string]attr.Value{
				"policy":             types.StringValue("pack"),
				"max_skew":           types.Int64Null(),
				"min_domains":        types.Int64Null(),
				"when_unsatisfiable": types.StringNull(),
			},
		),
		ServerSpec: types.ObjectValueMust(
			PlacementServerSpecModelAttributeType.AttrTypes,
			map[string]attr.Value{
				"image_id":                     types.StringValue("ubuntu-24.04"),
				"ssh_certificate_authority_id": types.StringNull(),
				"user_data":                    types.StringNull(),
				"networking": types.ObjectValueMust(
					PlacementServerNetworkingModelAttributeType.AttrTypes,
					map[string]attr.Value{
						"enable_public_ip": types.BoolNull(),
						"security_group_ids": types.ListValueMust(
							types.StringType,
							[]attr.Value{types.StringValue("sg-1")},
						),
						// Omitted by the user; Computed ⇒ unknown at plan time.
						"allowed_source_addresses": types.ListUnknown(types.StringType),
					},
				),
			},
		),
	}

	params, diagnostics := model.NscalePlacementCreateParams(t.Context())
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}

	if params.Spec.ServerSpec.Networking == nil {
		t.Fatalf("Networking is nil, want populated")
	}
	if params.Spec.ServerSpec.Networking.SecurityGroups == nil ||
		len(*params.Spec.ServerSpec.Networking.SecurityGroups) != 1 {
		t.Errorf("SecurityGroups = %v, want one element", params.Spec.ServerSpec.Networking.SecurityGroups)
	}
	if got := params.Spec.ServerSpec.Networking.AllowedSourceAddresses; got != nil && len(*got) != 0 {
		t.Errorf("AllowedSourceAddresses = %v, want empty (unknown omitted)", *got)
	}
}

// TestNewPlacementModelNetworkingNilLists reproduces the "inconsistent result
// after apply: was [], now null" error. The API does not round-trip empty
// networking lists — it returns nil — so the flatten must surface them as
// empty (known) lists rather than null, matching a configured `[]`.
func TestNewPlacementModelNetworkingNilLists(t *testing.T) {
	source := &reservationapi.PlacementV2Read{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:                 "placement-3",
			Name:               "n",
			OrganizationId:     "org-1",
			ProjectId:          "project-1",
			CreationTime:       time.Date(2026, time.April, 28, 11, 3, 12, 0, time.UTC),
			ProvisioningStatus: coreapi.ResourceProvisioningStatusProvisioned,
			HealthStatus:       coreapi.ResourceHealthStatusHealthy,
		},
		Spec: reservationapi.PlacementV2Spec{
			Count:       1,
			Constraints: reservationapi.PlacementConstraintsV2{Policy: reservationapi.Pack},
			ServerSpec: reservationapi.PlacementServerSpecV2{
				ImageId: "ubuntu-24.04",
				// API echoes the networking object but omits the list fields.
				Networking: &reservationapi.PlacementServerNetworkingV2{
					SecurityGroups:         nil,
					AllowedSourceAddresses: nil,
				},
			},
		},
		Status: reservationapi.PlacementV2Status{
			RegionId:      "region-1",
			ReservationId: "reservation-1",
			NetworkId:     "network-1",
		},
	}

	model := NewPlacementModel(source)

	var serverSpec PlacementServerSpecModel
	if diagnostics := model.ServerSpec.As(t.Context(), &serverSpec, objectAsOptions()); diagnostics.HasError() {
		t.Fatalf("server_spec As: %v", diagnostics)
	}
	if serverSpec.Networking.IsNull() {
		t.Fatalf("Networking is null, want populated")
	}

	var networking PlacementServerNetworkingModel
	if diagnostics := serverSpec.Networking.As(t.Context(), &networking, objectAsOptions()); diagnostics.HasError() {
		t.Fatalf("networking As: %v", diagnostics)
	}
	if networking.SecurityGroupIDs.IsNull() {
		t.Errorf("SecurityGroupIDs is null, want empty list")
	}
	if networking.AllowedSourceAddresses.IsNull() {
		t.Errorf("AllowedSourceAddresses is null, want empty list")
	}
	if got := len(networking.AllowedSourceAddresses.Elements()); got != 0 {
		t.Errorf("AllowedSourceAddresses len = %d, want 0", got)
	}
}

func TestValidatePlacementConstraints(t *testing.T) {
	testCases := []struct {
		name        string
		constraints PlacementConstraintsModel
		hostCount   types.Int64
		wantErrors  int
	}{
		{
			name: "spread with all fields is valid",
			constraints: PlacementConstraintsModel{
				Policy:            types.StringValue("spread"),
				MaxSkew:           types.Int64Value(1),
				MinDomains:        types.Int64Value(3),
				WhenUnsatisfiable: types.StringValue("fail"),
			},
			hostCount:  types.Int64Value(8),
			wantErrors: 0,
		},
		{
			name: "pack with no spread-only fields is valid",
			constraints: PlacementConstraintsModel{
				Policy:            types.StringValue("pack"),
				MaxSkew:           types.Int64Null(),
				MinDomains:        types.Int64Null(),
				WhenUnsatisfiable: types.StringNull(),
			},
			hostCount:  types.Int64Value(8),
			wantErrors: 0,
		},
		{
			name: "pack with all spread-only fields set reports each",
			constraints: PlacementConstraintsModel{
				Policy:            types.StringValue("pack"),
				MaxSkew:           types.Int64Value(1),
				MinDomains:        types.Int64Value(2),
				WhenUnsatisfiable: types.StringValue("fail"),
			},
			hostCount:  types.Int64Value(8),
			wantErrors: 3,
		},
		{
			name: "min_domains greater than host_count is invalid",
			constraints: PlacementConstraintsModel{
				Policy:            types.StringValue("spread"),
				MaxSkew:           types.Int64Null(),
				MinDomains:        types.Int64Value(9),
				WhenUnsatisfiable: types.StringNull(),
			},
			hostCount:  types.Int64Value(8),
			wantErrors: 1,
		},
		{
			name: "unknown spread-only fields under pack are deferred",
			constraints: PlacementConstraintsModel{
				Policy:            types.StringValue("pack"),
				MaxSkew:           types.Int64Unknown(),
				MinDomains:        types.Int64Unknown(),
				WhenUnsatisfiable: types.StringUnknown(),
			},
			hostCount:  types.Int64Value(8),
			wantErrors: 0,
		},
		{
			name: "unknown host_count defers min_domains comparison",
			constraints: PlacementConstraintsModel{
				Policy:            types.StringValue("spread"),
				MaxSkew:           types.Int64Null(),
				MinDomains:        types.Int64Value(9),
				WhenUnsatisfiable: types.StringNull(),
			},
			hostCount:  types.Int64Unknown(),
			wantErrors: 0,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			diagnostics := validatePlacementConstraints(testCase.constraints, testCase.hostCount)
			if got := diagnostics.ErrorsCount(); got != testCase.wantErrors {
				t.Errorf("ErrorsCount() = %d, want %d (%v)", got, testCase.wantErrors, diagnostics)
			}
		})
	}
}
