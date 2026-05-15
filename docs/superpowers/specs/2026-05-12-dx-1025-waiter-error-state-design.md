# DX-1025: Waiter behavior when a resource enters `error` state

**Linear:** https://linear.app/nscale-workspace/issue/DX-1025
**Status:** Design approved, ready for implementation plan
**Date:** 2026-05-12

## Problem

When a `nscale_instance` (or any other Nscale resource) transitions to `provisioningStatus: error` during create/update/delete, the Terraform waiter exits with an uninformative message:

```
Error: Failed to Wait for Instance to be Created
An error occurred while waiting for the instance to be created:
unexpected state 'error', wanted target 'provisioned'. last error: %!s(<nil>)
```

The `%!s(<nil>)` is a Go formatting artifact from `terraform-plugin-sdk/v2/helper/retry.UnexpectedStateError` when the refresh function returns a state outside `Pending ∪ Target` without an accompanying Go error. The resource ID is already saved to Terraform state before the wait, so the failed instance sits in state half-created and re-running `apply` does not auto-recover.

## Goals

1. Surface a clear, actionable diagnostic when an Nscale resource enters `error` state during create / update / delete.
2. Make the failure recoverable through Terraform's normal tainted-replacement flow — one re-apply heals everything.
3. Apply the fix symmetrically across all resources via the shared waiter helpers in `internal/nscale/helper.go`, not per-resource.
4. Do not change schema, provider configuration, or any resource's public surface.

## Non-goals

- Surfacing *why* the API returned `error`. The upstream `unikorn-cloud/core` `ProjectScopedResourceReadMetadata` does not currently carry a failure reason / message / conditions field. A separate upstream enhancement request will be filed; this PR does what it can without it.
- Auto-deprovisioning failed resources. Preserves diagnostic state on expensive GPU baremetal; matches Equinix Metal's convention; reversible via Phase 2 if operations later demonstrates a need.
- Distinguishing transient vs. permanent `error`. The API's enum is flat; `error` is treated as terminal.
- Migrating off `terraform-plugin-sdk/v2/helper/retry.StateChangeConf` to a framework-native waiter.

## Design

### Mechanism

`StateChangeConf` exits the wait loop immediately when `Refresh` returns a state outside `Pending ∪ Target`, producing `UnexpectedStateError` with a `nil` `LastError`. There are two ways to produce a useful diagnostic:

- **(i)** return a non-nil error from `Refresh` when `ProvisioningStatus == error`, short-circuiting the loop.
- **(ii)** add `error` to `Target`, let the wait return cleanly, and inspect the final state in the caller.

We pick **(ii)**. It is the Equinix Metal convention, it preserves the refreshed resource body for the diagnostic, and it keeps the loop's normal contract intact (the only "expected" exits are user-defined target states, not synthetic errors).

### Changes to `internal/nscale/helper.go`

**`CreateStateWatcher.Wait`:**
- Add `string(coreapi.ResourceProvisioningStatusError)` to `Target`.
- After `WaitForStateContext` returns successfully, type-assert the result to `*T` and call `GetFunc` again *only if needed* to recover the metadata (or thread the metadata through the refresh closure via a captured variable — preferred, no extra API call).
- If the captured metadata's `ProvisioningStatus == error`, append an error diagnostic with the resource title, ID, and the action hint, and return `(result, false)`. The state setter in the caller has already persisted the ID, so we don't touch state here.
- If `ProvisioningStatus == provisioned`, return `(result, true)` as today.

**`UpdateStateWatcher.Wait`:**
- The current implementation tracks a write-operation tag rather than `ProvisioningStatus` directly, so its state vocabulary is already `{updating, updated, errored}` (synthetic, not the API enum). Use approach (i) here for consistency: extend `Refresh` to return a new `UpdateStateProvisioningError` synthetic state when it observes `ProvisioningStatus == error`, add that state to `Target`, and inspect post-wait. This keeps the operation-tag logic intact and avoids mixing two state vocabularies in one `StateChangeConf`.

