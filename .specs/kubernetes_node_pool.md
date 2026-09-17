# Spec: nscale_kubernetes_node_pool

> The workers for an NKS cluster. Resource + data source.
>
> Source ticket: DX-2008 (blocked by DX-2007). Due Friday 2026-08-28.
>
> **This is what makes DX-2007 usable.** A `nscale_kubernetes_cluster` on its
> own is a control plane with nothing to schedule on.

## Correction: the last review round used the wrong source of truth

**Read this before the rest of the document.** The revision that "corrected the
node pool shape against the shipped SDK schema" checked against
`nscale-sdk-go` v0.3.0's vendored `kubernetes/openapi.yaml`. That is not what
this provider compiles against.

`internal/nks/` is generated from the **canonical** spec at
[nscaledev/openapi `nks-core/main/openapi.yaml`][canonical], which
`internal/nks/gen.go` explains is deliberately *ahead* of the SDK's vendored
copy — the whole reason the provider generates its own client rather than
consuming the SDK's. So all three of that round's "schema corrections" were
inverted, and each has been reversed here:

| Claim in the previous revision | Actually, per the canonical spec |
| --- | --- |
| `labels` does not exist | **It does.** `nodePoolLabelsV1`, `maxProperties: 64`, value `maxLength: 63`. Present on both `nodePoolSpecV1` and `nodePoolRequestSpecV1`. |
| A node pool has no platform release | **It has one.** `nodePoolStatusV1.release` is a `nodePoolReleaseStatusV1` with `appliedId`, `kubernetesVersion`, `deprecated`, `withdrawn`. |
| `taints[].propagation` is new and required | **No such field.** `nodePoolTaintV1` is `{key, value, effect}`; only `key` and `effect` are required. |

The DX-2008 ticket agrees with the canonical spec on all three — it asks for
`labels` and for "pinned release info (`status.release`: applied id, kubernetes
version, deprecated/withdrawn)", and never mentions propagation. The ticket and
the generated client were consistent with each other the whole time; only this
document diverged.

**Consequences.** Open question 11, described below as "the only blocker", does
not exist: there is no per-taint propagation choice, and `nodePoolTaintV1`'s own
description settles the semantics outright — a taint "is applied during worker
node registration, so it takes effect once during node initialization and is not
reconciled onto running nodes afterwards". The roll narrative is correct as
written, and now covers `labels` too, whose schema description says the same
thing in the same words.

The lesson is the one the previous round claimed to have learned and then got
wrong anyway: **verify against `internal/nks/nks.gen.go` and
`internal/nks/openapi.yaml`**, which are what the code is compiled against, not
against a version of the SDK the provider does not import.

[canonical]: https://raw.githubusercontent.com/nscaledev/openapi/main/nks-core/main/openapi.yaml

## Kind and package

- **Terraform type name:** `nscale_kubernetes_node_pool`
- **Resource, data source, or both:** both
- **Service package:** `internal/services/kubernetesnodepool/` (**new**)

Separate package rather than folding into `kubernetescluster/`, matching the
ticket and the API's own modelling — node pools are a top-level resource with
their own endpoints, not a sub-object of the cluster.

## Backing API

Verified against **`internal/nks/`** — this repo's own generated client and its
verbatim copy of the canonical spec. That is the source of truth, not
`nscale-sdk-go`'s vendored copy; see the Correction at the top for what checking
the wrong one cost.

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
`NodePoolReservationV1`, `NodePoolTaintV1`, `NodePoolReservationStatusV1`,
`NodePoolLabelsV1`, `NodePoolReleaseStatusV1`, `NodePoolProvisioningModeV1`.

The last three are the ones a previous revision declared non-existent. All are
in `internal/nks/nks.gen.go`.

Scope is inherited the same way as the cluster: no `projectId` on the write
path. A pool takes `clusterId`, and project/organization/region follow from the
cluster (which got them from its network). Same `Computed`-only treatment.
`nodePoolSpecV1` and `nodePoolRequestSpecV1` are both
`additionalProperties: false`, so the field list below is exhaustive.

## Schema shape, as generated

Verified against `internal/nks/nks.gen.go` and `internal/nks/openapi.yaml` —
the client this provider actually compiles against. See the Correction above for
why the previous revision's version of this section was wrong.

