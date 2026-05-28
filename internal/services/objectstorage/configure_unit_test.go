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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Configure on each resource/data source has two unhappy branches:
//  1. ProviderData is nil → early return (already covered by happy-path acc
//     tests since the framework always calls Configure twice).
//  2. ProviderData is set but not a *nscale.Client → add an error
//     diagnostic and bail. This second branch is unreachable in normal
//     flows but exists as defence-in-depth; without these unit tests it
//     would be a permanent 0%-coverage island.

func TestObjectStorageAccessKeyResource_Configure_WrongType(t *testing.T) {
	r := &ObjectStorageAccessKeyResource{}
	resp := &resource.ConfigureResponse{Diagnostics: diag.Diagnostics{}}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "not a client"}, resp)
	assertWrongTypeDiagnostic(t, resp.Diagnostics)
}

func TestObjectStorageEndpointResource_Configure_WrongType(t *testing.T) {
	r := &ObjectStorageEndpointResource{}
	resp := &resource.ConfigureResponse{Diagnostics: diag.Diagnostics{}}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: 42}, resp)
	assertWrongTypeDiagnostic(t, resp.Diagnostics)
}

func TestObjectStorageAccessKeyDataSource_Configure_WrongType(t *testing.T) {
	s := &ObjectStorageAccessKeyDataSource{}
	resp := &datasource.ConfigureResponse{Diagnostics: diag.Diagnostics{}}
	s.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: struct{}{}}, resp)
	assertWrongTypeDiagnostic(t, resp.Diagnostics)
}

func TestObjectStorageEndpointDataSource_Configure_WrongType(t *testing.T) {
	s := &ObjectStorageEndpointDataSource{}
	resp := &datasource.ConfigureResponse{Diagnostics: diag.Diagnostics{}}
	s.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: struct{}{}}, resp)
	assertWrongTypeDiagnostic(t, resp.Diagnostics)
}

func TestObjectStorageEndpointClassDataSource_Configure_WrongType(t *testing.T) {
	s := &ObjectStorageEndpointClassDataSource{}
	resp := &datasource.ConfigureResponse{Diagnostics: diag.Diagnostics{}}
	s.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: struct{}{}}, resp)
	assertWrongTypeDiagnostic(t, resp.Diagnostics)
}

// TestObjectStorage_Configure_NilProviderDataNoOps locks in the second
// invariant: when ProviderData is nil (the first of two Configure calls the
// framework always makes), every Configure must return cleanly without
// emitting diagnostics.
func TestObjectStorage_Configure_NilProviderDataNoOps(t *testing.T) {
	t.Run("access_key_resource", func(t *testing.T) {
		r := &ObjectStorageAccessKeyResource{}
		resp := &resource.ConfigureResponse{Diagnostics: diag.Diagnostics{}}
		r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("nil ProviderData should be a no-op, got %v", resp.Diagnostics.Errors())
		}
	})
	t.Run("endpoint_resource", func(t *testing.T) {
		r := &ObjectStorageEndpointResource{}
		resp := &resource.ConfigureResponse{Diagnostics: diag.Diagnostics{}}
		r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("nil ProviderData should be a no-op, got %v", resp.Diagnostics.Errors())
		}
	})
}

func assertWrongTypeDiagnostic(t *testing.T, diags diag.Diagnostics) {
	t.Helper()
	if !diags.HasError() {
		t.Fatal("expected an error diagnostic for non-*nscale.Client ProviderData")
	}
	for _, d := range diags.Errors() {
		if strings.Contains(d.Summary(), "Unexpected Resource Configuration Type") {
			return
		}
	}
	t.Errorf("expected an 'Unexpected Resource Configuration Type' summary; got %v", diags.Errors())
}
