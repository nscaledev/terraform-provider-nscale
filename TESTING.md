# Testing Strategy — `terraform-provider-nscale`

This document describes how this provider is tested today, what each layer
catches, and the gaps we know we have. Contributors adding a new resource
should follow the **"What to write for a new resource"** checklist near the
bottom; reviewers should use the **"What this catches / does not catch"**
table to decide what additional tests to ask for.

---

## Goals

We want to catch, in roughly this order:

1. **Bugs in the provider's plan/apply lifecycle** — drift, perpetual diffs,
   computed-value churn, broken `terraform import`, missed state-watcher
   transitions.
2. **Drift between our schema and the upstream OpenAPI** — fields that change
   shape, become required, or disappear, and request bodies that no longer
   round-trip.
3. **Regressions in user-facing behaviour** — a JSON document that used to
   round-trip cleanly suddenly producing a diff, a sensitive value leaking,
   an immutable field becoming silently mutable, etc.
4. **Auth and configuration failures** — env-var precedence, missing
   credentials surfaced as clear diagnostics, multiple service URLs wired
   correctly.

These rank above raw line-coverage; we'd rather have a small number of
load-bearing tests than chase coverage for its own sake.

---

## The four layers

We borrow the structure HashiCorp's testing docs use, plus the spike notes in
`.context/attachments/pasted_text_2026-05-08_09-15-21.txt`.

### Layer 1 — Pure unit tests

Plain `go test` against package-internal helpers and converters. No Terraform
CLI, no network.

**What it catches well:** mistakes in `model ↔ API` conversion, helper
functions, validators, plan modifiers (when extracted to a function), default
resolution. Cheap, fast, deterministic.

**Today:**
- ✅ `internal/nscale/helper_test.go` — covers the create/update/delete state
  watcher logic.
- ❌ No unit tests on per-service model converters. New services (including
  `objectstorage`) ship without them.

**Recommended for new services:** at least one test per non-trivial converter
(`NewXModel`, `NscaleXCreateParams`, `NscaleXUpdateParams`) covering happy
path + nil/optional field handling. Especially worth it for resources with
JSON round-trip, custom permissions structs, or whitespace-sensitive input.

### Layer 2 — Schema snapshot

A golden-file diff of `terraform providers schema -json` against a committed
baseline.

**What it catches well:** accidental field renames, removals, required→optional
flips, dropped resources, registration omissions, and silent MarkdownDescription
edits. Catches the entire user-facing API surface without the maintainer having
to remember to update a registry list.

**Today:** ✅ Present (DX-1250). `scripts/check-provider-schema.sh` (wired as
`make schema-check`) renders `terraform providers schema -json | jq -S` via a
`dev_overrides` `.terraformrc` and diffs it against the committed baseline at
`testdata/schema/provider-schema.golden.json`. It runs as a credential-free
`schema` job in CI on every PR. Updating the baseline is a deliberate review
step — `make schema-update` (`./scripts/regenerate-schema.sh`) — the same way
we treat `make generate` output for docs. The regenerated baseline's diff is
the user-facing API change, so reviewers read it directly.

### Layer 3 — Replay / contract tests

`resource.UnitTest` (note: Unit, not Test) with a local stub or replay HTTP
server. Runs the real Terraform CLI lifecycle but without `TF_ACC` and without
hitting Nscale.

**What it catches well:** provider-block parsing, server-factory wiring,
explicit-config-overrides-env, `Configure` propagating clients into resources,
end-to-end create/refresh/destroy plumbing for one canonical scenario per
resource. Good for fast, deterministic PR feedback.

**Today:** ❌ Not present. No replay corpus, no stub server, no
`resource.UnitTest` callers.

**Recommended next step (medium effort):** start with one replay scenario per
service — typically `Create → Read → Destroy` of the simplest resource — and
grow only when a regression demands it. Replay payloads live under
`internal/services/<service>/testdata/fixtures/replay/*.json`. Replays are not
a permanent substitute for live acceptance — they cannot detect upstream auth
changes, API drift, or quota issues — but they make the PR-time loop
fast and offline-capable.

### Layer 4 — Live acceptance

`resource.Test` against the real Nscale API, gated by `TF_ACC=1`.

