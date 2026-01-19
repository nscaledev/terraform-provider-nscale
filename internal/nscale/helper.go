/*
Copyright 2025 Nscale

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
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
)

func assertState[T any](state any, diagnostics *diag.Diagnostics) (*T, bool) {
	var zero *T

	result, ok := state.(*T)
	if !ok || result == nil {
		diagnostics.AddError(
			"Unexpected Resource Type",
			fmt.Sprintf("Expected %T, got: %T. Please contact the Nscale team for support.", zero, result),
		)
		return zero, false
	}

	return result, true
}

type CreateStateWatcher[T any] struct {
	ResourceTitle string
	ResourceName  string
	GetFunc       func(ctx context.Context) (*T, *coreapi.ProjectScopedResourceReadMetadata, error)
}

func (w *CreateStateWatcher[T]) Wait(ctx context.Context, response *resource.CreateResponse) (*T, bool) {
	stateWatcher := retry.StateChangeConf{
		Timeout: 30 * time.Minute,
		Pending: []string{
			string(coreapi.ResourceProvisioningStatusProvisioning),
			string(coreapi.ResourceProvisioningStatusUnknown),
		},
		Target: []string{
			string(coreapi.ResourceProvisioningStatusProvisioned),
		},
		Refresh: func() (any, string, error) {
			result, metadata, err := w.GetFunc(ctx)
			if err != nil {
				if IsStatusCodeError(err, http.StatusNotFound) {
					// FIXME: Temporary workaround for resources that might not yet be visible in the cache-backed client. Should be revisited once API consistency is guaranteed.
					return nil, string(coreapi.ResourceProvisioningStatusUnknown), nil
				}
				return nil, "", err
			}
			return result, string(metadata.ProvisioningStatus), nil
		},
	}

	var zero *T

	state, err := stateWatcher.WaitForStateContext(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			fmt.Sprintf("Failed to Wait for %s to be Created", w.ResourceTitle),
			fmt.Sprintf("An error occurred while waiting for the %s to be created: %s", w.ResourceName, err),
		)
		return zero, false
	}

	return assertState[T](state, &response.Diagnostics)
}

type ResourceReader[T any] struct {
	ResourceTitle string
	ResourceName  string
	GetFunc       func(ctx context.Context, id string) (*T, *coreapi.ProjectScopedResourceReadMetadata, error)
}

func (r *ResourceReader[T]) Read(ctx context.Context, id string, response *resource.ReadResponse) (*T, bool) {
	var zero *T

	result, _, err := r.GetFunc(ctx, id)
	if err != nil {
		if IsStatusCodeError(err, http.StatusNotFound) {
			response.Diagnostics.AddWarning(
				fmt.Sprintf("%s Not Found", r.ResourceTitle),
				fmt.Sprintf("The %s with ID %s was not found on the server and will be removed from the state file.", r.ResourceName, id),
			)
			response.State.RemoveResource(ctx)
			return zero, false
		}

		response.Diagnostics.AddError(
			fmt.Sprintf("Failed to Read %s", r.ResourceTitle),
			fmt.Sprintf("An error occurred while retrieving the %s: %s", r.ResourceName, err),
		)
		return zero, false
	}

	return result, true
}

func WriteOperationTag(metadata *coreapi.ResourceWriteMetadata) string {
	operationKey := fmt.Sprintf("terraform.nscale.com/%s", uuid.New().String())

	if metadata.Tags == nil {
		var tags []coreapi.Tag
		metadata.Tags = &tags
	}

	*metadata.Tags = append(*metadata.Tags, coreapi.Tag{
		Name:  operationKey,
		Value: "0",
	})

	return operationKey
}

func HasOperationTag(tags *[]coreapi.Tag, operationTag string) bool {
	if tags == nil {
		return false
	}

	for _, tag := range *tags {
		if tag.Name == operationTag {
			return true
		}
	}

	return false
}

const (
	UpdateStateUpdating = "updating"
	UpdateStateErrored  = "errored"
	UpdateStateUpdated  = "updated"
)

type UpdateStateWatcher[T any] struct {
	ResourceTitle string
	ResourceName  string
	GetFunc       func(ctx context.Context) (*T, *coreapi.ProjectScopedResourceReadMetadata, error)
}

func (w *UpdateStateWatcher[T]) Wait(ctx context.Context, operationTagKey string, response *resource.UpdateResponse) (*T, bool) {
	stateWatcher := retry.StateChangeConf{
		Timeout: 30 * time.Minute,
		Pending: []string{UpdateStateUpdating},
		Target:  []string{UpdateStateUpdated},
		Refresh: func() (any, string, error) {
			result, metadata, err := w.GetFunc(ctx)
			if err != nil {
				return nil, UpdateStateErrored, err
			}

			if HasOperationTag(metadata.Tags, operationTagKey) {
				return result, UpdateStateUpdated, nil
			}

			return result, UpdateStateUpdating, nil
		},
	}

	var zero *T

	state, err := stateWatcher.WaitForStateContext(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			fmt.Sprintf("Failed to Wait for %s to be Updated", w.ResourceTitle),
			fmt.Sprintf("An error occurred while waiting for the %s to be updated: %s", w.ResourceName, err),
		)
		return zero, false
	}

	return assertState[T](state, &response.Diagnostics)
}

const (
	DeleteStateDeleting = "deleting"
	DeleteStateErrored  = "errored"
	DeleteStateDeleted  = "deleted"
)

type DeleteStateWatcher struct {
	ResourceTitle string
	ResourceName  string
	GetFunc       func(ctx context.Context) (any, *coreapi.ProjectScopedResourceReadMetadata, error)
}

func (w *DeleteStateWatcher) Wait(ctx context.Context, response *resource.DeleteResponse) bool {
	stateWatcher := retry.StateChangeConf{
		Timeout: 30 * time.Minute,
		Pending: []string{DeleteStateDeleting},
		Target:  []string{DeleteStateDeleted},
		Refresh: func() (any, string, error) {
			_, _, err := w.GetFunc(ctx)
			if err == nil {
				return struct{}{}, DeleteStateDeleting, nil
			}

			if IsStatusCodeError(err, http.StatusNotFound) {
				return struct{}{}, DeleteStateDeleted, nil
			}

			return nil, DeleteStateErrored, err
		},
	}

	if _, err := stateWatcher.WaitForStateContext(ctx); err != nil {
		response.Diagnostics.AddError(
			fmt.Sprintf("Failed to Wait for %s to be Deleted", w.ResourceTitle),
			fmt.Sprintf("An error occurred while waiting for the %s to be deleted: %s", w.ResourceName, err),
		)
		return false
	}

	return true
}
