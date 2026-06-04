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
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// DataSourceAdapter captures the per-data-source variation for the read+map
// data-source lifecycle: the SDK lookup, the model mapping, and the display
// strings. It is the read-only counterpart of ResourceAdapter, for data sources
// that look a resource up by id and map it into their model.
type DataSourceAdapter[TFModel any, APIRead any] struct {
	// TypeNameSuffix is appended to the provider name, e.g. "_identity_project".
	TypeNameSuffix string
	// Title and Name are the human-readable strings used in diagnostics.
	Title string
	Name  string

	// Get looks the object up by id.
	Get func(ctx context.Context, client *Client, id string) (*APIRead, error)

	// ToModel maps an API read object into a fresh TF model.
	ToModel func(api *APIRead) TFModel

	// IDFromModel reads the configured id off the model.
	IDFromModel func(m TFModel) string
}

// GenericDataSource implements the datasource.DataSource lifecycle once, driven
// by a DataSourceAdapter. Concrete data sources embed *GenericDataSource and
// supply only a Schema method plus a constructor wiring the adapter.
type GenericDataSource[TFModel any, APIRead any] struct {
	client  *Client
	adapter DataSourceAdapter[TFModel, APIRead]
}

// NewGenericDataSource builds a GenericDataSource for the given adapter. The
// client is set later, in Configure.
func NewGenericDataSource[TFModel, APIRead any](
	adapter DataSourceAdapter[TFModel, APIRead],
) *GenericDataSource[TFModel, APIRead] {
	// client is populated later, in Configure.
	return &GenericDataSource[TFModel, APIRead]{client: nil, adapter: adapter}
}

func (s *GenericDataSource[TFModel, APIRead]) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}

	client, ok := request.ProviderData.(*Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configuration Type",
			fmt.Sprintf(
				"Expected *nscale.Client, got: %T. Please contact the Nscale team for support.",
				request.ProviderData,
			),
		)
		return
	}

	s.client = client
}

func (s *GenericDataSource[TFModel, APIRead]) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + s.adapter.TypeNameSuffix
}

func (s *GenericDataSource[TFModel, APIRead]) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	data, diagnostics := ReadTerraformState[TFModel](ctx, request.Config.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	api, err := s.adapter.Get(ctx, s.client, s.adapter.IDFromModel(data))
	if err != nil {
		TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			fmt.Sprintf("Failed to Read %s", s.adapter.Title),
			fmt.Sprintf("An error occurred while retrieving the %s: %s", s.adapter.Name, err),
		)
		return
	}

	data = s.adapter.ToModel(api)
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
