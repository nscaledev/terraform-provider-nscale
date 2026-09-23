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

package kubernetesnodepool

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"
)

func testCreationTime(t *testing.T) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, "2026-09-16T10:30:00Z")
	if err != nil {
		t.Fatalf("parsing fixture time: %s", err)
	}

	return parsed
}

// objectValue builds a nested object value, failing the test rather than
// panicking if the attribute types and values disagree.
func objectValue(t *testing.T, attrTypes map[string]attr.Type, values map[string]attr.Value) types.Object {
	t.Helper()

	object, diagnostics := types.ObjectValue(attrTypes, values)
	if diagnostics.HasError() {
		t.Fatalf("building object value: %v", diagnostics)
	}

	return object
}

// fullComputePool is a compute pool read with every optional field populated —
// the shape a provisioned, healthy pool comes back as.
func fullComputePool(t *testing.T) *kubernetesapi.NodePoolV1Read {
	t.Helper()

	return &kubernetesapi.NodePoolV1Read{
		Metadata: kubernetesapi.ProjectScopedResourceReadMetadataV1{
			Id:                 "pool-abc",
			Name:               "workers",
			Description:        new("general purpose workers"),
			OrganizationId:     "org-1",
			ProjectId:          "proj-1",
			Generation:         3,
			CreationTime:       testCreationTime(t),
			ProvisioningStatus: kubernetesapi.ResourceProvisioningStatusProvisioned,
			HealthStatus:       kubernetesapi.ResourceHealthStatusHealthy,
			Tags:               &kubernetesapi.TagList{{Name: "env", Value: "prod"}},
		},
		Spec: kubernetesapi.NodePoolSpecV1{
			ClusterId:        "cluster-1",
			ProvisioningMode: kubernetesapi.NodePoolProvisioningModeV1Compute,
			Replicas:         3,
			Compute:          &kubernetesapi.NodePoolComputeV1{FlavorId: new("flavor-1")},
			Taints: &[]kubernetesapi.NodePoolTaintV1{
				{Key: "workload", Value: new("general"), Effect: kubernetesapi.NodePoolTaintV1EffectPreferNoSchedule},
				// A taint with no value is legal, and is the row that catches a
				// converter treating the empty string and absent as the same.
				{Key: "dedicated", Effect: kubernetesapi.NodePoolTaintV1EffectNoSchedule},
			},
			Labels: &kubernetesapi.NodePoolLabelsV1{"tier": "standard", "blank": ""},
		},
		Status: kubernetesapi.NodePoolStatusV1{
			RegionId:           "region-1",
			ObservedGeneration: new(int64(3)),
			KubernetesVersion:  new("v1.33.1"),
			DesiredReplicas:    new(3),
			CurrentReplicas:    new(3),
			ReadyReplicas:      new(3),
			UpToDateReplicas:   new(3),
			Release: &kubernetesapi.NodePoolReleaseStatusV1{
				AppliedId:         "rel-2",
				KubernetesVersion: "v1.33.1",
				Deprecated:        new(false),
				Withdrawn:         new(false),
			},
		},
	}
}