**What it catches well:** real-world apply semantics, timing/retry behaviour,
async state watchers, auth, network failures, provider-side bugs that only
manifest against a real backend. The only layer that proves the provider
actually works against production-shaped infrastructure.

**Today:** ✅ Each service has its own `acc_test.go` + `*_test.go` files
following a consistent pattern. Pre-checks `t.Skipf` if env vars are absent
so `make test` stays green without `TF_ACC=1`.

**Cost:** real resources, real quota, real time (endpoint provisioning can take
minutes). Tests run sequentially (fixtures share a project), and resource
names are deterministic so concurrent runs collide. Cleanup is automatic via
the test framework's destroy phase, but a hard kill mid-test can leave
orphans.

**Recommendation:** keep these as the gate before tagging a release, and run
on a nightly schedule plus a maintainer-applied `tf-acc` PR label. Don't
require them on every PR.

---

## Current per-service coverage

| Service | Layer 1 | Layer 4 |
|---|---|---|
| `internal/nscale` (helpers) | ✅ `helper_test.go` | n/a |
| `sshca` | ❌ | ✅ resource + DS + import |
| `filestorage` | ❌ | (none committed yet, but pattern available) |
| `instance` | ❌ | ✅ resource + DS + cross-resource (SSH CA) |
| `securitygroup` | ❌ | ❌ |
| `network` | ❌ | ❌ |
| `region` | ❌ | ❌ |
| `computecluster` | ❌ | ❌ |
| `objectstorage` *(new)* | ❌ | ✅ endpoint resource + update + DS, access key resource + DS, endpoint class DS |

Object storage acceptance tests in detail:

- `TestAccObjectStorageEndpointResource_basic` — create + plan-noop guard +
  import.
- `TestAccObjectStorageEndpointResource_update` — rename + identity-policy
  swap; exercises the PUT path and `UpdateStateWatcher`.
- `TestAccObjectStorageEndpointDataSource_basic` — round-trip via DS.
- `TestAccObjectStorageAccessKeyResource_basic` — create + secret captured +
  plan-noop guard for `UseStateForUnknown` on `secret` + composite-id import
  (verifies `ImportStateVerifyIgnore: ["secret"]` is required).
- `TestAccObjectStorageAccessKeyDataSource_basic` — round-trip via DS, with a
  `TestCheckNoResourceAttr("secret")` assertion confirming the data source
  intentionally hides the secret.
- `TestAccObjectStorageEndpointClassDataSource_basic` — list-and-filter
  lookup by id.

These all gate behind `TF_ACC=1` and the env vars listed below; without them,
`make test` runs them as `SKIP`.

---

## What this catches / does not catch

| Concern | Caught by | Not caught by |
|---|---|---|
| Plan/apply lifecycle bugs against real API | Layer 4 | 1, 2, 3 |
| Perpetual diff regressions (e.g. a missing `UseStateForUnknown`) | Layer 4 (via `PlanOnly: true` step) | 1, 2 |
| `model ↔ API` converter bugs | Layer 1 (if written) | 4 only catches *if* the bug surfaces as user-visible drift |
| Schema rename / removal / required-flip | Layer 2 (`make schema-check`) | 1, 4 — neither would notice an unused field disappearing |
| Provider-config env-var precedence | Layer 1 (if `resolveConfig`-style helper extracted) or Layer 3 | 4 catches it via auth failures only |
| Async state watcher timing | Layer 4 | 1, 3 unless replay encodes the polling sequence |
| Sensitive-value leakage to logs | Manual code review + Layer 4 with `TF_LOG=DEBUG` | 1, 2, 3 |
| Upstream API drift (field shape changes) | Layer 4 + `go mod` upgrades | 1 catches it at compile time only if the type genuinely moved |

---

## Required env vars for Layer 4 (acceptance)

Common across all services:

```
TF_ACC=1
NSCALE_SERVICE_TOKEN=...                 # bearer token; sensitive
NSCALE_REGION_ID=<uuid>
NSCALE_ORGANIZATION_ID=<uuid>
NSCALE_PROJECT_ID=<uuid>

# Override service URLs only if not the default *.unikorn.nscale.com hosts:
NSCALE_REGION_SERVICE_API_ENDPOINT=...
NSCALE_COMPUTE_SERVICE_API_ENDPOINT=...
NSCALE_STORAGE_SERVICE_API_ENDPOINT=...
```

