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

package objectstorage

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"
)

type ObjectStorageEndpointClassModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Description            types.String `tfsdk:"description"`
	RegionID               types.String `tfsdk:"region_id"`
	SupportedEndpointTypes types.List   `tfsdk:"supported_endpoint_types"`
	CreationTime           types.String `tfsdk:"creation_time"`
}

func NewObjectStorageEndpointClassModel(
	source *storageapi.ObjectStorageEndpointClassRead,
) ObjectStorageEndpointClassModel {
	supported := make([]attr.Value, 0, len(source.Spec.SupportedEndpointType))
	for _, t := range source.Spec.SupportedEndpointType {
		supported = append(supported, types.StringValue(string(t)))
	}

	return ObjectStorageEndpointClassModel{
		ID:                     types.StringValue(source.Metadata.Id),
		Name:                   types.StringValue(source.Metadata.Name),
		Description:            types.StringPointerValue(source.Metadata.Description),
		RegionID:               types.StringValue(source.Spec.RegionId),
		SupportedEndpointTypes: types.ListValueMust(types.StringType, supported),
		CreationTime:           types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}
