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

package kubernetescluster

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
)

func testCreationTime(t *testing.T) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, "2026-08-25T10:30:00Z")
	if err != nil {
		t.Fatalf("parsing fixture time: %s", err)
	}

	return parsed
}

// fullCluster is a cluster read with every optional field populated — the shape
// a provisioned, healthy cluster comes back as.
func fullCluster(t *testing.T) *nks.ClusterV1Read {
	t.Helper()

	return &nks.ClusterV1Read{
		Metadata: nks.ProjectScopedResourceReadMetadataV1{
			Id:                 "cluster-abc",
			Name:               "production",
			Description:        new("primary cluster"),
			OrganizationId:     "org-1",
			ProjectId:          "proj-1",
			Generation:         4,
			CreationTime:       testCreationTime(t),
			ProvisioningStatus: nks.ResourceProvisioningStatusProvisioned,
			HealthStatus:       nks.ResourceHealthStatusHealthy,
			Tags:               &nks.TagList{{Name: "env", Value: "prod"}},
		},
		Spec: nks.ClusterSpecV1{
			NetworkId:         "net-1",
			PlatformReleaseId: "rel-2",
			ApiServer: &nks.ClusterApiServerAccessV1{
				PublicIP:     new(true),
				AllowedCidrs: &[]string{"203.0.113.0/24"},
			},
			ClusterNetwork: &nks.ClusterNetworkV1{
				PodCidr:     new("10.240.0.0/12"),
				ServiceCidr: new("10.96.0.0/16"),
			},
			Addons: &nks.ClusterAddonsV1{
				Hardware: new(true),
			},
		},
		Status: nks.ClusterStatusV1{
			RegionId:           "region-1",
			ObservedGeneration: new(int64(4)),
			KubernetesVersion: &nks.ClusterKubernetesVersionStatusV1{
				Target:   new("v1.33.1"),
				Observed: new("v1.33.1"),
			},
			ApiServer: &nks.ClusterApiServerStatusV1{
				CertificateAuthorityData: "Y2E=",
				Endpoints: nks.ClusterApiServerEndpointsStatusV1{
					Private: nks.ClusterApiServerEndpointStatusV1{Address: "10.0.0.1", Port: 6443},
					Public:  &nks.ClusterApiServerEndpointStatusV1{Address: "198.51.100.7", Port: 6443},
				},
			},
			Release: &nks.ClusterReleaseStatusV1{
				AppliedId:        "rel-2",
				Deprecated:       new(false),
				Withdrawn:        new(false),
				UpgradeAvailable: new(true),
				EligibleTargets:  &[]string{"rel-3", "rel-4"},
			},
		},
	}
}

// minimalCluster is a cluster read taken before any status projection has
// happened: everything optional is absent. Every converter has to survive this,
// because it is what the create response looks like.
func minimalCluster(t *testing.T) *nks.ClusterV1Read {
	t.Helper()

	return &nks.ClusterV1Read{
		Metadata: nks.ProjectScopedResourceReadMetadataV1{
			Id:                 "cluster-new",
			Name:               "fresh",
			OrganizationId:     "org-1",
			ProjectId:          "proj-1",
			Generation:         1,
			CreationTime:       testCreationTime(t),
			ProvisioningStatus: nks.ResourceProvisioningStatusPending,
			HealthStatus:       nks.ResourceHealthStatusUnknown,
		},
		Spec: nks.ClusterSpecV1{
			NetworkId:         "net-1",
			PlatformReleaseId: "rel-1",
		},
		Status: nks.ClusterStatusV1{
			RegionId: "region-1",
		},
	}
}

