# NKS in the Terraform provider — implementation notes

Companion to [`kubernetes_cluster.md`](kubernetes_cluster.md). That file is the
spec — what we set out to build and why. This one is the record of what was
actually built: how every field behaves, what forces a rebuild, how the waiters
work, and what evidence stands behind each claim.

Written for a reviewer who has to decide whether to trust this, and for whoever
picks up node pools next.

Ships three Terraform types:

| Type | Kind |
| --- | --- |
| `nscale_kubernetes_cluster` | resource |
| `nscale_kubernetes_cluster` | data source |
| `nscale_kubernetes_platform_releases` | data source (list) |

---

## 1. The one thing to understand first

**A cluster's project, organization and region are not settable.** They are
inherited from `network_id`.

`clusterCreateSpecV1` is `additionalProperties: false` and has no `projectId`,
no `organizationId`, no header parameter. There is nowhere to put them. NKS
resolves all three from the network you attach to, and returns them on read.

This is why the resource has no `project_id` argument, unlike every other
project-scoped resource in this provider. You choose a cluster's scope by
choosing its network. Attempting to set any of the three is rejected by the
framework at plan time (verified: `TestAccKubernetesClusterResource_rejectsSettingScope`).

The knock-on effect is that `network_id` is doing two jobs — it picks the
network *and* it picks the project. Changing it is a move between projects,
which is part of why it forces replacement.

---

## 2. Every field, and how it behaves

### Arguments

| Field | Kind | On change | Notes |
| --- | --- | --- | --- |
| `name` | Required | **in-place** | `metadata.name`. Validated by `NameValidator()`. |
| `network_id` | Required | **REBUILD** | The only replacing field. See §3. |
| `platform_release_id` | Required | **in-place** | Rolling control-plane upgrade. Use the releases data source; do not hardcode. |
| `description` | Optional | **in-place** | `metadata.description`. |
| `tags` | Optional+Computed | **in-place** | Reserved `terraform.nscale.com/` prefix rejected by validator. |
| `api_server.public_ip` | Optional+Computed | **in-place** | Server default `false`. |
| `api_server.allowed_cidrs` | Optional+Computed | **in-place** | Server default `["0.0.0.0/0"]`. Set, 1–32 entries, IPv4 CIDR validated. |
| `cluster_network.pod_cidr` | Optional+Computed | **in-place** | Server default `10.240.0.0/12`. See §4 — this one is a judgement call. |
| `cluster_network.service_cidr` | Optional+Computed | **in-place** | Server default `10.96.0.0/16`. Same caveat. |
| `addons.hardware` | Optional+Computed | **in-place** | Server default **`true`**. See §5. |

### Computed

| Field | `UseStateForUnknown`? | Source |
| --- | --- | --- |
| `id` | yes | `metadata.id` |
| `project_id` | yes | `metadata.projectId` — inherited from network |
| `organization_id` | yes | `metadata.organizationId` — inherited |
| `region_id` | yes | `status.regionId` — inherited |
| `creation_time` | yes | `metadata.creationTime` |
| `provisioning_status` | **no** | `metadata.provisioningStatus` |
| `health_status` | **no** | `metadata.healthStatus` |
| `kubernetes_version_target` | no | `status.kubernetesVersion.target` |
| `kubernetes_version_observed` | no | `status.kubernetesVersion.observed` |
| `api_server_endpoint.*` | no | `status.apiServer` — null until the control plane is reachable |
| `applied_platform_release_id` | no | `status.release.appliedId` — lags during upgrade |
| `platform_release_deprecated` | no | `status.release.deprecated` |
| `platform_release_withdrawn` | no | `status.release.withdrawn` |
| `upgrade_available` | no | `status.release.upgradeAvailable` |
| `eligible_upgrade_target_ids` | no | `status.release.eligibleTargets`, in upgrade order |

**Why the split.** `UseStateForUnknown` copies prior state into the plan so you
don't get `(known after apply)` noise on values that cannot change. That is
right for identity and scope. It is wrong for status: those fields are
*expected* to change between plans, and pinning them would report stale values
as current. Playbook anti-pattern #12.

The visible consequence is that any update plan shows the status fields going
`-> (known after apply)`. That is correct, not noise.

### Deliberately not exposed

`status.addons` component detail, `status.controlPlane` replica counts,
`status.nodePools` summary counts. All are observability data that churns every
plan and cannot be acted on from HCL. Revisit if users ask.

---

## 3. What forces a rebuild

**`network_id`, and nothing else.**

