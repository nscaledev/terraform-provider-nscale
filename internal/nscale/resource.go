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
	"net/http"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// ResourceAdapter captures everything that varies between resources, so the
// generic CRUD control flow can live once in GenericResource. Closures receive
// the configured *Client as a parameter (rather than capturing it) because a
// resource is constructed at provider-registration time, before Configure runs.
//
// A nil Update marks the resource immutable: the base then rejects in-place
// updates. The Get closure returns an already-adapted ResourceStatus, so whether
// a resource is project- or organization-scoped is invisible to the base.
type ResourceAdapter[TFModel any, APIRead any] struct {
	// TypeNameSuffix is appended to the provider name, e.g. "_identity_project".
	TypeNameSuffix string
	// Title and Name are the human-readable strings used in diagnostics,
	// e.g. "Project" and "project".
	Title string
	Name  string

	// Create issues the create call and returns the freshly-read API object
	// (used for the id and the first state write before the create wait).
	Create func(ctx context.Context, client *Client, plan TFModel) (*APIRead, diag.Diagnostics)

	// Get reads one object by id, already adapted to ResourceStatus.
	Get func(ctx context.Context, client *Client, id string) (*APIRead, ResourceStatus, error)

	// Update issues the update call (writing an operation tag into its params)
	// and returns the tag key the update watcher waits for. A nil Update marks
	// the resource immutable.
	Update func(ctx context.Context, client *Client, id string, plan TFModel) (operationTagKey string, diags diag.Diagnostics)

	// Delete issues the delete call. The base owns the delete-poll watcher and
	// tolerates a 404 (already gone).
	Delete func(ctx context.Context, client *Client, id string) error

	// ToModel maps an API read object INTO dst, leaving fields the API does not
	// own (notably dst's timeouts) intact.
	ToModel func(api *APIRead, dst *TFModel)

	// IDFromModel and TimeoutsFromModel let the base read the id and timeouts off
	// the model without knowing its concrete type.
	IDFromModel       func(m TFModel) string
	TimeoutsFromModel func(m TFModel) tftimeouts.Value
}

// GenericResource implements the resource.Resource lifecycle once, driven by a
// ResourceAdapter. Concrete resources embed *GenericResource and supply only a
// Schema method plus a constructor wiring the adapter.
type GenericResource[TFModel any, APIRead any] struct {
	client  *Client
	adapter ResourceAdapter[TFModel, APIRead]
}

// NewGenericResource builds a GenericResource for the given adapter. The client
// is set later, in Configure.
func NewGenericResource[TFModel, APIRead any](
	adapter ResourceAdapter[TFModel, APIRead],
) *GenericResource[TFModel, APIRead] {
	// client is populated later, in Configure.
	return &GenericResource[TFModel, APIRead]{client: nil, adapter: adapter}
}

func (r *GenericResource[TFModel, APIRead]) Configure(
	_ context.Context,
	request resource.ConfigureRequest,
	response *resource.ConfigureResponse,
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

	r.client = client
}

func (r *GenericResource[TFModel, APIRead]) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + r.adapter.TypeNameSuffix
}

func (r *GenericResource[TFModel, APIRead]) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *GenericResource[TFModel, APIRead]) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	data, diagnostics := ReadTerraformState[TFModel](ctx, request.Plan.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	api, diagnostics := r.adapter.Create(ctx, r.client, data)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	// Record the ID before waiting so a timeout does not orphan the resource.
	r.adapter.ToModel(api, &data)
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := r.adapter.IDFromModel(data)

	stateWatcher := CreateStateWatcher[APIRead]{
		ResourceTitle: r.adapter.Title,
		ResourceName:  r.adapter.Name,
		GetFunc: func(ctx context.Context) (*APIRead, ResourceStatus, error) {
			return r.adapter.Get(ctx, r.client, id)
		},
	}

	final, ok := stateWatcher.Wait(ctx, r.adapter.TimeoutsFromModel(data), response)
	if !ok {
		return
	}

	r.adapter.ToModel(final, &data)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *GenericResource[TFModel, APIRead]) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	data, diagnostics := ReadTerraformState[TFModel](ctx, request.State.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := ResourceReader[APIRead]{
		ResourceTitle: r.adapter.Title,
		ResourceName:  r.adapter.Name,
		GetFunc: func(ctx context.Context, id string) (*APIRead, ResourceStatus, error) {
			return r.adapter.Get(ctx, r.client, id)
		},
	}

	api, ok := resourceReader.Read(ctx, r.adapter.IDFromModel(data), response)
	if !ok {
		return
	}

	r.adapter.ToModel(api, &data)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *GenericResource[TFModel, APIRead]) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	if r.adapter.Update == nil {
		response.Diagnostics.AddError(
			"Update Not Supported",
			fmt.Sprintf(
				"%s resources are immutable and cannot be updated in-place. All changes require resource replacement.",
				r.adapter.Title,
			),
		)
		return
	}

	data, diagnostics := ReadTerraformState[TFModel](ctx, request.Plan.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := r.adapter.IDFromModel(data)

	operationTagKey, diagnostics := r.adapter.Update(ctx, r.client, id, data)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	stateWatcher := UpdateStateWatcher[APIRead]{
		ResourceTitle: r.adapter.Title,
		ResourceName:  r.adapter.Name,
		GetFunc: func(ctx context.Context) (*APIRead, ResourceStatus, error) {
			return r.adapter.Get(ctx, r.client, id)
		},
	}

	final, ok := stateWatcher.Wait(ctx, operationTagKey, r.adapter.TimeoutsFromModel(data), response)
	if !ok {
		return
	}

	r.adapter.ToModel(final, &data)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *GenericResource[TFModel, APIRead]) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	data, diagnostics := ReadTerraformState[TFModel](ctx, request.State.Get)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := r.adapter.IDFromModel(data)

	if err := r.adapter.Delete(ctx, r.client, id); err != nil {
		if e, ok := AsAPIError(err); !ok || e.StatusCode != http.StatusNotFound {
			TerraformDebugLogAPIResponseBody(ctx, err)
			response.Diagnostics.AddError(
				fmt.Sprintf("Failed to Delete %s", r.adapter.Title),
				fmt.Sprintf("An error occurred while deleting the %s: %s", r.adapter.Name, err),
			)
			return
		}
	}

	stateWatcher := DeleteStateWatcher{
		ResourceTitle: r.adapter.Title,
		ResourceName:  r.adapter.Name,
		GetFunc: func(ctx context.Context) (any, ResourceStatus, error) {
			return r.adapter.Get(ctx, r.client, id)
		},
	}

	stateWatcher.Wait(ctx, r.adapter.TimeoutsFromModel(data), response)
}
