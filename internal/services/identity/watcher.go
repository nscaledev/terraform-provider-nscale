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

package identity

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// defaultStateTimeout bounds the create/delete state watchers. Identity
// resources provision and deprovision in seconds in practice, so this is a
// generous ceiling rather than an expected wait.
const defaultStateTimeout = 10 * time.Minute

// stateDeprovisioning and stateDeleted are synthetic states for the delete
// watcher; the identity API has no "deleted" status, it just 404s.
const (
	stateDeprovisioning = "deprovisioning"
	stateDeleted        = "deleted"
)

// getFunc reads an identity resource by ID.
type getFunc[T any] func(ctx context.Context, id string) (*T, error)

// statusFunc extracts the provisioning status from an identity resource.
type statusFunc[T any] func(resource *T) coreapi.ResourceProvisioningStatus

// waitForProvisioned polls an identity resource until it reaches a terminal
// provisioning state. Identity resources expose a provisioning status and may
// provision asynchronously (projects return "pending" on create); groups are
// effectively synchronous but follow the same path for consistency.
func waitForProvisioned[T any](
	ctx context.Context,
	id string,
	timeout time.Duration,
	get getFunc[T],
	status statusFunc[T],
) (*T, error) {
	watcher := retry.StateChangeConf{
		Timeout: timeout,
		Pending: []string{
			string(coreapi.ResourceProvisioningStatusPending),
			string(coreapi.ResourceProvisioningStatusProvisioning),
			string(coreapi.ResourceProvisioningStatusUnknown),
		},
		Target: []string{
			string(coreapi.ResourceProvisioningStatusProvisioned),
			string(coreapi.ResourceProvisioningStatusError),
		},
		Refresh: func() (any, string, error) {
			resource, err := get(ctx, id)
			if err != nil {
				if e, ok := nscale.AsAPIError(err); ok && e.StatusCode == http.StatusNotFound {
					// The resource may not be visible yet in a cache-backed read.
					return nil, string(coreapi.ResourceProvisioningStatusUnknown), nil
				}
				return nil, "", fmt.Errorf("reading resource %s: %w", id, err)
			}
			return resource, string(status(resource)), nil
		},
	}

	result, err := watcher.WaitForStateContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("waiting for resource %s to provision: %w", id, err)
	}

	resource, ok := result.(*T)
	if !ok || resource == nil {
		return nil, fmt.Errorf("resource %s returned no data after provisioning", id)
	}

	if status(resource) == coreapi.ResourceProvisioningStatusError {
		return resource, fmt.Errorf("resource %s transitioned to 'error' instead of 'provisioned'", id)
	}

	return resource, nil
}

// waitForDeleted polls until the API reports the resource gone (404). Identity
// resources may deprovision asynchronously (projects return 202 on delete and
// linger in "deprovisioning").
func waitForDeleted[T any](
	ctx context.Context,
	id string,
	timeout time.Duration,
	get getFunc[T],
) error {
	watcher := retry.StateChangeConf{
		Timeout: timeout,
		Pending: []string{stateDeprovisioning},
		Target:  []string{stateDeleted},
		Refresh: func() (any, string, error) {
			_, err := get(ctx, id)
			if err != nil {
				if e, ok := nscale.AsAPIError(err); ok && e.StatusCode == http.StatusNotFound {
					return struct{}{}, stateDeleted, nil
				}
				return nil, "", fmt.Errorf("reading resource %s: %w", id, err)
			}
			return struct{}{}, stateDeprovisioning, nil
		},
	}

	if _, err := watcher.WaitForStateContext(ctx); err != nil {
		return fmt.Errorf("waiting for resource %s to delete: %w", id, err)
	}

	return nil
}
