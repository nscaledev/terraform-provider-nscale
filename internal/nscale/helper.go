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
	"strings"
	"time"

	"github.com/google/uuid"
	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
)

const (
	TerraformOperationTagPrefix = "terraform.nscale.com/"
	defaultStateWatcherTimeout  = 30 * time.Minute
)

type StateReaderFunc func(ctx context.Context, target any) diag.Diagnostics

func ReadTerraformState[T any](ctx context.Context, fn StateReaderFunc, mutates ...func(*T)) (T, diag.Diagnostics) {
	var data T

	if diagnostics := fn(ctx, &data); diagnostics.HasError() {
		return data, diagnostics
	}

	for _, mutate := range mutates {
		mutate(&data)
	}

	return data, nil
}

// ParseID parses a typed SDK ID and appends a consistent Terraform diagnostic on failure.
func ParseID[T any](raw, label string, parse func(string) (T, error), diagnostics *diag.Diagnostics) (T, bool) {
	id, err := parse(raw)
	if err != nil {
		diagnostics.AddError(
			fmt.Sprintf("Invalid %s ID", label),
			fmt.Sprintf("Could not parse %s ID %q: %s", strings.ToLower(label), raw, err),
		)
		return id, false
	}

	return id, true
}

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

func addProvisioningErrorDiagnostic(
	diagnostics *diag.Diagnostics,
	resourceTitle string,
	status ResourceStatus,
	found bool,
	detail string,
) bool {
	if !found || status.ProvisioningStatus != coreapi.ResourceProvisioningStatusError {
		return false
	}

	diagnostics.AddError(
		fmt.Sprintf("%s Entered Error State", resourceTitle),
		fmt.Sprintf("%s %s (name %s) %s", resourceTitle, status.ID, status.Name, detail),
	)

	return true
}

type CreateStateWatcher[T any] struct {
	ResourceTitle string
	ResourceName  string
	GetFunc       func(ctx context.Context) (*T, ResourceStatus, error)
}

func (w *CreateStateWatcher[T]) Wait(
	ctx context.Context,
	timeouts tftimeouts.Value,
	response *resource.CreateResponse,
) (*T, bool) {
	timeout, diagnostics := timeouts.Create(ctx, defaultStateWatcherTimeout)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return nil, false
	}

	var lastStatus ResourceStatus
	var haveStatus bool

	stateWatcher := retry.StateChangeConf{
		Timeout: timeout,
		Pending: []string{
			string(coreapi.ResourceProvisioningStatusProvisioning),
			string(coreapi.ResourceProvisioningStatusPending),
			string(coreapi.ResourceProvisioningStatusUnknown),
		},
		Target: []string{
			string(coreapi.ResourceProvisioningStatusProvisioned),
			string(coreapi.ResourceProvisioningStatusError),
		},
		Refresh: func() (any, string, error) {
			result, status, err := w.GetFunc(ctx)
			if err != nil {
				if e, ok := AsAPIError(err); ok && e.StatusCode == http.StatusNotFound {
					// FIXME: Temporary workaround for resources that might not yet be visible in the cache-backed client. Should be revisited once API consistency is guaranteed.
					return nil, string(coreapi.ResourceProvisioningStatusUnknown), nil
				}
				return nil, "", err
			}
			lastStatus = status
			haveStatus = true
			return result, string(status.ProvisioningStatus), nil
		},
	}

	var zero *T

	state, err := stateWatcher.WaitForStateContext(ctx)
	if err != nil {
		TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			fmt.Sprintf("Failed to Wait for %s to be Created", w.ResourceTitle),
			fmt.Sprintf("An error occurred while waiting for the %s to be created: %s", w.ResourceName, err),
		)
		return zero, false
	}

	result, ok := assertState[T](state, &response.Diagnostics)
	if !ok {
		return zero, false
	}

	if addProvisioningErrorDiagnostic(
		&response.Diagnostics,
		w.ResourceTitle,
		lastStatus,
		haveStatus,
		"was created but transitioned to 'error' instead of 'provisioned'. Run 'terraform apply' to try again, or reach out to support.",
	) {
		return result, false
	}

	return result, true
}

type ResourceReader[T any] struct {
	ResourceTitle string
	ResourceName  string
	GetFunc       func(ctx context.Context, id string) (*T, ResourceStatus, error)
}

func (r *ResourceReader[T]) Read(ctx context.Context, id string, response *resource.ReadResponse) (*T, bool) {
	var zero *T

	result, _, err := r.GetFunc(ctx, id)
	if err != nil {
		if e, ok := AsAPIError(err); ok && e.StatusCode == http.StatusNotFound {
			response.Diagnostics.AddWarning(
				fmt.Sprintf("%s Not Found", r.ResourceTitle),
				fmt.Sprintf(
					"The %s with ID %s was not found on the server and will be removed from the state file.",
					r.ResourceName,
					id,
				),
			)
			response.State.RemoveResource(ctx)
			return zero, false
		}

		TerraformDebugLogAPIResponseBody(ctx, err)

		response.Diagnostics.AddError(
			fmt.Sprintf("Failed to Read %s", r.ResourceTitle),
			fmt.Sprintf("An error occurred while retrieving the %s: %s", r.ResourceName, err),
		)

		return zero, false
	}

	return result, true
}

