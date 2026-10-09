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

package nscale

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// KeepStateWhenUnchanged plans no change when the plan differs from prior
// state only in unconfigured computed values that are unknown. Call it from a
// resource's ModifyPlan.
//
// The framework compares Terraform's proposed state with prior state before
// any plan modifier runs, and on a difference marks every computed attribute
// the configuration leaves null unknown. A plan modifier that settles on the
// prior value, such as for a respelled UUID, therefore still plans an update
// of those attributes.
func KeepStateWhenUnchanged(
	_ context.Context,
	request resource.ModifyPlanRequest,
	response *resource.ModifyPlanResponse,
) {
	if request.State.Raw.IsNull() || request.Plan.Raw.IsNull() {
		return
	}

	restored, err := tftypes.Transform(
		request.Plan.Raw,
		func(attributePath *tftypes.AttributePath, value tftypes.Value) (tftypes.Value, error) {
			if value.IsKnown() {
				return value, nil
			}

			config, ok := valueAt(request.Config.Raw, attributePath)
			if !ok || !config.IsNull() {
				return value, nil
			}

			prior, found := valueAt(request.State.Raw, attributePath)
			if !found {
				return value, nil
			}

			return prior, nil
		},
	)
	if err != nil || !restored.Equal(request.State.Raw) {
		return
	}

	response.Plan.Raw = request.State.Raw
}

func valueAt(root tftypes.Value, attributePath *tftypes.AttributePath) (tftypes.Value, bool) {
	found, _, err := tftypes.WalkAttributePath(root, attributePath)
	if err != nil {
		return tftypes.Value{}, false
	}

	value, ok := found.(tftypes.Value)

	return value, ok
}