func TestNewKubernetesClusterModelFull(t *testing.T) {
	t.Parallel()

	model := NewKubernetesClusterModel(fullCluster(t))

	if got := model.ID.ValueString(); got != "cluster-abc" {
		t.Errorf("ID = %q, want cluster-abc", got)
	}
	if got := model.Name.ValueString(); got != "production" {
		t.Errorf("Name = %q, want production", got)
	}
	if got := model.Description.ValueString(); got != "primary cluster" {
		t.Errorf("Description = %q, want primary cluster", got)
	}
	if got := model.NetworkID.ValueString(); got != "net-1" {
		t.Errorf("NetworkID = %q, want net-1", got)
	}
	if got := model.PlatformReleaseID.ValueString(); got != "rel-2" {
		t.Errorf("PlatformReleaseID = %q, want rel-2", got)
	}

	// project/org/region are all inherited from the network and must be surfaced,
	// since users cannot set them.
	if got := model.ProjectID.ValueString(); got != "proj-1" {
		t.Errorf("ProjectID = %q, want proj-1", got)
	}
	if got := model.OrganizationID.ValueString(); got != "org-1" {
		t.Errorf("OrganizationID = %q, want org-1", got)
	}
	if got := model.RegionID.ValueString(); got != "region-1" {
		t.Errorf("RegionID = %q, want region-1", got)
	}

	if got := model.CreationTime.ValueString(); got != "2026-08-25T10:30:00Z" {
		t.Errorf("CreationTime = %q, want 2026-08-25T10:30:00Z", got)
	}
	if got := model.KubernetesVersionTarget.ValueString(); got != "v1.33.1" {
		t.Errorf("KubernetesVersionTarget = %q, want v1.33.1", got)
	}
	if got := model.AppliedPlatformReleaseID.ValueString(); got != "rel-2" {
		t.Errorf("AppliedPlatformReleaseID = %q, want rel-2", got)
	}
	if !model.UpgradeAvailable.ValueBool() {
		t.Error("UpgradeAvailable = false, want true")
	}

	// eligibleTargets ordering is meaningful (upgrade order), hence a list.
	var targets []string
	if diagnostics := model.EligibleUpgradeTargetIDs.ElementsAs(
		context.Background(), &targets, false,
	); diagnostics.HasError() {
		t.Fatalf("reading eligible targets: %v", diagnostics)
	}
	if len(targets) != 2 || targets[0] != "rel-3" || targets[1] != "rel-4" {
		t.Errorf("EligibleUpgradeTargetIDs = %v, want [rel-3 rel-4]", targets)
	}

	endpoint := model.APIServerEndpoint.Attributes()
	if got := endpoint["public_host"].(types.String).ValueString(); got != "198.51.100.7" {
		t.Errorf("public_host = %q, want 198.51.100.7", got)
	}
	if got := endpoint["private_port"].(types.Int64).ValueInt64(); got != 6443 {
		t.Errorf("private_port = %d, want 6443", got)
	}
	if got := endpoint["certificate_authority_data"].(types.String).ValueString(); got != "Y2E=" {
		t.Errorf("certificate_authority_data = %q, want Y2E=", got)
	}
}

// TestNewKubernetesClusterModelMinimal is the important nil-safety case: a
// freshly-created cluster has no status projection, so every optional status
// sub-object is absent.
func TestNewKubernetesClusterModelMinimal(t *testing.T) {
	t.Parallel()

	model := NewKubernetesClusterModel(minimalCluster(t))

	if !model.Description.IsNull() {
		t.Error("Description should be null when the API omits it")
	}
	if !model.Tags.IsNull() {
		t.Error("Tags should be null when the API omits them")
	}
	if !model.APIServer.IsNull() {
		t.Error("APIServer should be null when spec.apiServer is absent")
	}
	if !model.ClusterNetwork.IsNull() {
		t.Error("ClusterNetwork should be null when spec.clusterNetwork is absent")
	}
	if !model.Addons.IsNull() {
		t.Error("Addons should be null when spec.addons is absent")
	}
	if !model.APIServerEndpoint.IsNull() {
		t.Error("APIServerEndpoint should be null before the control plane is reachable")
	}
	if !model.KubernetesVersionTarget.IsNull() {
		t.Error("KubernetesVersionTarget should be null when status.kubernetesVersion is absent")
	}
	if !model.AppliedPlatformReleaseID.IsNull() {
		t.Error("AppliedPlatformReleaseID should be null when status.release is absent")
	}
	if !model.UpgradeAvailable.IsNull() {
		t.Error("UpgradeAvailable should be null when status.release is absent")
	}
	if !model.EligibleUpgradeTargetIDs.IsNull() {
		t.Error("EligibleUpgradeTargetIDs should be null when status.release is absent")
	}
}

