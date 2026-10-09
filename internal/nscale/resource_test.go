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
	"testing"
	"time"
)

func TestResourceAdapterUpdateTimeoutDefault(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		adapterDefault time.Duration
		want           time.Duration
	}{
		"unset keeps the shared default": {adapterDefault: 0, want: defaultStateWatcherTimeout},
		"set overrides it":               {adapterDefault: 2 * time.Hour, want: 2 * time.Hour},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			adapter := ResourceAdapter[struct{}, struct{}]{DefaultUpdateTimeout: tt.adapterDefault}
			if got := adapter.updateTimeoutDefault(); got != tt.want {
				t.Errorf("updateTimeoutDefault() = %s, want %s", got, tt.want)
			}
		})
	}
}
