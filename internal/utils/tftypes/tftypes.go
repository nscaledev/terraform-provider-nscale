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

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tags"
)

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

// UUIDStringValue renders a UUID-typed API field as the string held in state.
//
// The canonical specs type resource identifiers as UUIDs, while Terraform
// carries every identifier as a string, so reads convert on the way in and
// ValueUUID converts on the way out.
func UUIDStringValue(id uuid.UUID) basetypes.StringValue {
	return types.StringValue(id.String())
}

// UUIDStringPointerValue is UUIDStringValue for an optional field, mapping an
// absent identifier to null rather than to the zero UUID.
func UUIDStringPointerValue(id *uuid.UUID) basetypes.StringValue {
	if id == nil {
		return types.StringNull()
	}

	return types.StringValue(id.String())
}

// ValueUUID parses a string attribute into the UUID the SDK expects, reporting a
// malformed identifier as a Terraform diagnostic against the attribute rather
// than letting the API reject it later.
func ValueUUID(value basetypes.StringValue, attributeName string) (uuid.UUID, diag.Diagnostics) {
	id, err := uuid.Parse(value.ValueString())
	if err != nil {
		var diagnostics diag.Diagnostics
		diagnostics.AddError(
			fmt.Sprintf("Invalid %s", attributeName),
			fmt.Sprintf("Expected %s to be a UUID, got %q: %s", attributeName, value.ValueString(), err),
		)

		return uuid.UUID{}, diagnostics
	}

	return id, nil
}

// ValueUUIDPointer is ValueUUID for an optional attribute: a null or unset value
// yields a nil pointer with no diagnostic, so the field is simply omitted.
func ValueUUIDPointer(value basetypes.StringValue, attributeName string) (*uuid.UUID, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return nil, nil
	}

	id, diagnostics := ValueUUID(value, attributeName)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &id, nil
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

// TagMapValueMust renders tags as the map attribute held in state. It is generic
// over the tag type so it accepts a response's tags straight from any service
// package as well as the provider's own tags.List.
func TagMapValueMust[T tags.SDKTag](tagList *[]T) basetypes.MapValue {
	if tagList == nil || len(*tagList) == 0 {
		return basetypes.NewMapNull(types.StringType)
	}

	elements := make(map[string]attr.Value, len(*tagList))
	for _, tag := range *tags.FromAPI(tagList) {
		elements[tag.Name] = types.StringValue(tag.Value)
	}

	return basetypes.NewMapValueMust(types.StringType, elements)
}

// ValueTagListPointer reads a map attribute into the provider's own tag list.
// Callers building a request body convert to their service's tag type at the
// point of use, with nscale.TagsToAPI.
func ValueTagListPointer(tagMap basetypes.MapValue) (*tags.List, diag.Diagnostics) {
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

	tagList := make(tags.List, 0, len(data))
	for name, value := range data {
		tagList = append(tagList, tags.Tag{
			Name:  name,
			Value: value,
		})
	}

	return &tagList, nil
}
