# DX-1025: Waiter error-state handling — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `CreateStateWatcher`, `UpdateStateWatcher`, and `DeleteStateWatcher` in `internal/nscale/helper.go` recognise `provisioningStatus: error` as a terminal state and surface a clear diagnostic instead of `unexpected state 'error', wanted target 'provisioned'. last error: %!s(<nil>)`.

**Architecture:** Each watcher's `retry.StateChangeConf.Refresh` closure captures the last-seen metadata via a closure-bound pointer. `error` is added to the watcher's `Target` set (or to a new synthetic terminal state for update/delete) so `WaitForStateContext` returns cleanly. After the wait, the watcher inspects the captured metadata and, if `ProvisioningStatus == error`, appends an `AddError` diagnostic with the resource title, name, ID, and a stage-specific action hint, then returns `ok=false` to the caller. Resource IDs are already saved in state before the wait, so Terraform Core marks the resource tainted automatically — the next `apply` does destroy-then-create.

**Tech Stack:** Go, `github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry`, `github.com/hashicorp/terraform-plugin-framework`, `github.com/unikorn-cloud/core/pkg/openapi`.

**Spec:** `docs/superpowers/specs/2026-05-12-dx-1025-waiter-error-state-design.md`

---

## File Structure

- **Modify:** `internal/nscale/helper.go` — three functions (`CreateStateWatcher.Wait`, `UpdateStateWatcher.Wait`, `DeleteStateWatcher.Wait`) plus two new constants in the update/delete state-name blocks.
- **Modify:** `internal/nscale/helper_test.go` — three new test functions covering the error-state path for each watcher.
- **No new files.**

The change is intentionally contained to one helper file. No resource files (`internal/services/**`) are touched; the fix flows through the shared watcher.

---

## Task 1: `CreateStateWatcher` — error state surfaces a diagnostic

**Files:**
- Modify: `internal/nscale/helper_test.go`
- Modify: `internal/nscale/helper.go:70-119`

- [ ] **Step 1: Write the failing test**

Append to `internal/nscale/helper_test.go`:

```go
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
```

