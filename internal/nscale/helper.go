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

	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tags"
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

// ParseID parses a resource identifier and appends a consistent Terraform
// diagnostic on failure. label is the resource's human name in title case
// ("Network", "File Storage"), so the diagnostic reads "Invalid Network ID".
//
// The canonical specs type every identifier as a UUID, so this is the provider's
// single conversion point from the strings Terraform carries: a malformed
// identifier is reported against the resource that owns it rather than sent to
// the API as an opaque string.
func ParseID(raw, label string, diagnostics *diag.Diagnostics) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		diagnostics.AddError(
			fmt.Sprintf("Invalid %s ID", label),
			fmt.Sprintf("Could not parse %s ID %q: %s", strings.ToLower(label), raw, err),
		)
		return uuid.UUID{}, false
	}

	return id, true
}

// ParseOptionalID is ParseID for an optional identifier: an unset value yields a
// nil pointer with no diagnostic, so the field is simply omitted from the
// request. A null or unknown attribute reaches here as the empty string.
func ParseOptionalID(raw, label string, diagnostics *diag.Diagnostics) (*uuid.UUID, bool) {
	if raw == "" {
		return nil, true
	}

	id, ok := ParseID(raw, label, diagnostics)
	if !ok {
		return nil, false
	}

	return &id, true
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
	if !found || status.ProvisioningStatus != ProvisioningStatusError {
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
			string(ProvisioningStatusProvisioning),
			string(ProvisioningStatusPending),
			string(ProvisioningStatusUnknown),
		},
		Target: []string{
			string(ProvisioningStatusProvisioned),
			string(ProvisioningStatusError),
		},
		Refresh: func() (any, string, error) {
			result, status, err := w.GetFunc(ctx)
			if err != nil {
				if e, ok := AsAPIError(err); ok && e.StatusCode == http.StatusNotFound {
					// FIXME: Temporary workaround for resources that might not yet be visible in the cache-backed client. Should be revisited once API consistency is guaranteed.
					return nil, string(ProvisioningStatusUnknown), nil
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

// AppendOperationTag returns tagList with a freshly minted operation tag
// appended, along with that tag's key for the update watcher to poll for.
//
// It takes and returns the tags alone rather than the enclosing write metadata:
// every service declares its own metadata struct, and a generic function cannot
// reach a field on one. Callers assign the result back:
//
//	params.Metadata.Tags, operationTagKey = nscale.AppendOperationTag(params.Metadata.Tags)
func AppendOperationTag[T tags.SDKTag](tagList *[]T) (*[]T, string) {
	operationKey := TerraformOperationTagPrefix + uuid.NewString()

	appended := TagList{}
	if existing := tags.FromAPI(tagList); existing != nil {
		appended = append(appended, *existing...)
	}

	appended = append(appended, Tag{
		Name:  operationKey,
		Value: time.Now().Format(time.RFC3339),
	})

	return TagsToAPI[T](&appended), operationKey
}

func HasOperationTag(tagList *TagList, operationTag string) bool {
	if tagList == nil {
		return false
	}

	for _, tag := range *tagList {
		if tag.Name == operationTag {
			return true
		}
	}

	return false
}

func RemoveOperationTags[T tags.SDKTag](tagList *[]T) *TagList {
	converted := tags.FromAPI(tagList)
	if converted == nil {
		return nil
	}

	// Operation tags are internal bookkeeping written by the update watcher to
	// confirm a write propagated. They must never surface in Terraform state (the
	// schema forbids users from setting reserved-prefix tags), otherwise an update
	// that wrote one produces an "inconsistent result after apply" on the tags
	// attribute. Strip every operation tag regardless of age.
	var filtered TagList
	for _, tag := range *converted {
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

			if status.ProvisioningStatus == ProvisioningStatusError {
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
				if status.ProvisioningStatus == ProvisioningStatusError {
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
