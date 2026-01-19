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

package filestorage

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

type FileStorageClassModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Protocols   types.List   `tfsdk:"protocols"`
	RegionID    types.String `tfsdk:"region_id"`
}

func NewFileStorageClassModel(source *regionapi.StorageClassV2Read) FileStorageClassModel {
	protocols := make([]attr.Value, 0, len(source.Spec.Protocols))
	for _, protocol := range source.Spec.Protocols {
		protocols = append(protocols, types.StringValue(string(protocol)))
	}

	return FileStorageClassModel{
		ID:          types.StringValue(source.Metadata.Id),
		Name:        types.StringValue(source.Metadata.Name),
		Description: types.StringPointerValue(source.Metadata.Description),
		Protocols:   tftypes.NullableListValueMust(types.StringType, protocols),
		RegionID:    types.StringValue(source.Spec.RegionId),
	}
}
