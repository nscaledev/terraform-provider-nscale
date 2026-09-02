package nscale

import (
	"context"
	"strings"
	"testing"
	"time"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type waitTestResource struct {
	name string
}

func TestParseIDReturnsParsedID(t *testing.T) {
	const raw = "f51ac0e0-d2e4-4648-99cf-c18a19c4934a"

	var diagnostics diag.Diagnostics

	got, ok := ParseID(raw, "Project", &diagnostics)
	if !ok {
		t.Fatalf("ParseID() returned ok=false with diagnostics: %#v", diagnostics)
	}

	if got.String() != raw {
		t.Fatalf("ParseID() returned %q, want %q", got, raw)
	}

	if len(diagnostics) != 0 {
		t.Fatalf("ParseID() returned unexpected diagnostics: %#v", diagnostics)
	}
}

func TestParseIDAddsDiagnostic(t *testing.T) {
	var diagnostics diag.Diagnostics

	_, ok := ParseID("not-id", "File Storage", &diagnostics)
	if ok {
		t.Fatal("ParseID() returned ok=true, want ok=false")
	}

	errs := diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("ParseID() returned %d error diagnostics, want 1: %#v", len(errs), diagnostics)
	}

	if errs[0].Summary() != "Invalid File Storage ID" {
		t.Fatalf("ParseID() summary = %q, want %q", errs[0].Summary(), "Invalid File Storage ID")
	}

	// The label is lowercased in the detail so it reads as prose, and the raw
	// value is quoted back so the operator can spot which identifier is wrong.
	const wantPrefix = `Could not parse file storage ID "not-id": `
	if !strings.HasPrefix(errs[0].Detail(), wantPrefix) {
		t.Fatalf("ParseID() detail = %q, want prefix %q", errs[0].Detail(), wantPrefix)
	}
}

// TestParseOptionalIDSkipsUnsetValue covers the case an optional attribute
// relies on: a null or unknown attribute reaches ParseOptionalID as the empty
// string and must be omitted from the request rather than reported as malformed.
func TestParseOptionalIDSkipsUnsetValue(t *testing.T) {
	var diagnostics diag.Diagnostics

	got, ok := ParseOptionalID("", "SSH Certificate Authority", &diagnostics)
	if !ok {
		t.Fatalf("ParseOptionalID() returned ok=false with diagnostics: %#v", diagnostics)
	}

	if got != nil {
		t.Fatalf("ParseOptionalID() returned %v, want nil", got)
	}

	if len(diagnostics) != 0 {
		t.Fatalf("ParseOptionalID() returned unexpected diagnostics: %#v", diagnostics)
	}
}

func TestParseOptionalIDAddsDiagnostic(t *testing.T) {
	var diagnostics diag.Diagnostics

	got, ok := ParseOptionalID("not-id", "SSH Certificate Authority", &diagnostics)
	if ok {
		t.Fatal("ParseOptionalID() returned ok=true, want ok=false")
	}

	if got != nil {
		t.Fatalf("ParseOptionalID() returned %v, want nil", got)
	}

	errs := diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("ParseOptionalID() returned %d error diagnostics, want 1: %#v", len(errs), diagnostics)
	}

	if errs[0].Summary() != "Invalid SSH Certificate Authority ID" {
		t.Fatalf("ParseOptionalID() summary = %q, want %q", errs[0].Summary(), "Invalid SSH Certificate Authority ID")
	}
}

