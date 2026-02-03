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

package computecluster

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
)

type ComputeClusterModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	WorkloadPools      types.List   `tfsdk:"workload_pools"`
	SSHPrivateKey      types.String `tfsdk:"ssh_private_key"`
	Tags               types.Map    `tfsdk:"tags"`
	RegionID           types.String `tfsdk:"region_id"`
	ProvisioningStatus types.String `tfsdk:"provisioning_status"`
	CreationTime       types.String `tfsdk:"creation_time"`
}

func NewComputeClusterModel(source *computeapi.ComputeClusterRead) ComputeClusterModel {
	var workloadPoolStatuses *computeapi.ComputeClusterWorkloadPoolsStatus
	if source.Status != nil {
		workloadPoolStatuses = source.Status.WorkloadPools
	}

	var sshPrivateKey types.String
	if source.Status != nil {
		sshPrivateKey = types.StringPointerValue(source.Status.SshPrivateKey)
	}

	tags := nscale.RemoveOperationTags(source.Metadata.Tags)

	return ComputeClusterModel{
		ID:                 types.StringValue(source.Metadata.Id),
		Name:               types.StringValue(source.Metadata.Name),
		Description:        types.StringPointerValue(source.Metadata.Description),
		WorkloadPools:      NewWorkloadPoolModels(source.Spec.WorkloadPools, workloadPoolStatuses),
		SSHPrivateKey:      sshPrivateKey,
		Tags:               tftypes.TagMapValueMust(tags),
		RegionID:           types.StringValue(source.Spec.RegionId),
		ProvisioningStatus: types.StringValue(string(source.Metadata.ProvisioningStatus)),
		CreationTime:       types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

func (m *ComputeClusterModel) NscaleComputeCluster() (computeapi.ComputeClusterWrite, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return computeapi.ComputeClusterWrite{}, diagnostics
	}

	tags = nscale.RemoveOperationTags(tags)

	var sourceWorkloadPools []WorkloadPoolModel
	if diagnostics = m.WorkloadPools.ElementsAs(context.TODO(), &sourceWorkloadPools, false); diagnostics.HasError() {
		return computeapi.ComputeClusterWrite{}, diagnostics
	}

	workloadPools := make([]computeapi.ComputeClusterWorkloadPool, 0, len(sourceWorkloadPools))
	for _, source := range sourceWorkloadPools {
		workloadPool, diagnostics := source.NscaleWorkloadPool()
		if diagnostics.HasError() {
			return computeapi.ComputeClusterWrite{}, diagnostics
		}
		workloadPools = append(workloadPools, workloadPool)
	}

	computeCluster := computeapi.ComputeClusterWrite{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: computeapi.ComputeClusterSpec{
			RegionId:      m.RegionID.ValueString(),
			WorkloadPools: workloadPools,
		},
	}

	return computeCluster, nil
}

var AllowedAddressPairModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"cidr":        types.StringType,
		"mac_address": types.StringType,
	},
}

type AllowedAddressPairModel struct {
	Cidr       types.String `tfsdk:"cidr"`
	MacAddress types.String `tfsdk:"mac_address"`
}

func NewAllowedAddressPairModel(source computeapi.AllowedAddressPair) attr.Value {
	return types.ObjectValueMust(
		AllowedAddressPairModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"cidr":        types.StringValue(source.Cidr),
			"mac_address": types.StringPointerValue(source.MacAddress),
		},
	)
}

func (m *AllowedAddressPairModel) NscaleAllowedAddressPair() computeapi.AllowedAddressPair {
	return computeapi.AllowedAddressPair{
		Cidr:       m.Cidr.ValueString(),
		MacAddress: m.MacAddress.ValueStringPointer(),
	}
}

var WorkloadPoolModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":      types.StringType,
		"replicas":  types.Int64Type,
		"image_id":  types.StringType,
		"flavor_id": types.StringType,
		//"disk_size":         types.Int64Type,
		"user_data":        types.StringType,
		"enable_public_ip": types.BoolType,
		"allowed_address_pairs": types.SetType{
			ElemType: AllowedAddressPairModelAttributeType,
		},
		"firewall_rules": types.ListType{
			ElemType: FirewallRuleModelAttributeType,
		},
		"machines": types.ListType{
			ElemType: MachineModelAttributeType,
		},
	},
}

type WorkloadPoolModel struct {
	Name     types.String `tfsdk:"name"`
	Replicas types.Int64  `tfsdk:"replicas"`
	// REVIEW_ME: Should we accept the image and flavor names instead of their IDs?
	ImageID  types.String `tfsdk:"image_id"`
	FlavorID types.String `tfsdk:"flavor_id"`
	//DiskSize          types.Int64  `tfsdk:"disk_size"`
	UserData            types.String `tfsdk:"user_data"`
	EnablePublicIP      types.Bool   `tfsdk:"enable_public_ip"`
	AllowedAddressPairs types.Set    `tfsdk:"allowed_address_pairs"`
	FirewallRules       types.List   `tfsdk:"firewall_rules"`
	Machines            types.List   `tfsdk:"machines"`
}