Confirmed with the NKS team (Matt Pryor, 2026-08-24): networkId is immutable for
the life of a cluster. The `"Immutable after creation"` note appears only on
`clusterUpdateSpecV1`, not the create spec, because PUT is full-object
replacement — the field must be *present* in every update carrying the value it
was created with. A different value returns 422.

Two consequences worth stating:

1. `RequiresReplace` isn't just cosmetic. Without it, Terraform would generate a
   PUT with a changed networkId and every such apply would fail with a 422.
2. Because it forces replacement, the update converter can pass the planned
   networkId straight through — it is always equal to the prior value, since
   any change destroys and recreates instead. The "same networkID in the POST
   and every subsequent PUT" rule is satisfied structurally rather than by
   convention we have to remember.

**Teardown ordering.** Replacement is destroy-then-create; `create_before_destroy`
would be wrong here since two clusters cannot share a name. There is a window
with no cluster, and if the create fails the old one is already gone. Normal
Terraform, but worth knowing before typing yes.

---

## 4. Pod and service CIDRs — a judgement call, flagged

These are modelled as **mutable in place**. Decided 2026-08-26.

The honest position: the API accepts them in `clusterUpdateSpecV1`, but so does
every other field, because PUT is full replacement — their presence there proves
nothing. Changing pod/service CIDRs on a live Kubernetes cluster would be
unusual, and the NKS team was asked about `networkId` rather than
`clusterNetwork`, so this was never confirmed.

We chose the permissive option knowingly:

- If mutable is **right**, users can change them.
- If mutable is **wrong**, the failure is a rejected apply or silent drift —
  recoverable, visible.
- Going the other way later (mutable → `RequiresReplace`) is a **breaking
  change**: every existing cluster would plan a rebuild on next apply.
  Permissive-now is the reversible direction.

`TestAccKubernetesClusterResource_updateClusterNetwork` exists to settle it.
**It has not been run.** Silent-ignore is the sneaky failure mode: the apply
succeeds and the next plan shows a permanent diff.

---

## 5. Defaults are the server's, not ours

`addons.hardware` defaults to `true`, `public_ip` to `false`, `allowed_cidrs` to
`["0.0.0.0/0"]`, the CIDRs to their documented values — **all server-side**. The
provider sets no framework `Default` on any of them.

This follows playbook §1.5: the default belongs to whoever owns the semantics.
A provider-side `booldefault` on `hardware` would also have been subtly wrong —
it only fires when the `addons` block is present, so omitting the block and
omitting just the field would behave differently for no reason a user could
predict.

This only works because **NKS echoes applied defaults back on read**. Confirmed
by the team and then verified directly against a provisioned cluster: `GET`
returns all three optional blocks fully populated. Every one of them
round-trips, proven by a `terraform plan` reporting "No changes" after apply.

Had the API *not* echoed them back, Optional+Computed would produce a permanent
diff and the schema would have needed restructuring.

---

## 6. How we wait

Custom waiters in `kubernetes_cluster_wait.go`, deliberately **not** the shared
`internal/nscale` watchers. Three reasons, all NKS-specific:

1. `nscale.ResourceStatus` is typed on `nscale-sdk-go/common`; NKS generates its
   own structurally-identical enums and Go will not bridge them.
2. The shared watchers have no concept of `observedGeneration`.
3. They inspect only `provisioningStatus`, and for NKS `provisioned` +
   `healthStatus: error` is a failure.

### The settledness rule

This is the part that matters.

```
settled ⟺ status.observedGeneration != nil && *observedGeneration >= metadata.generation
```

NKS applies writes asynchronously **and** projects status asynchronously and
independently. For a window after any write, `metadata.provisioningStatus`
still describes the *previous* generation of the spec. A `provisioned` read
taken during that window is about the old cluster.

Without this check, create and update would return success while writing stale
endpoints, versions and release IDs into state — and the resource would look
fine. Nothing else in the response distinguishes the two cases.

`observedGeneration == nil` means no projection has completed. Treated as **not
settled**, never as settled-at-zero.

The repo already solves the equivalent problem for the cache-backed region API
with the operation-tag trick (`nscale.WriteOperationTag` writes a
`terraform.nscale.com/<uuid>` tag and polls until it echoes back).
`observedGeneration` is strictly better: nothing lands in user-visible tags, so
there is no strip-on-read step, and it covers create as well as update. We did
**not** port the tag approach to NKS.

### The state machine

`classify()` maps one read to one state. **Settledness is checked first** — until
status has caught up, every other field describes the previous generation and
must not be acted on, *including an error*, which may belong to a spec the user
has already replaced.