- **`labels`** is `nodePoolLabelsV1`, an object of string values with
  `maxProperties: 64`; each value has `maxLength: 63` and the Kubernetes
  label-value pattern. An empty value is explicitly legal and meaningful: the
  schema notes that "Kubernetes permits an empty value, so an omitted value
  registers the label with the empty string". Labels carry the same roll story
  as taints, in the same words: "not continuously reconciled onto running nodes,
  so a change applies to newly created workers only and rolls the pool's
  existing workers".

- **`status.release`** is a `nodePoolReleaseStatusV1`: `appliedId` and
  `kubernetesVersion` required, `deprecated`, `withdrawn`, `withdrawalReason`
  and `withdrawalMessage` optional. A pool has no `platformReleaseId` *argument*
  — it inherits the cluster's — but it does report which release it was pinned
  to. Note it also carries a `kubernetesVersion` the cluster's
  `clusterReleaseStatusV1` does not, distinct from `status.kubernetesVersion`
  (the version applied to workers): pinned versus observed.

- **`taints[]`** is `{key, value, effect}` with `key` and `effect` required.
  There is no `propagation`. `key` is `minLength: 1`, `maxLength: 317` with the
  qualified-name pattern; `value` is optional, `maxLength: 63`; `effect` is
  `NoSchedule` | `PreferNoSchedule` | `NoExecute`. The array caps at 64.

- **`replicas`** is `int` with **no `omitempty`** in the generated client, so `0`
  serialises correctly and scale-to-zero reaches the API. Bounds are 0 to
  2147483647. No wrapper needed — but a unit test pins it, because a future
  regeneration adding `omitempty` would silently turn scale-to-zero into a no-op
  ([playbook §1.6](../.claude/skills/tf-provider-feature/reference/playbook.md)).

- **`compute.flavorId`** is `minLength: 1`, `maxLength: 128`.
  **`reservation.reservationId`** carries no length bounds.

- Metadata carries `provisioningStatusDetail` and `healthStatusDetail`, each
  `{reason, message}`. Both are documented as derived and "never stored".

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
| `labels` | Map(String) | Optional | **`RequiresReplaceIf`** mode is `reservation` | Max 64 entries, value maxLength 63. Same terms as `taints`: in place on compute pools, and **rolls every worker**. |

`labels` is a plain `Optional` map rather than `Optional+Computed`: the API sets
no labels of its own, so omitted stays omitted and there is no server default to
read back. An **empty value is preserved**, not dropped — Kubernetes still
applies the label, so `""` and absent are different facts and a converter that
conflates them changes what lands on the node.

There is **no `taints[].propagation`**. The field does not exist in the canonical
spec (see the Correction at the top of this document), so there is no
per-taint behavioural choice to model and nothing for the provider to default.
`nodePoolTaintV1`'s own description fixes the semantics for every taint: applied
during node registration, never reconciled onto running nodes.

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
| `applied_platform_release_id` | String | no | `status.release.appliedId` — inherited from the cluster |
| `platform_release_kubernetes_version` | String | no | `status.release.kubernetesVersion` — the *pinned* version, versus `kubernetes_version` which is the *observed* one |
| `platform_release_deprecated` | Bool | no | `status.release.deprecated` |
| `platform_release_withdrawn` | Bool | no | `status.release.withdrawn` |

**A pool reports a platform release but takes no release argument.** It inherits
the cluster's, so moving a pool onto a newer release means upgrading the
cluster's `platform_release_id`. Naming mirrors the cluster's
(`applied_platform_release_id`, `platform_release_*`) so the two read alike.

`withdrawalReason` and `withdrawalMessage` are not exposed, matching the
cluster, which does not expose its equivalents either. `upgrade_available` and
`eligible_upgrade_target_ids` have no node-pool counterpart — eligibility is a
cluster-level decision.

`status.desiredReplicas` is **not** exposed: it duplicates the `replicas`
argument and would invite confusion about which is authoritative. The three
observed counts (`current`, `ready`, `upToDate`) are the ones that tell a user
something they don't already know.

`status.reservation.reservationId` is **not** exposed either, for the same
reason — it is the reservation the pool resolved to, which on a pool whose
`reservation.reservation_id` is immutable is always the one you asked for.
`placementId` is exposed because it is genuinely new information: the placement
the pool created inside that reservation.

`provisioning_status_detail` and `health_status_detail` are **not** exposed as
schema attributes, reversing an earlier draft's decision. Two reasons, and the
second is the one that decides it:

1. The API documents both as derived from status and "never stored". They are a
   diagnostic rendering, not resource state.