func NewWorkloadPoolModel(spec computeapi.ComputeClusterWorkloadPool, status *computeapi.ComputeClusterWorkloadPoolStatus) attr.Value {
	var userData types.String
	if spec.Machine.UserData != nil {
		userData = types.StringValue(string(*spec.Machine.UserData))
	}

	enablePublicIP := types.BoolValue(true)
	if spec.Machine.PublicIPAllocation != nil {
		enablePublicIP = types.BoolValue(spec.Machine.PublicIPAllocation.Enabled)
	}

	firewallRules := types.ListNull(FirewallRuleModelAttributeType)
	if spec.Machine.Firewall != nil {
		firewallRules = NewFirewallRuleModels(*spec.Machine.Firewall)
	}

	allowedAddressPairs := types.SetNull(AllowedAddressPairModelAttributeType)
	if spec.Machine.AllowedAddressPairs != nil && len(*spec.Machine.AllowedAddressPairs) > 0 {
		pairList := make([]attr.Value, 0, len(*spec.Machine.AllowedAddressPairs))
		for _, pair := range *spec.Machine.AllowedAddressPairs {
			pairList = append(pairList, NewAllowedAddressPairModel(pair))
		}
		allowedAddressPairs = types.SetValueMust(AllowedAddressPairModelAttributeType, pairList)
	}

	machines := types.ListNull(MachineModelAttributeType)
	if status != nil && status.Machines != nil {
		machines = NewMachineModels(*status.Machines)
	}

	return types.ObjectValueMust(
		WorkloadPoolModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"name":     types.StringValue(spec.Name),
			"replicas": types.Int64Value(int64(spec.Machine.Replicas)),
			// FIXME: Some machines may not have an image ID but have an image selector. We need to check whether we could populate the image ID from the selector.
			"image_id":  types.StringPointerValue(spec.Machine.Image.Id),
			"flavor_id": types.StringValue(spec.Machine.FlavorId),
			//// FIXME: Some machines may not have a disk size specified as it's inherited from the flavor. We need to check whether we could populate the disk size from the flavor.
			//"disk_size":               types.Int64Value(int64(spec.Machine.Disk.Size)),
			"user_data":             userData,
			"enable_public_ip":      enablePublicIP,
			"allowed_address_pairs": allowedAddressPairs,
			"firewall_rules":        firewallRules,
			"machines":              machines,
		},
	)
}

func NewWorkloadPoolModels(specs []computeapi.ComputeClusterWorkloadPool, statuses *computeapi.ComputeClusterWorkloadPoolsStatus) types.List {
	statusMemo := make(map[string]*computeapi.ComputeClusterWorkloadPoolStatus)
	if statuses != nil {
		workloadPools := *statuses
		statusMemo = make(map[string]*computeapi.ComputeClusterWorkloadPoolStatus, len(workloadPools))
		for _, workloadPool := range workloadPools {
			statusMemo[workloadPool.Name] = &workloadPool
		}
	}

	pools := make([]attr.Value, 0, len(specs))
	for _, spec := range specs {
		status := statusMemo[spec.Name]
		pools = append(pools, NewWorkloadPoolModel(spec, status))
	}

	return types.ListValueMust(WorkloadPoolModelAttributeType, pools)
}

func (m *WorkloadPoolModel) NscaleWorkloadPool() (computeapi.ComputeClusterWorkloadPool, diag.Diagnostics) {
	var disk *computeapi.Volume
	//if !m.DiskSize.IsNull() && !m.DiskSize.IsUnknown() {
	//	disk = &computeapi.Volume{
	//		Size: int(m.DiskSize.ValueInt64()),
	//	}
	//}

	var sourceFirewallRules []FirewallRuleModel
	if diagnostics := m.FirewallRules.ElementsAs(context.TODO(), &sourceFirewallRules, false); diagnostics.HasError() {
		return computeapi.ComputeClusterWorkloadPool{}, diagnostics
	}

	firewallRules := make([]computeapi.FirewallRule, 0, len(sourceFirewallRules))
	for _, source := range sourceFirewallRules {
		firewallRule, diagnostics := source.NscaleFirewallRule()
		if diagnostics.HasError() {
			return computeapi.ComputeClusterWorkloadPool{}, diagnostics
		}
		firewallRules = append(firewallRules, firewallRule)
	}

	var userData *[]byte
	if !m.UserData.IsNull() && !m.UserData.IsUnknown() {
		temp := []byte(m.UserData.ValueString())
		userData = &temp
	}

	var allowedAddressPairs *computeapi.AllowedAddressPairList
	if !m.AllowedAddressPairs.IsNull() && !m.AllowedAddressPairs.IsUnknown() {
		var pairModels []AllowedAddressPairModel
		if diagnostics := m.AllowedAddressPairs.ElementsAs(context.TODO(), &pairModels, false); diagnostics.HasError() {
			return computeapi.ComputeClusterWorkloadPool{}, diagnostics
		}
		if len(pairModels) > 0 {
			pairs := make(computeapi.AllowedAddressPairList, 0, len(pairModels))
			for _, pairModel := range pairModels {
				pair := pairModel.NscaleAllowedAddressPair()
				pairs = append(pairs, pair)
			}
			allowedAddressPairs = &pairs
		}
	}

	workloadPool := computeapi.ComputeClusterWorkloadPool{
		Machine: computeapi.MachinePool{
			AllowedAddressPairs: allowedAddressPairs,
			Disk:                disk,
			Firewall:            &firewallRules,
			FlavorId:            m.FlavorID.ValueString(),
			Image: computeapi.ComputeImage{
				Id: m.ImageID.ValueStringPointer(),
			},
			PublicIPAllocation: &computeapi.PublicIPAllocation{
				Enabled: m.EnablePublicIP.ValueBool(),
			},
			Replicas: int(m.Replicas.ValueInt64()),
			UserData: userData,
		},
		Name: m.Name.ValueString(),
	}

	return workloadPool, nil
}

var FirewallRuleModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"direction": types.StringType,
		"protocol":  types.StringType,
		"ports":     types.StringType,
		"prefixes": types.SetType{
			ElemType: types.StringType,
		},
	},
}

type FirewallRuleModel struct {
	Direction types.String `tfsdk:"direction"`
	Protocol  types.String `tfsdk:"protocol"`
	Ports     types.String `tfsdk:"ports"`
	Prefixes  types.Set    `tfsdk:"prefixes"`
}

func NewFirewallRuleModel(source computeapi.FirewallRule) attr.Value {
	ports := strconv.Itoa(source.Port)
	if source.PortMax != nil {
		ports += "-" + strconv.Itoa(*source.PortMax)
	}

	prefixes := make([]attr.Value, 0, len(source.Prefixes))
	for _, prefix := range source.Prefixes {
		prefixes = append(prefixes, types.StringValue(prefix))
	}

	return types.ObjectValueMust(
		FirewallRuleModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"direction": types.StringValue(string(source.Direction)),
			"protocol":  types.StringValue(string(source.Protocol)),
			"ports":     types.StringValue(ports),
			"prefixes":  tftypes.NullableSetValueMust(types.StringType, prefixes),
		},
	)
}

func NewFirewallRuleModels(source []computeapi.FirewallRule) types.List {
	rules := make([]attr.Value, 0, len(source))
	for _, data := range source {
		rules = append(rules, NewFirewallRuleModel(data))
	}
	return types.ListValueMust(FirewallRuleModelAttributeType, rules)
}

func (m *FirewallRuleModel) NscaleFirewallRule() (computeapi.FirewallRule, diag.Diagnostics) {
	ports := strings.Split(m.Ports.ValueString(), "-")
	if len(ports) > 2 {
		diagnostics := NewErrorDiagnostics(
			"Invalid Port Format",
			"Firewall rule ports must be either a single port or a range in the format 'start-end'.",
		)
		return computeapi.FirewallRule{}, diagnostics
	}

	portNumbers := make([]int, 0, len(ports))
	for _, port := range ports {
		portNumber, err := strconv.Atoi(port)
		if err != nil {
			diagnostics := NewErrorDiagnostics(
				"Failed to Parse Port Number",
				fmt.Sprintf("An error occurred while parsing the port number: %s", err),
			)
			return computeapi.FirewallRule{}, diagnostics
		}
		portNumbers = append(portNumbers, portNumber)
	}

	var portMax *int
	if len(portNumbers) > 1 {
		portMax = &portNumbers[1]
	}

	var prefixes []string
	if diagnostics := m.Prefixes.ElementsAs(context.Background(), &prefixes, false); diagnostics.HasError() {
		return computeapi.FirewallRule{}, diagnostics
	}

	firewallRule := computeapi.FirewallRule{
		Direction: computeapi.FirewallRuleDirection(m.Direction.ValueString()),
		Port:      portNumbers[0],
		PortMax:   portMax,
		Prefixes:  prefixes,
		Protocol:  computeapi.FirewallRuleProtocol(m.Protocol.ValueString()),
	}

	return firewallRule, nil
}

var MachineModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"hostname":   types.StringType,
		"private_ip": types.StringType,
		"public_ip":  types.StringType,
	},
}

type MachineModel struct {
	Hostname  types.String `tfsdk:"hostname"`
	PrivateIP types.String `tfsdk:"private_ip"`
	PublicIP  types.String `tfsdk:"public_ip"`
}

func NewMachineModel(source computeapi.ComputeClusterMachineStatus) attr.Value {
	return types.ObjectValueMust(
		MachineModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"hostname":   types.StringValue(source.Hostname),
			"private_ip": types.StringPointerValue(source.PrivateIP),
			"public_ip":  types.StringPointerValue(source.PublicIP),
		},
	)
}

func NewMachineModels(source []computeapi.ComputeClusterMachineStatus) types.List {
	machines := make([]attr.Value, 0, len(source))
	for _, data := range source {
		machines = append(machines, NewMachineModel(data))
	}
	return types.ListValueMust(MachineModelAttributeType, machines)
}

func NewErrorDiagnostics(summary, detail string) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	diagnostics.AddError(summary, detail)
	return diagnostics
}
