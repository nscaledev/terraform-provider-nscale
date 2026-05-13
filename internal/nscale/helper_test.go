package nscale

import (
	"context"
	"strings"
	"testing"
	"time"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
)

type waitTestResource struct {
	name string
}

// TestCreateStateWatcherWaitHandlesTransientProvisioningStates ensures create waits continue polling through non-terminal provisioning states.
func TestCreateStateWatcherWaitHandlesTransientProvisioningStates(t *testing.T) {
	testCases := []struct {
		name          string
		initialStatus coreapi.ResourceProvisioningStatus
	}{
		{
			name:          "pending",
			initialStatus: coreapi.ResourceProvisioningStatusPending,
		},
		{
			name:          "unknown",
			initialStatus: coreapi.ResourceProvisioningStatusUnknown,
		},
		{
			name:          "provisioning",
			initialStatus: coreapi.ResourceProvisioningStatusProvisioning,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var calls int

			finalResult := &waitTestResource{name: "ready"}

			watcher := CreateStateWatcher[waitTestResource]{
				ResourceTitle: "Test Resource",
				ResourceName:  "test resource",
				GetFunc: func(ctx context.Context) (*waitTestResource, *coreapi.ProjectScopedResourceReadMetadata, error) {
					calls++

					if calls == 1 {
						return &waitTestResource{name: "creating"}, &coreapi.ProjectScopedResourceReadMetadata{
							ProvisioningStatus: testCase.initialStatus,
						}, nil
					}

					return finalResult, &coreapi.ProjectScopedResourceReadMetadata{
						ProvisioningStatus: coreapi.ResourceProvisioningStatusProvisioned,
					}, nil
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			var response resource.CreateResponse

			var timeouts tftimeouts.Value
			got, ok := watcher.Wait(ctx, timeouts, &response)
			if !ok {
				t.Fatalf("Wait() returned ok=false with diagnostics: %#v", response.Diagnostics)
			}

			if got != finalResult {
				t.Fatalf("Wait() returned %p, want %p", got, finalResult)
			}

			if calls != 2 {
				t.Fatalf("GetFunc call count = %d, want 2", calls)
			}

			if response.Diagnostics.HasError() {
				t.Fatalf("Wait() returned unexpected error diagnostics: %#v", response.Diagnostics)
			}

			if len(response.Diagnostics) != 0 {
				t.Fatalf("Wait() returned unexpected diagnostics: %#v", response.Diagnostics)
			}
		})
	}
}

// TestCreateStateWatcherWaitTreatsErrorAsTerminal ensures the create waiter exits cleanly with a
// diagnostic when the API reports provisioningStatus=error, instead of producing
// `unexpected state 'error', wanted target 'provisioned'. last error: %!s(<nil>)`.
func TestCreateStateWatcherWaitTreatsErrorAsTerminal(t *testing.T) {
	const resourceID = "f51ac0e0-d2e4-4648-99cf-c18a19c4934a"

	var calls int

	watcher := CreateStateWatcher[waitTestResource]{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context) (*waitTestResource, *coreapi.ProjectScopedResourceReadMetadata, error) {
			calls++

			if calls == 1 {
				return &waitTestResource{name: "creating"}, &coreapi.ProjectScopedResourceReadMetadata{
					Id:                 resourceID,
					ProvisioningStatus: coreapi.ResourceProvisioningStatusProvisioning,
				}, nil
			}

			return &waitTestResource{name: "failed"}, &coreapi.ProjectScopedResourceReadMetadata{
				Id:                 resourceID,
				ProvisioningStatus: coreapi.ResourceProvisioningStatusError,
			}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var response resource.CreateResponse
	var timeouts tftimeouts.Value

	_, ok := watcher.Wait(ctx, timeouts, &response)
	if ok {
		t.Fatalf("Wait() returned ok=true, want ok=false on error state")
	}

	if !response.Diagnostics.HasError() {
		t.Fatalf("Wait() did not produce error diagnostics: %#v", response.Diagnostics)
	}

	var found bool
	for _, d := range response.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), resourceID) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Wait() diagnostics did not include resource ID %q: %#v", resourceID, response.Diagnostics)
	}
}
