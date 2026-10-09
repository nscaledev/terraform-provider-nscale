# Spec: nscale_placement

> Allocates a set of hosts from a `nscale_reservation` and drives pinned Region
> server creation for each selected host. Consumes capacity from the reservation;
> the network determines the InfiniBand partition boundary (all hosts in a
> placement share one partition key).

## Kind and package

- **Terraform type name:** `nscale_placement`
- **Resource, data source, or both:** both
- **Service package:** `internal/services/reservation/` (same package as `nscale_reservation`)

## Backing API

- **Base service URL env:** `NSCALE_RESERVATION_SERVICE_API_ENDPOINT` (shared with reservation)
- **List endpoint:** `GET /api/v2/placements` (not used by the resource)
- **Read endpoint:** `GET /api/v2/placements/{placementID}` → `200 PlacementV2Read`
- **Create endpoint:** `POST /api/v2/placements` → `202 PlacementV2Read` (async)
- **Update endpoint:** `PUT /api/v2/placements/{placementID}` → `200 PlacementV2Read`. Only `spec.serverSpec.imageId` and `spec.updateStrategy` may change; any other spec change is rejected with `400`. Metadata is ignored.
- **Delete endpoint:** `DELETE /api/v2/placements/{placementID}` → `202` (async; deletes all Region servers it created)
- **OpenAPI types used:** `reservationapi.PlacementV2Read`, `PlacementV2Create`, `PlacementV2CreateSpec`, `PlacementV2Update`, `PlacementV2Spec`, `PlacementConstraintsV2`, `PlacementServerSpecV2`, `PlacementServerNetworkingV2`, `PlacementPolicyV2`, `WhenUnsatisfiableV2`.

Create body needs no org/project — placement is scoped through its reservation.
Metadata read is **project-scoped** → project it onto `nscale.ResourceStatus` in
the get helper via the package's `statusOf`.

## Attributes

| Name | Type | R/O/C | Plan modifiers | Notes |
| --- | --- | --- | --- | --- |
| `id` | String | Computed | `UseStateForUnknown` | `metadata.id` |
| `name` | String | Required | `RequiresReplace` | `metadata.name`; `NameValidator()` |
| `description` | String | Optional | `RequiresReplace` | `metadata.description`; removal replaces too, as the PUT ignores metadata |
| `tags` | Map(String) | Optional+Computed | `RequiresReplaceIfConfigured` | `NoReservedPrefix` |
| `reservation_id` | String | Required | `RequiresReplace` | `spec.reservationId` (create) / `status.reservationId` (read) |
| `network_id` | String | Required | `RequiresReplace` | `spec.networkId` (create) / `status.networkId` (read) |
| `host_count` | Int64 | Required | `RequiresReplace` | `spec.count`; `>= 1`. Non-pointer int, no `omitempty` |
| `constraints` | SingleNested | Required | per attribute (see below) | `spec.constraints` (see below) |
| `server_spec` | SingleNested | Required | per attribute (see below) | `spec.serverSpec` (see below) |
| `update_strategy` | SingleNested | Optional+Computed | `UseStateForUnknown` | `spec.updateStrategy` (see below). **Updates in place.** |
| `region_id` | String | Computed | `UseStateForUnknown` | `status.regionId` |
| `ready_host_count` | Int64 | Computed | — | `status.readyHostCount` (optional in API) |
| `updated_host_count` | Int64 | Computed | — | `status.updatedHostCount`; trust only when `status.statusCurrent` |
| `drifted_host_count` | Int64 | Computed | — | `status.driftedHostCount`; trust only when `status.statusCurrent` |
| `project_id` | String | Computed | `UseStateForUnknown` | `metadata.projectId` |
| `creation_time` | String | Computed | `UseStateForUnknown` | `metadata.creationTime` |
| `provisioning_status` | String | Computed | `UseStateForUnknown` | `metadata.provisioningStatus` |

### `constraints` (SingleNestedAttribute, required)

| Name | Type | R/O/C | Notes |
| --- | --- | --- | --- |
| `policy` | String | Required | `OneOf("pack","spread")`; `RequiresReplace`. Pack fills domains sequentially; spread distributes evenly. |
| `max_skew` | Int64 | Optional | `>= 1`; only meaningful when `policy = spread`. `*int omitempty` in API. |
| `min_domains` | Int64 | Optional | `>= 1`; only meaningful when `policy = spread`; must be `<= count`. `*int omitempty`. |
| `when_unsatisfiable` | String | Optional | `OneOf("fail","bestEffort")`. `*enum omitempty`. |

