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
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"
)

func ptr[T any](v T) *T { return &v }

func TestNewObjectStorageAccessKeyModel(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	source := &storageapi.ObjectStorageAccessKeyRead{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:           "ak-123",
			Name:         "writer",
			Description:  ptr("ingest credential"),
			ProjectId:    "proj-1",
			CreationTime: created,
		},
		Spec: storageapi.ObjectStorageAccessKeySpec{
			AccessKeyId:    ptr("AKIA000000000000"),
			IdentityPolicy: "bucket-admin",
		},
	}

	got := NewObjectStorageAccessKeyModel(source)

	if got.ID.ValueString() != "ak-123" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "ak-123")
	}
	if got.Name.ValueString() != "writer" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "writer")
	}
	if got.Description.ValueString() != "ingest credential" {
		t.Errorf("Description = %q, want %q", got.Description.ValueString(), "ingest credential")
	}
	if got.IdentityPolicy.ValueString() != "bucket-admin" {
		t.Errorf("IdentityPolicy = %q, want %q", got.IdentityPolicy.ValueString(), "bucket-admin")
	}
	if got.AccessKeyID.ValueString() != "AKIA000000000000" {
		t.Errorf("AccessKeyID = %q, want %q", got.AccessKeyID.ValueString(), "AKIA000000000000")
	}
	if got.ProjectID.ValueString() != "proj-1" {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID.ValueString(), "proj-1")
	}
	if got.CreationTime.ValueString() != created.Format(time.RFC3339) {
		t.Errorf("CreationTime = %q, want %q", got.CreationTime.ValueString(), created.Format(time.RFC3339))
	}
	// Secret and EndpointID are caller-managed; the converter must leave
	// them as zero values so the resource's stash-on-Read logic can re-attach them.
	if !got.Secret.IsNull() {
		t.Errorf("Secret should be null after Read-shape conversion, got %q", got.Secret.ValueString())
	}
	if !got.EndpointID.IsNull() {
		t.Errorf("EndpointID should be null after Read-shape conversion, got %q", got.EndpointID.ValueString())
	}
}

func TestNewObjectStorageAccessKeyModel_NilOptionalFields(t *testing.T) {
	source := &storageapi.ObjectStorageAccessKeyRead{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:           "ak-bare",
			Name:         "writer",
			ProjectId:    "proj-1",
			CreationTime: time.Now(),
		},
		Spec: storageapi.ObjectStorageAccessKeySpec{
			// AccessKeyId is *string and may be nil before the controller settles.
			AccessKeyId:    nil,
			IdentityPolicy: "bucket-admin",
		},
	}

	got := NewObjectStorageAccessKeyModel(source)

	if !got.Description.IsNull() {
		t.Errorf("Description should be null when source description is nil, got %q", got.Description.ValueString())
	}
	if !got.AccessKeyID.IsNull() {
		t.Errorf("AccessKeyID should be null when source is nil, got %q", got.AccessKeyID.ValueString())
	}
}

func TestNewObjectStorageAccessKeyModelFromCreate_PopulatesSecret(t *testing.T) {
	source := &storageapi.ObjectStorageAccessKeyCreateResponseBody{
		Metadata: coreapi.ProjectScopedResourceReadMetadata{
			Id:           "ak-1",
			Name:         "writer",
			ProjectId:    "proj-1",
			CreationTime: time.Now(),
		},
		Spec: storageapi.ObjectStorageAccessKeyCreateResponseSpec{
			AccessKeyId:    "AKIA000000000000",
			IdentityPolicy: "bucket-admin",
			Secret:         "super-secret",
		},
	}

	got := NewObjectStorageAccessKeyModelFromCreate(source)

	if got.Secret.ValueString() != "super-secret" {
		t.Errorf("Secret = %q, want %q", got.Secret.ValueString(), "super-secret")
	}
	if got.AccessKeyID.ValueString() != "AKIA000000000000" {
		t.Errorf("AccessKeyID = %q, want %q", got.AccessKeyID.ValueString(), "AKIA000000000000")
	}
}

func TestNscaleObjectStorageAccessKeyCreateParams(t *testing.T) {
	m := &ObjectStorageAccessKeyModel{
		Name:           types.StringValue("writer"),
		Description:    types.StringValue("ingest"),
		IdentityPolicy: types.StringValue("bucket-admin"),
	}

	params := m.NscaleObjectStorageAccessKeyCreateParams()

	if params.Metadata.Name != "writer" {
		t.Errorf("Metadata.Name = %q, want %q", params.Metadata.Name, "writer")
	}
	if params.Metadata.Description == nil || *params.Metadata.Description != "ingest" {
		t.Errorf("Metadata.Description = %v, want pointer to %q", params.Metadata.Description, "ingest")
	}
	if params.Spec.IdentityPolicy != "bucket-admin" {
		t.Errorf("Spec.IdentityPolicy = %q, want %q", params.Spec.IdentityPolicy, "bucket-admin")
	}
	// Confirm the request marshals to JSON without surprises (no permissions field).
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(string(encoded), "permissions") {
		t.Errorf("request JSON should not contain `permissions`: %s", encoded)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
