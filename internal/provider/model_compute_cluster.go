package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	nscale "github.com/nscaledev/terraform-provider-nscale/internal/client"
	externalRef0 "github.com/unikorn-cloud/core/pkg/openapi"
)

type ComputeClusterModel struct {
	ID                 types.String        `tfsdk:"id"`
	Name               types.String        `tfsdk:"name"`
	Description        types.String        `tfsdk:"description"`
	WorkloadPools      []WorkloadPoolModel `tfsdk:"workload_pool"`
	SSHPrivateKey      types.String        `tfsdk:"ssh_private_key"`
	RegionID           types.String        `tfsdk:"region_id"`
	ProvisioningStatus types.String        `tfsdk:"provisioning_status"`
	CreationTime       types.String        `tfsdk:"creation_time"`
}

func NewComputeClusterModel(source *nscale.ComputeClusterRead) ComputeClusterModel {
	var sshPrivateKey types.String
	if source.Status != nil {
		sshPrivateKey = types.StringPointerValue(source.Status.SshPrivateKey)
	}

	var workloadPoolStatuses *nscale.ComputeClusterWorkloadPoolsStatus
	if source.Status != nil {
		workloadPoolStatuses = source.Status.WorkloadPools
	}

	return ComputeClusterModel{
		ID:                 types.StringValue(source.Metadata.Id),
		Name:               types.StringValue(source.Metadata.Name),
		Description:        types.StringPointerValue(source.Metadata.Description),
		WorkloadPools:      NewWorkloadPoolModels(source.Spec.WorkloadPools, workloadPoolStatuses),
		SSHPrivateKey:      sshPrivateKey,
		RegionID:           types.StringValue(source.Spec.RegionId),
		ProvisioningStatus: types.StringValue(string(source.Metadata.ProvisioningStatus)),
		CreationTime:       types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

func (m *ComputeClusterModel) NscaleComputeCluster() (nscale.ComputeClusterWrite, diag.Diagnostics) {
	workloadPools := make([]nscale.ComputeClusterWorkloadPool, 0, len(m.WorkloadPools))
	for _, data := range m.WorkloadPools {
		workloadPool, diagnostics := data.NscaleWorkloadPool()
		if diagnostics.HasError() {
			return nscale.ComputeClusterWrite{}, diagnostics
		}
		workloadPools = append(workloadPools, workloadPool)
	}

	computeCluster := nscale.ComputeClusterWrite{
		Metadata: externalRef0.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			// REVIEW_ME: Not sure what the tags are for. Even the UI doesn’t provide a way to set them, so leaving it as nil for now.
			Tags: nil,
		},
		Spec: nscale.ComputeClusterSpec{
			RegionId:      m.RegionID.ValueString(),
			WorkloadPools: workloadPools,
		},
	}

	return computeCluster, nil
}

type WorkloadPoolModel struct {
	Name     types.String `tfsdk:"name"`
	Replicas types.Int64  `tfsdk:"replicas"`
	// REVIEW_ME: Should we accept the image and flavor names instead of their IDs?
	ImageID        types.String        `tfsdk:"image_id"`
	FlavorID       types.String        `tfsdk:"flavor_id"`
	DiskSize       types.Int64         `tfsdk:"disk_size"`
	UserData       types.String        `tfsdk:"user_data"`
	EnablePublicIP types.Bool          `tfsdk:"enable_public_ip"`
	FirewallRules  []FirewallRuleModel `tfsdk:"firewall_rule"`
	Machines       []MachineModel      `tfsdk:"machine"`
}

func NewWorkloadPoolModel(spec nscale.ComputeClusterWorkloadPool, status *nscale.ComputeClusterWorkloadPoolStatus) WorkloadPoolModel {
	var userData types.String
	if spec.Machine.UserData != nil {
		userData = types.StringValue(string(*spec.Machine.UserData))
	}

	enablePublicIP := types.BoolValue(true)
	if spec.Machine.PublicIPAllocation != nil {
		enablePublicIP = types.BoolValue(spec.Machine.PublicIPAllocation.Enabled)
	}

	var firewallRules []FirewallRuleModel
	if spec.Machine.Firewall != nil {
		firewallRules = NewFirewallRuleModels(*spec.Machine.Firewall)
	}

	var machines []MachineModel
	if status != nil && status.Machines != nil {
		machines = NewMachineModels(*status.Machines)
	}

	return WorkloadPoolModel{
		Name:     types.StringValue(spec.Name),
		Replicas: types.Int64Value(int64(spec.Machine.Replicas)),
		// FIXME: Some machines may not have an image ID but have an image selector. We need to check whether we could populate the image ID from the selector.
		ImageID:  types.StringPointerValue(spec.Machine.Image.Id),
		FlavorID: types.StringValue(spec.Machine.FlavorId),
		// FIXME: Some machines may not have a disk size specified as it's inherited from the flavor. We need to check whether we could populate the disk size from the flavor.
		DiskSize:       types.Int64Value(int64(spec.Machine.Disk.Size)),
		UserData:       userData,
		EnablePublicIP: enablePublicIP,
		FirewallRules:  firewallRules,
		Machines:       machines,
	}
}

func NewWorkloadPoolModels(specs []nscale.ComputeClusterWorkloadPool, statuses *nscale.ComputeClusterWorkloadPoolsStatus) []WorkloadPoolModel {
	statusMemo := make(map[string]*nscale.ComputeClusterWorkloadPoolStatus)
	if statuses != nil {
		workloadPools := *statuses
		statusMemo = make(map[string]*nscale.ComputeClusterWorkloadPoolStatus, len(workloadPools))
		for _, workloadPool := range workloadPools {
			statusMemo[workloadPool.Name] = &workloadPool
		}
	}

	pools := make([]WorkloadPoolModel, 0, len(specs))
	for _, spec := range specs {
		status := statusMemo[spec.Name]
		pools = append(pools, NewWorkloadPoolModel(spec, status))
	}

	return pools
}

func (m *WorkloadPoolModel) NscaleWorkloadPool() (nscale.ComputeClusterWorkloadPool, diag.Diagnostics) {
	var disk *nscale.Volume
	if !m.DiskSize.IsNull() && !m.DiskSize.IsUnknown() {
		disk = &nscale.Volume{
			Size: int(m.DiskSize.ValueInt64()),
		}
	}

	firewallRules := make([]nscale.FirewallRule, 0, len(m.FirewallRules))
	for _, rule := range m.FirewallRules {
		firewallRule, diagnostics := rule.NscaleFirewallRule()
		if diagnostics.HasError() {
			return nscale.ComputeClusterWorkloadPool{}, diagnostics
		}
		firewallRules = append(firewallRules, firewallRule)
	}

	var userData *[]byte
	if !m.UserData.IsNull() && !m.UserData.IsUnknown() {
		temp := []byte(m.UserData.ValueString())
		userData = &temp
	}

	workloadPool := nscale.ComputeClusterWorkloadPool{
		Machine: nscale.MachinePool{
			// REVIEW_ME: Not sure what the allowed_address_pairs are for. Even the UI doesn’t provide a way to set them, so leaving it as nil for now.
			AllowedAddressPairs: nil,
			Disk:                disk,
			Firewall:            &firewallRules,
			FlavorId:            m.FlavorID.ValueString(),
			Image: nscale.ComputeImage{
				Id: m.ImageID.ValueStringPointer(),
			},
			PublicIPAllocation: &nscale.PublicIPAllocation{
				Enabled: m.EnablePublicIP.ValueBool(),
			},
			Replicas: int(m.Replicas.ValueInt64()),
			UserData: userData,
		},
		Name: m.Name.ValueString(),
	}

	return workloadPool, nil
}

type FirewallRuleModel struct {
	Direction types.String `tfsdk:"direction"`
	Protocol  types.String `tfsdk:"protocol"`
	Ports     types.String `tfsdk:"ports"`
	Prefixes  types.Set    `tfsdk:"prefixes"`
}

func NewFirewallRuleModel(source nscale.FirewallRule) FirewallRuleModel {
	ports := strconv.Itoa(source.Port)
	if source.PortMax != nil {
		ports += "-" + strconv.Itoa(*source.PortMax)
	}

	prefixes := make([]attr.Value, 0, len(source.Prefixes))
	for _, prefix := range source.Prefixes {
		prefixes = append(prefixes, types.StringValue(prefix))
	}

	return FirewallRuleModel{
		Direction: types.StringValue(string(source.Direction)),
		Protocol:  types.StringValue(string(source.Protocol)),
		Ports:     types.StringValue(ports),
		Prefixes:  types.SetValueMust(types.StringType, prefixes),
	}
}

func NewFirewallRuleModels(source []nscale.FirewallRule) []FirewallRuleModel {
	rules := make([]FirewallRuleModel, 0, len(source))
	for _, data := range source {
		rules = append(rules, NewFirewallRuleModel(data))
	}
	return rules
}

func (m *FirewallRuleModel) NscaleFirewallRule() (nscale.FirewallRule, diag.Diagnostics) {
	ports := strings.Split(m.Ports.ValueString(), "-")
	if len(ports) > 2 {
		diagnostics := NewErrorDiagnostics(
			"Invalid Port Format",
			"Firewall rule ports must be either a single port or a range in the format 'start-end'.",
		)
		return nscale.FirewallRule{}, diagnostics
	}

	portNumbers := make([]int, 0, len(ports))
	for _, port := range ports {
		portNumber, err := strconv.Atoi(port)
		if err != nil {
			diagnostics := NewErrorDiagnostics(
				"Failed to Parse Port Number",
				fmt.Sprintf("An error occurred while parsing the port number: %s", err),
			)
			return nscale.FirewallRule{}, diagnostics
		}
		portNumbers = append(portNumbers, portNumber)
	}

	var portMax *int
	if len(portNumbers) > 1 {
		portMax = &portNumbers[1]
	}

	var prefixes []string
	if diagnostics := m.Prefixes.ElementsAs(context.Background(), &prefixes, false); diagnostics.HasError() {
		return nscale.FirewallRule{}, diagnostics
	}

	firewallRule := nscale.FirewallRule{
		Direction: nscale.FirewallRuleDirection(m.Direction.ValueString()),
		Port:      portNumbers[0],
		PortMax:   portMax,
		Prefixes:  prefixes,
		Protocol:  nscale.FirewallRuleProtocol(m.Protocol.ValueString()),
	}

	return firewallRule, nil
}

type MachineModel struct {
	Hostname  types.String `tfsdk:"hostname"`
	PrivateIP types.String `tfsdk:"private_ip"`
	PublicIP  types.String `tfsdk:"public_ip"`
}

func NewMachineModel(source nscale.ComputeClusterMachineStatus) MachineModel {
	return MachineModel{
		Hostname:  types.StringValue(source.Hostname),
		PrivateIP: types.StringPointerValue(source.PrivateIP),
		PublicIP:  types.StringPointerValue(source.PublicIP),
	}
}

func NewMachineModels(source []nscale.ComputeClusterMachineStatus) []MachineModel {
	machines := make([]MachineModel, 0, len(source))
	for _, data := range source {
		machines = append(machines, NewMachineModel(data))
	}
	return machines
}

func NewErrorDiagnostics(summary, detail string) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	diagnostics.AddError(summary, detail)
	return diagnostics
}
