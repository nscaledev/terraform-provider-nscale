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
| `name` | String | Required | — | `metadata.name`; `NameValidator()` |
| `cluster_id` | String | Required | **`RequiresReplace`** | `spec.clusterId`. A pool cannot move clusters — see Open questions. |
| `provisioning_mode` | String | Required | **`RequiresReplace`** | `compute` \| `reservation`. `stringvalidator.OneOf`. See Open questions. |
| `replicas` | Int64 | Required | — | `spec.replicas`. **min 0**, max 2147483647. Scale-to-zero is legal. |
| `description` | String | Optional | — | `metadata.description` |
| `tags` | Map(String) | Optional+Computed | — | `metadata.tags`; `NoReservedPrefix` |
| `compute` | SingleNested | Optional | — | Required iff mode is `compute` |
| `compute.flavor_id` | String | Required within block | — | `spec.compute.flavorId` |
| `reservation` | SingleNested | Optional | — | Required iff mode is `reservation` |
| `reservation.reservation_id` | String | Required within block | — | `spec.reservation.reservationId` |
| `taints` | ListNested | Optional | — | Max 64. **Disruptive** — see §Taints. |
| `taints[].key` | String | Required | — | maxLength 317, K8s qualified-name pattern |
| `taints[].value` | String | Optional | — | maxLength 63, K8s label-value pattern |
| `taints[].effect` | String | Required | — | `NoSchedule` \| `PreferNoSchedule` \| `NoExecute` |
| `labels` | Map(String) | Optional | — | Max 64 entries, value maxLength 63. **Disruptive** — see §Taints. |

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

The node pool pins its own platform release, distinct from the cluster's —
hence `applied_platform_release_id` and its version here as well as on the
cluster.

### Cross-field validation

`ConfigValidators` on the resource, enforcing at **plan time**:

- `provisioning_mode = "compute"` → `compute` set, `reservation` unset
- `provisioning_mode = "reservation"` → `reservation` set, `compute` unset

The ticket's acceptance criterion is explicit that mis-configuration must fail
at plan/validate, not apply. Prefer the framework's
`resourcevalidator.Conflicting` / `RequiredTogether` where they fit; a small
custom validator otherwise, following the shape of
`computecluster/validator.go`
([playbook §3.3](../.claude/skills/tf-provider-feature/reference/playbook.md)).

## Taints and labels are disruptive — say so loudly

Straight from the spec:

> Taints are not continuously reconciled onto running nodes, so a change applies
> to newly created workers only **and rolls the pool's existing workers** so the
> new taints take effect.

Same wording for labels. So editing either **replaces every node in the pool**.

This must appear in the `MarkdownDescription` of both attributes, not just the
prose docs — it is the one place a user will see it before they apply. A user
who thinks they are adding a label and instead recycles their entire fleet has
been badly served.

Terraform will show these as ordinary in-place updates, because that is what
they are at the API level. We cannot make the plan itself warn, which is exactly
why the attribute description has to carry the weight.

## Lifecycle

- **Create:** `POST` → `201`, async.
- **Read:** `GET` by id. 404 → `RemoveResource`.
- **Update:** `PUT`, full replacement, async. Scaling `replicas` is the common
  case and **must not disturb taints or labels** — build the whole
  `nodePoolUpdateSpecV1` from config every time.
- **Delete:** `DELETE` → `202`, async, poll to not-found.

### Waiters — reuse, don't duplicate

The settledness rules are **identical** to the cluster: `observedGeneration >=
generation` before trusting status, `provisioned` + `healthStatus: error` is a
failure, delete polls to not-found with a post-deprovisioning error being
terminal.

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

Node pools should be **faster than a control plane** — they are worker VMs
joining an existing cluster, not a new control plane. But this is an assumption,
and DX-2007 taught us that guessing here is exactly how you ship a default that
fails on a slow day.

Proposed starting point, **to be corrected by measurement in Phase 4**:

| | Create | Update | Delete |
| --- | --- | --- | --- |
| Proposed | 30m | 30m | 30m |

Update gets the same as create because a taint/label edit rolls every worker —
effectively a full re-provision of the pool. Scaling up is bounded by the same
per-node time. Record the observed numbers in the code comment as DX-2007 did.

## Immutability

- `cluster_id` — a pool belongs to one cluster.
- `provisioning_mode` — compute and reservation are different capacity sources.

Everything else is in-place: `name`, `description`, `tags`, `replicas`,
`compute.flavor_id`, `reservation.reservation_id`, `taints`, `labels`.

**Both of these are assumptions, not confirmed.** See Open questions.

`compute.flavor_id` being mutable is worth its own thought: changing the flavour
of a running pool means replacing every node. If the API accepts it, it is an
in-place update that happens to be maximally disruptive — the same category as
taints, and it needs the same warning in its description. If the API rejects it,
it needs `RequiresReplace`.

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

- **`replicas` minimum is 0.** Scale-to-zero is legal, and worth an acceptance
  test — it is both a plausible user action and the `omitempty` canary.
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

  # Changing taints rolls every worker in this pool.
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
- `_taints`: add a taint → in-place → `PlanOnly`.
- `_replace`: change `provisioning_mode` → plans a replace.
- `_reservation`: gated on a reservation env var, skipped when absent.
- Data source by id agrees with the resource.
- Negative: mode/block mismatch fails at **plan**, asserted with `ExpectError`
  and `PlanOnly: true`.

**Cost:** each pool is real compute. Cheaper than a control plane, but the
suite still wants a separate CI lane, as the cluster one does.

## Open questions

1. **Is `cluster_id` actually immutable?** Near-certain, but the NKS spec carries
   **no `immutable` annotation on it** — the only one in the whole document is
   on the cluster's `networkId`. DX-2007 taught us that the annotation's absence
   proves nothing either way, since PUT is full replacement and every field
   appears in the update shape regardless. Worth one line to the NKS team rather
   than an assumption, because `RequiresReplace` is a breaking change to add or
   remove later.

2. **Is `provisioning_mode` immutable?** Same reasoning, less obvious answer.
   Switching a pool from compute to reservation could plausibly be a supported
   in-place migration. If it is, `RequiresReplace` would be needlessly
   destructive — and destructive-by-default is the harder mistake to walk back.

3. **Is `compute.flavor_id` mutable, and if so does it roll the pool?** Governs
   whether it needs `RequiresReplace` or a disruption warning.

4. **Do node pools need their own platform release?** `status.release` exists on
   the pool, but `nodePoolRequestSpecV1` has **no** `platformReleaseId` field —
   so the pool appears to inherit the cluster's. Confirm, because if a pool can
   be pinned independently, an argument is missing from this spec.

5. **What does a reservation-backed pool need in place first?** The example
   references `nscale_reservation`, but whether the reservation must be
   `provisioned`, in the same region, and unclaimed is not something the spec
   states. Affects both the example and whether the acceptance test can be
   self-contained.

6. **Cluster deletion cascades to node pools** (the cluster ticket says delete
   removes "a cluster and its node pools"). What does Terraform see if a user
   destroys a cluster while pool resources still exist in state? Ideally the
   pool's delete gets a 404 and treats it as success — that is already the
   behaviour — but the *ordering* matters: Terraform will normally destroy pools
   first via the `cluster_id` dependency. Worth an explicit destroy test of the
   whole stack.

Questions 1–3 are the ones that block schema decisions and should go to the NKS
team as a batch. 4–6 can be settled empirically in Phase 4.