// TestAPIServerEndpointPrivateOnly covers a private-only cluster: the public
// endpoint is absent while the private one is present.
func TestAPIServerEndpointPrivateOnly(t *testing.T) {
	t.Parallel()

	cluster := fullCluster(t)
	cluster.Status.ApiServer.Endpoints.Public = nil

	endpoint := NewKubernetesClusterModel(cluster).APIServerEndpoint.Attributes()

	if !endpoint["public_host"].(types.String).IsNull() {
		t.Error("public_host should be null when there is no public endpoint")
	}
	if !endpoint["public_port"].(types.Int64).IsNull() {
		t.Error("public_port should be null when there is no public endpoint")
	}
	if got := endpoint["private_host"].(types.String).ValueString(); got != "10.0.0.1" {
		t.Errorf("private_host = %q, want 10.0.0.1", got)
	}
}

// TestEligibleTargetsEmptyVersusAbsent pins the distinction the NKS spec draws:
// an empty eligibleTargets array means "eligibility was observed, no upgrade
// available", which is a different fact from never having observed it.
func TestEligibleTargetsEmptyVersusAbsent(t *testing.T) {
	t.Parallel()

	cluster := fullCluster(t)
	cluster.Status.Release.EligibleTargets = &[]string{}

	model := NewKubernetesClusterModel(cluster)
	if model.EligibleUpgradeTargetIDs.IsNull() {
		t.Error("an empty eligibleTargets array should map to an empty list, not null")
	}
	if got := len(model.EligibleUpgradeTargetIDs.Elements()); got != 0 {
		t.Errorf("element count = %d, want 0", got)
	}

	cluster.Status.Release.EligibleTargets = nil
	if !NewKubernetesClusterModel(cluster).EligibleUpgradeTargetIDs.IsNull() {
		t.Error("an absent eligibleTargets should map to null")
	}
}

func objectValue(t *testing.T, attrTypes map[string]attr.Type, values map[string]attr.Value) types.Object {
	t.Helper()

	object, diagnostics := types.ObjectValue(attrTypes, values)
	if diagnostics.HasError() {
		t.Fatalf("building object: %v", diagnostics)
	}

	return object
}

// TestCreateParamsBoolZeroValues is the omitempty-bool guard from playbook §1.6.
//
// The NKS spec currently generates these as *bool, so a configured false
// survives. This test exists so that if a future spec regeneration turns either
// into a non-pointer bool with omitempty, it fails here rather than as
// "Provider produced inconsistent result after apply" in a user's plan.
func TestCreateParamsBoolZeroValues(t *testing.T) {
	t.Parallel()

	model := &KubernetesClusterModel{
		Name:              types.StringValue("cluster"),
		NetworkID:         types.StringValue("net-1"),
		PlatformReleaseID: types.StringValue("rel-1"),
		Tags:              types.MapNull(types.StringType),
		APIServer: objectValue(t, apiServerAttrTypes(), map[string]attr.Value{
			"public_ip":     types.BoolValue(false),
			"allowed_cidrs": types.SetNull(types.StringType),
		}),
		ClusterNetwork: types.ObjectNull(clusterNetworkAttrTypes()),
		Addons: objectValue(t, addonsAttrTypes(), map[string]attr.Value{
			"hardware": types.BoolValue(false),
		}),
	}

	params, diagnostics := model.NscaleClusterCreateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building create params: %v", diagnostics)
	}

	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshalling create params: %s", err)
	}

	var decoded struct {
		Spec struct {
			APIServer *struct {
				PublicIP *bool `json:"publicIP"`
			} `json:"apiServer"`
			Addons *struct {
				Hardware *bool `json:"hardware"`
			} `json:"addons"`
		} `json:"spec"`
	}
	if decodeErr := json.Unmarshal(encoded, &decoded); decodeErr != nil {
		t.Fatalf("decoding create params: %s", decodeErr)
	}

	if decoded.Spec.APIServer == nil || decoded.Spec.APIServer.PublicIP == nil {
		t.Fatalf("publicIP was dropped from the payload: %s", encoded)
	}
	if *decoded.Spec.APIServer.PublicIP {
		t.Errorf("publicIP = true, want false: %s", encoded)
	}

	if decoded.Spec.Addons == nil || decoded.Spec.Addons.Hardware == nil {
		t.Fatalf("hardware was dropped from the payload: %s", encoded)
	}
	if *decoded.Spec.Addons.Hardware {
		t.Errorf("hardware = true, want false: %s", encoded)
	}
}

