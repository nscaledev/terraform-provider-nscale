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

package kubernetescluster

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// resourceOnlyAttributes are on the resource but deliberately not the data
// source: they configure provider behaviour rather than describe the cluster.
var resourceOnlyAttributes = []string{"wait_for_provisioned", "timeouts"}

// TestDataSourceSchemaMatchesResource keeps the two schemas in step. They share
// KubernetesClusterModel, so an attribute added to the model and the resource
// but not the data source compiles fine and only fails at State.Set.
func TestDataSourceSchemaMatchesResource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var resourceResponse resource.SchemaResponse
	NewKubernetesClusterResource().Schema(ctx, resource.SchemaRequest{}, &resourceResponse)

	var dataSourceResponse datasource.SchemaResponse
	NewKubernetesClusterDataSource().Schema(ctx, datasource.SchemaRequest{}, &dataSourceResponse)

	resourceTypes := resourceResponse.Schema.Type().(types.ObjectType).AttrTypes
	dataSourceTypes := dataSourceResponse.Schema.Type().(types.ObjectType).AttrTypes

	for _, name := range resourceOnlyAttributes {
		if _, ok := resourceTypes[name]; !ok {
			t.Errorf("%q is listed as resource-only but is not on the resource", name)
		}
		delete(resourceTypes, name)
	}

	for name, resourceType := range resourceTypes {
		dataSourceType, ok := dataSourceTypes[name]
		if !ok {
			t.Errorf("%q is on the resource but missing from the data source", name)
			continue
		}
		if !resourceType.Equal(dataSourceType) {
			t.Errorf("%q type differs: resource %s, data source %s", name, resourceType, dataSourceType)
		}
	}

	for name := range dataSourceTypes {
		if _, ok := resourceTypes[name]; !ok {
			t.Errorf("%q is on the data source but missing from the resource", name)
		}
	}
}

// TestDataSourceSchemaFitsModel is the runtime failure the parity test guards
// against, caught here instead of in an apply.
func TestDataSourceSchemaFitsModel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var response datasource.SchemaResponse
	NewKubernetesClusterDataSource().Schema(ctx, datasource.SchemaRequest{}, &response)

	state := tfsdk.State{Schema: response.Schema}
	model := NewKubernetesClusterModel(fullCluster(t))

	if diagnostics := state.Set(ctx, &model); diagnostics.HasError() {
		t.Fatalf("setting data source state from the model: %v", diagnostics)
	}
}