| Read | State |
| --- | --- |
| not settled | `settling` (pending) |
| `error` | `failed` |
| `deprovisioning` | `deleting` |
| `pending` / `provisioning` | `provisioning` |
| `provisioned` + health `error` | `failed` |
| `provisioned` + health `healthy` | `ready` (target) |
| `provisioned` + health `degraded`/`unknown` | `provisioning` — still transitional |

Create and update share `waitClusterProvisioned`: the settledness rule makes
them the same problem. `gone` is *pending* during create, because a cluster may
not be readable through the cache immediately after POST.

`failed` is deliberately in neither Pending nor Target, so the SDK drops out of
the state machine and we replace its generic error with the API's own
explanation via `failureDetail()` — preferring
`provisioningStatusDetail` / `healthStatusDetail` over the bare enums.

### Delete

`waitClusterDeleted` polls to 404. Narrower than the shared delete watcher,
which treats `error` as terminal from the first poll: a cluster already sitting
in `error` before the destroy must still be allowed to deprovision. An error
only becomes terminal **once deprovisioning has actually been observed** —
then we fail immediately rather than burning the remaining timeout on a 404
that is never coming.

### Timeouts

| | Create | Update | Delete |
| --- | --- | --- | --- |
| Default | **60m** | **90m** | **30m** |

Set from measurement, not guesswork. Observed create times on uni-dev ranged
**6m27s to 32m18s** — a 5× spread in the same region on the same release.
Delete was consistently ~2m.

The original 30m create default sat *inside* that range: it would have passed on
a fast day and failed on a slow one, which is the worst kind of default. It was
caught by the very first real apply, at 32m18s.

Raise these rather than lower them. An over-long default costs a slow failure on
a cluster that was never coming up. A short one fails an apply on a healthy
cluster and leaves state and reality disagreeing — with the cluster still
running and still billing.

Polling is 15s/15s (delay/min-interval), deliberately slower than the playbook's
5s/3s: a build takes tens of minutes, so a tighter interval adds API load and
log noise without finishing sooner.

---

## 7. Other decisions worth knowing

### The NKS client is generated in-tree

`internal/nks/`, from the canonical spec, via the `oapi-codegen` tool already
declared in `go.mod`. Refresh with `make nks-spec`.

The SDK route was rejected for two independent reasons:

1. `nscale-sdk-go@v0.2.0` carries the NKS spec but **deletes the `common`
   package** and renames the identity/region operations. 32 files here import
   `common` as `coreapi`, including `ResourceStatus` and every service model.
   Go permits one version of a module per build, so adopting v0.2.0 for NKS
   means refactoring the type foundation under every existing resource in the
   same change.
2. The SDK's vendored spec is **already behind** the canonical one — it predates
   the platform-release `organizationID` filter and the required
   `usableOrganizationIds` field.

When the SDK-wide v0.2.0 migration lands, delete `internal/nks/` and swap the
import path. The generated types are identical: same spec, same generator.

Side effect: `oapi-codegen` had to go v2.5.0 → v2.8.0, because v2.5.0 no longer
compiles against the repo's `kin-openapi` v0.144.0 (bumped by dependabot in
e9a4c2e). Pre-existing breakage nobody had hit, since nothing in-repo ran the
generator.

### No secrets anywhere

NKS is credential-free by design: no kubeconfig endpoint, no token endpoint.
`certificate_authority_data` is a **public CA bundle**, not a secret, and is not
marked `Sensitive`. Consequently there is no write-once field, no stash-on-Read,
and no "Handling the secret" docs section.

Cluster auth is handed to the `kubernetes`/`helm` providers via an `exec` block
invoking the nscale CLI — documented in the resource page. This keeps any
credential out of Terraform state entirely, which an EKS-style
`_auth` data source would not. The CLI's `token` subcommand is not
feature-gated, so a stock released binary works.

### Import

Passthrough on `id`. Everything round-trips — no `ImportStateVerifyIgnore`
beyond `timeouts`, which is config-only and never returned by any API.

One wrinkle, verified: a `terraform plan` immediately after import shows an
update, caused **solely** by `timeouts` being absent from imported state while
present in config. Removing the block from config gives a clean "No changes".
This is inherent framework behaviour affecting every resource with a timeouts
block, not something specific to this one.

### The endpoint has no default

`nks_service_api_endpoint` / `NSCALE_NKS_SERVICE_API_ENDPOINT` defaults to the
empty string, because NKS is pending a migration and its production hostname is
not settled. Guessing would be worse than nothing.

