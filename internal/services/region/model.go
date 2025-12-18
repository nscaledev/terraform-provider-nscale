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

package region

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

type RegionModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func NewRegionModel(source *regionapi.RegionRead) RegionModel {
	return RegionModel{
		ID:          types.StringValue(source.Metadata.Id),
		Name:        types.StringValue(source.Metadata.Name),
		Description: types.StringPointerValue(source.Metadata.Description),
	}
}
