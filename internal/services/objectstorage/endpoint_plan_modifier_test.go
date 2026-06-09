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
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestNormalizeJSONPlanModifier_EquivalentJSONSuppressesDiff verifies the
// core invariant of the plan modifier: two JSON documents that mean the same
// thing — different whitespace, different key ordering — should not produce
// a diff. Without this, every `jsonencode({...})` re-render would re-trigger
// an apply.
func TestNormalizeJSONPlanModifier_EquivalentJSONSuppressesDiff(t *testing.T) {
	state := types.StringValue(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`,
	)
	plan := types.StringValue(`{
		"Statement": [{"Resource": "*", "Action": "s3:*", "Effect": "Allow"}],
		"Version": "2012-10-17"
	}`)

	req := planmodifier.StringRequest{StateValue: state, PlanValue: plan}
	resp := &planmodifier.StringResponse{PlanValue: plan}

	normalizeJSONPlanModifier{}.PlanModifyString(context.Background(), req, resp)

	if resp.PlanValue.ValueString() != state.ValueString() {
		t.Errorf("equivalent JSON should collapse plan to state; got %q want %q",
			resp.PlanValue.ValueString(), state.ValueString())
	}
}

// TestNormalizeJSONPlanModifier_DifferentJSONPreservesDiff verifies the
// counterpart: when the documents genuinely differ, the plan must keep its
// new value so the apply re-renders it.
func TestNormalizeJSONPlanModifier_DifferentJSONPreservesDiff(t *testing.T) {
	state := types.StringValue(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject"}]}`)
	plan := types.StringValue(`{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"s3:*"}]}`)

	req := planmodifier.StringRequest{StateValue: state, PlanValue: plan}
	resp := &planmodifier.StringResponse{PlanValue: plan}

	normalizeJSONPlanModifier{}.PlanModifyString(context.Background(), req, resp)

	if resp.PlanValue.ValueString() != plan.ValueString() {
		t.Errorf("different JSON should preserve plan value; got %q want %q",
			resp.PlanValue.ValueString(), plan.ValueString())
	}
}

// TestNormalizeJSONPlanModifier_NullState covers the early-return branch:
// at create time the state has no prior value, and there is nothing to
// compare against.
func TestNormalizeJSONPlanModifier_NullState(t *testing.T) {
	plan := types.StringValue(`{"Version":"2012-10-17"}`)
	req := planmodifier.StringRequest{StateValue: types.StringNull(), PlanValue: plan}
	resp := &planmodifier.StringResponse{PlanValue: plan}

	normalizeJSONPlanModifier{}.PlanModifyString(context.Background(), req, resp)

	if resp.PlanValue.ValueString() != plan.ValueString() {
		t.Errorf("null state should leave plan untouched; got %q want %q",
			resp.PlanValue.ValueString(), plan.ValueString())
	}
}

// TestNormalizeJSONPlanModifier_UnknownPlan covers the other early-return
// branch — a plan value that hasn't been computed yet.
func TestNormalizeJSONPlanModifier_UnknownPlan(t *testing.T) {
	state := types.StringValue(`{"Version":"2012-10-17"}`)
	req := planmodifier.StringRequest{StateValue: state, PlanValue: types.StringUnknown()}
	resp := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}

	normalizeJSONPlanModifier{}.PlanModifyString(context.Background(), req, resp)

	if !resp.PlanValue.IsUnknown() {
		t.Errorf("unknown plan should remain unknown; got %v", resp.PlanValue)
	}
}

// TestNormalizeJSONPlanModifier_InvalidJSONNoOp covers the recovery path: if
// either side is not valid JSON (shouldn't happen given the validators but
// defence-in-depth) the modifier silently leaves the plan as-is rather than
// panicking.
func TestNormalizeJSONPlanModifier_InvalidJSONNoOp(t *testing.T) {
	state := types.StringValue(`{"Version":"2012-10-17"}`)
	plan := types.StringValue("not-json-at-all")
	req := planmodifier.StringRequest{StateValue: state, PlanValue: plan}
	resp := &planmodifier.StringResponse{PlanValue: plan}

	normalizeJSONPlanModifier{}.PlanModifyString(context.Background(), req, resp)

	if resp.PlanValue.ValueString() != plan.ValueString() {
		t.Errorf("invalid plan JSON should be left alone, not crash; got %q",
			resp.PlanValue.ValueString())
	}
}

// TestNormalizeJSONPlanModifier_Descriptions just covers the doc string
// methods to ensure they don't panic and return non-empty text.
func TestNormalizeJSONPlanModifier_Descriptions(t *testing.T) {
	m := normalizeJSONPlanModifier{}
	ctx := context.Background()
	if m.Description(ctx) == "" {
		t.Error("Description should be non-empty")
	}
	if m.MarkdownDescription(ctx) == "" {
		t.Error("MarkdownDescription should be non-empty")
	}
}