2. The cluster does not expose its equivalents. Two NKS resources disagreeing
   about whether status detail is part of the schema is a worse outcome than
   either choice on its own.

They are still used, where they actually help: `nkswait.FailureDetail` quotes
them in failure and timeout messages, which is what open question 7 was really
asking for — legibility when a roll is stuck, not an attribute to reference.
Exposing them on both resources remains a reasonable follow-up.

`modifiedTime`, `modifiedBy`, `createdBy` and `deletionTime` also exist on the
metadata. Skipped, because no other resource in this provider exposes their
equivalents — consistency across the provider matters more here than
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

### Labels roll the pool too

`labels` carries the identical story, and the schema says so in the identical
words: "not continuously reconciled onto running nodes, so a change applies to
newly created workers only and rolls the pool's existing workers so the new
labels take effect". Anywhere this document says a taint edit rolls the pool,
read it as taints *or* labels.

There is no `propagation` to complicate it. The previous revision built a long
open question on a field that does not exist, and concluded that the docs should
"keep saying a taint edit rolls the pool, because that is the answer that cannot
hurt anyone if it is wrong". That turns out to be the answer that is simply
right: `nodePoolTaintV1` states the taint "takes effect once during node
initialization and is not reconciled onto running nodes afterwards", so a change
can only land by replacing workers. No empirical test is needed to settle it,
though `_taints` asserts it anyway by watching `up_to_date_replicas` drop and
recover.

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

**Implemented as `internal/nkswait/`.** Not `internal/nks/wait/`, because
`internal/nks/gen.go` says that package is to be deleted wholesale when the
SDK-wide migration lands and its import path swapped for
`nscale-sdk-go/kubernetes` — a subpackage there would be collateral. Not
`internal/services/kubernetescommon/` either, since `internal/services/` holds
Terraform-exposed services.

The shared surface is `nkswait.Provisioned[T]` / `Deleted[T]`, generic over the
resource's own read type so callers get `*ClusterV1Read` or `*NodePoolV1Read`
back without a cast. Each caller supplies a `Target[T]` naming the resource
("cluster", "node pool"), how to read it, and how to project one read onto
`Status{Metadata, ObservedGeneration}` — which is all the decision table needs,
and is why one `classify` can serve a per-resource status type.

There is **one `Classify`, one `IsSettled` and one `FailureDetail`**, and the
10-row decision table's unit tests moved with them into
`internal/nkswait/nkswait_test.go`. The cluster's
`kubernetes_cluster_wait.go` now holds only its own timeouts and its `Target`
constructor.

Two incidental cleanups fell out of the same change. `isNotFound` became
`nkswait.IsNotFound`, shared by both resources' `Read` and `Delete`. And the
cluster's NKS-local tag helpers turned out to be unnecessary all along:
`tags.SDKTag` is a *structural* constraint, so `nks.Tag` satisfies it on shape
alone and `tftypes.TagMapValueMust` / `nscale.TagsToAPI[nks.Tag]` work directly.
Both copies are gone rather than a third being written for the node pool.

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
| `labels` | **in place** (rolls the pool) | replace |
| `description`, `tags` | in place | in place |

The right-hand column follows from §Rolls: a placement never rolls, so the API
refuses edits that could never take effect rather than accepting them silently.

`compute.flavor_id` is the one the first draft got wrong. It reasoned that a
flavour change *could* be an in-place update that happens to be maximally
disruptive, in the same category as taints. It is not — the API rejects it, so
it needs `RequiresReplace`, and a plan that shows a full replacement is telling
the truth.

### Implementation note

`replicas`, `taints` and `labels` need `RequiresReplaceIf` rather than a plain
`RequiresReplace` — `int64planmodifier`, `listplanmodifier` and
`mapplanmodifier` all provide it.
The condition reads `provisioning_mode` and fires only for `reservation`.
Because `provisioning_mode` itself forces replacement, reading it from config or
state is equivalent: a mode change replaces the resource regardless.

Three attributes sharing one predicate wants a single shared predicate rather
than three copies of the same logic. The three plan modifiers cannot share one
closure — `planmodifier.Int64Request`, `ListRequest` and `MapRequest` are
distinct types — but they can and do share `isReservationMode`, which is also
what the config validator reads. Both live in `provisioning_mode.go` for exactly
that reason: the mode gating the capacity block and the mode gating
immutability must never disagree.

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
- `labels` caps at 64 entries, each value maxLength 63 and regex-constrained. An
  empty value is legal and meaningful — the label still applies — so it must
  round-trip rather than being treated as absent.
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

  # Editing taints OR labels on a compute pool rolls every worker, one at a
  # time, draining each.
  taints = [{
    key    = "workload"
    value  = "general"
    effect = "PreferNoSchedule"
  }]

  labels = {
    tier = "standard"
  }
}

