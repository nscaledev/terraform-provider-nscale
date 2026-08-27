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
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
)

// KubernetesClusterModel is the Terraform view of an NKS cluster.
//
// project_id, organization_id and region_id are computed rather than
// configurable: the NKS create body has no field for any of them
// (clusterCreateSpecV1 is additionalProperties:false) and no header parameter
// carries them. All three are inherited from the referenced network_id, so
// scope is chosen by choosing the network.
type KubernetesClusterModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Tags        types.Map    `tfsdk:"tags"`

	NetworkID         types.String `tfsdk:"network_id"`
	PlatformReleaseID types.String `tfsdk:"platform_release_id"`

	APIServer      types.Object `tfsdk:"api_server"`
	ClusterNetwork types.Object `tfsdk:"cluster_network"`
	Addons         types.Object `tfsdk:"addons"`

	ProjectID      types.String `tfsdk:"project_id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	RegionID       types.String `tfsdk:"region_id"`
	CreationTime   types.String `tfsdk:"creation_time"`

	ProvisioningStatus types.String `tfsdk:"provisioning_status"`
	HealthStatus       types.String `tfsdk:"health_status"`

	KubernetesVersionTarget   types.String `tfsdk:"kubernetes_version_target"`
	KubernetesVersionObserved types.String `tfsdk:"kubernetes_version_observed"`

	APIServerEndpoint types.Object `tfsdk:"api_server_endpoint"`

	AppliedPlatformReleaseID  types.String `tfsdk:"applied_platform_release_id"`
	PlatformReleaseDeprecated types.Bool   `tfsdk:"platform_release_deprecated"`
	PlatformReleaseWithdrawn  types.Bool   `tfsdk:"platform_release_withdrawn"`
	UpgradeAvailable          types.Bool   `tfsdk:"upgrade_available"`
	EligibleUpgradeTargetIDs  types.List   `tfsdk:"eligible_upgrade_target_ids"`
}

// apiServerModel mirrors clusterApiServerAccessV1 — the requested API server
// exposure. Distinct from apiServerEndpointModel, which is the observed result.
type apiServerModel struct {
	PublicIP     types.Bool `tfsdk:"public_ip"`
	AllowedCIDRs types.Set  `tfsdk:"allowed_cidrs"`
}

type clusterNetworkModel struct {
	PodCIDR     types.String `tfsdk:"pod_cidr"`
	ServiceCIDR types.String `tfsdk:"service_cidr"`
}

type addonsModel struct {
	Hardware types.Bool `tfsdk:"hardware"`
}

func apiServerAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"public_ip":     types.BoolType,
		"allowed_cidrs": types.SetType{ElemType: types.StringType},
	}
}

func clusterNetworkAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"pod_cidr":     types.StringType,
		"service_cidr": types.StringType,
	}
}

func addonsAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"hardware": types.BoolType,
	}
}

// apiServerEndpointAttrTypes describes the flattened status.apiServer object.
// The CA bundle is not marked sensitive: it is public key material, and NKS is
// credential-free by design — there is no token or kubeconfig endpoint, so
// nothing secret transits here.
func apiServerEndpointAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"certificate_authority_data": types.StringType,
		"private_host":               types.StringType,
		"private_port":               types.Int64Type,
		"public_host":                types.StringType,
		"public_port":                types.Int64Type,
	}
}

// NewKubernetesClusterModel maps an API read object into the Terraform model.
func NewKubernetesClusterModel(source *nks.ClusterV1Read) KubernetesClusterModel {
	metadata := source.Metadata
	spec := source.Spec
	status := source.Status

	return KubernetesClusterModel{
		ID:          types.StringValue(metadata.Id),
		Name:        types.StringValue(metadata.Name),
		Description: types.StringPointerValue(metadata.Description),
		Tags:        tagMapValueMust(metadata.Tags),

		NetworkID:         types.StringValue(spec.NetworkId),
		PlatformReleaseID: types.StringValue(spec.PlatformReleaseId),

		APIServer:      apiServerObjectValue(spec.ApiServer),
		ClusterNetwork: clusterNetworkObjectValue(spec.ClusterNetwork),
		Addons:         addonsObjectValue(spec.Addons),

		ProjectID:      types.StringValue(metadata.ProjectId),
		OrganizationID: types.StringValue(metadata.OrganizationId),
		RegionID:       types.StringValue(status.RegionId),
		CreationTime:   types.StringValue(metadata.CreationTime.Format(time.RFC3339)),

		ProvisioningStatus: types.StringValue(string(metadata.ProvisioningStatus)),
		HealthStatus:       types.StringValue(string(metadata.HealthStatus)),

		KubernetesVersionTarget:   kubernetesVersionValue(status.KubernetesVersion, versionTarget),
		KubernetesVersionObserved: kubernetesVersionValue(status.KubernetesVersion, versionObserved),

		APIServerEndpoint: apiServerEndpointObjectValue(status.ApiServer),

		AppliedPlatformReleaseID:  appliedReleaseIDValue(status.Release),
		PlatformReleaseDeprecated: releaseBoolValue(status.Release, releaseDeprecated),
		PlatformReleaseWithdrawn:  releaseBoolValue(status.Release, releaseWithdrawn),
		UpgradeAvailable:          releaseBoolValue(status.Release, releaseUpgradeAvailable),
		EligibleUpgradeTargetIDs:  eligibleTargetsValue(status.Release),
	}
}

func apiServerObjectValue(source *nks.ClusterApiServerAccessV1) types.Object {
	if source == nil {
		return types.ObjectNull(apiServerAttrTypes())
	}

	allowedCIDRs := types.SetNull(types.StringType)
	if source.AllowedCidrs != nil {
		elements := make([]attr.Value, 0, len(*source.AllowedCidrs))
		for _, cidr := range *source.AllowedCidrs {
			elements = append(elements, types.StringValue(cidr))
		}
		allowedCIDRs = types.SetValueMust(types.StringType, elements)
	}

	return types.ObjectValueMust(apiServerAttrTypes(), map[string]attr.Value{
		"public_ip":     types.BoolPointerValue(source.PublicIP),
		"allowed_cidrs": allowedCIDRs,
	})
}

func clusterNetworkObjectValue(source *nks.ClusterNetworkV1) types.Object {
	if source == nil {
		return types.ObjectNull(clusterNetworkAttrTypes())
	}

	return types.ObjectValueMust(clusterNetworkAttrTypes(), map[string]attr.Value{
		"pod_cidr":     types.StringPointerValue(source.PodCidr),
		"service_cidr": types.StringPointerValue(source.ServiceCidr),
	})
}

func addonsObjectValue(source *nks.ClusterAddonsV1) types.Object {
	if source == nil {
		return types.ObjectNull(addonsAttrTypes())
	}

	return types.ObjectValueMust(addonsAttrTypes(), map[string]attr.Value{
		"hardware": types.BoolPointerValue(source.Hardware),
	})
}

func apiServerEndpointObjectValue(source *nks.ClusterApiServerStatusV1) types.Object {
	// status.apiServer is absent until the control plane is reachable, so a null
	// object here is the normal pre-provisioned state, not an error.
	if source == nil {
		return types.ObjectNull(apiServerEndpointAttrTypes())
	}

	publicHost := types.StringNull()
	publicPort := types.Int64Null()
	if source.Endpoints.Public != nil {
		publicHost = types.StringValue(source.Endpoints.Public.Address)
		publicPort = types.Int64Value(int64(source.Endpoints.Public.Port))
	}

	return types.ObjectValueMust(apiServerEndpointAttrTypes(), map[string]attr.Value{
		"certificate_authority_data": types.StringValue(source.CertificateAuthorityData),
		"private_host":               types.StringValue(source.Endpoints.Private.Address),
		"private_port":               types.Int64Value(int64(source.Endpoints.Private.Port)),
		"public_host":                publicHost,
		"public_port":                publicPort,
	})
}

type versionField int

const (
	versionTarget versionField = iota
	versionObserved
)

func kubernetesVersionValue(source *nks.ClusterKubernetesVersionStatusV1, field versionField) types.String {
	if source == nil {
		return types.StringNull()
	}

	if field == versionTarget {
		return types.StringPointerValue(source.Target)
	}

	return types.StringPointerValue(source.Observed)
}

type releaseField int

const (
	releaseDeprecated releaseField = iota
	releaseWithdrawn
	releaseUpgradeAvailable
)

func releaseBoolValue(source *nks.ClusterReleaseStatusV1, field releaseField) types.Bool {
	if source == nil {
		return types.BoolNull()
	}

	switch field {
	case releaseDeprecated:
		return types.BoolPointerValue(source.Deprecated)
	case releaseWithdrawn:
		return types.BoolPointerValue(source.Withdrawn)
	case releaseUpgradeAvailable:
		return types.BoolPointerValue(source.UpgradeAvailable)
	}

	return types.BoolNull()
}

func appliedReleaseIDValue(source *nks.ClusterReleaseStatusV1) types.String {
	if source == nil {
		return types.StringNull()
	}

	return types.StringValue(source.AppliedId)
}

func eligibleTargetsValue(source *nks.ClusterReleaseStatusV1) types.List {
	if source == nil {
		return types.ListNull(types.StringType)
	}

	return stringSliceValuePointer(source.EligibleTargets)
}

// writeSpec collects the parts of the desired state shared by create and
// update. NKS models create and update with two distinct Go types that carry
// identical fields, so building this once keeps the two request bodies from
// drifting apart.
type writeSpec struct {
	networkID         string
	platformReleaseID string
	apiServer         *nks.ClusterApiServerAccessV1
	clusterNetwork    *nks.ClusterNetworkV1
	addons            *nks.ClusterAddonsCreateV1
}

func (m *KubernetesClusterModel) writeSpec(ctx context.Context) (writeSpec, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	apiServer, apiServerDiagnostics := m.apiServerRequest(ctx)
	diagnostics.Append(apiServerDiagnostics...)

	clusterNetwork, clusterNetworkDiagnostics := m.clusterNetworkRequest(ctx)
	diagnostics.Append(clusterNetworkDiagnostics...)

	addons, addonsDiagnostics := m.addonsRequest(ctx)
	diagnostics.Append(addonsDiagnostics...)

	if diagnostics.HasError() {
		return writeSpec{}, diagnostics
	}

	return writeSpec{
		networkID:         m.NetworkID.ValueString(),
		platformReleaseID: m.PlatformReleaseID.ValueString(),
		apiServer:         apiServer,
		clusterNetwork:    clusterNetwork,
		addons:            addons,
	}, diagnostics
}

func (m *KubernetesClusterModel) metadataRequest(ctx context.Context) (nks.ResourceMetadata, diag.Diagnostics) {
	tags, diagnostics := valueTagListPointer(ctx, m.Tags)
	if diagnostics.HasError() {
		return nks.ResourceMetadata{}, diagnostics
	}

	return nks.ResourceMetadata{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueStringPointer(),
		Tags:        tags,
	}, diagnostics
}

// NscaleClusterCreateParams builds the POST body.
func (m *KubernetesClusterModel) NscaleClusterCreateParams(
	ctx context.Context,
) (nks.ClusterV1Create, diag.Diagnostics) {
	metadata, diagnostics := m.metadataRequest(ctx)
	if diagnostics.HasError() {
		return nks.ClusterV1Create{}, diagnostics
	}

	spec, specDiagnostics := m.writeSpec(ctx)
	diagnostics.Append(specDiagnostics...)
	if diagnostics.HasError() {
		return nks.ClusterV1Create{}, diagnostics
	}

	return nks.ClusterV1Create{
		Metadata: metadata,
		Spec: nks.ClusterCreateSpecV1{
			NetworkId:         spec.networkID,
			PlatformReleaseId: spec.platformReleaseID,
			ApiServer:         spec.apiServer,
			ClusterNetwork:    spec.clusterNetwork,
			Addons:            spec.addons,
		},
	}, diagnostics
}

// NscaleClusterUpdateParams builds the PUT body.
//
// NKS update is a full object replacement — there is no PATCH endpoint, and the
// team has deliberately not added one because merge semantics are ambiguous. So
// this is built entirely from the plan, with no read-modify-write merging of
// computed fields: any field omitted here is cleared, not left alone.
//
// networkId must be present and must match the value the cluster was created
// with; a different one is rejected. That holds automatically because
// network_id is RequiresReplace, so a changed network destroys and recreates
// rather than ever reaching this path.
func (m *KubernetesClusterModel) NscaleClusterUpdateParams(
	ctx context.Context,
) (nks.ClusterV1Update, diag.Diagnostics) {
	metadata, diagnostics := m.metadataRequest(ctx)
	if diagnostics.HasError() {
		return nks.ClusterV1Update{}, diagnostics
	}

	spec, specDiagnostics := m.writeSpec(ctx)
	diagnostics.Append(specDiagnostics...)
	if diagnostics.HasError() {
		return nks.ClusterV1Update{}, diagnostics
	}

	return nks.ClusterV1Update{
		Metadata: metadata,
		Spec: nks.ClusterUpdateSpecV1{
			NetworkId:         spec.networkID,
			PlatformReleaseId: spec.platformReleaseID,
			ApiServer:         spec.apiServer,
			ClusterNetwork:    spec.clusterNetwork,
			Addons:            spec.addons,
		},
	}, diagnostics
}

func (m *KubernetesClusterModel) apiServerRequest(
	ctx context.Context,
) (*nks.ClusterApiServerAccessV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	// Unknown means Terraform has not resolved the value yet (a computed
	// attribute during plan); null means the practitioner omitted the block. Both
	// mean "send nothing and let the API apply its defaults".
	if m.APIServer.IsNull() || m.APIServer.IsUnknown() {
		return nil, diagnostics
	}

	var model apiServerModel
	if diagnostics = m.APIServer.As(ctx, &model, basetypesObjectOptions()); diagnostics.HasError() {
		return nil, diagnostics
	}

	var allowedCIDRs *[]string

	if !model.AllowedCIDRs.IsNull() && !model.AllowedCIDRs.IsUnknown() {
		var cidrs []string
		if diagnostics = model.AllowedCIDRs.ElementsAs(ctx, &cidrs, false); diagnostics.HasError() {
			return nil, diagnostics
		}
		allowedCIDRs = &cidrs
	}

	return &nks.ClusterApiServerAccessV1{
		// ValueBoolPointer yields a non-nil *bool for a configured false, which is
		// what keeps `public_ip = false` on the wire. The generated field is
		// already *bool, so encoding/json emits it explicitly.
		PublicIP:     model.PublicIP.ValueBoolPointer(),
		AllowedCidrs: allowedCIDRs,
	}, diagnostics
}

func (m *KubernetesClusterModel) clusterNetworkRequest(
	ctx context.Context,
) (*nks.ClusterNetworkV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if m.ClusterNetwork.IsNull() || m.ClusterNetwork.IsUnknown() {
		return nil, diagnostics
	}

	var model clusterNetworkModel
	if diagnostics = m.ClusterNetwork.As(ctx, &model, basetypesObjectOptions()); diagnostics.HasError() {
		return nil, diagnostics
	}

	return &nks.ClusterNetworkV1{
		PodCidr:     model.PodCIDR.ValueStringPointer(),
		ServiceCidr: model.ServiceCIDR.ValueStringPointer(),
	}, diagnostics
}

func (m *KubernetesClusterModel) addonsRequest(ctx context.Context) (*nks.ClusterAddonsCreateV1, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if m.Addons.IsNull() || m.Addons.IsUnknown() {
		return nil, diagnostics
	}

	var model addonsModel
	if diagnostics = m.Addons.As(ctx, &model, basetypesObjectOptions()); diagnostics.HasError() {
		return nil, diagnostics
	}

	return &nks.ClusterAddonsCreateV1{
		Hardware: model.Hardware.ValueBoolPointer(),
	}, diagnostics
}
