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
	"encoding/base64"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

type tag interface {
	~struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
}

type tagValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Base64StringValue renders raw bytes returned by the API as the base64 string
// held in configuration and state.
//
// The SDK models these fields as []byte and lets encoding/json base64-encode
// them on the wire, so what reaches us here is already decoded. Attributes such
// as user_data are configured as base64, so re-encode to make the value
// round-trip. Use with ValueBase64BytesPointer on the write path.
func Base64StringValue(value *[]byte) basetypes.StringValue {
	if value == nil {
		return types.StringNull()
	}

	return types.StringValue(base64.StdEncoding.EncodeToString(*value))
}

// ValueBase64BytesPointer decodes a base64-configured string attribute into the
// raw bytes the SDK expects.
//
// The SDK serializes its []byte fields as base64 itself, so passing the encoded
// string's own bytes would send base64(base64(plaintext)) and break consumers
// such as cloud-init. Attributes using this must carry a
// validators.Base64Validator so malformed input is caught during validation
// rather than here.
func ValueBase64BytesPointer(value basetypes.StringValue, attributeName string) (*[]byte, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(value.ValueString())
	if err != nil {
		var diagnostics diag.Diagnostics
		diagnostics.AddError(
			fmt.Sprintf("Invalid %s", attributeName),
			fmt.Sprintf("Failed to decode base64 %s: %s", attributeName, err),
		)

		return nil, diagnostics
	}

	return &decoded, nil
}

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

func TagMapValueMust[T tag](tags *[]T) basetypes.MapValue {
	if tags == nil || len(*tags) == 0 {
		return basetypes.NewMapNull(types.StringType)
	}

	elements := make(map[string]attr.Value, len(*tags))
	for _, tag := range *tags {
		value := tagValue(tag)
		elements[value.Name] = types.StringValue(value.Value)
	}

	return basetypes.NewMapValueMust(types.StringType, elements)
}

func ValueTagListPointer[T tag](tagMap basetypes.MapValue) (*[]T, diag.Diagnostics) {
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

	tags := make([]T, 0, len(data))
	for name, value := range data {
		tags = append(tags, T(tagValue{
			Name:  name,
			Value: value,
		}))
	}

	return &tags, nil
}