An unset endpoint is **not** a provider-level error. `nscale.NewClient` skips
building the client, and `Client.RequireNKS()` produces a resource-level
diagnostic naming both the attribute and the environment variable — so
configurations that touch no Kubernetes resources are entirely unaffected.
Verified with a real plan.

### The list data source needed a newer test harness

`nscale_kubernetes_platform_releases` is the provider's first plural data source
and correctly has no top-level `id`. SDKv2's `helper/resource` requires one on
every state object, so this package's acceptance tests use
`terraform-plugin-testing` v1.14.1 (pinned to match the repo's
`plugin-go` v0.29.0; `@latest` pulls a version that breaks `framework` v1.16.1).

The alternative — bolting a meaningless `id` onto the public schema to satisfy a
legacy harness — would have shown up in the docs and the schema baseline and
confused users. The repo's only other id-less data source, `instance_ssh_key`,
simply has no acceptance test, so this had never surfaced.

---

## 8. What we did to cover it, and what we didn't

### Verified against the live API (uni-dev)

Full manual lifecycle, driven by hand:

```
plan → apply (32m18s) → plan ("No changes") → state rm → import → plan → destroy (2m11s)
```

Then, separately: create → in-place update (15s) → replace-on-network-change.

- **Create** — waits to settled + provisioned + healthy; endpoints, versions and
  applied release all populated at apply time.
- **Read** — verified against a real provisioned cluster, including one owned by
  someone else, exercising the converters on data we did not author.
- **Update** — in-place, 15s, no rebuild; `data_source_agrees = true` after.
- **Delete** — 2m11s via `terraform destroy`.
- **Import** — round-trips cleanly (modulo `timeouts`).
- **Defaults** — all three optional blocks echo back and produce no diff.
- **Failure path** — NKS reported `error` with "add-ons are degraded"; the waiter
  detected it, surfaced the API's own message, and failed the apply rather than
  reporting success. First real exercise of that path.
- **422 handling** — creating on a deprecated release surfaced
  `platform release is deprecated` verbatim, with the offending resource and line.

### Automated

- **19 unit tests**: converters both directions, nil/absent optional sub-objects,
  empty-vs-absent `eligibleTargets`, the settledness predicate, a 10-row
  `classify` decision table, and query-string assertions proving unset filters
  are **omitted** rather than sent empty.
- **Acceptance, passing**: `_basic` (apply + PlanOnly + import/verify),
  `DataSource_byID`, `_rejectsSettingScope`, and all three platform-release tests.
- `make fmt lint test schema-check` green; example `terraform validate`s.

### Not covered — read this before shipping

| Gap | Why it matters |
| --- | --- |
| `_updateClusterNetwork` **never run** | The mutable-CIDR call in §4 is unverified. Silent-ignore would be a permanent diff. |
| `_boolZeroValues` **never run** | `hardware` defaults *true* server-side; a dropped `false` flips the value. Unit-tested, not acceptance-tested. |
| `_updateRelease` **never run** | uni-dev has one eligible release, so `upgrade_available` is always false. Needs a second release to exist. |
| **Node pools do not exist** | A cluster here has no workers. NKS models them as a separate top-level resource. What ships is a control plane with nothing to schedule on. |
| Production endpoint unknown | Pending the NKS migration. |

Two environment bugs found along the way, neither in the provider:

- **nks-core**: an org-scoped `nks:clusters create` grant fails with
  `403 forbidden: unexpected status verifying project exists`, because nks-core
  re-checks the project by calling identity as itself and lacks
  `identity:projects read`. Hits every org admin on every project. Worked around
  with a project-scoped `nks-project-admin` group. The 403 body carries no
  `trace_id` despite the error schema defining one.
- **uni-dev docs**: the CLI env vars are documented as bare hostnames; the CLI
  needs the `https://` scheme or it builds `Get "/identity.../api/v1/..."` and
  fails with `unsupported protocol scheme`.

### Where the evidence lives

- Spec and resolved open questions: [`kubernetes_cluster.md`](kubernetes_cluster.md)
- Waiter rationale in code: `internal/services/kubernetescluster/kubernetes_cluster_wait.go`
- Client rationale in code: `internal/nks/gen.go`
- Manual test config: `.context/tf/` (gitignored)
- User-facing docs: `website/docs/r/kubernetes_cluster.html.markdown`,
  `website/docs/d/kubernetes_cluster.html.markdown`,
  `website/docs/d/kubernetes_platform_releases.html.markdown`
