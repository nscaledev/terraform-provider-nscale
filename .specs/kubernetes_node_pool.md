# Spec: nscale_kubernetes_node_pool

> The workers for an NKS cluster. Resource + data source.
>
> Source ticket: DX-2008 (blocked by DX-2007). Due Friday 2026-08-28.
>
> **This is what makes DX-2007 usable.** A `nscale_kubernetes_cluster` on its
> own is a control plane with nothing to schedule on.

## Kind and package

- **Terraform type name:** `nscale_kubernetes_node_pool`
- **Resource, data source, or both:** both
- **Service package:** `internal/services/kubernetesnodepool/` (**new**)

Separate package rather than folding into `kubernetescluster/`, matching the
ticket and the API's own modelling — node pools are a top-level resource with
their own endpoints, not a sub-object of the cluster.

## Backing API

Verified against **`nscale-sdk-go` v0.3.0** (`kubernetes/openapi.yaml` and the
generated `kubernetes.gen.go`). The node pool API is unchanged between v0.2.0
and v0.3.0; v0.2.0 is where it first appeared.

> ### The SDK prerequisite is already met
>
> This spec was first drafted while the provider pinned v0.0.4, which predates
> the API entirely — at that version `kubernetes/` is the old unikorn-derived
> shape, where worker pools are a `kubernetesClusterWorkloadPool` array embedded
> in the cluster, and there are no `/api/v1/nodepools` endpoints at all.
>
> **[#74][pr74] has since landed on `main`**, taking the provider to v0.3.0. It
> was not a version-number change: v0.2.0 deleted the shared `common` package,
> which 22 non-test files referenced, so the provider now owns its metadata and
> tag types and converts at the SDK boundary. Nothing imports `common` any more.
>
> Nothing yet imports `kubernetes/` either, so node pools get a clean start on a
> dependency that is already current.

[pr74]: https://github.com/nscaledev/terraform-provider-nscale/pull/74

Same service, same client and same base URL as DX-2007, and no new plumbing.

| Operation | Verb + path | Response |
| --- | --- | --- |
| `listNodePools` | `GET /api/v1/nodepools` | `200 []NodePoolV1Read` |
| `createNodePool` | `POST /api/v1/nodepools` | `201 NodePoolV1Read` (async) |
| `getNodePool` | `GET /api/v1/nodepools/{nodePoolID}` | `200 NodePoolV1Read` |
| `updateNodePool` | `PUT /api/v1/nodepools/{nodePoolID}` | `200 NodePoolV1Read` (async, full replacement) |
| `deleteNodePool` | `DELETE /api/v1/nodepools/{nodePoolID}` | `202` (async) |

`updateNodePool` documents **409 and 422**, which is where immutability is
enforced. Nothing in the OpenAPI document marks a field immutable — there is no
`x-immutable`, and the CEL rules are not projected into the schema. **The SDK
cannot be the source of truth for which fields force replacement**; the 422
behaviour is, and it has to be pinned by acceptance tests rather than read off
the spec.

**Types:** `NodePoolV1Read/Create/Update`, `NodePoolRequestSpecV1` (create and
update are both aliases of this — one shape for both writes),
`NodePoolSpecV1`, `NodePoolStatusV1`, `NodePoolComputeV1`,
`NodePoolReservationV1`, `NodePoolTaintV1`, `NodePoolReservationStatusV1`.

Two types the earlier draft named **do not exist**: `NodePoolLabelsV1` and
`NodePoolReleaseStatusV1`. See §Schema corrections.

Scope is inherited the same way as the cluster: no `projectId` on the write
path. A pool takes `clusterId`, and project/organization/region follow from the
cluster (which got them from its network). Same `Computed`-only treatment.
`nodePoolSpecV1` and `nodePoolRequestSpecV1` are both
`additionalProperties: false`, so the field list below is exhaustive.

## Schema corrections

Three things the earlier draft asserted are wrong against the shipped schema.
All three were written from the ticket and a draft API document rather than from
generated code, which is how they survived a round of review.

1. **`labels` does not exist.** `nodePoolRequestSpecV1` and `nodePoolSpecV1`
   have no `labels` property, and there is no `nodePoolLabelsV1` schema. The
   only `labels` anywhere in the SDK's `kubernetes` package is on the *old*
   v0.1.0 `kubernetesClusterWorkloadPool`, which this API replaced. So the
   argument, its 64-entry cap, its empty-string-value round-trip test and its
   share of the disruption warning all come out. **Taints alone carry the roll
   story.**

2. **A node pool has no platform release.** `nodePoolStatusV1` has no `release`
   property. `clusterReleaseStatusV1` exists and is cluster-only. This answers
   open question 4: a pool **inherits** the cluster's release and does not
   report one of its own. All six `applied_platform_release_id` /
   `platform_release_*` computed attributes come out of both the resource and
   the data source. (The feedback's note that `platformReleaseID` is immutable
   on reservation pools describes something below the public API — there is no
   field here to make immutable.)

3. **`taints[].propagation` is new, and required.** Enum `Always` |
   `OnInitialization`, no default, listed in `nodePoolTaintV1.required`
   alongside `key` and `effect`. It is missing from the draft entirely, and it
   bears directly on the roll semantics — see §Rolls.

Smaller corrections folded into the tables below: `compute.flavorId` is
`minLength: 1, maxLength: 128`; taint `key` is `minLength: 1` as well as
`maxLength: 317`; `status.reservation` now carries `reservationId` as well as
`placementId`; and metadata gained `provisioningStatusDetail`,
`healthStatusDetail`, `modifiedTime`, `modifiedBy`, `createdBy` and
`deletionTime`.

## Attributes

### Arguments

| Name | Type | R/O/C | Plan modifiers | Notes |
| --- | --- | --- | --- | --- |
| `name` | String | Required | **`RequiresReplace`** | `metadata.name`; `NameValidator()`. Immutable server-side. |
| `cluster_id` | String | Required | **`RequiresReplace`** | `spec.clusterId`. Immutable server-side (422). |
| `provisioning_mode` | String | Required | **`RequiresReplace`** | `compute` \| `reservation`. `stringvalidator.OneOf`. Immutable server-side (422). |
| `replicas` | Int64 | Required | **`RequiresReplaceIf`** mode is `reservation` | `spec.replicas`. **min 0**, max 2147483647. Scale-to-zero is legal on compute pools; reservation pools cannot be scaled at all. |
| `description` | String | Optional | — | `metadata.description` |
| `tags` | Map(String) | Optional+Computed | — | `metadata.tags`; `NoReservedPrefix` |
| `compute` | SingleNested | Optional | — | Required iff mode is `compute` |
| `compute.flavor_id` | String | Required within block | **`RequiresReplace`** | `spec.compute.flavorId`. minLength 1, maxLength 128. Immutable server-side (422). |
| `reservation` | SingleNested | Optional | — | Required iff mode is `reservation` |
| `reservation.reservation_id` | String | Required within block | **`RequiresReplace`** | `spec.reservation.reservationId`. Immutable server-side (422). |
| `taints` | ListNested | Optional | **`RequiresReplaceIf`** mode is `reservation` | Max 64. In place on compute pools, and **rolls every worker** — see §Rolls. |
| `taints[].key` | String | Required | — | minLength 1, maxLength 317, K8s qualified-name pattern |
| `taints[].value` | String | Optional | — | maxLength 63, K8s label-value pattern |
| `taints[].effect` | String | Required | — | `NoSchedule` \| `PreferNoSchedule` \| `NoExecute` |
| `taints[].propagation` | String | **Required** | — | `Always` \| `OnInitialization`. Required in the API with no default — see below. |

There is **no `labels` argument**: the API has no such field (§Schema
corrections).

`taints[].propagation` has no server default and is in the schema's `required`
list, so the provider has three options and only one is honest:

- **Required in Terraform too.** Every taint spells out its propagation. Verbose
  for the common case, but it never guesses, and it never silently changes
  meaning if the API later adds a default.
- Optional with a provider-side default. Requires picking one, and the choice is
  a behavioural decision the API deliberately left open.
- Optional+Computed. Would let the server decide, except the server has no
  default to fall back on and would 400 the request.

**Take the first.** A taint's propagation is the difference between a pool that
rolls and one that does not; making a user state it is proportionate.

Immutability is **confirmed**, not assumed: each of the fields marked
`RequiresReplace` is guarded by a CEL `self == oldSelf` rule and rejected by the
server with a 422 (see §Answered). One of the first draft's assumptions was
answered against us — `compute.flavor_id` is *not* an in-place update.

`taints` is a **List**, not a Set: the API returns an array and Kubernetes taint
ordering is stable, so a list avoids spurious reordering diffs while keeping the
round-trip exact. (Revisit if the API turns out to reorder them, which would
make a set correct instead.)

`replicas` is `int` **without `omitempty`** in the generated client, so `0`
serialises correctly. No wrapper needed — but a unit test pins it, because a
future regeneration adding `omitempty` would silently break scale-to-zero
([playbook §1.6](../.claude/skills/tf-provider-feature/reference/playbook.md)).

### Computed

| Name | Type | `UseStateForUnknown` | Source |
| --- | --- | --- | --- |
| `id` | String | yes | `metadata.id` |
| `project_id` | String | yes | `metadata.projectId` — inherited |
| `organization_id` | String | yes | `metadata.organizationId` — inherited |
| `region_id` | String | yes | `status.regionId` |
| `creation_time` | String | yes | `metadata.creationTime` |
| `provisioning_status` | String | no | `metadata.provisioningStatus` |
| `health_status` | String | no | `metadata.healthStatus` |
| `current_replicas` | Int64 | no | `status.currentReplicas` |
| `ready_replicas` | Int64 | no | `status.readyReplicas` |
| `up_to_date_replicas` | Int64 | no | `status.upToDateReplicas` |
| `kubernetes_version` | String | no | `status.kubernetesVersion` |
| `placement_id` | String | no | `status.reservation.placementId` — reservation mode only |

**No platform release attributes.** `nodePoolStatusV1` has no `release`
property; a pool inherits the cluster's. Read
`nscale_kubernetes_cluster.applied_platform_release_id` instead. The six
attributes the draft listed here are deleted (§Schema corrections).

`status.desiredReplicas` is **not** exposed: it duplicates the `replicas`
argument and would invite confusion about which is authoritative. The three
observed counts (`current`, `ready`, `upToDate`) are the ones that tell a user
something they don't already know.

`status.reservation.reservationId` is **not** exposed either, for the same
reason — it is the reservation the pool resolved to, which on a pool whose
`reservation.reservation_id` is immutable is always the one you asked for.
`placementId` is exposed because it is genuinely new information: the placement
the pool created inside that reservation.

Two metadata fields the draft did not know about are worth taking:

| Name | Type | Source | Why |
| --- | --- | --- | --- |
| `provisioning_status_detail` | String | `metadata.provisioningStatusDetail` | The text behind an `error`, and the best candidate for making a PDB-blocked roll legible (open question 7) |
| `health_status_detail` | String | `metadata.healthStatusDetail` | Same, for `degraded` and `error` health |

`modifiedTime`, `modifiedBy`, `createdBy` and `deletionTime` also exist on the
metadata. Skip them unless the other resources in this provider already expose
their equivalents — consistency across the provider matters more here than
completeness against the API.

### Cross-field validation

`ConfigValidators` on the resource, enforcing at **plan time**:

- `provisioning_mode = "compute"` → `compute` set, `reservation` unset
- `provisioning_mode = "reservation"` → `reservation` set, `compute` unset

`provisioning_mode` now does double duty: it gates which capacity block is
valid, *and* it gates whether `replicas` and `taints` force replacement (see
§Immutability). Both readings of it should come from the same
place in the code, so the two never disagree about what mode a pool is in.

The ticket's acceptance criterion is explicit that mis-configuration must fail
at plan/validate, not apply. Prefer the framework's
`resourcevalidator.Conflicting` / `RequiredTogether` where they fit; a small
custom validator otherwise, following the shape of
`computecluster/validator.go`
([playbook §3.3](../.claude/skills/tf-provider-feature/reference/playbook.md)).

## Rolls: what a taint edit actually does

### Compute pools

A compute pool is a plain CAPI v1.14 `MachineDeployment`. Editing `taints`
re-hashes the `KubeadmConfigTemplate` name, which changes the
`MachineDeployment`'s template ref, which makes CAPI roll every worker in the
pool. So a taint edit **replaces every node in the pool** — but it does so as a
controlled rolling update, not a stampede:

- nks-core renders **no rollout strategy and no drain, volume-detach or
  deletion timeouts**, so upstream CAPI defaults apply: `RollingUpdate` with
  `maxSurge: 1` and `maxUnavailable: 0`.
- The Machine controller **cordons and drains** each node through the Eviction
  API before deleting it, so **PodDisruptionBudgets are honoured**.
- With no drain timeout set, an **unsatisfiable PDB blocks the roll
  indefinitely** — and the Terraform waiter simply times out. The same is true
  of a pool delete and a cluster delete.

Two practical consequences for this resource:

1. `maxSurge: 1`/`maxUnavailable: 0` means the roll is **serial** — one extra
   node at a time, each drained before its predecessor goes. Wall-clock is
   roughly `replicas × per-node (provision + join + drain)`, which is what the
   update timeout has to cover. See §Timeouts.
2. A timed-out apply on a PDB-blocked roll is **not** a failed roll. The
   `MachineDeployment` is still mid-rollout when Terraform gives up, and the
   next apply picks up where it left off. The error message should say so
   rather than implying the pool is broken.

The CAPI default behaviour above is upstream behaviour, taken as read. What was
verified in nks-core is the narrower and more load-bearing fact: **nks-core sets
no override**, so the defaults are what apply.

### `propagation` may change this, and we do not yet know how

The draft's whole disruption story rests on one sentence from the original API
document: taints "are not continuously reconciled onto running nodes", so a
change applies to new workers and rolls the old ones to take effect.

The shipped schema makes that a **per-taint choice**:

| Value | Reading |
| --- | --- |
| `OnInitialization` | Applied when a worker is created, and not afterwards — the behaviour the draft described |
| `Always` | Continuously reconciled onto running workers |

If `Always` means the controller patches running nodes, then the whole point of
it is to make the taint take effect **without** a roll — and the roll narrative
applies to `OnInitialization` only. But the taint is still part of the machine
template, so the template hash still changes, so CAPI would still roll the pool
regardless of propagation. Those two readings give opposite answers to "does
adding a taint recycle my fleet", which is the single most consequential
sentence in these docs.

**This is not resolvable from the schema** — the enum has no description beyond
"Taint propagation behavior". It needs the same treatment the immutability
questions got: ask, then verify against a real pool. Until then the docs say a
taint edit rolls the pool, because that is the answer that cannot get someone
hurt if it turns out to be wrong. See open question 11.

### Reservation pools

A reservation pool is backed by a placement, and **a placement never rolls**.
CAPNS documents no in-place resize, no rolling update and no per-node drain for
that backend. Nothing a template change would express can take effect, which is
why `replicas` and `taints` are both immutable in this mode — the API rejects
the edit rather than accepting a change that could never land. (The feedback
lists the platform release as frozen here too; there is no node-pool release
field in the public API, so that constraint has nothing to attach to.)

For the provider this means a reservation pool is **effectively immutable**:
only `description` and `tags` update in place. Everything else forces
replacement, and replacement releases and re-claims the placement.

### What the schema has to say

The disruption must appear in the `MarkdownDescription` of `taints`, not just
the prose docs — it is the one place a user sees it before
they apply. On a compute pool Terraform shows a taint edit as an ordinary
in-place update, because that is what it is at the API level; the plan cannot
warn, so the attribute description carries the weight. On a reservation pool the
plan *does* warn, because `RequiresReplaceIf` fires.

## Lifecycle

- **Create:** `POST` → `201`, async.
- **Read:** `GET` by id. 404 → `RemoveResource`.
- **Update:** `PUT`, full replacement, async. On a compute pool the common case
  is scaling `replicas`, which **must not disturb taints** — build the whole
  `nodePoolUpdateSpecV1` from config every time. On a reservation pool
  only `description` and `tags` reach this path; everything else has already
  been diverted to replacement by `RequiresReplaceIf`.
- **Delete:** `DELETE` → `202`, async, poll to not-found. On a compute pool this
  drains every node, so it is as slow as a roll and blocks on PDBs the same way.

### Waiters — reuse, don't duplicate

The settledness rules are **identical** to the cluster, and the cluster's
`classify` is stricter than "not an error". Success requires all three of:

1. `status.observedGeneration >= metadata.generation`
2. `provisioningStatus == provisioned`
3. `healthStatus == healthy`

`error` provisioning, or `provisioned` with `healthStatus: error`, fails the
apply. `degraded` and `unknown` keep polling. Delete polls to 404, and treats
`error` as terminal only once `deprovisioning` has been observed.

**The provider must not compare replica counts itself**, and the cluster
implementation does not. Upstream, a compute pool reaching `provisioned`
already requires the `MachineDeployment` status to be current, none of
`RollingOut`, `ScalingUp`, `ScalingDown`, `Remediating` or `Deleting` to be
true, `MachinesUpToDate` true, and `spec.replicas == desired ==
status.replicas`; `healthy` additionally requires `Available` and
`MachinesReady` true with `readyReplicas == replicas`. A settled
`provisioned` + `healthy` therefore already implies current, ready and
up-to-date all equal desired. Adding a count comparison in the provider would
duplicate that check, and would be the copy that drifts.

Reservation pools are the weaker case: readiness there is decided on counts
alone (`readyReplicas == spec.replicas`), and the ready count is coarse. The
waiter is unchanged, but a reservation pool reporting `healthy` is a softer
guarantee than a compute pool doing so, and the docs should not overclaim.

One cross-resource consequence worth knowing: a cluster is not `provisioned`
while any of its pools is scaling or rolling, because cluster `provisioned`
aggregates infrastructure, control plane, core and hardware add-ons,
authorization *and* every node pool being `Provisioned`. So a cluster update
applied concurrently with a pool roll waits for the roll — the cluster's
timeout has to survive a node pool's slowest operation, not just its own.

The ticket is explicit, and correct, that this should be **one generic watcher
shared by clusters and node pools** — the CLI centralises it in `statuswait`
and we should not have two copies drifting apart.

**Refactor plan:** generalise `kubernetescluster/kubernetes_cluster_wait.go`
into something both packages use. Both `ClusterV1Read` and `NodePoolV1Read`
carry `ProjectScopedResourceReadMetadataV1` and a status with
`ObservedGeneration`, so the shared waiter can take two small accessor funcs
(`metadata`, `observedGeneration`) or a narrow interface, and keep the entire
`classify` decision table in one place.

Candidate home: `internal/nks/wait/` or `internal/services/kubernetescommon/`.
Deciding that is part of implementation, but **the outcome must be one
`classify`, not two.** The 10-row decision table already has unit tests; those
move with it.

### Timeouts

A single worker joining an existing cluster is **faster than a control plane**.
A *roll* of a whole pool is not: `maxSurge: 1`/`maxUnavailable: 0` makes it
serial, so it costs roughly `replicas ×` per-node time, plus drain.

Proposed starting point, **to be corrected by measurement in Phase 4**:

| | Create | Update | Delete |
| --- | --- | --- | --- |
| Proposed | 30m | **60m** | **60m** |

Update and delete get double the create budget, revised up from the first
draft's flat 30m, because both walk the pool one node at a time and both drain:

- **Update** covers the worst case, a taint or label edit rolling every worker.
  At a plausible 5m per node a 10-worker pool needs ~50m, which the original
  30m would not have survived. If measurement shows per-node time is high, the
  honest answer may be to document "raise `timeouts.update` for large pools"
  rather than pick a default big enough for any pool.
- **Delete** drains every node on the way out, so it is bounded the same way.

Neither default can be made safe against a PDB that cannot be satisfied —
there is no drain timeout upstream, so the block is indefinite and the
Terraform timeout is the only bound. That is a documentation problem, not a
tuning one.

Record the observed numbers in the code comment as DX-2007 did.

## Immutability

Confirmed against the API's CEL rules and the server's 422 responses. It is
**mode-dependent**, which the first draft did not anticipate.

| Field | Compute pool | Reservation pool |
| --- | --- | --- |
| `name` | replace | replace |
| `cluster_id` | replace | replace |
| `provisioning_mode` | replace | replace |
| `compute.flavor_id` | replace | n/a |
| `reservation.reservation_id` | n/a | replace |
| `replicas` | **in place** | replace |
| `taints` | **in place** (rolls the pool) | replace |
| `description`, `tags` | in place | in place |

The right-hand column follows from §Rolls: a placement never rolls, so the API
refuses edits that could never take effect rather than accepting them silently.

`compute.flavor_id` is the one the first draft got wrong. It reasoned that a
flavour change *could* be an in-place update that happens to be maximally
disruptive, in the same category as taints. It is not — the API rejects it, so
it needs `RequiresReplace`, and a plan that shows a full replacement is telling
the truth.

### Implementation note

`replicas` and `taints` need `RequiresReplaceIf` rather than a plain
`RequiresReplace` — `int64planmodifier` and `listplanmodifier` both provide it.
The condition reads `provisioning_mode` and fires only for `reservation`.
Because `provisioning_mode` itself forces replacement, reading it from config or
state is equivalent: a mode change replaces the resource regardless.

Two attributes sharing one predicate still wants a single helper
(`requiresReplaceIfReservation`) rather than two copies of the same closure —
the draft had three before `labels` came out, and the argument is unchanged.

Scaling a reservation pool destroying and recreating it is a sharp edge worth
saying out loud in the docs: `replicas 2 → 3` on a reservation pool is not a
scale, it is a rebuild, and it releases and re-claims the placement.

## Write-once / sensitive fields

**None.** Same as the cluster — NKS returns no secrets.

## Import

Passthrough on `id`:

```sh
terraform import nscale_kubernetes_node_pool.workers <node-pool-id>
```

Everything should round-trip. Expect the same `timeouts`-only diff on the first
post-import plan that DX-2007 has, for the same reason.

## Known API constraints

- **`replicas` minimum is 0.** Scale-to-zero is legal on a compute pool, and
  worth an acceptance test — it is both a plausible user action and the
  `omitempty` canary. A reservation pool cannot be scaled at all.
- **There is no drain timeout.** Upstream CAPI cordons and drains each node
  through the Eviction API and nks-core sets no `nodeDrainTimeout`, so an
  unsatisfiable PodDisruptionBudget blocks a roll, a pool delete or a cluster
  delete indefinitely. Terraform's timeout is the only bound.
- **Reservation-pool readiness is coarse** — decided on `readyReplicas ==
  spec.replicas` alone, with no equivalent of the compute pool's
  `MachinesUpToDate` / `Available` conditions.
- `taints` max 64. Key minLength 1 / maxLength 317, value maxLength 63, both
  regex-constrained. `compute.flavorId` minLength 1 / maxLength 128. Mirror all
  of these as schema validators so a bad value fails at plan time.
- **`taints[].propagation` is required with no default.** A create that omits it
  is a 400, not a server-side default.
- List `name` filters are exact and case-sensitive, and the schema now says so
  outright — "Every matching resource is returned because display names are not
  unique". Data source stays id-based.
- `listNodePools` takes a **`clusterID` filter**, which the draft did not know
  about. It does not change the id-based data source, but it is the right call
  for any future "all pools in this cluster" data source, and it is how an
  acceptance test should find the pools it created.
- Reservation-backed pools need capacity to exist. A create against a
  reservation with none available presumably fails at apply; the acceptance test
  must be skippable when staging has no reservation.

## Examples

Extend `examples/kubernetescluster/` (or add `examples/kubernetesnodepool/`)
with a cluster plus two pools:

```hcl
resource "nscale_kubernetes_node_pool" "workers" {
  name              = "workers"
  cluster_id        = nscale_kubernetes_cluster.main.id
  provisioning_mode = "compute"
  replicas          = 3

  compute = {
    flavor_id = data.nscale_instance_flavor.worker.id
  }

  # Editing taints on a compute pool rolls every worker, one at a time.
  taints = [{
    key         = "workload"
    value       = "general"
    effect      = "PreferNoSchedule"
    propagation = "OnInitialization"
  }]
}

resource "nscale_kubernetes_node_pool" "gpu" {
  name              = "gpu"
  cluster_id        = nscale_kubernetes_cluster.main.id
  provisioning_mode = "reservation"
  replicas          = 2

  reservation = {
    reservation_id = nscale_reservation.gpu.id
  }

  # A reservation pool never rolls, so editing these — or replicas — replaces
  # the pool and re-claims the placement.
  taints = [{
    key         = "nvidia.com/gpu"
    value       = "true"
    effect      = "NoSchedule"
    propagation = "Always"
  }]
}
```

The reservation-backed pool tying into the existing `nscale_reservation`
resource is the part worth getting into the example — it is the cross-resource
story neither ticket exercises otherwise.

The two `propagation` values are there to show both, not as a recommendation.
Which one belongs in an example depends on open question 11, and the example
should be revisited once that is answered.

## Test plan

**Unit:**

- `NodePoolV1Read` → model, both modes, all status sub-objects nil and populated.
- Create/update param builders produce identical specs from the same model
  (they share `NodePoolRequestSpecV1`, so the risk is one converter drifting).
- **`replicas = 0` serialises explicitly** — the omitempty canary.
- Taints round-trip including a taint with no `value`, and one of each
  `propagation` value.
- **`propagation` is always serialised.** It is required with no server default,
  so a taint that reaches the wire without it is a 400. This is the same class
  of bug as the `omitempty` canary and deserves its own test.
- Validator matrix: mode × (compute set?) × (reservation set?) — all four
  combinations, two valid, two rejected.

**Acceptance:**

- `_basic` compute pool: create → check `ready_replicas` → `PlanOnly` → import.
- `_scale`: `replicas` 1 → 3 → in-place, ID unchanged, `PlanOnly` after.
- `_scaleToZero`: `replicas` → 0, in-place, `PlanOnly` after.
- `_taints`: add a taint on a compute pool → in-place, ID unchanged → `PlanOnly`.
  Assert `up_to_date_replicas` returns to `replicas`, which is what proves the
  roll finished rather than the waiter returning early.
- `_replace`: one case per immutable field — `provisioning_mode`, `name`,
  `cluster_id`, `compute.flavor_id` — each plans a replace. `flavor_id` is the
  one that regressed from the first draft, so it is the one that matters most.
- `_reservationImmutable`: on a reservation pool, `replicas` and `taints` each
  plan a **replace**, not an update. This is the `RequiresReplaceIf` predicate
  under test, and the failure mode if it is wrong is a plan that promises an
  in-place change and then 422s at apply.
- `_propagation`: change a taint's `propagation` and nothing else on a compute
  pool. Whether this rolls the pool is open question 11, so this test exists to
  **answer** it — assert on `up_to_date_replicas` dropping and recovering, and
  write down whichever way it goes.
- `_reservation`: gated on a reservation env var, skipped when absent.
- Data source by id agrees with the resource.
- Negative: mode/block mismatch fails at **plan**, asserted with `ExpectError`
  and `PlanOnly: true`.
- Negative: a taint without `propagation` fails at plan, not with a 400 at
  apply.

`_reservationImmutable` can run as `PlanOnly` against a single created pool —
it asserts on plan output, so it does not need to pay for two rebuilds.

**Cost:** each pool is real compute. Cheaper than a control plane, but the
suite still wants a separate CI lane, as the cluster one does.

## Answered

The three questions that blocked schema decisions have come back from the NKS
team, confirmed against CEL rules and server 422s.

1. ~~**Is `cluster_id` actually immutable?**~~ **Yes.** Server 422s.
   `RequiresReplace` is right. The absence of an `immutable` annotation in the
   published spec proved nothing, as suspected.

2. ~~**Is `provisioning_mode` immutable?**~~ **Yes.** Server 422s. There is no
   in-place compute→reservation migration. `RequiresReplace` is right, and is
   not needlessly destructive.

3. ~~**Is `compute.flavor_id` mutable?**~~ **No** — and neither is
   `reservation.reservation_id`. Both are CEL-guarded and 422 on change. Both
   need `RequiresReplace`. This is the one the draft guessed wrong: it assumed
   in-place-but-disruptive, and had we shipped that, every flavour change would
   have planned clean and failed at apply.

That also surfaced something the draft did not ask about: **immutability is
mode-dependent**, and reservation pools additionally freeze `replicas` and
`taints`. See §Immutability.

4. ~~**Does a node pool carry its own `platformReleaseId`?**~~ **No.** Settled
   by the schema rather than by asking: `nodePoolStatusV1` has no `release`
   property, and `clusterReleaseStatusV1` is cluster-only. A pool inherits the
   cluster's release, reports nothing of its own, and this spec exposes no
   release attributes at all. The draft had six.

One reading to confirm: `name` was reported as immutable alongside the
reservation-pool answers, and this spec takes it as immutable in **both** modes,
matching the cluster (whose server message is "cluster names are immutable"). If
it turns out to be reservation-only, `name` should drop to a plain in-place
update on compute pools — cheap to change now, breaking to change after release.

## Open questions

11. **What does `taints[].propagation` mean for rolling?** The blocking one.
    `Always` versus `OnInitialization` plausibly decides whether a taint edit
    recycles the fleet or not, and the enum carries no description beyond "Taint
    propagation behavior". The taint is part of the machine template either way,
    which argues the pool rolls regardless — but if that were so, `Always` would
    have nothing to offer. Both the resource docs and the example depend on the
    answer. `_propagation` in the test plan is designed to settle it empirically
    if the NKS team's answer is slow.

12. ~~**Is the `nscale-sdk-go` bump scheduled?**~~ **Done.** [#74][pr74] landed
    on `main` and the provider is on v0.3.0, with the `common` removal absorbed.
    This was raised as a blocker and stopped being one before anyone had to act
    on it. Nothing gates implementation on the dependency now.

5. **What does a reservation-backed pool need in place first?** Whether the
   reservation must be `provisioned`, in the same region, and unclaimed is not
   something the spec states. Affects both the example and whether the
   acceptance test can be self-contained.

6. **Cluster deletion cascades to node pools.** Terraform normally destroys
   pools first via the `cluster_id` dependency, and a pool delete that 404s is
   already treated as success. Worth an explicit destroy test of the whole
   stack. Note the drain interaction: a cluster delete drains worker nodes the
   same way a pool delete does, so a PDB that cannot be satisfied blocks the
   cluster destroy too.

7. **Does a PDB-blocked roll surface usefully?** If the waiter times out
   mid-rollout, the pool is still rolling and the next apply resumes. Confirm
   that is what actually happens, and make the timeout error say it — "timed out
   waiting for the pool to settle; the rollout is still in progress, and may be
   blocked by a PodDisruptionBudget" is a materially better message than a bare
   deadline. The updated schema gives us a lever the draft did not have:
   `metadata.provisioningStatusDetail` and `healthStatusDetail`. Check whether
   either carries the drain-blocked reason, and quote it in the timeout error
   if so.

### Worker networking — owned by the cluster spec

The worker addressing, pod-egress SNAT and `Service type=LoadBalancer`
questions this spec previously carried as 8–10 now live in
[`kubernetes_cluster.md`](kubernetes_cluster.md) — see §Attaching an existing
corporate-routed network and its open question 8. That is the right home: the
arguments involved (`network_id`, `cluster_network.pod_cidr`,
`cluster_network.service_cidr`, `api_server.public_ip`) are all cluster
arguments, and the cluster spec has since answered the only schema-relevant part
of them — `loadBalancer`, `nodePort`, `securityGroup` and `ingress` appear
nowhere in the NKS API, so there is no cluster-level LoadBalancer surface to
model and nothing missing from either resource's schema.

One consequence is genuinely node-pool-side and stays here:

13. **Should the pool expose its workers' addresses?** If workers do take
    addresses from the attached network's prefix, a user who needs to firewall
    them corporate-side has nothing in Terraform to reference — this spec
    exposes no node address at all, and `nodePoolStatusV1` carries none either.
    Whether that is a gap depends on the answer to the cluster spec's question,
    which is why it is filed rather than designed. If the answer is yes and the
    API later exposes per-machine addresses, this is where they belong.

**11 is the only blocker.** It gates the schema and the safety warning that is
the loudest thing on the page. 13 is documentation-or-gap pending the cluster
spec's networking answers, and 5–7 can be settled empirically in Phase 4.