// TestCreateStateWatcherWaitHandlesTransientProvisioningStates ensures create waits continue polling through non-terminal provisioning states.
func TestCreateStateWatcherWaitHandlesTransientProvisioningStates(t *testing.T) {
	testCases := []struct {
		name          string
		initialStatus ProvisioningStatus
	}{
		{
			name:          "pending",
			initialStatus: ProvisioningStatusPending,
		},
		{
			name:          "unknown",
			initialStatus: ProvisioningStatusUnknown,
		},
		{
			name:          "provisioning",
			initialStatus: ProvisioningStatusProvisioning,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var calls int

			finalResult := &waitTestResource{name: "ready"}

			watcher := CreateStateWatcher[waitTestResource]{
				ResourceTitle: "Test Resource",
				ResourceName:  "test resource",
				GetFunc: func(ctx context.Context) (*waitTestResource, ResourceStatus, error) {
					calls++

					if calls == 1 {
						return &waitTestResource{
								name: "creating",
							}, ResourceStatus{
								ProvisioningStatus: testCase.initialStatus,
							}, nil
					}

					return finalResult, ResourceStatus{
						ProvisioningStatus: ProvisioningStatusProvisioned,
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
	const (
		resourceID   = "f51ac0e0-d2e4-4648-99cf-c18a19c4934a"
		wantSummary  = "Instance Entered Error State"
		oldBugMarker = "%!s(<nil>)"
	)

	var calls int

	watcher := CreateStateWatcher[waitTestResource]{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context) (*waitTestResource, ResourceStatus, error) {
			calls++

			if calls == 1 {
				return &waitTestResource{
						name: "creating",
					}, ResourceStatus{
						ID:                 resourceID,
						ProvisioningStatus: ProvisioningStatusProvisioning,
					}, nil
			}

			return &waitTestResource{
					name: "failed",
				}, ResourceStatus{
					ID:                 resourceID,
					ProvisioningStatus: ProvisioningStatusError,
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

	if calls < 2 {
		t.Fatalf(
			"GetFunc call count = %d, want >= 2 (watcher must poll through provisioning before recognizing error)",
			calls,
		)
	}

	if !response.Diagnostics.HasError() {
		t.Fatalf("Wait() did not produce error diagnostics: %#v", response.Diagnostics)
	}

	var found bool
	for _, d := range response.Diagnostics.Errors() {
		if d.Summary() != wantSummary {
			continue
		}
		if !strings.Contains(d.Detail(), resourceID) {
			t.Fatalf("Wait() diagnostic detail did not include resource ID %q: %s", resourceID, d.Detail())
		}
		if strings.Contains(d.Detail(), oldBugMarker) {
			t.Fatalf("Wait() diagnostic detail contains pre-fix bug marker %q: %s", oldBugMarker, d.Detail())
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("Wait() did not produce a diagnostic with summary %q: %#v", wantSummary, response.Diagnostics)
	}
}

// TestUpdateStateWatcherWaitTreatsErrorAsTerminal ensures the update waiter exits cleanly with a
// diagnostic when the API reports provisioningStatus=error during an update.
func TestUpdateStateWatcherWaitTreatsErrorAsTerminal(t *testing.T) {
	const (
		resourceID      = "fe563485-0631-4707-bec7-0d661cf20efc"
		operationTagKey = TerraformOperationTagPrefix + "test-op"
		wantSummary     = "Instance Entered Error State"
		oldBugMarker    = "%!s(<nil>)"
	)

	watcher := UpdateStateWatcher[waitTestResource]{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context) (*waitTestResource, ResourceStatus, error) {
			return &waitTestResource{
					name: "failed",
				}, ResourceStatus{
					ID:                 resourceID,
					ProvisioningStatus: ProvisioningStatusError,
				}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var response resource.UpdateResponse
	var timeouts tftimeouts.Value

	_, ok := watcher.Wait(ctx, operationTagKey, timeouts, &response)
	if ok {
		t.Fatalf("Wait() returned ok=true, want ok=false on error state")
	}

	if !response.Diagnostics.HasError() {
		t.Fatalf("Wait() did not produce error diagnostics: %#v", response.Diagnostics)
	}

	var found bool
	for _, d := range response.Diagnostics.Errors() {
		if d.Summary() != wantSummary {
			continue
		}
		if !strings.Contains(d.Detail(), resourceID) {
			t.Fatalf("Wait() diagnostic detail did not include resource ID %q: %s", resourceID, d.Detail())
		}
		if strings.Contains(d.Detail(), oldBugMarker) {
			t.Fatalf("Wait() diagnostic detail contains pre-fix bug marker %q: %s", oldBugMarker, d.Detail())
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("Wait() did not produce a diagnostic with summary %q: %#v", wantSummary, response.Diagnostics)
	}
}

// TestDeleteStateWatcherWaitTreatsErrorAsTerminal ensures the delete waiter exits cleanly with a
// diagnostic when the API reports provisioningStatus=error instead of 404'ing.
func TestDeleteStateWatcherWaitTreatsErrorAsTerminal(t *testing.T) {
	const (
		resourceID   = "c2b8d351-c7b1-4fd5-a2c3-0f897a1df29c"
		wantSummary  = "Instance Entered Error State"
		oldBugMarker = "%!s(<nil>)"
	)

	watcher := DeleteStateWatcher{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context) (any, ResourceStatus, error) {
			return struct{}{}, ResourceStatus{
				ID:                 resourceID,
				ProvisioningStatus: ProvisioningStatusError,
			}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var response resource.DeleteResponse
	var timeouts tftimeouts.Value

	ok := watcher.Wait(ctx, timeouts, &response)
	if ok {
		t.Fatalf("Wait() returned ok=true, want ok=false on error state")
	}

	if !response.Diagnostics.HasError() {
		t.Fatalf("Wait() did not produce error diagnostics: %#v", response.Diagnostics)
	}

	var found bool
	for _, d := range response.Diagnostics.Errors() {
		if d.Summary() != wantSummary {
			continue
		}
		if !strings.Contains(d.Detail(), resourceID) {
			t.Fatalf("Wait() diagnostic detail did not include resource ID %q: %s", resourceID, d.Detail())
		}
		if strings.Contains(d.Detail(), oldBugMarker) {
			t.Fatalf("Wait() diagnostic detail contains pre-fix bug marker %q: %s", oldBugMarker, d.Detail())
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("Wait() did not produce a diagnostic with summary %q: %#v", wantSummary, response.Diagnostics)
	}
}
