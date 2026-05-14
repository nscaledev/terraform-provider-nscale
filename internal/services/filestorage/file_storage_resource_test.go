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

package filestorage

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFileStorageResourceModelPreserveSizeIfUsageRefreshDisabled(t *testing.T) {
	tests := []struct {
		name              string
		refreshUsage      types.Bool
		currentSize       int64
		previousSize      int64
		expectedFinalSize int64
	}{
		{
			name:              "refresh enabled keeps current size",
			refreshUsage:      types.BoolValue(true),
			currentSize:       9,
			previousSize:      3,
			expectedFinalSize: 9,
		},
		{
			name:              "refresh disabled preserves previous size",
			refreshUsage:      types.BoolValue(false),
			currentSize:       9,
			previousSize:      3,
			expectedFinalSize: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := FileStorageResourceModel{
				FileStorageModel: FileStorageModel{
					Size: types.Int64Value(tt.currentSize),
				},
				RefreshUsage: tt.refreshUsage,
			}

			model.preserveSizeIfUsageRefreshDisabled(types.Int64Value(tt.previousSize))

			if got := model.Size.ValueInt64(); got != tt.expectedFinalSize {
				t.Fatalf("Size = %d, want %d", got, tt.expectedFinalSize)
			}
		})
	}
}
