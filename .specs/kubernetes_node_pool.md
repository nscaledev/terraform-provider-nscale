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

Same service, same client, same base URL as DX-2007. **No new plumbing** —
`internal/nks` already has every operation and type, and
`Client.RequireNKS()` already exists.

| Operation | Verb + path | Response |
| --- | --- | --- |
| `listNodePools` | `GET /api/v1/nodepools` | `200 []NodePoolV1Read` |
| `createNodePool` | `POST /api/v1/nodepools` | `201 NodePoolV1Read` (async) |
| `getNodePool` | `GET /api/v1/nodepools/{nodePoolID}` | `200 NodePoolV1Read` |
| `updateNodePool` | `PUT /api/v1/nodepools/{nodePoolID}` | `200 NodePoolV1Read` (async, full replacement) |
| `deleteNodePool` | `DELETE /api/v1/nodepools/{nodePoolID}` | `202` (async) |

**Types:** `NodePoolV1Read/Create/Update`, `NodePoolRequestSpecV1` (create and
update are both aliases of this — one shape for both writes),
`NodePoolSpecV1`, `NodePoolStatusV1`, `NodePoolComputeV1`,
`NodePoolReservationV1`, `NodePoolTaintV1`, `NodePoolLabelsV1`,
`NodePoolReleaseStatusV1`, `NodePoolReservationStatusV1`.

Scope is inherited the same way as the cluster: no `projectId` on the write
path. A pool takes `clusterId`, and project/organization/region follow from the
cluster (which got them from its network). Same `Computed`-only treatment.

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
| `compute.flavor_id` | String | Required within block | **`RequiresReplace`** | `spec.compute.flavorId`. Immutable server-side (422). |
| `reservation` | SingleNested | Optional | — | Required iff mode is `reservation` |
| `reservation.reservation_id` | String | Required within block | **`RequiresReplace`** | `spec.reservation.reservationId`. Immutable server-side (422). |
| `taints` | ListNested | Optional | **`RequiresReplaceIf`** mode is `reservation` | Max 64. In place on compute pools, and **rolls every worker** — see §Rolls. |
| `taints[].key` | String | Required | — | maxLength 317, K8s qualified-name pattern |
| `taints[].value` | String | Optional | — | maxLength 63, K8s label-value pattern |
| `taints[].effect` | String | Required | — | `NoSchedule` \| `PreferNoSchedule` \| `NoExecute` |
| `labels` | Map(String) | Optional | **`RequiresReplaceIf`** mode is `reservation` | Max 64 entries, value maxLength 63. Same as `taints`. |

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
| `applied_platform_release_id` | String | no | `status.release.appliedId` |
| `platform_release_kubernetes_version` | String | no | `status.release.kubernetesVersion` |
| `platform_release_deprecated` | Bool | no | `status.release.deprecated` |
| `platform_release_withdrawn` | Bool | no | `status.release.withdrawn` |
| `placement_id` | String | no | `status.reservation.placementId` — reservation mode only |

`status.desiredReplicas` is **not** exposed: it duplicates the `replicas`
argument and would invite confusion about which is authoritative. The three
observed counts (`current`, `ready`, `upToDate`) are the ones that tell a user
something they don't already know.

The node pool **reports** its own platform release, separately from the
cluster's — hence `applied_platform_release_id` and its version here as well as
on the cluster. Whether a pool can be *pinned* independently is still open
(question 4): the release is read-only in this spec either way.

### Cross-field validation

`ConfigValidators` on the resource, enforcing at **plan time**:

- `provisioning_mode = "compute"` → `compute` set, `reservation` unset
- `provisioning_mode = "reservation"` → `reservation` set, `compute` unset

`provisioning_mode` now does double duty: it gates which capacity block is
valid, *and* it gates whether `replicas`, `taints` and `labels` force
replacement (see §Immutability). Both readings of it should come from the same
place in the code, so the two never disagree about what mode a pool is in.

The ticket's acceptance criterion is explicit that mis-configuration must fail
at plan/validate, not apply. Prefer the framework's
`resourcevalidator.Conflicting` / `RequiredTogether` where they fit; a small
custom validator otherwise, following the shape of
`computecluster/validator.go`
([playbook §3.3](../.claude/skills/tf-provider-feature/reference/playbook.md)).

## Rolls: what a taint or label edit actually does

### Compute pools

A compute pool is a plain CAPI v1.14 `MachineDeployment`. Editing `taints` or
`labels` re-hashes the `KubeadmConfigTemplate` name, which changes the
`MachineDeployment`'s template ref, which makes CAPI roll every worker in the
pool. So editing either **replaces every node in the pool** — but it does so as
a controlled rolling update, not a stampede:

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

### Reservation pools

A reservation pool is backed by a placement, and **a placement never rolls**.
CAPNS documents no in-place resize, no rolling update and no per-node drain for
that backend. Nothing a template change would express can take effect, which is
why `replicas`, `taints`, `labels` and the platform release are all immutable in
this mode — the API rejects the edit rather than accepting a change that could
never land.

For the provider this means a reservation pool is **effectively immutable**:
only `description` and `tags` update in place. Everything else forces
replacement, and replacement releases and re-claims the placement.

### What the schema has to say

The disruption must appear in the `MarkdownDescription` of `taints` and
`labels`, not just the prose docs — it is the one place a user sees it before
they apply. On a compute pool Terraform shows a taint edit as an ordinary
in-place update, because that is what it is at the API level; the plan cannot
warn, so the attribute description carries the weight. On a reservation pool the
plan *does* warn, because `RequiresReplaceIf` fires.

