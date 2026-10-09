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

package reservation

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/nscaledev/terraform-provider-nscale/internal/utils/uuidtype"
)

func TestPlacementImageIDUsesUUIDType(t *testing.T) {
	ctx := context.Background()
	imageIDPath := path.Root("server_spec").AtName("image_id")

	var resourceSchema resource.SchemaResponse
	NewPlacementResource().Schema(ctx, resource.SchemaRequest{}, &resourceSchema)

	resourceAttribute, diagnostics := resourceSchema.Schema.AttributeAtPath(ctx, imageIDPath)
	if diagnostics.HasError() {
		t.Fatalf("resource AttributeAtPath(%s) diagnostics = %v", imageIDPath, diagnostics)
	}

	if got := resourceAttribute.GetType(); !got.Equal(uuidtype.Type{}) {
		t.Errorf("resource %s type = %s, want %s", imageIDPath, got, uuidtype.Type{})
	}

	var dataSourceSchema datasource.SchemaResponse
	NewPlacementDataSource().Schema(ctx, datasource.SchemaRequest{}, &dataSourceSchema)

	dataSourceAttribute, diagnostics := dataSourceSchema.Schema.AttributeAtPath(ctx, imageIDPath)
	if diagnostics.HasError() {
		t.Fatalf("data source AttributeAtPath(%s) diagnostics = %v", imageIDPath, diagnostics)
	}

	if got := dataSourceAttribute.GetType(); !got.Equal(uuidtype.Type{}) {
		t.Errorf("data source %s type = %s, want %s", imageIDPath, got, uuidtype.Type{})
	}
}