func TestNewKubernetesNodePoolModelFull(t *testing.T) {
	t.Parallel()

	model := NewKubernetesNodePoolModel(fullComputePool(t))

	if got := model.ID.ValueString(); got != "pool-abc" {
		t.Errorf("ID = %q, want pool-abc", got)
	}
	if got := model.Name.ValueString(); got != "workers" {
		t.Errorf("Name = %q, want workers", got)
	}
	if got := model.ClusterID.ValueString(); got != "cluster-1" {
		t.Errorf("ClusterID = %q, want cluster-1", got)
	}
	if got := model.ProvisioningMode.ValueString(); got != "compute" {
		t.Errorf("ProvisioningMode = %q, want compute", got)
	}
	if got := model.Replicas.ValueInt64(); got != 3 {
		t.Errorf("Replicas = %d, want 3", got)
	}
	if got := model.RegionID.ValueString(); got != "region-1" {
		t.Errorf("RegionID = %q, want region-1", got)
	}
	if got := model.CreationTime.ValueString(); got != "2026-09-16T10:30:00Z" {
		t.Errorf("CreationTime = %q, want the RFC3339 fixture", got)
	}

	// Scope is inherited from the cluster, not configured.
	if got := model.ProjectID.ValueString(); got != "proj-1" {
		t.Errorf("ProjectID = %q, want proj-1", got)
	}
	if got := model.OrganizationID.ValueString(); got != "org-1" {
		t.Errorf("OrganizationID = %q, want org-1", got)
	}

	// The observed counts, but never desiredReplicas — it duplicates the
	// replicas argument and is deliberately not exposed.
	if got := model.CurrentReplicas.ValueInt64(); got != 3 {
		t.Errorf("CurrentReplicas = %d, want 3", got)
	}
	if got := model.ReadyReplicas.ValueInt64(); got != 3 {
		t.Errorf("ReadyReplicas = %d, want 3", got)
	}
	if got := model.UpToDateReplicas.ValueInt64(); got != 3 {
		t.Errorf("UpToDateReplicas = %d, want 3", got)
	}
	if got := model.KubernetesVersion.ValueString(); got != "v1.33.1" {
		t.Errorf("KubernetesVersion = %q, want v1.33.1", got)
	}

	// A pool has no platform_release_id argument, but it does report the release
	// it inherited from the cluster.
	if got := model.AppliedPlatformReleaseID.ValueString(); got != "rel-2" {
		t.Errorf("AppliedPlatformReleaseID = %q, want rel-2", got)
	}
	if got := model.PlatformReleaseKubernetesVersion.ValueString(); got != "v1.33.1" {
		t.Errorf("PlatformReleaseKubernetesVersion = %q, want v1.33.1", got)
	}
	if model.PlatformReleaseDeprecated.IsNull() || model.PlatformReleaseDeprecated.ValueBool() {
		t.Error("PlatformReleaseDeprecated should be an explicit false")
	}

	// A compute pool has no reservation, so neither the selector nor the
	// placement it would have created.
	if !model.Reservation.IsNull() {
		t.Error("Reservation should be null on a compute pool")
	}
	if !model.PlacementID.IsNull() {
		t.Error("PlacementID should be null on a compute pool")
	}

	flavor := model.Compute.Attributes()["flavor_id"]
	if got := flavor.(types.String).ValueString(); got != "flavor-1" { //nolint:forcetypeassert // fixture is a string
		t.Errorf("compute.flavor_id = %q, want flavor-1", got)
	}
}

// TestNewKubernetesNodePoolModelTaints pins the taint round trip, including the
// valueless taint and the API's ordering.
func TestNewKubernetesNodePoolModelTaints(t *testing.T) {
	t.Parallel()

	model := NewKubernetesNodePoolModel(fullComputePool(t))

	elements := model.Taints.Elements()
	if len(elements) != 2 {
		t.Fatalf("got %d taints, want 2", len(elements))
	}

	first, ok := elements[0].(types.Object)
	if !ok {
		t.Fatalf("taint 0 is %T, want types.Object", elements[0])
	}
	if got := first.Attributes()["key"].(types.String).ValueString(); got != "workload" { //nolint:forcetypeassert // fixture is a string
		t.Errorf("taints[0].key = %q, want workload (ordering must be preserved)", got)
	}
	if got := first.Attributes()["effect"].(types.String).ValueString(); got != "PreferNoSchedule" { //nolint:forcetypeassert // fixture is a string
		t.Errorf("taints[0].effect = %q, want PreferNoSchedule", got)
	}

	second, ok := elements[1].(types.Object)
	if !ok {
		t.Fatalf("taint 1 is %T, want types.Object", elements[1])
	}
	// An absent value must stay null rather than becoming "", so that a config
	// omitting it round-trips instead of showing a diff.
	if value := second.Attributes()["value"]; !value.IsNull() {
		t.Errorf("taints[1].value = %v, want null for a taint with no value", value)
	}
}

// TestNewKubernetesNodePoolModelLabels covers the empty label value, which
// Kubernetes treats as meaningful — the label still applies.
func TestNewKubernetesNodePoolModelLabels(t *testing.T) {
	t.Parallel()

	model := NewKubernetesNodePoolModel(fullComputePool(t))

	elements := model.Labels.Elements()
	if len(elements) != 2 {
		t.Fatalf("got %d labels, want 2", len(elements))
	}
	if got := elements["tier"].(types.String).ValueString(); got != "standard" { //nolint:forcetypeassert // fixture is a string
		t.Errorf("labels[tier] = %q, want standard", got)
	}

	blank, ok := elements["blank"]
	if !ok {
		t.Fatal("the empty-valued label was dropped; Kubernetes still applies it")
	}
	if blank.IsNull() {
		t.Error("labels[blank] should be an empty string, not null")
	}
	if got := blank.(types.String).ValueString(); got != "" { //nolint:forcetypeassert // fixture is a string
		t.Errorf("labels[blank] = %q, want the empty string", got)
	}
}

