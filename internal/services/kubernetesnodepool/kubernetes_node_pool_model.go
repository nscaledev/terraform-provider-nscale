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
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
)

// KubernetesNodePoolModel is the Terraform view of an NKS node pool.
//
// project_id, organization_id and region_id are computed rather than
// configurable, for the same reason as the cluster: the NKS write body has no
// field for any of them (nodePoolRequestSpecV1 is additionalProperties:false)
// and no header parameter carries them. A pool inherits all three from its
// cluster, which in turn inherited them from its network — so scope is chosen
// by choosing the cluster.
type KubernetesNodePoolModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Tags        types.Map    `tfsdk:"tags"`

	ClusterID        types.String `tfsdk:"cluster_id"`
	ProvisioningMode types.String `tfsdk:"provisioning_mode"`
	Replicas         types.Int64  `tfsdk:"replicas"`

	Compute     types.Object `tfsdk:"compute"`
	Reservation types.Object `tfsdk:"reservation"`
	Taints      types.List   `tfsdk:"taints"`
	Labels      types.Map    `tfsdk:"labels"`

	ProjectID      types.String `tfsdk:"project_id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	RegionID       types.String `tfsdk:"region_id"`
	CreationTime   types.String `tfsdk:"creation_time"`

	ProvisioningStatus types.String `tfsdk:"provisioning_status"`
	HealthStatus       types.String `tfsdk:"health_status"`

	CurrentReplicas   types.Int64  `tfsdk:"current_replicas"`
	ReadyReplicas     types.Int64  `tfsdk:"ready_replicas"`
	UpToDateReplicas  types.Int64  `tfsdk:"up_to_date_replicas"`
	KubernetesVersion types.String `tfsdk:"kubernetes_version"`

	PlacementID types.String `tfsdk:"placement_id"`

	AppliedPlatformReleaseID         types.String `tfsdk:"applied_platform_release_id"`
	PlatformReleaseKubernetesVersion types.String `tfsdk:"platform_release_kubernetes_version"`
	PlatformReleaseDeprecated        types.Bool   `tfsdk:"platform_release_deprecated"`
	PlatformReleaseWithdrawn         types.Bool   `tfsdk:"platform_release_withdrawn"`
}

// computeModel mirrors nodePoolComputeV1 — the compute-backed capacity
// selector, valid only when provisioning_mode is `compute`.
type computeModel struct {
	FlavorID types.String `tfsdk:"flavor_id"`
}

// reservationModel mirrors nodePoolReservationV1 — the reservation-backed
// capacity selector, valid only when provisioning_mode is `reservation`.
type reservationModel struct {
	ReservationID types.String `tfsdk:"reservation_id"`
}

// taintModel mirrors nodePoolTaintV1.
//
// There is no `propagation` field. The API applies a taint during worker node
// registration and does not reconcile it onto running nodes afterwards, so the
// behaviour is fixed rather than per-taint: see the roll note on the taints
// attribute in the resource schema.
type taintModel struct {
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
	Effect types.String `tfsdk:"effect"`
}

func computeAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"flavor_id": types.StringType,
	}
}

func reservationAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"reservation_id": types.StringType,
	}
}

func taintAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"key":    types.StringType,
		"value":  types.StringType,
		"effect": types.StringType,
	}
}

func taintObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: taintAttrTypes()}
}

// NewKubernetesNodePoolModel maps an API read object into the Terraform model.
func NewKubernetesNodePoolModel(source *nks.NodePoolV1Read) KubernetesNodePoolModel {
	metadata := source.Metadata
	spec := source.Spec
	status := source.Status

	return KubernetesNodePoolModel{
		ID:          types.StringValue(metadata.Id),
		Name:        types.StringValue(metadata.Name),
		Description: types.StringPointerValue(metadata.Description),
		Tags:        tftypes.TagMapValueMust(metadata.Tags),

		ClusterID:        types.StringValue(spec.ClusterId),
		ProvisioningMode: types.StringValue(string(spec.ProvisioningMode)),
		Replicas:         types.Int64Value(int64(spec.Replicas)),

		Compute:     computeObjectValue(spec.Compute),
		Reservation: reservationObjectValue(spec.Reservation),
		Taints:      taintsListValue(spec.Taints),
		Labels:      labelsMapValue(spec.Labels),

		ProjectID:      types.StringValue(metadata.ProjectId),
		OrganizationID: types.StringValue(metadata.OrganizationId),
		RegionID:       types.StringValue(status.RegionId),
		CreationTime:   types.StringValue(metadata.CreationTime.Format(time.RFC3339)),

		ProvisioningStatus: types.StringValue(string(metadata.ProvisioningStatus)),
		HealthStatus:       types.StringValue(string(metadata.HealthStatus)),

		CurrentReplicas:   int64PointerValue(status.CurrentReplicas),
		ReadyReplicas:     int64PointerValue(status.ReadyReplicas),
		UpToDateReplicas:  int64PointerValue(status.UpToDateReplicas),
		KubernetesVersion: types.StringPointerValue(status.KubernetesVersion),

		PlacementID: placementIDValue(status.Reservation),

		AppliedPlatformReleaseID:         releaseStringValue(status.Release, releaseAppliedID),
		PlatformReleaseKubernetesVersion: releaseStringValue(status.Release, releaseKubernetesVersion),
		PlatformReleaseDeprecated:        releaseBoolValue(status.Release, releaseDeprecated),
		PlatformReleaseWithdrawn:         releaseBoolValue(status.Release, releaseWithdrawn),
	}
}

func computeObjectValue(source *nks.NodePoolComputeV1) types.Object {
	if source == nil {
		return types.ObjectNull(computeAttrTypes())
	}

	return types.ObjectValueMust(computeAttrTypes(), map[string]attr.Value{
		"flavor_id": types.StringPointerValue(source.FlavorId),
	})
}

func reservationObjectValue(source *nks.NodePoolReservationV1) types.Object {
	if source == nil {
		return types.ObjectNull(reservationAttrTypes())
	}

	return types.ObjectValueMust(reservationAttrTypes(), map[string]attr.Value{
		"reservation_id": types.StringPointerValue(source.ReservationId),
	})
}

// taintsListValue flattens the taint array, preserving the API's ordering.
//
// A list rather than a set: the API returns an array and Kubernetes taint
// ordering is stable, so a list round-trips exactly without inviting spurious
// reordering diffs. Absent stays null rather than becoming an empty list —
// "this pool has no taints configured" and "the API did not report taints" are
// the same fact here, but only null matches a config that omits the attribute.
func taintsListValue(source *[]nks.NodePoolTaintV1) types.List {
	if source == nil {
		return types.ListNull(taintObjectType())
	}

	elements := make([]attr.Value, 0, len(*source))
	for _, taint := range *source {
		elements = append(elements, types.ObjectValueMust(taintAttrTypes(), map[string]attr.Value{
			"key":    types.StringValue(taint.Key),
			"value":  types.StringPointerValue(taint.Value),
			"effect": types.StringValue(string(taint.Effect)),
		}))
	}

	return types.ListValueMust(taintObjectType(), elements)
}

// labelsMapValue flattens the labels object. An empty label value is legal in
// Kubernetes and meaningful — the label is still applied — so it maps to an
// empty string rather than being dropped.
func labelsMapValue(source *nks.NodePoolLabelsV1) types.Map {
	if source == nil {
		return types.MapNull(types.StringType)
	}

	elements := make(map[string]attr.Value, len(*source))
	for name, value := range *source {
		elements[name] = types.StringValue(value)
	}

	return types.MapValueMust(types.StringType, elements)
}

// int64PointerValue widens an optional API count into the Int64 the schema
// exposes. The API models every replica count as an optional int, and absent
// means "not yet observed" rather than zero.
func int64PointerValue(value *int) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}

	return types.Int64Value(int64(*value))
}

// placementIDValue reads the placement the pool created inside its reservation.
// Null on a compute pool, and on a reservation pool until the placement exists.
func placementIDValue(source *nks.NodePoolReservationStatusV1) types.String {
	if source == nil {
		return types.StringNull()
	}

	return types.StringPointerValue(source.PlacementId)
}

type releaseStringField int

const (
	releaseAppliedID releaseStringField = iota
	releaseKubernetesVersion
)

// releaseStringValue reads one required string off the pinned release status. A
// nil release is the API not having reported one yet, which is distinct from an
// empty value.
func releaseStringValue(source *nks.NodePoolReleaseStatusV1, field releaseStringField) types.String {
	if source == nil {
		return types.StringNull()
	}

	if field == releaseAppliedID {
		return types.StringValue(source.AppliedId)
	}

	return types.StringValue(source.KubernetesVersion)
}

type releaseBoolField int

const (
	releaseDeprecated releaseBoolField = iota
	releaseWithdrawn
)

func releaseBoolValue(source *nks.NodePoolReleaseStatusV1, field releaseBoolField) types.Bool {
	if source == nil {
		return types.BoolNull()
	}

	switch field {
	case releaseDeprecated:
		return types.BoolPointerValue(source.Deprecated)
	case releaseWithdrawn:
		return types.BoolPointerValue(source.Withdrawn)
	}

	return types.BoolNull()
}

// NscaleNodePoolCreateParams builds the POST body.
func (m *KubernetesNodePoolModel) NscaleNodePoolCreateParams(
	ctx context.Context,
) (nks.NodePoolV1Create, diag.Diagnostics) {
	metadata, diagnostics := m.metadataRequest()
	if diagnostics.HasError() {
		return nks.NodePoolV1Create{}, diagnostics
	}

	spec, specDiagnostics := m.writeSpec(ctx)
	diagnostics.Append(specDiagnostics...)
	if diagnostics.HasError() {
		return nks.NodePoolV1Create{}, diagnostics
	}

	return nks.NodePoolV1Create{Metadata: metadata, Spec: spec}, diagnostics
}

// NscaleNodePoolUpdateParams builds the PUT body.
//
// NKS update is a full object replacement — there is no PATCH endpoint. So the
// spec is rebuilt entirely from the plan, with no read-modify-write merging:
// any field omitted here is cleared, not left alone. That is what makes the
// common case safe, because scaling `replicas` re-sends taints and labels
// unchanged rather than dropping them.
//
// Create and update share one builder because the API models them with one
// type (nodePoolRequestSpecV1 is aliased by both nodePoolCreateSpecV1 and
// nodePoolUpdateSpecV1). Keeping a single implementation is what stops the two
// request bodies drifting apart.
func (m *KubernetesNodePoolModel) NscaleNodePoolUpdateParams(
	ctx context.Context,
) (nks.NodePoolV1Update, diag.Diagnostics) {
	metadata, diagnostics := m.metadataRequest()
	if diagnostics.HasError() {
		return nks.NodePoolV1Update{}, diagnostics
	}

	spec, specDiagnostics := m.writeSpec(ctx)
	diagnostics.Append(specDiagnostics...)
	if diagnostics.HasError() {
		return nks.NodePoolV1Update{}, diagnostics
	}

	return nks.NodePoolV1Update{Metadata: metadata, Spec: spec}, diagnostics
}

func (m *KubernetesNodePoolModel) metadataRequest() (nks.ResourceMetadata, diag.Diagnostics) {
	tagList, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return nks.ResourceMetadata{}, diagnostics
	}

	return nks.ResourceMetadata{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueStringPointer(),
		Tags:        nscale.TagsToAPI[nks.Tag](tagList),
	}, diagnostics
}

// writeSpec builds the request spec shared by create and update.
func (m *KubernetesNodePoolModel) writeSpec(ctx context.Context) (nks.NodePoolRequestSpecV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	compute, computeDiagnostics := m.computeRequest(ctx)
	diagnostics.Append(computeDiagnostics...)

	reservation, reservationDiagnostics := m.reservationRequest(ctx)
	diagnostics.Append(reservationDiagnostics...)

	taints, taintsDiagnostics := m.taintsRequest(ctx)
	diagnostics.Append(taintsDiagnostics...)

	labels, labelsDiagnostics := m.labelsRequest(ctx)
	diagnostics.Append(labelsDiagnostics...)

	if diagnostics.HasError() {
		return nks.NodePoolRequestSpecV1{}, diagnostics
	}

	return nks.NodePoolRequestSpecV1{
		ClusterId:        m.ClusterID.ValueString(),
		ProvisioningMode: nks.NodePoolProvisioningModeV1(m.ProvisioningMode.ValueString()),
		// Replicas is a plain int with no omitempty in the generated client, so a
		// zero serialises explicitly and scale-to-zero reaches the API. The
		// schema bounds the value to the API's 0..2147483647, so the narrowing
		// conversion cannot overflow. TestCreateParamsSerialisesZeroReplicas
		// pins both halves of that.
		Replicas:    int(m.Replicas.ValueInt64()),
		Compute:     compute,
		Reservation: reservation,
		Taints:      taints,
		Labels:      labels,
	}, diagnostics
}

func (m *KubernetesNodePoolModel) computeRequest(
	ctx context.Context,
) (*nks.NodePoolComputeV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	// Unknown means Terraform has not resolved the value yet; null means the
	// practitioner omitted the block, which on a reservation pool is correct and
	// required. Both mean "send nothing".
	if m.Compute.IsNull() || m.Compute.IsUnknown() {
		return nil, diagnostics
	}

	var model computeModel
	if diagnostics = m.Compute.As(ctx, &model, basetypesObjectOptions()); diagnostics.HasError() {
		return nil, diagnostics
	}

	return &nks.NodePoolComputeV1{
		FlavorId: model.FlavorID.ValueStringPointer(),
	}, diagnostics
}

func (m *KubernetesNodePoolModel) reservationRequest(
	ctx context.Context,
) (*nks.NodePoolReservationV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if m.Reservation.IsNull() || m.Reservation.IsUnknown() {
		return nil, diagnostics
	}

	var model reservationModel
	if diagnostics = m.Reservation.As(ctx, &model, basetypesObjectOptions()); diagnostics.HasError() {
		return nil, diagnostics
	}

	return &nks.NodePoolReservationV1{
		ReservationId: model.ReservationID.ValueStringPointer(),
	}, diagnostics
}

// taintsRequest builds the taint array. An omitted attribute sends nothing; a
// configured one is sent verbatim, including an explicitly empty list, so that
// clearing taints is expressible.
func (m *KubernetesNodePoolModel) taintsRequest(
	ctx context.Context,
) (*[]nks.NodePoolTaintV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if m.Taints.IsNull() || m.Taints.IsUnknown() {
		return nil, diagnostics
	}

	var models []taintModel
	if diagnostics = m.Taints.ElementsAs(ctx, &models, false); diagnostics.HasError() {
		return nil, diagnostics
	}

	taints := make([]nks.NodePoolTaintV1, 0, len(models))
	for _, model := range models {
		taints = append(taints, nks.NodePoolTaintV1{
			Key:    model.Key.ValueString(),
			Value:  model.Value.ValueStringPointer(),
			Effect: nks.NodePoolTaintV1Effect(model.Effect.ValueString()),
		})
	}

	return &taints, diagnostics
}

// labelsRequest builds the labels object, on the same omitted-versus-empty
// terms as taintsRequest.
func (m *KubernetesNodePoolModel) labelsRequest(
	ctx context.Context,
) (*nks.NodePoolLabelsV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if m.Labels.IsNull() || m.Labels.IsUnknown() {
		return nil, diagnostics
	}

	var data map[string]string
	if diagnostics = m.Labels.ElementsAs(ctx, &data, false); diagnostics.HasError() {
		return nil, diagnostics
	}

	labels := nks.NodePoolLabelsV1(data)

	return &labels, diagnostics
}