## Lifecycle

- **Create:** `POST` → `201`, async.
- **Read:** `GET` by id. 404 → `RemoveResource`.
- **Update:** `PUT`, full replacement, async. On a compute pool the common case
  is scaling `replicas`, which **must not disturb taints or labels** — build the
  whole `nodePoolUpdateSpecV1` from config every time. On a reservation pool
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
| `labels` | **in place** (rolls the pool) | replace |
| platform release | in place | replace |
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
`mapplanmodifier` all provide it. The condition reads `provisioning_mode` and
fires only for `reservation`. Because `provisioning_mode` itself forces
replacement, reading it from config or state is equivalent: a mode change
replaces the resource regardless.

Three attributes sharing one predicate wants a single helper
(`requiresReplaceIfReservation`) rather than three copies of the same closure.

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
- `taints` max 64; `labels` max 64 entries.
- Taint key maxLength 317, value maxLength 63, both regex-constrained. Mirror
  the patterns as schema validators so a bad key fails at plan time.
- List `name` filters are exact, case-sensitive, and **not unique across
  clusters** — data source stays id-based.
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

  # Editing these on a compute pool rolls every worker, one at a time.
  labels = {
    "workload" = "general"
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

  # A reservation pool never rolls, so editing these — or replicas — replaces
  # the pool and re-claims the placement.
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

## Test plan

**Unit:**

- `NodePoolV1Read` → model, both modes, all status sub-objects nil and populated.
- Create/update param builders produce identical specs from the same model
  (they share `NodePoolRequestSpecV1`, so the risk is one converter drifting).
- **`replicas = 0` serialises explicitly** — the omitempty canary.
- Taints round-trip including a taint with no `value`.
- Labels round-trip including an empty-string value (Kubernetes permits it, and
  the spec calls it out explicitly).
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
- `_reservationImmutable`: on a reservation pool, `replicas`, `taints` and
  `labels` each plan a **replace**, not an update. This is the
  `RequiresReplaceIf` predicate under test, and the failure mode if it is wrong
  is a plan that promises an in-place change and then 422s at apply.
- `_reservation`: gated on a reservation env var, skipped when absent.
- Data source by id agrees with the resource.
- Negative: mode/block mismatch fails at **plan**, asserted with `ExpectError`
  and `PlanOnly: true`.

`_reservationImmutable` can run as `PlanOnly` against a single created pool —
it asserts on plan output, so it does not need to pay for three rebuilds.

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
mode-dependent**, and reservation pools additionally freeze `replicas`,
`taints`, `labels` and the platform release. See §Immutability.

One reading to confirm: `name` was reported as immutable alongside the
reservation-pool answers, and this spec takes it as immutable in **both** modes,
matching the cluster (whose server message is "cluster names are immutable"). If
it turns out to be reservation-only, `name` should drop to a plain in-place
update on compute pools — cheap to change now, breaking to change after release.

## Open questions

4. **Does a node pool carry its own `platformReleaseId` on the write path?**
   The evidence conflicts. `nodePoolRequestSpecV1` has **no**
   `platformReleaseId` field, which says the pool inherits the cluster's — but
   the immutability answers describe `platformReleaseID` as immutable *on
   reservation pools*, which implies a per-pool field exists somewhere below the
   public API. Either the field is internal and `status.release` simply reports
   what was inherited, or an argument is missing from this spec. One line to the
   NKS team settles it; until then the spec exposes the release read-only.

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
   deadline.

### Worker networking — belongs to the cluster, felt on the pool

These have been asked by users and are not answered anywhere yet. The
*arguments* involved (`network_id`, `pod_cidr`, `service_cidr`,
`api_server.public_ip`) all live on `nscale_kubernetes_cluster`, so the answers
belong in the cluster docs — but every one of them is observed on the workers,
which is why they are tracked here too.

For the usual deployment — an existing corporate-routed Nscale network, with
separate pod and Service CIDRs:

```yaml
spec:
  networkId: <corporate-routed network>
  clusterNetwork:
    podCidr: 100.65.0.0/16
    serviceCidr: 172.20.0.0/16
  apiServer:
    publicIP: false
```

8. **Do workers get addresses from the network's own prefix** (e.g. a
   `7.247.16.0/20`) while pod and Service addresses stay in the separate CIDRs
   above? If so, `nscale_kubernetes_node_pool` should expose the worker
   addresses, or at least say where they come from — today the spec exposes no
   node address at all, and a user who needs to firewall their workers has
   nothing to reference.

9. **Is pod egress SNAT'd to the worker's address?** When a pod reaches a
   corporate `10.0.0.0/8` route, does NKS SNAT it to its worker's `7.247.x.x`
   address? This decides whether corporate ACLs can be written against the
   network prefix or have to admit the pod CIDR as well. It is the single
   question most likely to be asked in a review of the docs.

10. **Service `type=LoadBalancer`: private and public.** Can NKS provision both?
    How is one selected — a Service annotation, or something cluster-level? And
    are security-group and NodePort rules managed automatically as a
    consequence? If selection is per-Service, this is outside Terraform's scope
    entirely and the docs should say so plainly and point at the NKS
    documentation, rather than leaving a user to guess there is a missing
    provider argument. If it is cluster-level, the cluster resource is missing
    an argument.

8–10 go to the NKS team with 4; they gate documentation, and 10 may gate a
cluster schema decision. 5–7 can be settled empirically in Phase 4.