Per service, additional `NSCALE_TEST_*` vars:

| Service | Variable |
|---|---|
| `instance` | `NSCALE_TEST_IMAGE_ID`, `NSCALE_TEST_FLAVOR_ID` |
| `objectstorage` | `NSCALE_TEST_OBJECT_STORAGE_ENDPOINT_CLASS_ID` |

Each `acc_test.go` lists the exact vars its `testAccPreCheck` requires. Missing
vars produce `t.Skipf` — they do not fail.

---

## What to write for a new resource

Today's minimum bar (matches what the new `objectstorage` package shipped):

1. `acc_test.go` with `testAccProtoV6ProviderFactories` and a `testAccPreCheck`
   listing every env var your tests need.
2. `<resource>_resource_test.go` with at least:
   - `_basic` — apply, assert key computed fields, then a separate `PlanOnly:
     true, ExpectNonEmptyPlan: false` step (this is the regression guard for
     `UseStateForUnknown` and any whitespace-normalising plan modifier), then
     an `ImportState: true` step with `ImportStateVerifyIgnore` for any
     attributes that don't round-trip (e.g. `timeouts`, `secret`).
3. `<resource>_data_source_test.go` — apply the resource, look it up via the
   data source, assert `TestCheckResourceAttrPair` for the user-visible fields.
4. For mutable resources, a separate `_update` test that exercises every
   in-place mutation path (rename, tag swap, scaling, etc.).

Strongly encouraged but currently missing across the repo:

5. `<resource>_model_test.go` (Layer 1) — table-tested converters covering
   nil/optional pointer fields, JSON document round-tripping, tag stripping
   via `nscale.RemoveOperationTags`, and any custom `Permissions`/`Spec`
   nested struct.

Plus, whenever your change alters the schema (it almost always does — a new
resource, attribute, or even a `Description` edit counts):

6. Regenerate the schema baseline: `make schema-update`, then commit
   `testdata/schema/provider-schema.golden.json`. CI's `schema` job fails if
   you forget. See Layer 2 above.

Out of scope for individual resource PRs (would be its own piece of work):
7. Replay corpus (Layer 3) — needs an HTTP recording harness and a stub
   server in tree first.

---

## Recommended next moves

Roughly in priority order, none of which are part of DX-958 but each has
clear value.

1. ✅ **Done (DX-1250) — Layer 2 schema snapshot.** `make schema-check` diffs
   `terraform providers schema -json | jq -S` against
   `testdata/schema/provider-schema.golden.json`; runs in CI on every PR.
   Catches drift across every service for the cost of one shell script.
2. **Add Layer 1 model-converter unit tests for `objectstorage`** as a
   pattern. Then the same pattern can be propagated to the other services
   gradually.
3. **Add Layer 3 replay tests** for at least the access-key happy path —
   it's the most subtle resource in the repo (create-once secret, composite
   import), and a replay would let us catch regressions in the
   secret-preservation logic without booking a staging endpoint every time.
4. **CI split.** Today everything runs serially in `make test`. Splitting
   into `unit + schema (every PR)`, `replay (every PR once corpus exists)`,
   `acceptance (nightly + label-gated)` keeps PR feedback fast.

---

## Conventions

- Test files live alongside the code: `internal/services/<svc>/<x>_test.go`.
  `package <svc>_test` for acceptance/integration tests, `package <svc>` for
  internal unit tests.
- Resource names in test fixtures are `tf-acc-<svc>-<short>`. **They are
  deterministic, not random**, so two acceptance-test runs against the same
  project will collide — run them serially or in dedicated test projects.
- Acceptance tests should always finish with `terraform destroy`. The test
  framework calls it automatically; do not skip the destroy phase.
- Sensitive values must be `Sensitive: true` in schema and never appear in
  any `tflog.*` call. Audit with `grep -n 'tflog\|fmt.Print\|log\.'` in any
  resource that handles credentials.
- Composite import IDs (parent-scoped resources) use `/` as the separator
  and are documented in the resource's `website/docs/r/<x>.html.markdown`
  Import section.
