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

// Package uuidtype provides a string attribute type whose values are equal when
// they spell the same UUID.
//
// The API answers with identifiers in canonical lowercase form, so a
// configuration written in another spelling (upper case, braces, urn:uuid:)
// would otherwise read back as a different value. Semantic equality lets the
// framework keep the configured spelling in state when the API returns the same
// UUID.
package uuidtype

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	_ basetypes.StringTypable                    = Type{}
	_ basetypes.StringValuableWithSemanticEquals = Value{}
	_ planmodifier.String                        = useStateForSameUUID{}
)

// Type is the attribute type to set as a schema attribute's CustomType.
type Type struct {
	basetypes.StringType
}

func (t Type) Equal(o attr.Type) bool {
	other, ok := o.(Type)
	if !ok {
		return false
	}

	return t.StringType.Equal(other.StringType)
}

func (t Type) String() string {
	return "uuidtype.Type"
}

//nolint:ireturn // implements basetypes.StringTypable
func (t Type) ValueFromString(
	_ context.Context,
	in basetypes.StringValue,
) (basetypes.StringValuable, diag.Diagnostics) {
	return Value{StringValue: in}, nil
}

func (t Type) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("reading UUID string value: %w", err)
	}

	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type of %T", attrValue)
	}

	return Value{StringValue: stringValue}, nil
}

func (t Type) ValueType(_ context.Context) attr.Value {
	return Value{}
}

// Value is the model field type for an attribute whose CustomType is Type.
type Value struct {
	basetypes.StringValue
}

// NewValue returns a known value holding s as written.
func NewValue(s string) Value {
	return Value{StringValue: basetypes.NewStringValue(s)}
}

// NewNull returns a null value.
func NewNull() Value {
	return Value{StringValue: basetypes.NewStringNull()}
}

// NewUnknown returns an unknown value.
func NewUnknown() Value {
	return Value{StringValue: basetypes.NewStringUnknown()}
}

// FromUUID renders a UUID-typed API field in its canonical form.
func FromUUID(id uuid.UUID) Value {
	return NewValue(id.String())
}

// FromUUIDPointer is FromUUID for an optional field, mapping an absent
// identifier to null rather than to the zero UUID.
func FromUUIDPointer(id *uuid.UUID) Value {
	if id == nil {
		return NewNull()
	}

	return FromUUID(*id)
}

func (v Value) Equal(o attr.Value) bool {
	other, ok := o.(Value)
	if !ok {
		return false
	}

	return v.StringValue.Equal(other.StringValue)
}

//nolint:ireturn // implements attr.Value
func (v Value) Type(_ context.Context) attr.Type {
	return Type{}
}

// StringSemanticEquals reports whether two values name the same UUID. Values
// that do not both parse as UUIDs fall back to exact string comparison, so a
// malformed identifier never matches anything but itself.
func (v Value) StringSemanticEquals(
	_ context.Context,
	newValuable basetypes.StringValuable,
) (bool, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	newValue, ok := newValuable.(Value)
	if !ok {
		diagnostics.AddError(
			"Semantic Equality Check Error",
			fmt.Sprintf(
				"Expected value type %T but got %T. Please contact the Nscale team for support.",
				v,
				newValuable,
			),
		)

		return false, diagnostics
	}

	if v.IsNull() || v.IsUnknown() || newValue.IsNull() || newValue.IsUnknown() {
		return v.Equal(newValue), diagnostics
	}

	prior, priorErr := uuid.Parse(v.ValueString())
	next, nextErr := uuid.Parse(newValue.ValueString())

	if priorErr != nil || nextErr != nil {
		return v.ValueString() == newValue.ValueString(), diagnostics
	}

	return prior == next, diagnostics
}

// UseStateForSameUUID returns a plan modifier that keeps the prior state's
// spelling when the configuration names the same UUID, so respelling an
// identifier plans no change. Semantic equality applies only to values the
// provider returns, not to the plan; Terraform accepts a planned value equal to
// prior state in place of an equivalent configured one, computed or not.
//
// Put it before any modifier that compares the plan with state, such as
// RequiresReplace.
//
//nolint:ireturn // plan modifiers are returned by interface
func UseStateForSameUUID() planmodifier.String {
	return useStateForSameUUID{}
}

type useStateForSameUUID struct{}

func (useStateForSameUUID) Description(context.Context) string {
	return "Keeps the prior spelling when the configuration names the same UUID."
}

func (m useStateForSameUUID) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (useStateForSameUUID) PlanModifyString(
	ctx context.Context,
	request planmodifier.StringRequest,
	response *planmodifier.StringResponse,
) {
	if request.StateValue.IsNull() || request.PlanValue.IsNull() || request.PlanValue.IsUnknown() {
		return
	}

	same, diagnostics := Value{StringValue: request.StateValue}.StringSemanticEquals(
		ctx,
		Value{StringValue: request.PlanValue},
	)
	response.Diagnostics.Append(diagnostics...)

	if same {
		response.PlanValue = request.StateValue
	}
}