func WriteOperationTag(metadata *coreapi.ResourceWriteMetadata) string {
	operationKey := TerraformOperationTagPrefix + uuid.NewString()

	if metadata.Tags == nil {
		var tags []coreapi.Tag
		metadata.Tags = &tags
	}

	*metadata.Tags = append(*metadata.Tags, coreapi.Tag{
		Name:  operationKey,
		Value: time.Now().Format(time.RFC3339),
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

func RemoveOperationTags(tags *[]coreapi.Tag) *[]coreapi.Tag {
	if tags == nil {
		return nil
	}

	// Operation tags are internal bookkeeping written by the update watcher to
	// confirm a write propagated. They must never surface in Terraform state (the
	// schema forbids users from setting reserved-prefix tags), otherwise an update
	// that wrote one produces an "inconsistent result after apply" on the tags
	// attribute. Strip every operation tag regardless of age.
	var filtered []coreapi.Tag
	for _, tag := range *tags {
		if strings.HasPrefix(tag.Name, TerraformOperationTagPrefix) {
			continue
		}
		filtered = append(filtered, tag)
	}

	return &filtered
}

const (
	UpdateStateUpdating          = "updating"
	UpdateStateErrored           = "errored"
	UpdateStateUpdated           = "updated"
	UpdateStateProvisioningError = "provisioning_error"
)

type UpdateStateWatcher[T any] struct {
	ResourceTitle string
	ResourceName  string
	GetFunc       func(ctx context.Context) (*T, ResourceStatus, error)
}

func (w *UpdateStateWatcher[T]) Wait(
	ctx context.Context,
	operationTagKey string,
	timeouts tftimeouts.Value,
	response *resource.UpdateResponse,
) (*T, bool) {
	timeout, diagnostics := timeouts.Update(ctx, defaultStateWatcherTimeout)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return nil, false
	}

	var lastStatus ResourceStatus
	var haveStatus bool

	stateWatcher := retry.StateChangeConf{
		Timeout: timeout,
		Pending: []string{UpdateStateUpdating},
		Target:  []string{UpdateStateUpdated, UpdateStateProvisioningError},
		Refresh: func() (any, string, error) {
			result, status, err := w.GetFunc(ctx)
			if err != nil {
				return nil, UpdateStateErrored, err
			}

			lastStatus = status
			haveStatus = true

			if status.ProvisioningStatus == coreapi.ResourceProvisioningStatusError {
				return result, UpdateStateProvisioningError, nil
			}

			if HasOperationTag(status.Tags, operationTagKey) {
				return result, UpdateStateUpdated, nil
			}

			return result, UpdateStateUpdating, nil
		},
	}

	var zero *T

	state, err := stateWatcher.WaitForStateContext(ctx)
	if err != nil {
		TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			fmt.Sprintf("Failed to Wait for %s to be Updated", w.ResourceTitle),
			fmt.Sprintf("An error occurred while waiting for the %s to be updated: %s", w.ResourceName, err),
		)
		return zero, false
	}

	result, ok := assertState[T](state, &response.Diagnostics)
	if !ok {
		return zero, false
	}

	if addProvisioningErrorDiagnostic(&response.Diagnostics, w.ResourceTitle, lastStatus, haveStatus,
		"transitioned to 'error' during update. Run 'terraform apply' to try again, or reach out to support.") {
		return result, false
	}

	return result, true
}

const (
	DeleteStateDeleting          = "deleting"
	DeleteStateErrored           = "errored"
	DeleteStateDeleted           = "deleted"
	DeleteStateProvisioningError = "provisioning_error"
)

type DeleteStateWatcher struct {
	ResourceTitle string
	ResourceName  string
	GetFunc       func(ctx context.Context) (any, ResourceStatus, error)
}

func (w *DeleteStateWatcher) Wait(
	ctx context.Context,
	timeouts tftimeouts.Value,
	response *resource.DeleteResponse,
) bool {
	timeout, diagnostics := timeouts.Delete(ctx, defaultStateWatcherTimeout)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return false
	}

	var lastStatus ResourceStatus
	var haveStatus bool

	stateWatcher := retry.StateChangeConf{
		Timeout: timeout,
		Pending: []string{DeleteStateDeleting},
		Target:  []string{DeleteStateDeleted, DeleteStateProvisioningError},
		Refresh: func() (any, string, error) {
			_, status, err := w.GetFunc(ctx)
			if err == nil {
				lastStatus = status
				haveStatus = true
				if status.ProvisioningStatus == coreapi.ResourceProvisioningStatusError {
					return struct{}{}, DeleteStateProvisioningError, nil
				}
				return struct{}{}, DeleteStateDeleting, nil
			}

			if e, ok := AsAPIError(err); ok && e.StatusCode == http.StatusNotFound {
				return struct{}{}, DeleteStateDeleted, nil
			}

			return nil, DeleteStateErrored, err
		},
	}

	if _, err := stateWatcher.WaitForStateContext(ctx); err != nil {
		TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			fmt.Sprintf("Failed to Wait for %s to be Deleted", w.ResourceTitle),
			fmt.Sprintf("An error occurred while waiting for the %s to be deleted: %s", w.ResourceName, err),
		)
		return false
	}

	if addProvisioningErrorDiagnostic(
		&response.Diagnostics,
		w.ResourceTitle,
		lastStatus,
		haveStatus,
		"transitioned to 'error' during deprovisioning instead of being removed. Re-run 'terraform destroy' to try again, or reach out to support.",
	) {
		return false
	}

	return true
}