**`DeleteStateWatcher.Wait`:**
- Add `string(coreapi.ResourceProvisioningStatusError)` to `Target` (alongside the implicit "404 = deleted" terminal).
- If the wait returns and the resource is still present with `ProvisioningStatus == error`, surface a diagnostic that says deprovision failed and may need manual cleanup. Do NOT call `RemoveResource` — the resource still exists on the backend.

### Diagnostic wording

All three diagnostics share a structure:

- **Summary line:** `"<Resource> entered error state"` (e.g. `"Instance entered error state"`).
- **Detail line:** name + ID + the lifecycle stage (create / update / delete) + a one-line action hint.
- The hint differs per stage:
  - Create: *"The resource was registered in the Nscale API but did not reach `provisioned`. The resource ID has been saved to state and will be replaced on the next `terraform apply`."*
  - Update: *"The resource transitioned to `error` during update. The resource will be replaced on the next `terraform apply`."*
  - Delete: *"Deprovisioning failed. The resource remains in Terraform state; manual cleanup via the Nscale console may be required before re-running `terraform destroy`."*

The exact wording is owned by the team that maintains user-facing diagnostics across the provider; this spec fixes the structure and the data points, not the prose.

### Behaviour after the fix (user-visible)

**Failing `apply`:**
- Waiter sees `error`, exits cleanly, returns the diagnostic.
- Resource ID stays in state. Other resources in the same plan complete normally.
- Exit code 1.

**Next `terraform plan`:**
- The failed resource is shown as `-/+ destroy and then create replacement` with `(tainted)`.
- Triggered automatically by Terraform Core because `Create` returned an error after the ID was saved to state.

**Next `terraform apply`:**
- `Delete` runs against the error-state instance (issues `DELETE /api/v2/instances/{id}`, waits for deprovision).
- `Create` runs for a fresh instance.
- One re-apply heals the world.

**`terraform destroy`:**
- Works normally — issues `DELETE`, waits for deprovision, removes from state.

### What this fix does NOT do

- Does not auto-delete the failed resource (would destroy diagnostic state on expensive hardware).
- Does not retry — `error` is treated as terminal.
- Does not require any new schema attribute or provider config.
- Does not surface a per-resource failure reason — the API doesn't expose one yet.

## Testing

Unit tests in `internal/nscale/helper_test.go` to extend:

- `CreateStateWatcher` returns a diagnostic with the resource ID when the API reports `error`.
- `CreateStateWatcher` returns success when the API reaches `provisioned` (existing test, ensure unchanged).
- `UpdateStateWatcher` returns a diagnostic with the resource ID when the API reports `error` after the operation tag is observed.
- `DeleteStateWatcher` returns a diagnostic when the API reports `error` instead of 404.

Acceptance tests are not strictly required for this fix — the error path is hard to exercise against a live API. Unit-level coverage via the existing fake-server pattern in `helper_test.go` is sufficient.

## Open question / pre-merge confirmation

- **Does `DELETE /api/v2/instances/{id}` succeed against an `error`-state instance?** The fix assumes it does — that is what makes the tainted-replacement flow work. Confirm with the platform team before merge. If `DELETE` is rejected, the user is stuck with `terraform state rm` + console cleanup; the design doesn't change but the diagnostic should call this out explicitly.

## Follow-up (separate from this PR)

- File an upstream enhancement request against `unikorn-cloud/core` (or the relevant Nscale fork) to add a free-form failure-reason field (e.g. `provisioningStatusDetail` or a Kubernetes-style `statusConditions[]`) on `ProjectScopedResourceReadMetadata`. Once available, wire it into the diagnostic.

## Out of scope (deferred to Phase 2 if signals warrant)

- Provider-level `delete_on_create_failure` toggle. Revisit only if support tickets demonstrate that orphaned error-state instances accumulating cost is a real operational issue.
- Follow-up `GET /events` on error to inline the most recent event message. Revisit if user complaints about diagnostic actionability accumulate.
