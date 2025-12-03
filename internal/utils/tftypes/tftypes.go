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
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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
