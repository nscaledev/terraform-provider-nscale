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

package instance

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/nscaledev/terraform-provider-nscale/internal/utils/uuidtype"
)

func TestInstanceUUIDAttributesUseUUIDType(t *testing.T) {
	ctx := context.Background()

	var resourceSchema resource.SchemaResponse
	NewInstanceResource().Schema(ctx, resource.SchemaRequest{}, &resourceSchema)

	var dataSourceSchema datasource.SchemaResponse
	NewInstanceDataSource().Schema(ctx, datasource.SchemaRequest{}, &dataSourceSchema)

	for _, name := range []string{"image_id", "flavor_id", "ssh_certificate_authority_id"} {
		attributePath := path.Root(name)

		resourceAttribute, diagnostics := resourceSchema.Schema.AttributeAtPath(ctx, attributePath)
		if diagnostics.HasError() {
			t.Fatalf("resource AttributeAtPath(%s) diagnostics = %v", attributePath, diagnostics)
		}

		if got := resourceAttribute.GetType(); !got.Equal(uuidtype.Type{}) {
			t.Errorf("resource %s type = %s, want %s", attributePath, got, uuidtype.Type{})
		}

		dataSourceAttribute, diagnostics := dataSourceSchema.Schema.AttributeAtPath(ctx, attributePath)
		if diagnostics.HasError() {
			t.Fatalf("data source AttributeAtPath(%s) diagnostics = %v", attributePath, diagnostics)
		}

		if got := dataSourceAttribute.GetType(); !got.Equal(uuidtype.Type{}) {
			t.Errorf("data source %s type = %s, want %s", attributePath, got, uuidtype.Type{})
		}
	}
}
