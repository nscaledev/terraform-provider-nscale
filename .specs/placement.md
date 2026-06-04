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
- **Update endpoint:** N/A — **no PATCH/PUT. Immutable.**
- **Delete endpoint:** `DELETE /api/v2/placements/{placementID}` → `202` (async; deletes all Region servers it created)
- **OpenAPI types used:** `reservationapi.PlacementV2Read`, `PlacementV2Create`, `PlacementV2CreateSpec`, `PlacementConstraintsV2`, `PlacementServerSpecV2`, `PlacementServerNetworkingV2`, `PlacementPolicyV2`, `WhenUnsatisfiableV2`.

Create body needs no org/project — placement is scoped through its reservation.
Metadata read is **project-scoped** → `nscale.StatusFromProjectScoped`.

## Attributes

| Name | Type | R/O/C | Plan modifiers | Notes |
| --- | --- | --- | --- | --- |
| `id` | String | Computed | `UseStateForUnknown` | `metadata.id` |
| `name` | String | Required | `RequiresReplace` | `metadata.name`; `NameValidator()` |
| `description` | String | Optional | `RequiresReplaceIfConfigured` | `metadata.description` |
| `tags` | Map(String) | Optional+Computed | `RequiresReplaceIfConfigured` | `NoReservedPrefix` |
| `reservation_id` | String | Required | `RequiresReplace` | `spec.reservationId` (create) / `status.reservationId` (read) |
| `network_id` | String | Required | `RequiresReplace` | `spec.networkId` (create) / `status.networkId` (read) |
| `host_count` | Int64 | Required | `RequiresReplace` | `spec.count`; `>= 1`. Non-pointer int, no `omitempty` |
| `constraints` | SingleNested | Required | `RequiresReplace` | `spec.constraints` (see below) |
| `server_spec` | SingleNested | Required | `RequiresReplace` | `spec.serverSpec` (see below) |
| `region_id` | String | Computed | `UseStateForUnknown` | `status.regionId` |
| `ready_host_count` | Int64 | Computed | — | `status.readyHostCount` (optional in API) |
| `project_id` | String | Computed | `UseStateForUnknown` | `metadata.projectId` |
| `creation_time` | String | Computed | `UseStateForUnknown` | `metadata.creationTime` |
| `provisioning_status` | String | Computed | `UseStateForUnknown` | `metadata.provisioningStatus` |

### `constraints` (SingleNestedAttribute, required)

| Name | Type | R/O/C | Notes |
| --- | --- | --- | --- |
| `policy` | String | Required | `OneOf("pack","spread")`. Pack fills domains sequentially; spread distributes evenly. |
| `max_skew` | Int64 | Optional | `>= 1`; only meaningful when `policy = spread`. `*int omitempty` in API. |
| `min_domains` | Int64 | Optional | `>= 1`; only meaningful when `policy = spread`; must be `<= count`. `*int omitempty`. |
| `when_unsatisfiable` | String | Optional | `OneOf("fail","bestEffort")`. `*enum omitempty`. |

All sub-fields round-trip via `spec.constraints` on read, so plain `Optional` (not Computed) is correct.

### `server_spec` (SingleNestedAttribute, required) — Region server options per pinned host

| Name | Type | R/O/C | Notes |
| --- | --- | --- | --- |
| `image_id` | String | Required | `spec.serverSpec.imageId` |
| `ssh_certificate_authority_id` | String | Optional | `*string omitempty` |
| `user_data` | String | Optional | base64-encoded; `Base64Validator{}`. API type `*[]byte` |
| `networking` | SingleNested | Optional | see below |

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
- **Update:** rejected — `Update: nil` (immutable).
- **Delete:** async (202). `DeleteStateWatcher` polls until 404. Deletes all backing Region servers.

## Provisioning states (async)

Same shared mechanism as reservation: `metadata.provisioningStatus`, target
`provisioned`/`error`. `status.readyHostCount` is informational only (not used as the readiness gate).

## Immutability

All configurable fields require replacement (no update API): `name`,
`description`, `tags`, `reservation_id`, `network_id`, `host_count`, `constraints`
(whole object), `server_spec` (whole object).

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
- **Acceptance:** `_basic` (create against a reservation + network + security group + image; assert id/region_id/ready_host_count; `PlanOnly` guard; import with `ImportStateVerifyIgnore: ["timeouts"]`); data source round-trip via id. No `_update`.
- **Negative:** 409 conflict / 404 missing reservation surface cleanly (staging only).
- Gate behind `TF_ACC=1`; needs `NSCALE_TEST_IMAGE_ID`. **Expected to fail until staging reservation service is deployed.**

## Open questions

- Does `server_spec.networking` round-trip exactly on read (it lives in `spec`)? Assumed yes.
- `placement servers` (`GET /api/v2/placements/{id}/servers`, reboot/stop) are a richer read surface — deferred. A `nscale_placement` data source exposing per-server IPs could come later; not in this PR.
