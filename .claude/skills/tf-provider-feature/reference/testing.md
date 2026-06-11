# Testing strategy

How this provider is tested, what each layer catches, the gaps we know about, and the minimum bar for a new resource. Companion to [playbook.md](playbook.md).

> The canonical version of this document is `TESTING.md` at the repo root. This file is the skill-companion view; if the two diverge, `TESTING.md` wins — update both.

---

## Goals

Catch, in roughly this order:

1. **Bugs in the provider's plan/apply lifecycle** — drift, perpetual diffs, computed-value churn, broken `terraform import`, missed state-watcher transitions.
2. **Drift between our schema and the upstream openapi** — fields that change shape, become required, or disappear; request bodies that no longer round-trip (the `omitempty` bool class).
3. **Regressions in user-facing behaviour** — a JSON document that used to round-trip cleanly suddenly producing a diff, a sensitive value leaking, an immutable field becoming silently mutable.
4. **Auth and configuration failures** — env-var precedence, missing credentials surfaced as clear diagnostics, multiple service URLs wired correctly.

These rank above raw line-coverage. Small numbers of load-bearing tests beat chasing coverage.

---

## The four layers

### Layer 1 — Pure unit tests

`go test` against package-internal helpers and converters. No Terraform CLI, no network.

**Catches well:** mistakes in `model ↔ API` conversion, helper functions, validators, plan modifiers (when extracted to a function), default resolution, the `omitempty` bool class ([playbook.md](playbook.md) §1.6). Cheap, fast, deterministic.

**Today:**

- ✅ `internal/nscale/helper_test.go` — covers the create/update/delete state watcher logic.
- ❌ No unit tests on per-service model converters. New services (including `objectstorage`) ship without them.

**Recommended for new services:** at least one test per non-trivial converter (`NewXModel`, `NscaleXCreateParams`, `NscaleXUpdateParams`) covering happy path + nil/optional field handling. Especially worth it for resources with JSON round-trip, custom permissions structs, or whitespace-sensitive input.

For every `Required` bool: a round-trip test that sets it to its zero value. This is the cheapest way to catch the `omitempty` class:

```go
// internal/services/storage/bucket_model_test.go
func TestBucketAPIRequest_VersionedFalseRoundTrip(t *testing.T) {
    in := bucketAPIRequest{Name: "x", Versioned: ptr.To(false)}
    b, err := json.Marshal(in)
    require.NoError(t, err)
    assert.JSONEq(t, `{"name":"x","versioned":false}`, string(b))
}
```

### Layer 2 — Schema snapshot

A golden-file diff of `terraform providers schema -json` against a committed baseline.

**Catches well:** accidental field renames, removals, required→optional flips, dropped resources, registration omissions, silent `MarkdownDescription` edits. The entire user-facing API surface, without the maintainer having to remember to update a registry list.

**Today:** ✅ Present (DX-1250). `make schema-check` (`scripts/check-provider-schema.sh`) renders `terraform providers schema -json | jq -S` via a `dev_overrides` `.terraformrc` and diffs it against `testdata/schema/provider-schema.golden.json`. Runs as a credential-free `schema` job in CI on every PR. After an intentional schema change, run `make schema-update` (`./scripts/regenerate-schema.sh`) and commit the regenerated baseline — its diff is the user-facing API change, the same way we treat `make generate` output for docs.

### Layer 3 — Replay / contract tests

`resource.UnitTest` with a local stub or replay HTTP server. Runs the real Terraform CLI lifecycle without `TF_ACC` and without hitting Nscale.

**Catches well:** provider-block parsing, server-factory wiring, explicit-config-overrides-env, `Configure` propagating clients into resources, end-to-end create/refresh/destroy plumbing for one canonical scenario per resource. Good for fast, deterministic PR feedback.

**Today:** ❌ Not present. No replay corpus, no stub server, no `resource.UnitTest` callers.

**Recommended next step (medium effort):** start with one replay scenario per service — typically `Create → Read → Destroy` of the simplest resource — and grow only when a regression demands it. Replay payloads live under `internal/services/<service>/testdata/fixtures/replay/*.json`. Replays are not a permanent substitute for live acceptance — they cannot detect upstream auth changes, API drift, or quota issues — but they make PR feedback fast and offline-capable.

### Layer 4 — Live acceptance

`resource.Test` against the real Nscale API, gated by `TF_ACC=1`.

**Catches well:** real-world apply semantics, timing/retry behaviour, async state watchers, auth, network failures, provider-side bugs that only manifest against a real backend. The only layer that proves the provider actually works against production-shaped infrastructure.

**Today:** ✅ Each service has its own `acc_test.go` + `*_test.go` files following a consistent pattern. Pre-checks `t.Skipf` if env vars are absent so `make test` stays green without `TF_ACC=1`.

**Cost:** real resources, real quota, real time (endpoint provisioning takes minutes). Tests run sequentially (fixtures share a project); resource names are deterministic so concurrent runs collide. Cleanup is automatic via the test framework's destroy phase, but a hard kill mid-test can leave orphans.

