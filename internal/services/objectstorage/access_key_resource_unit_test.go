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
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestObjectStorageAccessKeyResource_Update_ReturnsError guards the
// defence-in-depth path inside Update: every configurable attribute carries
// RequiresReplace, so Update should be unreachable through the framework. If
// some future schema change removes RequiresReplace by mistake, the framework
// will start calling Update and we want a loud diagnostic instead of a silent
// no-op.
func TestObjectStorageAccessKeyResource_Update_ReturnsError(t *testing.T) {
	r := &ObjectStorageAccessKeyResource{}
	resp := &resource.UpdateResponse{Diagnostics: diag.Diagnostics{}}

	r.Update(context.Background(), resource.UpdateRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Update should always add an error diagnostic")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Update Not Supported") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an 'Update Not Supported' summary; got %v", resp.Diagnostics.Errors())
	}
}