// TestNewKubernetesNodePoolModelReservation covers the other mode, and the
// placement the pool creates inside its reservation.
func TestNewKubernetesNodePoolModelReservation(t *testing.T) {
	t.Parallel()

	pool := &kubernetesapi.NodePoolV1Read{
		Metadata: kubernetesapi.ProjectScopedResourceReadMetadataV1{
			Id:                 "pool-gpu",
			Name:               "gpu",
			CreationTime:       testCreationTime(t),
			ProvisioningStatus: kubernetesapi.ResourceProvisioningStatusProvisioned,
			HealthStatus:       kubernetesapi.ResourceHealthStatusHealthy,
		},
		Spec: kubernetesapi.NodePoolSpecV1{
			ClusterId:        "cluster-1",
			ProvisioningMode: kubernetesapi.NodePoolProvisioningModeV1Reservation,
			Replicas:         2,
			Reservation:      &kubernetesapi.NodePoolReservationV1{ReservationId: new("res-1")},
		},
		Status: kubernetesapi.NodePoolStatusV1{
			RegionId: "region-1",
			Reservation: &kubernetesapi.NodePoolReservationStatusV1{
				ReservationId: new("res-1"),
				PlacementId:   new("place-1"),
			},
		},
	}

	model := NewKubernetesNodePoolModel(pool)

	if got := model.ProvisioningMode.ValueString(); got != "reservation" {
		t.Errorf("ProvisioningMode = %q, want reservation", got)
	}
	if !model.Compute.IsNull() {
		t.Error("Compute should be null on a reservation pool")
	}

	reservationID := model.Reservation.Attributes()["reservation_id"]
	if got := reservationID.(types.String).ValueString(); got != "res-1" { //nolint:forcetypeassert // fixture is a string
		t.Errorf("reservation.reservation_id = %q, want res-1", got)
	}

	// placementId is exposed because it is genuinely new information — the
	// placement the pool created. status.reservation.reservationId is not, since
	// on an immutable selector it is always the one that was asked for.
	if got := model.PlacementID.ValueString(); got != "place-1" {
		t.Errorf("PlacementID = %q, want place-1", got)
	}
}

// TestNewKubernetesNodePoolModelMinimal is the pre-provisioned shape: every
// optional status sub-object absent. Absent must map to null rather than to a
// zero value, or a fresh pool reports counts and a release it does not have.
func TestNewKubernetesNodePoolModelMinimal(t *testing.T) {
	t.Parallel()

	pool := &kubernetesapi.NodePoolV1Read{
		Metadata: kubernetesapi.ProjectScopedResourceReadMetadataV1{
			Id:                 "pool-new",
			Name:               "workers",
			CreationTime:       testCreationTime(t),
			ProvisioningStatus: kubernetesapi.ResourceProvisioningStatusPending,
			HealthStatus:       kubernetesapi.ResourceHealthStatusUnknown,
		},
		Spec: kubernetesapi.NodePoolSpecV1{
			ClusterId:        "cluster-1",
			ProvisioningMode: kubernetesapi.NodePoolProvisioningModeV1Compute,
			Replicas:         0,
			Compute:          &kubernetesapi.NodePoolComputeV1{FlavorId: new("flavor-1")},
		},
		Status: kubernetesapi.NodePoolStatusV1{RegionId: "region-1"},
	}

	model := NewKubernetesNodePoolModel(pool)

	if !model.Description.IsNull() {
		t.Error("Description should be null when absent")
	}
	if !model.Tags.IsNull() {
		t.Error("Tags should be null when absent")
	}
	if !model.Taints.IsNull() {
		t.Error("Taints should be null when absent, so an omitted config attribute round-trips")
	}
	if !model.Labels.IsNull() {
		t.Error("Labels should be null when absent")
	}
	for name, value := range map[string]types.Int64{
		"CurrentReplicas":  model.CurrentReplicas,
		"ReadyReplicas":    model.ReadyReplicas,
		"UpToDateReplicas": model.UpToDateReplicas,
	} {
		if !value.IsNull() {
			t.Errorf("%s should be null before any count is observed, got %v", name, value)
		}
	}
	if !model.KubernetesVersion.IsNull() {
		t.Error("KubernetesVersion should be null before it is observed")
	}
	if !model.AppliedPlatformReleaseID.IsNull() {
		t.Error("AppliedPlatformReleaseID should be null before a release is reported")
	}
	if !model.PlatformReleaseDeprecated.IsNull() {
		t.Error("PlatformReleaseDeprecated should be null before a release is reported")
	}
	if !model.PlacementID.IsNull() {
		t.Error("PlacementID should be null when status.reservation is absent")
	}

	// A real zero read back as a real zero, not as null.
	if model.Replicas.IsNull() || model.Replicas.ValueInt64() != 0 {
		t.Errorf("Replicas = %v, want an explicit 0", model.Replicas)
	}
}