**Recommendation:** gate before tagging a release, run on nightly schedule plus a maintainer-applied `tf-acc` PR label. Don't require them on every PR.

---

## Current per-service coverage

| Service | Layer 1 | Layer 4 |
| --- | --- | --- |
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

- `TestAccObjectStorageEndpointResource_basic` — create + plan-noop guard + import.
- `TestAccObjectStorageEndpointResource_update` — rename + identity-policy swap; exercises the PUT path and `UpdateStateWatcher`.
- `TestAccObjectStorageEndpointDataSource_basic` — round-trip via DS.
- `TestAccObjectStorageAccessKeyResource_basic` — create + secret captured + plan-noop guard for `UseStateForUnknown` on `secret` + composite-id import (verifies `ImportStateVerifyIgnore: ["secret"]` is required).
- `TestAccObjectStorageAccessKeyDataSource_basic` — round-trip via DS, with a `TestCheckNoResourceAttr("secret")` assertion confirming the data source intentionally hides the secret.
- `TestAccObjectStorageEndpointClassDataSource_basic` — list-and-filter lookup by id.

All gate behind `TF_ACC=1`; without env vars, `make test` runs them as `SKIP`.

---

## What this catches / does not catch

| Concern | Caught by | Not caught by |
| --- | --- | --- |
| Plan/apply lifecycle bugs against real API | Layer 4 | 1, 2, 3 |
| Perpetual diff regressions (missing `UseStateForUnknown`) | Layer 4 (via `PlanOnly: true` step) | 1, 2 |
| `model ↔ API` converter bugs | Layer 1 (if written) | 4 only catches *if* the bug surfaces as user-visible drift |
| `omitempty` bool round-trip | Layer 1 (JSON marshal test) + Layer 4 (`false` apply + `PlanOnly`) | Layer 2 or 3 alone won't catch it |
| Schema rename / removal / required-flip | Layer 2 (`make schema-check`) | 1, 4 — neither would notice an unused field disappearing |
| Provider-config env-var precedence | Layer 1 (with `resolveConfig`-style helper) or Layer 3 | 4 catches it via auth failures only |
| Async state watcher timing | Layer 4 | 1, 3 unless replay encodes the polling sequence |
| Sensitive-value leakage to logs | Manual code review + Layer 4 with `TF_LOG=DEBUG` | 1, 2, 3 |
| Upstream API drift (field shape changes) | Layer 4 + `go mod` upgrades | 1 catches it at compile time only if the type genuinely moved |

---

## Required env vars for Layer 4

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

Per-service additional `NSCALE_TEST_*` vars:

| Service | Variable |
| --- | --- |
| `instance` | `NSCALE_TEST_IMAGE_ID`, `NSCALE_TEST_FLAVOR_ID` |
| `objectstorage` | `NSCALE_TEST_OBJECT_STORAGE_ENDPOINT_CLASS_ID` |

Each `acc_test.go` lists the exact vars its `testAccPreCheck` requires. Missing vars produce `t.Skipf` — they do not fail.

---

## Minimum bar for a new resource

Required (matches the current `objectstorage` package):

1. **`acc_test.go`** with `testAccProtoV6ProviderFactories` and a `testAccPreCheck` listing every env var your tests need.
2. **`<resource>_resource_test.go`** with at least:
   - **`_basic`** — apply, assert key computed fields, then a separate `PlanOnly: true, ExpectNonEmptyPlan: false` step (the regression guard for `UseStateForUnknown` and any whitespace-normalising plan modifier), then an `ImportState: true` step with `ImportStateVerify: true` and `ImportStateVerifyIgnore` for any attributes that don't round-trip (`timeouts`, write-once secrets).
3. **`<resource>_data_source_test.go`** — apply the resource, look it up via the data source, assert `TestCheckResourceAttrPair` for the user-visible fields.
4. **For mutable resources, a separate `_update` test** that exercises every in-place mutation path (rename, tag swap, scaling).

### The canonical acceptance-test skeleton

```go
func TestAccBucket_basic(t *testing.T) {
    name := acctest.RandomWithPrefix("tf-acc-test")
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        PreCheck:                 func() { testAccPreCheck(t) },
        CheckDestroy:             testAccCheckBucketDestroy,
        Steps: []resource.TestStep{
            // 1. Create + Read
            {Config: testAccBucketConfig_basic(name), Check: resource.ComposeAggregateTestCheckFunc(
                resource.TestCheckResourceAttr("nscale_bucket.test", "name", name),
                resource.TestCheckResourceAttrSet("nscale_bucket.test", "id"),
            )},
            // 2. Update
            {Config: testAccBucketConfig_updated(name)},
            // 3. PlanOnly to confirm no drift
            {Config: testAccBucketConfig_updated(name), PlanOnly: true},
            // 4. Import
            {
                ResourceName:      "nscale_bucket.test",
                ImportState:       true,
                ImportStateVerify: true,
                // Write-once secret — see playbook §2.3
                ImportStateVerifyIgnore: []string{"secret"},
            },
        },
    })
}
```