resource "nscale_kubernetes_node_pool" "gpu" {
  name              = "gpu"
  cluster_id        = nscale_kubernetes_cluster.main.id
  provisioning_mode = "reservation"
  replicas          = 2

  reservation = {
    reservation_id = nscale_reservation.gpu.id
  }

  # A reservation pool never rolls, so editing these — or labels, or replicas —
  # replaces the pool and re-claims the placement.
  taints = [{
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NoSchedule"
  }]
}
```

The reservation-backed pool tying into the existing `nscale_reservation`
resource is the part worth getting into the example — it is the cross-resource
story neither ticket exercises otherwise.

Shipped in `examples/kubernetescluster/` rather than a new directory, so the
example tells the whole story — a cluster is not usable without a pool, and
splitting them would leave neither runnable on its own. Both pools are
`count`-gated on a variable (`worker_flavor_id`, `gpu_reservation_id`) so the
example still applies where there is no reserved capacity.

## Test plan

**Unit:**

- `NodePoolV1Read` → model, both modes, all status sub-objects nil and populated.
- Create/update param builders produce identical specs from the same model
  (they share `NodePoolRequestSpecV1`, so the risk is one converter drifting).
- **`replicas = 0` serialises explicitly** — the omitempty canary.
- Taints round-trip including a taint with no `value` — a null value must not
  become a pointer to the empty string, which would register a valued taint
  where the user asked for a valueless one.
- Labels round-trip including an **empty value**, which Kubernetes still
  applies.
- Update re-sends taints and labels on a plain `replicas` change. NKS update is
  a full replacement, so this is what stops the most common edit a user makes
  from clearing them.
- Validator matrix: mode × (compute set?) × (reservation set?) — every
  combination, plus the unresolved-mode row that must report nothing.

**Acceptance:**

- `_basic` compute pool: create → check `ready_replicas` → `PlanOnly` → import.
- `_scale`: `replicas` 1 → 3 → in-place, ID unchanged, `PlanOnly` after.
- `_scaleToZero`: `replicas` → 0, in-place, `PlanOnly` after.
- `_taints`: add a taint on a compute pool → in-place, ID unchanged → `PlanOnly`.
  Assert `up_to_date_replicas` returns to `replicas`, which is what proves the
  roll finished rather than the waiter returning early. Also remove the taints
  again, to prove the attribute returns to null rather than an empty list.
- `_labels`: the same for labels, including the empty-valued one.
- `_replace`: one case per immutable field — `provisioning_mode`, `name`,
  `cluster_id`, `compute.flavor_id` — each plans a replace. `flavor_id` is the
  one that regressed from the first draft, so it is the one that matters most.
- `_reservationImmutable`: on a reservation pool, `replicas`, `taints` and
  `labels` each plan a **replace**, not an update. This is the
  `RequiresReplaceIf` predicate under test, and the failure mode if it is wrong
  is a plan that promises an in-place change and then 422s at apply.
- The **mirror image** matters as much: the same edits on a *compute* pool must
  plan as `Update`. Without that, a predicate stuck at `true` would pass the
  reservation test and quietly destroy compute pools on every scale. Asserted
  with `plancheck.ExpectResourceAction` inside `_scale` and `_taints`, which
  already pay for the apply, so it costs nothing extra.
- `_reservation`: gated on a reservation env var, skipped when absent.
- Data source by id agrees with the resource.
- Negative: mode/block mismatch fails at **plan**, asserted with `ExpectError`
  and `PlanOnly: true`.
- Negative: an invalid taint `effect`, an out-of-range `replicas`, and an
  unrecognised `provisioning_mode` all fail at **plan**, with `ExpectError` and
  `PlanOnly: true`.

`_reservationImmutable` cannot be run as `PlanOnly`, contrary to this
document's earlier hope: `terraform-plugin-testing` rejects
`ConfigPlanChecks.PreApply` combined with `PlanOnly`, and
`ExpectNonEmptyPlan` alone cannot distinguish an update from a replacement —
which is the entire assertion. So it applies each edit for real and pays for
three pool rebuilds, gated behind `NSCALE_TEST_NKS_RESERVATION_ID` so it only
runs where reservation capacity was deliberately supplied. All three predicates
are exercised rather than one standing in for the others, because they are three
separate closures and a copy-paste slip in any of them is live.

**Cost:** each pool is real compute. Cheaper than a control plane, but the
suite still wants a separate CI lane, as the cluster one does.

The tests attach pools to a **pre-existing cluster** (`NSCALE_TEST_NKS_CLUSTER_ID`)
rather than creating one, on the same reasoning the cluster suite uses for
networks: a control plane took a measured 32 minutes, so creating one per test
case would put this package into the hours for coverage
`kubernetescluster` already provides. The worker flavor
(`NSCALE_TEST_NKS_FLAVOR_ID`) is given explicitly too, because
`nscale_instance_flavor` looks up by ID only — there is no "any small flavor in
this region" query to write. Documented in TESTING.md.

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

4. ~~**Does a node pool carry its own `platformReleaseId`?**~~ **Half right,
   and the half it got wrong was checked against the wrong document.** A pool
   takes no `platformReleaseId` *argument* — it inherits the cluster's, which is
   the part that held. But it does **report** one: `nodePoolStatusV1.release`
   exists in the canonical spec and is a `nodePoolReleaseStatusV1`. Four
   computed attributes come back (see §Computed), which is also what the ticket
   asked for. See the Correction at the top.

One reading to confirm: `name` was reported as immutable alongside the
reservation-pool answers, and this spec takes it as immutable in **both** modes,
matching the cluster (whose server message is "cluster names are immutable"). If
it turns out to be reservation-only, `name` should drop to a plain in-place
update on compute pools — cheap to change now, breaking to change after release.

## Open questions

11. ~~**What does `taints[].propagation` mean for rolling?**~~ **Void — the
    field does not exist.** It appears nowhere in the canonical spec, and was
    read off `nscale-sdk-go` v0.3.0's stale vendored copy. `nodePoolTaintV1` is
    `{key, value, effect}`. The semantics the question was trying to pin down
    are stated outright in the taint's own schema description: applied during
    node registration, not reconciled onto running nodes. So a taint or label
    edit rolls the pool, unconditionally, and the docs say so without hedging.
    See the Correction at the top of this document.

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

**Nothing blocks implementation.** 11, described in the previous revision as
"the only blocker", was void. 12 was already done. 5–7 are Phase 4 questions
that do not gate the schema:

- **5** (reservation prerequisites) is why every reservation acceptance test is
  gated behind `NSCALE_TEST_NKS_RESERVATION_ID` and skips when it is absent.
- **6** (cluster deletion cascade) is handled: a pool delete that 404s is
  treated as success, and Terraform destroys pools before the cluster via the
  `cluster_id` dependency. Worth an explicit whole-stack destroy in Phase 4.
- **7** (PDB-blocked roll legibility) is partly addressed. `nkswait.Provisioned`
  now distinguishes a timeout from a failure: on a non-failed timeout it quotes
  the last observed status detail and says the operation may still be running
  and that the next apply resumes it. Whether
  `provisioningStatusDetail`/`healthStatusDetail` actually carry a drain-blocked
  reason is still unverified against a real stuck roll.

13 remains documentation-or-gap pending the cluster spec's networking answers.

## What shipped

Implemented on branch `adam/dx-2008-a`:

| | |
| --- | --- |
| Package | `internal/services/kubernetesnodepool/` |
| Shared waiter | `internal/nkswait/` — one `Classify`, used by cluster and pool |
| Registered | `nscale_kubernetes_node_pool` resource + data source in `internal/provider/provider.go` |
| Docs | `website/docs/{r,d}/kubernetes_node_pool.html.markdown` |
| Example | `examples/kubernetescluster/` — cluster plus a compute pool and a reservation-backed pool |
| Schema baseline | regenerated; both types present |

Deviations from this spec as originally written, all recorded above: `labels` is
in, the four `platform_release_*`/`applied_platform_release_id` attributes are
in, `taints[].propagation` is out, and the two `*_status_detail` attributes are
out. The first three are forced by the canonical spec; the fourth is a judgement
call for symmetry with the cluster.

Timeouts shipped as proposed — 30m create, 60m update, 60m delete — and are
**still unmeasured**. They are reasoned from the CAPI rollout semantics in
§Rolls rather than observed, which is the one respect in which they are weaker
than the cluster's measured 60/90/30. Record real numbers in
`kubernetes_node_pool_wait.go` once a staging roll has been timed.