// computeModelFixture is a plan-shaped model for the request-builder tests.
func computeModelFixture(t *testing.T, replicas int64) *KubernetesNodePoolModel {
	t.Helper()

	return &KubernetesNodePoolModel{
		Name:             types.StringValue("workers"),
		ClusterID:        types.StringValue("cluster-1"),
		ProvisioningMode: types.StringValue("compute"),
		Replicas:         types.Int64Value(replicas),
		Tags:             types.MapNull(types.StringType),
		Compute: objectValue(t, computeAttrTypes(), map[string]attr.Value{
			"flavor_id": types.StringValue("flavor-1"),
		}),
		Reservation: types.ObjectNull(reservationAttrTypes()),
		Taints:      types.ListNull(taintObjectType()),
		Labels:      types.MapNull(types.StringType),
	}
}

// TestCreateParamsSerialisesZeroReplicas is the omitempty canary from playbook
// §1.6, and the reason scale-to-zero works.
//
// replicas is generated as a plain `int` with no omitempty, so 0 reaches the
// wire. This test exists so that if a future spec regeneration adds omitempty —
// or switches to a pointer that the converter then leaves nil — it fails here
// rather than as a silent no-op scale in a user's apply.
func TestCreateParamsSerialisesZeroReplicas(t *testing.T) {
	t.Parallel()

	params, diagnostics := computeModelFixture(t, 0).NscaleNodePoolCreateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building create params: %v", diagnostics)
	}

	if params.Spec.Replicas != 0 {
		t.Errorf("Replicas = %d, want 0", params.Spec.Replicas)
	}

	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshalling create params: %s", err)
	}

	var decoded struct {
		Spec struct {
			Replicas *int `json:"replicas"`
		} `json:"spec"`
	}
	if decodeErr := json.Unmarshal(encoded, &decoded); decodeErr != nil {
		t.Fatalf("decoding create params: %s", decodeErr)
	}

	if decoded.Spec.Replicas == nil {
		t.Fatalf("replicas was dropped from the payload, so scale-to-zero would be a no-op: %s", encoded)
	}
	if *decoded.Spec.Replicas != 0 {
		t.Errorf("replicas = %d, want 0: %s", *decoded.Spec.Replicas, encoded)
	}
}

// TestUpdateParamsMatchCreateParams guards the one-builder-for-both invariant.
// The API aliases nodePoolCreateSpecV1 and nodePoolUpdateSpecV1 to the same
// type, so the risk is not a type error but one converter quietly drifting.
func TestUpdateParamsMatchCreateParams(t *testing.T) {
	t.Parallel()

	model := computeModelFixture(t, 3)
	model.Description = types.StringValue("workers")
	model.Taints = types.ListValueMust(taintObjectType(), []attr.Value{
		objectValue(t, taintAttrTypes(), map[string]attr.Value{
			"key":    types.StringValue("workload"),
			"value":  types.StringValue("general"),
			"effect": types.StringValue("NoSchedule"),
		}),
	})
	model.Labels = types.MapValueMust(types.StringType, map[string]attr.Value{
		"tier": types.StringValue("standard"),
	})

	createParams, diagnostics := model.NscaleNodePoolCreateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building create params: %v", diagnostics)
	}

	updateParams, diagnostics := model.NscaleNodePoolUpdateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building update params: %v", diagnostics)
	}

	if !reflect.DeepEqual(createParams.Metadata, updateParams.Metadata) {
		t.Errorf("metadata differs between create and update:\ncreate: %+v\nupdate: %+v",
			createParams.Metadata, updateParams.Metadata)
	}
	if !reflect.DeepEqual(createParams.Spec, updateParams.Spec) {
		t.Errorf("spec differs between create and update:\ncreate: %+v\nupdate: %+v",
			createParams.Spec, updateParams.Spec)
	}
}