Rules:

- **Always** include a `PlanOnly` step after every Update. Catches the `omitempty` bool round-trip and most spurious-diff bugs.
- **Always** include an `Import` step with `ImportStateVerify: true`. The only acceptable use of `ImportStateVerifyIgnore` is for legitimately unrecoverable fields (write-once secrets, last-modified timestamps that float). **Add a comment naming why** every entry is there.
- **Always** include a `_disappears` test for resources with a corresponding management UI. Deletes the resource out-of-band and asserts plan shows a recreate.

### The bool round-trip test

For every `Required` bool, include an acceptance step that explicitly sets it to `false`:

```go
{Config: testAccBucketConfig_versioned(name, false), Check: resource.ComposeTestCheckFunc(
    resource.TestCheckResourceAttr("nscale_bucket.test", "versioned", "false"),
)},
{Config: testAccBucketConfig_versioned(name, false), PlanOnly: true}, // catches inconsistent result
```

If the `PlanOnly` fails with "Provider produced inconsistent result after apply", you've hit the `omitempty` bug. Fix the model converter (see [playbook.md](playbook.md) §1.6), not the test.

### Strongly encouraged but currently missing

5. **`<resource>_model_test.go`** (Layer 1) — table-tested converters covering nil/optional pointer fields, JSON document round-tripping, tag stripping via `nscale.RemoveOperationTags`, and any custom `Permissions`/`Spec` nested struct.

### Required whenever the schema changes

6. Regenerate the schema baseline: `make schema-update`, then commit `testdata/schema/provider-schema.golden.json` (Layer 2). A new resource/data source or attribute always trips this; CI's `schema` job fails if you forget.

### Out of scope for individual resource PRs

7. Replay corpus (Layer 3) — needs an HTTP recording harness and a stub server in tree first.

---

## Sweepers

Convention: every acceptance test resource name must be prefixed `tf-acc-test` (use `acctest.RandomWithPrefix`). Sweepers match on that prefix.

```go
// internal/services/storage/sweep_test.go
func init() {
    resource.AddTestSweepers("nscale_bucket", &resource.Sweeper{
        Name: "nscale_bucket",
        F:    sweepBuckets,
    })
}

func sweepBuckets(region string) error {
    c, err := sharedClientForRegion(region)
    if err != nil { return err }
    buckets, err := c.Storage.ListBuckets(context.Background())
    if err != nil { return err }
    for _, b := range buckets {
        if !strings.HasPrefix(b.Name, "tf-acc-test") { continue }
        _ = c.Storage.DeleteBucket(context.Background(), b.ID)
    }
    return nil
}
```

If the API doesn't have a list endpoint (object-storage endpoints currently don't), accept that we can't sweep — document the manual cleanup and rely on per-test destroy. Don't fake a list endpoint client-side.

---

## Parallelism and test inputs

- Use `acctest.RandomWithPrefix("tf-acc-test")` for every nameable resource. Never hardcode names.
- Scope tests by `NSCALE_TEST_PROJECT_ID` (one project per CI runner).
- Run with `-parallel 4` initially; raise as we gain confidence. Don't `t.Parallel()` tests that share a global resource (a single region's quota).

---

## Mocking

Don't mock the framework. It's wasted effort.

Mock at the converter level: pure functions, table tests. The fast feedback loop matters; the realistic loop is the acceptance test.

If you must exercise the HTTP client without a live API, use `httptest.NewServer` in a separate `*_integration_test.go` file with a build tag, but expect to throw most of these out the first time the openapi spec drifts.

---

## Recommended next moves (project-wide)

Priority order:

1. ✅ **Done (DX-1250) — Layer 2 (schema snapshot).** `make schema-check`, one CI job. Catches drift across every service.
2. **Layer 1 model-converter unit tests for `objectstorage`** as a pattern. Then propagate to the other services gradually.
3. **Layer 3 replay tests** for at least the access-key happy path — most subtle resource in the repo (create-once secret, composite import).
4. **CI split.** Today everything runs serially in `make test`. Split into `unit + schema (every PR)`, `replay (every PR once corpus exists)`, `acceptance (nightly + label-gated)` to keep PR feedback fast.

---

## Conventions

- Test files live alongside the code: `internal/services/<svc>/<x>_test.go`. `package <svc>_test` for acceptance/integration tests, `package <svc>` for internal unit tests.
- Resource names in test fixtures use `acctest.RandomWithPrefix("tf-acc-test")`. **Don't hardcode names** — two acceptance runs against the same project collide.
- Acceptance tests should always finish with `terraform destroy`. The test framework calls it automatically; do not skip the destroy phase.
- Sensitive values must be `Sensitive: true` in schema and never appear in any `tflog.*` call. Audit with `grep -n 'tflog\|fmt.Print\|log\.'` in any resource that handles credentials.
- Composite import IDs (parent-scoped resources) use `/` as the separator and are documented in the resource's `website/docs/r/<x>.html.markdown` Import section.