Add `"strings"` to the import block at the top of `helper_test.go` (currently has `"context"`, `"testing"`, `"time"`, plus the three external imports).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestCreateStateWatcherWaitTreatsErrorAsTerminal -v ./internal/nscale/...`

Expected: FAIL — the wait returns an error diagnostic with text `unexpected state 'error', wanted target 'provisioned'. last error: %!s(<nil>)`, but the diagnostic detail does not contain the resource ID.

- [ ] **Step 3: Implement the fix in `CreateStateWatcher.Wait`**

Replace the function body of `CreateStateWatcher.Wait` (currently `internal/nscale/helper.go:76-119`) with:

```go
func (w *CreateStateWatcher[T]) Wait(ctx context.Context, timeouts tftimeouts.Value, response *resource.CreateResponse) (*T, bool) {
	timeout, diagnostics := timeouts.Create(ctx, 30*time.Minute)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return nil, false
	}

	var lastMetadata *coreapi.ProjectScopedResourceReadMetadata

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
			result, metadata, err := w.GetFunc(ctx)
			if err != nil {
				if e, ok := AsAPIError(err); ok && e.StatusCode == http.StatusNotFound {
					// FIXME: Temporary workaround for resources that might not yet be visible in the cache-backed client. Should be revisited once API consistency is guaranteed.
					return nil, string(coreapi.ResourceProvisioningStatusUnknown), nil
				}
				return nil, "", err
			}
			lastMetadata = metadata
			return result, string(metadata.ProvisioningStatus), nil
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

	if lastMetadata != nil && lastMetadata.ProvisioningStatus == coreapi.ResourceProvisioningStatusError {
		response.Diagnostics.AddError(
			fmt.Sprintf("%s Entered Error State", w.ResourceTitle),
			fmt.Sprintf(
				"%s %s (name %s) was created but transitioned to 'error' instead of 'provisioned'. "+
					"Run 'terraform apply' to try again, or reach out to support.",
				w.ResourceTitle, lastMetadata.Id, lastMetadata.Name,
			),
		)
		return result, false
	}

	return result, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestCreateStateWatcherWaitTreatsErrorAsTerminal -v ./internal/nscale/...`

Expected: PASS.

- [ ] **Step 5: Run the existing transient-state test to confirm no regression**

Run: `go test -run TestCreateStateWatcherWaitHandlesTransientProvisioningStates -v ./internal/nscale/...`

Expected: PASS (all three sub-cases: `pending`, `unknown`, `provisioning`).

- [ ] **Step 6: Commit**

```bash
git add internal/nscale/helper.go internal/nscale/helper_test.go
git commit -m "fix(waiter): surface diagnostic when create enters error state (DX-1025)"
```

---

## Task 2: `UpdateStateWatcher` — error state surfaces a diagnostic

**Files:**
- Modify: `internal/nscale/helper_test.go`
- Modify: `internal/nscale/helper.go:203-253`

- [ ] **Step 1: Add the new state constant**

In `internal/nscale/helper.go:203-207`, replace the block:

```go
const (
	UpdateStateUpdating = "updating"
	UpdateStateErrored  = "errored"
	UpdateStateUpdated  = "updated"
)
```

with:

```go
const (
	UpdateStateUpdating          = "updating"
	UpdateStateErrored           = "errored"
	UpdateStateUpdated           = "updated"
	UpdateStateProvisioningError = "provisioning_error"
)
```

- [ ] **Step 2: Write the failing test**

Append to `internal/nscale/helper_test.go`:

```go
// TestUpdateStateWatcherWaitTreatsErrorAsTerminal ensures the update waiter exits cleanly with a
// diagnostic when the API reports provisioningStatus=error during an update.
func TestUpdateStateWatcherWaitTreatsErrorAsTerminal(t *testing.T) {
	const (
		resourceID      = "fe563485-0631-4707-bec7-0d661cf20efc"
		operationTagKey = TerraformOperationTagPrefix + "test-op"
	)

	watcher := UpdateStateWatcher[waitTestResource]{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context) (*waitTestResource, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return &waitTestResource{name: "failed"}, &coreapi.ProjectScopedResourceReadMetadata{
				Id:                 resourceID,
				ProvisioningStatus: coreapi.ResourceProvisioningStatusError,
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
		if strings.Contains(d.Detail(), resourceID) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Wait() diagnostics did not include resource ID %q: %#v", resourceID, response.Diagnostics)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -run TestUpdateStateWatcherWaitTreatsErrorAsTerminal -v ./internal/nscale/...`

Expected: FAIL — current update waiter sees `ProvisioningStatus == error`, finds no operation tag, returns `UpdateStateUpdating` (stays Pending) and eventually times out — producing a timeout error, not the structured diagnostic we want.

- [ ] **Step 4: Implement the fix in `UpdateStateWatcher.Wait`**

Replace the function body of `UpdateStateWatcher.Wait` (currently `internal/nscale/helper.go:215-253`) with:

```go
func (w *UpdateStateWatcher[T]) Wait(ctx context.Context, operationTagKey string, timeouts tftimeouts.Value, response *resource.UpdateResponse) (*T, bool) {
	timeout, diagnostics := timeouts.Update(ctx, 30*time.Minute)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return nil, false
	}

	var lastMetadata *coreapi.ProjectScopedResourceReadMetadata

	stateWatcher := retry.StateChangeConf{
		Timeout: timeout,
		Pending: []string{UpdateStateUpdating},
		Target:  []string{UpdateStateUpdated, UpdateStateProvisioningError},
		Refresh: func() (any, string, error) {
			result, metadata, err := w.GetFunc(ctx)
			if err != nil {
				return nil, UpdateStateErrored, err
			}

			lastMetadata = metadata

			if metadata.ProvisioningStatus == coreapi.ResourceProvisioningStatusError {
				return result, UpdateStateProvisioningError, nil
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

	if lastMetadata != nil && lastMetadata.ProvisioningStatus == coreapi.ResourceProvisioningStatusError {
		response.Diagnostics.AddError(
			fmt.Sprintf("%s Entered Error State", w.ResourceTitle),
			fmt.Sprintf(
				"%s %s (name %s) transitioned to 'error' during update. "+
					"Run 'terraform apply' to try again, or reach out to support.",
				w.ResourceTitle, lastMetadata.Id, lastMetadata.Name,
			),
		)
		return result, false
	}

	return result, true
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -run TestUpdateStateWatcherWaitTreatsErrorAsTerminal -v ./internal/nscale/...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/nscale/helper.go internal/nscale/helper_test.go
git commit -m "fix(waiter): surface diagnostic when update enters error state (DX-1025)"
```

---

## Task 3: `DeleteStateWatcher` — error state surfaces a diagnostic

**Files:**
- Modify: `internal/nscale/helper_test.go`
- Modify: `internal/nscale/helper.go:255-302`

- [ ] **Step 1: Add the new state constant**

In `internal/nscale/helper.go:255-259`, replace the block:

```go
const (
	DeleteStateDeleting = "deleting"
	DeleteStateErrored  = "errored"
	DeleteStateDeleted  = "deleted"
)
```

with:

```go
const (
	DeleteStateDeleting          = "deleting"
	DeleteStateErrored           = "errored"
	DeleteStateDeleted           = "deleted"
	DeleteStateProvisioningError = "provisioning_error"
)
```

- [ ] **Step 2: Write the failing test**

Append to `internal/nscale/helper_test.go`:

```go
// TestDeleteStateWatcherWaitTreatsErrorAsTerminal ensures the delete waiter exits cleanly with a
// diagnostic when the API reports provisioningStatus=error instead of 404'ing.
func TestDeleteStateWatcherWaitTreatsErrorAsTerminal(t *testing.T) {
	const resourceID = "c2b8d351-c7b1-4fd5-a2c3-0f897a1df29c"

	watcher := DeleteStateWatcher{
		ResourceTitle: "Instance",
		ResourceName:  "instance",
		GetFunc: func(ctx context.Context) (any, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return struct{}{}, &coreapi.ProjectScopedResourceReadMetadata{
				Id:                 resourceID,
				ProvisioningStatus: coreapi.ResourceProvisioningStatusError,
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
		if strings.Contains(d.Detail(), resourceID) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Wait() diagnostics did not include resource ID %q: %#v", resourceID, response.Diagnostics)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -run TestDeleteStateWatcherWaitTreatsErrorAsTerminal -v ./internal/nscale/...`

Expected: FAIL — current delete waiter sees a non-404 result and returns `DeleteStateDeleting` forever; the test's 2-second context times out.

- [ ] **Step 4: Implement the fix in `DeleteStateWatcher.Wait`**

Replace the function body of `DeleteStateWatcher.Wait` (currently `internal/nscale/helper.go:267-302`) with:

```go
func (w *DeleteStateWatcher) Wait(ctx context.Context, timeouts tftimeouts.Value, response *resource.DeleteResponse) bool {
	timeout, diagnostics := timeouts.Delete(ctx, 30*time.Minute)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return false
	}

	var lastMetadata *coreapi.ProjectScopedResourceReadMetadata

	stateWatcher := retry.StateChangeConf{
		Timeout: timeout,
		Pending: []string{DeleteStateDeleting},
		Target:  []string{DeleteStateDeleted, DeleteStateProvisioningError},
		Refresh: func() (any, string, error) {
			_, metadata, err := w.GetFunc(ctx)
			if err == nil {
				lastMetadata = metadata
				if metadata != nil && metadata.ProvisioningStatus == coreapi.ResourceProvisioningStatusError {
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

	if lastMetadata != nil && lastMetadata.ProvisioningStatus == coreapi.ResourceProvisioningStatusError {
		response.Diagnostics.AddError(
			fmt.Sprintf("%s Entered Error State", w.ResourceTitle),
			fmt.Sprintf(
				"Deprovisioning of %s %s (name %s) failed; it transitioned to 'error' instead of being removed. "+
					"Re-run 'terraform destroy' to try again, or reach out to support.",
				w.ResourceTitle, lastMetadata.Id, lastMetadata.Name,
			),
		)
		return false
	}

	return true
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -run TestDeleteStateWatcherWaitTreatsErrorAsTerminal -v ./internal/nscale/...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/nscale/helper.go internal/nscale/helper_test.go
git commit -m "fix(waiter): surface diagnostic when delete enters error state (DX-1025)"
```

---

## Task 4: Full verification

**Files:** none.

- [ ] **Step 1: Run the full unit test suite**

Run: `make test`

Expected: all tests pass.

- [ ] **Step 2: Run lint**

Run: `make lint`

Expected: clean (no findings).

- [ ] **Step 3: Run gofmt**

Run: `make fmt`

Expected: no diff after running (formatter is idempotent). Check with `git diff --stat` — should be empty.

- [ ] **Step 4: Confirm no resource files were touched**

Run: `git diff --stat origin/main -- internal/services/`

Expected: empty output. The fix is contained to `internal/nscale/helper.go` and `internal/nscale/helper_test.go`.

---

## Notes for the implementer

- **Why `lastMetadata` is captured via closure rather than threaded through the return value of `WaitForStateContext`:** the SDK's `WaitForStateContext` returns the raw `result` from refresh, not the state string. We need the metadata pointer (specifically `Id` and `ProvisioningStatus`) for the diagnostic. The refresh closure runs in a goroutine inside `WaitForStateContext`, but that goroutine has terminated by the time `WaitForStateContext` returns — so reading the captured pointer in the calling goroutine is safe (no race).

- **Why `error` is added to `Target` rather than returning a non-nil error from `Refresh`:** returning `(_, _, err)` from refresh works but loses access to the final metadata. Adding `error` to `Target` lets the wait exit cleanly *and* preserves the metadata for the diagnostic. Matches the convention used in `equinix/terraform-provider-metal` for its baremetal device resource.

- **Why we don't call `response.State.RemoveResource`:** the resource exists on the backend in `error` state. Terraform Core marks it tainted automatically because `Create` (or `Update`) returns an error with a non-null ID already saved in state. The next `apply` does destroy-then-create. Removing it from state would force a `terraform import` to recover.

- **The diagnostic wording is intentionally specific.** It names the lifecycle stage (create / update / delete-deprovision) and gives the user the next action (`terraform apply` for create/update, manual console cleanup for delete). If the team that owns user-facing wording across the provider wants to standardise the prose, this is the only place it needs to change.