// TestUpdateParamsResendsTaintsAndLabels is the scaling-must-not-clobber
// invariant. NKS update is a full replacement with no PATCH, so a spec built
// from anything less than the whole plan would drop taints and labels on a
// plain replicas change — the single most common update a user makes.
func TestUpdateParamsResendsTaintsAndLabels(t *testing.T) {
	t.Parallel()

	model := computeModelFixture(t, 5)
	model.Taints = types.ListValueMust(taintObjectType(), []attr.Value{
		objectValue(t, taintAttrTypes(), map[string]attr.Value{
			"key":    types.StringValue("workload"),
			"value":  types.StringNull(),
			"effect": types.StringValue("NoExecute"),
		}),
	})
	model.Labels = types.MapValueMust(types.StringType, map[string]attr.Value{
		"tier": types.StringValue("standard"),
	})

	params, diagnostics := model.NscaleNodePoolUpdateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building update params: %v", diagnostics)
	}

	if params.Spec.Replicas != 5 {
		t.Errorf("Replicas = %d, want 5", params.Spec.Replicas)
	}

	if params.Spec.Taints == nil || len(*params.Spec.Taints) != 1 {
		t.Fatalf("taints were dropped from the update payload: %+v", params.Spec.Taints)
	}
	taint := (*params.Spec.Taints)[0]
	if taint.Key != "workload" || taint.Effect != kubernetesapi.NodePoolTaintV1EffectNoExecute {
		t.Errorf("taint = %+v, want the configured workload/NoExecute taint", taint)
	}
	// A null value must not become a pointer to the empty string: that would
	// register a valued taint where the user asked for a valueless one.
	if taint.Value != nil {
		t.Errorf("taint value = %q, want nil for a taint configured with no value", *taint.Value)
	}

	if params.Spec.Labels == nil || (*params.Spec.Labels)["tier"] != "standard" {
		t.Errorf("labels were dropped from the update payload: %+v", params.Spec.Labels)
	}
}

// TestWriteSpecOmitsAbsentCapacityBlock checks that the block belonging to the
// other mode is never sent. The API rejects a spec carrying it, and a
// reservation pool must not acquire an empty compute selector.
func TestWriteSpecOmitsAbsentCapacityBlock(t *testing.T) {
	t.Parallel()

	model := &KubernetesNodePoolModel{
		Name:             types.StringValue("gpu"),
		ClusterID:        types.StringValue("cluster-1"),
		ProvisioningMode: types.StringValue("reservation"),
		Replicas:         types.Int64Value(2),
		Tags:             types.MapNull(types.StringType),
		Compute:          types.ObjectNull(computeAttrTypes()),
		Reservation: objectValue(t, reservationAttrTypes(), map[string]attr.Value{
			"reservation_id": types.StringValue("res-1"),
		}),
		Taints: types.ListNull(taintObjectType()),
		Labels: types.MapNull(types.StringType),
	}

	params, diagnostics := model.NscaleNodePoolCreateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building create params: %v", diagnostics)
	}

	if params.Spec.Compute != nil {
		t.Error("compute should be nil on a reservation pool, not an empty selector")
	}
	if params.Spec.Reservation == nil || *params.Spec.Reservation.ReservationId != "res-1" {
		t.Errorf("reservation = %+v, want res-1", params.Spec.Reservation)
	}
	if params.Spec.ProvisioningMode != kubernetesapi.NodePoolProvisioningModeV1Reservation {
		t.Errorf("ProvisioningMode = %q, want reservation", params.Spec.ProvisioningMode)
	}

	// Omitted taints and labels must not appear at all, rather than as empty
	// collections the API would read as "clear these".
	if params.Spec.Taints != nil {
		t.Error("taints should be omitted when the attribute is null")
	}
	if params.Spec.Labels != nil {
		t.Error("labels should be omitted when the attribute is null")
	}
}

// TestWriteSpecRoundTripsTags checks tags survive the provider's own tag type
// on the way to the NKS request body.
func TestWriteSpecRoundTripsTags(t *testing.T) {
	t.Parallel()

	model := computeModelFixture(t, 1)
	model.Tags = types.MapValueMust(types.StringType, map[string]attr.Value{
		"environment": types.StringValue("prod"),
	})

	params, diagnostics := model.NscaleNodePoolCreateParams(context.Background())
	if diagnostics.HasError() {
		t.Fatalf("building create params: %v", diagnostics)
	}

	if params.Metadata.Tags == nil || len(*params.Metadata.Tags) != 1 {
		t.Fatalf("tags were dropped: %+v", params.Metadata.Tags)
	}
	if tag := (*params.Metadata.Tags)[0]; tag.Name != "environment" || tag.Value != "prod" {
		t.Errorf("tag = %+v, want environment=prod", tag)
	}
}
