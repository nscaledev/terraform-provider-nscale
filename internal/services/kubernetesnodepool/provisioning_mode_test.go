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

package kubernetesnodepool

import (
	"reflect"
	"testing"
)

// TestCheckCapacityMode is the mode x compute x reservation matrix. Both valid
// rows and every invalid one, because the failure mode of getting this wrong is
// an apply that fails against the API after the user has already waited for a
// cluster.
func TestCheckCapacityMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		mode           string
		computeSet     bool
		reservationSet bool
		want           []capacityModeFinding
	}{
		{
			name:       "compute mode with a compute block is valid",
			mode:       "compute",
			computeSet: true,
			want:       nil,
		},
		{
			name:           "reservation mode with a reservation block is valid",
			mode:           "reservation",
			reservationSet: true,
			want:           nil,
		},
		{
			name: "compute mode with no capacity block",
			mode: "compute",
			want: []capacityModeFinding{
				{Attribute: computeAttribute, Issue: capacityModeMissing},
			},
		},
		{
			name:           "reservation mode with no capacity block",
			mode:           "reservation",
			want:           []capacityModeFinding{{Attribute: reservationAttribute, Issue: capacityModeMissing}},
			reservationSet: false,
		},
		{
			// The interesting wrong case: a plausible-looking config where the
			// user changed the mode and forgot to change the block.
			name:           "compute mode with only a reservation block",
			mode:           "compute",
			reservationSet: true,
			want: []capacityModeFinding{
				{Attribute: computeAttribute, Issue: capacityModeMissing},
				{Attribute: reservationAttribute, Issue: capacityModeUnexpected},
			},
		},
		{
			name:       "reservation mode with only a compute block",
			mode:       "reservation",
			computeSet: true,
			want: []capacityModeFinding{
				{Attribute: reservationAttribute, Issue: capacityModeMissing},
				{Attribute: computeAttribute, Issue: capacityModeUnexpected},
			},
		},
		{
			name:           "compute mode with both blocks",
			mode:           "compute",
			computeSet:     true,
			reservationSet: true,
			want: []capacityModeFinding{
				{Attribute: reservationAttribute, Issue: capacityModeUnexpected},
			},
		},
		{
			name:           "reservation mode with both blocks",
			mode:           "reservation",
			computeSet:     true,
			reservationSet: true,
			want: []capacityModeFinding{
				{Attribute: computeAttribute, Issue: capacityModeUnexpected},
			},
		},
		{
			// An unresolved mode cannot be validated against. Reporting a missing
			// block here would reject a config whose mode comes from a variable
			// or another resource's output.
			name:           "unknown mode reports nothing",
			mode:           "",
			computeSet:     true,
			reservationSet: true,
			want:           nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := checkCapacityMode(test.mode, test.computeSet, test.reservationSet)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("checkCapacityMode(%q, %t, %t) = %+v, want %+v",
					test.mode, test.computeSet, test.reservationSet, got, test.want)
			}
		})
	}
}
