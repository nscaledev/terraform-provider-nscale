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
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"
)

func TestNewObjectStorageEndpointClassModel(t *testing.T) {
	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	source := &storageapi.ObjectStorageEndpointClassRead{
		Metadata: coreapi.ResourceReadMetadata{
			Id:           "class-1",
			Name:         "standard",
			Description:  new("Standard public exposure"),
			CreationTime: created,
		},
		Spec: storageapi.ObjectStorageEndpointClassSpec{
			RegionId: "region-1",
			SupportedEndpointType: []storageapi.ObjectStorageEndpointClassSupportedEndpointType{
				"public",
				"private",
			},
		},
	}

	got := NewObjectStorageEndpointClassModel(source)

	if got.ID.ValueString() != "class-1" {
		t.Errorf("ID = %q, want class-1", got.ID.ValueString())
	}
	if got.Name.ValueString() != "standard" {
		t.Errorf("Name = %q, want standard", got.Name.ValueString())
	}
	if got.Description.ValueString() != "Standard public exposure" {
		t.Errorf("Description = %q, want %q", got.Description.ValueString(), "Standard public exposure")
	}
	if got.RegionID.ValueString() != "region-1" {
		t.Errorf("RegionID = %q, want region-1", got.RegionID.ValueString())
	}
	if got.CreationTime.ValueString() != created.Format(time.RFC3339) {
		t.Errorf("CreationTime = %q, want %q", got.CreationTime.ValueString(), created.Format(time.RFC3339))
	}

	elems := got.SupportedEndpointTypes.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 supported endpoint types, got %d: %v", len(elems), elems)
	}
	if s, _ := elems[0].(types.String); s.ValueString() != "public" {
		t.Errorf("supported[0] = %q, want public", s.ValueString())
	}
	if s, _ := elems[1].(types.String); s.ValueString() != "private" {
		t.Errorf("supported[1] = %q, want private", s.ValueString())
	}
}

// TestNewObjectStorageEndpointClassModel_NilDescription covers the nil-pointer
// branch on the optional Description field.
func TestNewObjectStorageEndpointClassModel_NilDescription(t *testing.T) {
	source := &storageapi.ObjectStorageEndpointClassRead{
		Metadata: coreapi.ResourceReadMetadata{
			Id:           "class-bare",
			Name:         "bare",
			Description:  nil,
			CreationTime: time.Now(),
		},
		Spec: storageapi.ObjectStorageEndpointClassSpec{
			RegionId:              "region-1",
			SupportedEndpointType: []storageapi.ObjectStorageEndpointClassSupportedEndpointType{},
		},
	}

	got := NewObjectStorageEndpointClassModel(source)
	if !got.Description.IsNull() {
		t.Errorf("Description should be null when source description is nil, got %q", got.Description.ValueString())
	}
	if len(got.SupportedEndpointTypes.Elements()) != 0 {
		t.Errorf("SupportedEndpointTypes should be empty, got %d", len(got.SupportedEndpointTypes.Elements()))
	}
}
