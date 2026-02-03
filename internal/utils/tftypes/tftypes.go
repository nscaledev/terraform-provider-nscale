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

package tftypes

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
)

func NullableListValueMust(elementType attr.Type, elements []attr.Value) basetypes.ListValue {
	if len(elements) == 0 {
		return basetypes.NewListNull(elementType)
	}
	return basetypes.NewListValueMust(elementType, elements)
}

func NullableSetValueMust(elementType attr.Type, elements []attr.Value) basetypes.SetValue {
	if len(elements) == 0 {
		return basetypes.NewSetNull(elementType)
	}
	return basetypes.NewSetValueMust(elementType, elements)
}

func TagMapValueMust(tags *[]coreapi.Tag) basetypes.MapValue {
	if tags == nil || len(*tags) == 0 {
		return basetypes.NewMapNull(types.StringType)
	}

	elements := make(map[string]attr.Value, len(*tags))
	for _, tag := range *tags {
		elements[tag.Name] = types.StringValue(tag.Value)
	}

	return basetypes.NewMapValueMust(types.StringType, elements)
}

func ValueTagListPointer(tagMap basetypes.MapValue) (*[]coreapi.Tag, diag.Diagnostics) {
	if tagMap.IsNull() || tagMap.IsUnknown() {
		return nil, nil
	}

	var data map[string]string
	if diagnostics := tagMap.ElementsAs(context.TODO(), &data, false); diagnostics.HasError() {
		return nil, diagnostics
	}

	if len(data) == 0 {
		return nil, nil
	}

	tags := make([]coreapi.Tag, 0, len(data))
	for name, value := range data {
		tags = append(tags, coreapi.Tag{
			Name:  name,
			Value: value,
		})
	}

	return &tags, nil
}
