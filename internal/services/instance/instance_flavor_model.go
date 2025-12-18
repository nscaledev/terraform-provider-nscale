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

package instance

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

type InstanceFlavorModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	CPUs        types.Int64  `tfsdk:"cpus"`
	MemorySize  types.Int64  `tfsdk:"memory_size"`
	DiskSize    types.Int64  `tfsdk:"disk_size"`
	GPU         types.Object `tfsdk:"gpu"`
	RegionID    types.String `tfsdk:"region_id"`
}

func NewInstanceFlavorModel(source *regionapi.Flavor, regionID string) InstanceFlavorModel {
	gpu := types.ObjectNull(InstanceFlavorGPUModelAttributeType.AttrTypes)
	if source.Spec.Gpu != nil {
		gpu = NewInstanceFlavorGPUModel(source.Spec.Gpu)
	}

	return InstanceFlavorModel{
		ID:          types.StringValue(source.Metadata.Id),
		Name:        types.StringValue(source.Metadata.Name),
		Description: types.StringPointerValue(source.Metadata.Description),
		CPUs:        types.Int64Value(int64(source.Spec.Cpus)),
		MemorySize:  types.Int64Value(int64(source.Spec.Memory)),
		DiskSize:    types.Int64Value(int64(source.Spec.Disk)),
		GPU:         gpu,
		RegionID:    types.StringValue(regionID),
	}
}

var InstanceFlavorGPUModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"vendor":         types.StringType,
		"model":          types.StringType,
		"physical_count": types.Int64Type,
		"logical_count":  types.Int64Type,
		"memory_size":    types.Int64Type,
	},
}

type InstanceFlavorGPUModel struct {
	Vendor        types.String `tfsdk:"vendor"`
	Model         types.String `tfsdk:"model"`
	PhysicalCount types.Int64  `tfsdk:"physical_count"`
	LogicalCount  types.Int64  `tfsdk:"logical_count"`
	MemorySize    types.Int64  `tfsdk:"memory_size"`
}

func NewInstanceFlavorGPUModel(source *regionapi.GpuSpec) types.Object {
	return types.ObjectValueMust(
		InstanceNetworkInterfaceModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"vendor":         types.StringValue(string(source.Vendor)),
			"model":          types.StringValue(source.Model),
			"physical_count": types.Int64Value(int64(source.PhysicalCount)),
			"logical_count":  types.Int64Value(int64(source.LogicalCount)),
			"memory_size":    types.Int64Value(int64(source.Memory)),
		},
	)
}