All sub-fields round-trip via `spec.constraints` on read, so plain `Optional` (not Computed) is correct.

### `server_spec` (SingleNestedAttribute, required) — Region server options per pinned host

| Name | Type | R/O/C | Notes |
| --- | --- | --- | --- |
| `image_id` | String | Required | `spec.serverSpec.imageId`; UUID semantic equality, and `uuidtype.UseStateForSameUUID` keeps the stored spelling at plan time. **Updates in place.** |
| `ssh_certificate_authority_id` | String | Optional | `*string omitempty`; `RequiresReplace` |
| `user_data` | String | Optional | base64-encoded; `Base64Validator{}`. API type `*[]byte`; `RequiresReplace` |
| `networking` | SingleNested | Optional | see below; replaces when added or removed |

### `update_strategy` (SingleNestedAttribute, optional + computed)

| Name | Type | R/O/C | Notes |
| --- | --- | --- | --- |
| `type` | String | Required | `OneOf("Manual","RollingUpdate")`; API default `Manual`, always materialised on read |
| `rolling_update` | SingleNested | Optional | `{ max_unavailable }`; echoed on read only when sent |
| `rolling_update.max_unavailable` | String | Optional | count (`"1"`) or percentage (`"25%"`); required when `type = "RollingUpdate"` (`ValidateConfig`); API rejects one resolving below 1 |

Optional+Computed with `UseStateForUnknown`, so the API's `Manual` default does
not diff. Terraform still proposes null for an unconfigured strategy, because
`type` is not computed and a non-null prior `type` looks configured, and the
framework then marks every unconfigured computed attribute unknown; see
"Plan" below. Removing the block keeps the stored strategy, matching the API, which
keeps it when a request omits `updateStrategy`; `type = "Manual"` switches back.
`ValidateConfig` rejects `type = "RollingUpdate"` when `rolling_update` or its
`max_unavailable` is null, so the API's default budget of `"1"` is never reached
from Terraform; values unknown at plan time pass validation.

### `server_spec.networking` (SingleNestedAttribute, optional)

| Name | Type | R/O/C | Notes |
| --- | --- | --- | --- |
| `enable_public_ip` | Bool | Optional | API `PublicIP *bool` — pointer, so null/false distinguishable; **no omitempty-bool risk** |
| `security_group_ids` | List(String) | Optional | `*[]string` |
| `allowed_source_addresses` | List(String) | Optional | `*[]string` |

Nested-attribute choice rationale (playbook §1.3): `SingleNestedAttribute` for the
fixed-shape `constraints` / `server_spec` / `networking` sub-objects (not `Block`,
per the framework convention for new resources). Lists (not sets) for the string
collections to mirror the instance resource and keep ordering deterministic.

## Lifecycle