// TestCreateParamsOmitsAbsentBlocks checks the other direction: an omitted
// nested block must not appear in the payload at all, so the API applies its own
// defaults rather than receiving explicit zero values.
func TestCreateParamsOmitsAbsentBlocks(t *testing.T) {
	t.Parallel()

	model := &KubernetesClusterModel{
		Name:              types.StringValue("cluster"),
		NetworkID:         types.StringValue("net-1"),
		PlatformReleaseID: types.StringValue("rel-1"),
		Tags:              types.MapNull(types.StringType),
		APIServer:         types.ObjectNull(apiServerAttrTypes()),
		ClusterNetwork:    types.ObjectNull(clusterNetworkAttrTypes()),
		Addons:            types.ObjectNull(addonsAttrTypes()),
	}

	params, diagnostics := model.NscaleClusterCreateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building create params: %v", diagnostics)
	}

	if params.Spec.ApiServer != nil {
		t.Error("apiServer should be nil when the block is omitted")
	}
	if params.Spec.ClusterNetwork != nil {
		t.Error("clusterNetwork should be nil when the block is omitted")
	}
	if params.Spec.Addons != nil {
		t.Error("addons should be nil when the block is omitted")
	}

	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshalling create params: %s", err)
	}

	for _, key := range []string{"apiServer", "clusterNetwork", "addons"} {
		var decoded map[string]map[string]json.RawMessage
		if decodeErr := json.Unmarshal(encoded, &decoded); decodeErr != nil {
			t.Fatalf("decoding create params: %s", decodeErr)
		}
		if _, present := decoded["spec"][key]; present {
			t.Errorf("%s should be absent from the payload, got: %s", key, encoded)
		}
	}
}

// TestUpdateParamsMatchCreateParams pins the two request bodies together. NKS
// models create and update as distinct Go types with identical fields, so the
// risk is one converter drifting from the other.
func TestUpdateParamsMatchCreateParams(t *testing.T) {
	t.Parallel()

	model := NewKubernetesClusterModel(fullCluster(t))

	createParams, diagnostics := model.NscaleClusterCreateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building create params: %v", diagnostics)
	}

	updateParams, diagnostics := model.NscaleClusterUpdateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building update params: %v", diagnostics)
	}

	if createParams.Spec.NetworkId != updateParams.Spec.NetworkId {
		t.Errorf(
			"networkId differs: create %q, update %q",
			createParams.Spec.NetworkId, updateParams.Spec.NetworkId,
		)
	}
	if createParams.Spec.PlatformReleaseId != updateParams.Spec.PlatformReleaseId {
		t.Errorf(
			"platformReleaseId differs: create %q, update %q",
			createParams.Spec.PlatformReleaseId, updateParams.Spec.PlatformReleaseId,
		)
	}

	// networkId must be present on every PUT, carrying the value the cluster was
	// created with — the API rejects a different one.
	if updateParams.Spec.NetworkId == "" {
		t.Error("networkId must always be sent on update")
	}
}

func TestUpdateParamsRoundTripsTags(t *testing.T) {
	t.Parallel()

	model := NewKubernetesClusterModel(fullCluster(t))

	params, diagnostics := model.NscaleClusterUpdateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building update params: %v", diagnostics)
	}

	if params.Metadata.Tags == nil {
		t.Fatal("tags should round-trip into the update payload")
	}

	tags := *params.Metadata.Tags
	if len(tags) != 1 || tags[0].Name != "env" || tags[0].Value != "prod" {
		t.Errorf("tags = %v, want [{env prod}]", tags)
	}
}
