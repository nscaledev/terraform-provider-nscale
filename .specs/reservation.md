# Spec: nscale_reservation

> Topology-aware bare-metal GPU capacity reservation. Reserves one or more
> contiguous accelerator reservation units in a region. A reservation is the
> capacity pool that `nscale_placement` allocates hosts from.

## Kind and package

- **Terraform type name:** `nscale_reservation`
- **Resource, data source, or both:** both
- **Service package:** `internal/services/reservation/` (**new**)

## Backing API

New service: **Unikorn Reservation API** (`reservation/openapi.yaml`, v0.5.0). Hosted
on its own base URL — needs provider wiring (see "Provider plumbing" below).

- **Base service URL env:** `NSCALE_RESERVATION_SERVICE_API_ENDPOINT`
  (default `https://reservation.unikorn.nscale.com`)
- **List endpoint:** `GET /api/v2/reservations` (not used by the resource; data source reads by id)
- **Read endpoint:** `GET /api/v2/reservations/{reservationID}` → `200 ReservationV2Read`
- **Create endpoint:** `POST /api/v2/reservations` → `202 ReservationV2Read` (async)
- **Update endpoint:** N/A — **no PATCH/PUT. Immutable.**
- **Delete endpoint:** `DELETE /api/v2/reservations/{reservationID}` → `202` (async; deletes all placements allocated from it)
- **OpenAPI types used:** `reservationapi.ReservationV2Read`, `reservationapi.ReservationV2Create`, `reservationapi.ReservationV2CreateSpec`, `coreapi.ResourceWriteMetadata`, `coreapi.ProjectScopedResourceReadMetadata`.

Auth scoping: standard. Create body carries `organizationId` + `projectId` (from
the provider-configured client) and `regionId` (from `region_id`, defaulting to
the provider region). Metadata read is **project-scoped** → use
`nscale.StatusFromProjectScoped` / `AdaptProjectScoped`.

## Provider plumbing (one-time, this PR)

Reservation lives behind a new base URL. Add, mirroring the existing region/compute/identity wiring:

- `provider.go`: `DefaultNscaleReservationServiceAPIEndpoint` const, `reservation_service_api_endpoint` attribute, model field, `resolveValue(..., "NSCALE_RESERVATION_SERVICE_API_ENDPOINT", default)`, pass into `nscale.NewClient`.
- `internal/nscale/client.go`: new `reservationServiceBaseURL` param, build `reservationapi.NewClient`, add `Reservation reservationapi.ClientInterface` field to `Client`.

## Attributes

| Name | Type | R/O/C | Plan modifiers | Sensitive | Notes |
| --- | --- | --- | --- | --- | --- |
| `id` | String | Computed | `UseStateForUnknown` | no | `metadata.id` |
| `name` | String | Required | `RequiresReplace` | no | `metadata.name`; `NameValidator()` |
| `description` | String | Optional | `RequiresReplaceIfConfigured` | no | `metadata.description` |
| `tags` | Map(String) | Optional+Computed | `RequiresReplaceIfConfigured` | no | `NoReservedPrefix`; operation tags stripped |
| `region_id` | String | Optional+Computed | `UseStateForUnknown`, `RequiresReplaceIfConfigured` | no | `spec.regionId`; defaults to provider region |
| `project_id` | String | Optional+Computed | `UseStateForUnknown`, `RequiresReplaceIfConfigured` | no | `metadata.projectId`; defaults to provider project |
| `accelerator` | String | Required | `RequiresReplace` | no | `spec.accelerator` e.g. `GB300` |
| `unit` | String | Required | `RequiresReplace` | no | `spec.unit` e.g. `NVL72` |
| `unit_count` | Int64 | Required | `RequiresReplace` | no | `spec.count`; `>= 1`. Non-pointer int, no `omitempty` → no round-trip risk |
| `machine_flavor_id` | String | Computed | `UseStateForUnknown` | no | `status.machineFlavorId` |
| `claimed_unit_count` | Int64 | Computed | `UseStateForUnknown` | no | `status.claimedUnitCount` |
| `topology_hash` | String | Computed | — | no | `status.topologyHash` (optional in API) |
| `topology_observed_at` | String | Computed | — | no | `status.topologyObservedAt` (RFC3339) |
| `creation_time` | String | Computed | `UseStateForUnknown` | no | `metadata.creationTime` |
| `provisioning_status` | String | Computed | `UseStateForUnknown` | no | `metadata.provisioningStatus` |

No nested objects. No `Required` bools → the `omitempty`-bool failure mode does not apply here.

**Why every configurable field is `RequiresReplace`:** there is no update API. The
generic base's `Update` is `nil`, so any in-place change would otherwise hit the
"Update Not Supported" error. Marking each user-settable field for replacement
makes Terraform plan a replace instead.

## Lifecycle

- **Create:** async (202). Record id, then `CreateStateWatcher` polls
  `provisioning_status` to `provisioned`/`error`.
- **Read:** GET by id; 404 → remove from state.
- **Update:** rejected — `Update: nil` in the adapter (immutable).
- **Delete:** async (202). `DeleteStateWatcher` polls until 404.

## Provisioning states (async)

- Signalled by `metadata.provisioningStatus` (`coreapi.ResourceProvisioningStatus`).
- Provisioned: `provisioned`. Failure: `error`. Pending: `pending`/`provisioning`/`unknown`.
- Handled entirely by the shared watchers in `internal/nscale/helper.go`.

## Immutability

All configurable fields require replacement (no update API): `name`,
`description`, `tags`, `region_id`, `project_id`, `accelerator`, `unit`, `unit_count`.

## Write-once / sensitive fields

None.

## Import

- **Shape:** passthrough ID (`resource.ImportStatePassthroughID` via generic base).
- **Recoverable:** all (id resolves the full object). `timeouts` is ignored in `ImportStateVerify`.
- **Unrecoverable:** none.

## Known API constraints

- **Insufficient capacity (`507`)** on create: "No contiguous set of reservation
  units of the requested size is available in the region." Surface the API error
  verbatim. Capacity is discoverable via the `reservation-units` list endpoint
  (a future `nscale_reservation_unit` data source — out of scope this PR).
- `accelerator` / `unit` are public capacity shapes (e.g. `GB300` / `NVL72`); valid
  combinations are region-specific and enforced server-side (400 otherwise).
- `count >= 1`.

## Examples

```hcl
resource "nscale_reservation" "training" {
  name        = "gb300-nvl72"
  description = "Reserved accelerator units for training"
  accelerator = "GB300"
  unit        = "NVL72"
  unit_count  = 2

  tags = {
    workload = "training"
  }
}
```

## Test plan

- **Unit converters:** `NewReservationModel` (status fields, optional topology fields nil/set, tag stripping); `NscaleReservationCreateParams` (org/project/region defaulting, count).
- **Acceptance:** `_basic` (create + assert id/machine_flavor_id/claimed_unit_count + `PlanOnly` guard + import with `ImportStateVerifyIgnore: ["timeouts"]`); data source round-trip via id. No `_update` (immutable).
- **Negative:** 507 insufficient capacity surfaces cleanly (manual / staging only).
- All acc tests gate behind `TF_ACC=1` and skip without env vars. **Expected to fail until the staging reservation service is deployed** — that is acceptable per the task.

## Open questions

- Confirmed default base URL host `reservation.unikorn.nscale.com` (assumed from the `*.unikorn.nscale.com` convention) — verify against staging once deployed.
- Does `region_id` accept the same UUID form as the provider `region_id`? Assumed yes (the example uses a UUID).