- **Create:** async (202). Record id, then `CreateStateWatcher` polls to `provisioned`/`error`.
- **Read:** GET by id; 404 → remove from state. `reservation_id`/`network_id` read back from `status`.
- **Update:** image and update strategy, via `UpdateAndWait` (not the operation-tag watcher: the PUT ignores tags).
  Read-modify-write: GET the placement, lay the planned image and (when known) update strategy over its `spec`, PUT it. The body is never built
  from the model — the model holds unset networking lists as `[]` where the API renders them absent, and the API
  rejects any spec that differs from its own rendering. A `409` (another writer between the read and the write) repeats the
  read-modify-write, up to 3 attempts. When the overlaid spec equals the read (only timeouts, or
  only the image UUID's spelling, changed) nothing is sent and nothing waited for. Then poll until `status.statusCurrent` is true,
  `provisioningStatus` is `provisioned`, and the counts account for every server (the service can report `statusCurrent`
  with every count zeroed when a reconcile stops before observing the servers): under `Manual`, `updatedHostCount` plus
  `driftedHostCount` equals `count`; under `RollingUpdate`, `updatedHostCount` equals `count` and `driftedHostCount`
  and `inFlightCount` are 0. `provisioningStatus = error` under either strategy, or `stalledCount > 0` under
  `RollingUpdate`, fails the wait once it has persisted for a grace period (rebuild rejections are
  retried by the service, and a placement errored before the update reads error until the update is observed).
- **Delete:** async (202). `DeleteStateWatcher` polls until 404. Deletes all backing Region servers.

## Provisioning states (async)

Same shared mechanism as reservation: `metadata.provisioningStatus`, target
`provisioned`/`error`. `status.readyHostCount` is informational only (not used as the readiness gate).

## Immutability

Every configurable field but `server_spec.image_id` and `update_strategy` requires replacement: `name`,
`description`, `tags`, `reservation_id`, `network_id`, `host_count`, every
`constraints` field, and `server_spec`'s `ssh_certificate_authority_id`,
`user_data` and `networking`.

Replacement is set on the attributes, not on the `constraints`/`server_spec`/
`networking` objects: whenever a resource changes, the framework marks
unconfigured computed attributes unknown before plan modifiers run, and an
object holding an unknown never equals its prior state, so an object-level
`RequiresReplace` would replace on every image change. The objects replace only when
wholly unknown at plan time, because the framework does not run the plan
modifiers of an unknown object's attributes.

## Plan

`ModifyPlan` calls `nscale.KeepStateWhenUnchanged`: when putting the
unconfigured computed values the framework marked unknown back to their prior
values leaves the plan equal to prior state, it plans prior state, so no change.
This covers an unconfigured `update_strategy` and a respelled `image_id`. Any
real change keeps them unknown: the counts in particular come from the read
the update returns.

## Write-once / sensitive fields

None. (`user_data` is user-supplied, not server-returned-once; it round-trips via spec.)

## Import

- **Shape:** passthrough ID.
- **Recoverable:** all. `timeouts` ignored in `ImportStateVerify`.
- **Unrecoverable:** none.

## Known API constraints

- **Conflict (`409`)** on create: capacity already consumed / overlapping allocation.
- **`404`** if `reservation_id` or `network_id` does not exist.
- `min_domains <= count` and `max_skew`/`min_domains` only apply to `spread`; server returns 400 otherwise — surface verbatim.
- `count >= 1`.

## Examples

```hcl
resource "nscale_placement" "workers" {
  name           = "training-workers"
  reservation_id = nscale_reservation.training.id
  network_id     = nscale_network.training.id
  host_count     = 8

  constraints = {
    policy             = "spread"
    max_skew           = 1
    min_domains        = 3
    when_unsatisfiable = "fail"
  }

  server_spec = {
    image_id = var.image_id

    networking = {
      security_group_ids = [nscale_security_group.training.id]
    }
  }
}
```

## Test plan

- **Unit converters:** `NewPlacementModel` (constraints with/without optional fields, networking nil/populated, user_data base64 round-trip, status fields); `NscalePlacementCreateParams` (constraints + serverSpec + networking expand, pointer-slice handling for empty lists).
- **Unit update:** `NscalePlacementUpdateParams` keeps the read spec bar the image; `placementUpdateProgress` per strategy; the wait rides out a transient error and fails a persistent one; `placementUpdateAndWait` against an `httptest` API; planning an image change through the provider server does not replace, other `server_spec`/`constraints` changes do.
- **Unit validation:** `validatePlacementConstraints`; `validatePlacementUpdateStrategy` rejects `RollingUpdate` without a budget and defers unknowns; the provider server's `ValidateResourceConfig` reports it.
- **Acceptance:** `_basic` (create against a reservation + network + security group + image; assert id/region_id/ready_host_count; `PlanOnly` guard; import with `ImportStateVerifyIgnore: ["timeouts"]`); `_updateImage` (needs `NSCALE_TEST_IMAGE_ID_ALT`; asserts the image change plans an update, keeps the id); data source round-trip via id.
- **Negative:** 409 conflict / 404 missing reservation surface cleanly (staging only).
- Gate behind `TF_ACC=1`; needs `NSCALE_TEST_IMAGE_ID`. **Expected to fail until staging reservation service is deployed.**

## Open questions

- Does `server_spec.networking` round-trip exactly on read (it lives in `spec`)? Assumed yes.
- `placement servers` (`GET /api/v2/placements/{id}/servers`, reboot/stop) are a richer read surface — deferred. A `nscale_placement` data source exposing per-server IPs could come later; not in this PR.
