# Spec: nscale_<name>

> Fill out every section before writing Go. Mark sections "N/A" with a one-line rationale rather than deleting them — the absent answer is itself signal.

## Kind and package

- **Terraform type name:** `nscale_<name>`
- **Resource, data source, or both:** <resource | data source | both>
- **Service package:** `internal/services/<service>/` (existing or new)

## Backing API

- **Base service URL env:** `NSCALE_<SERVICE>_SERVICE_API_ENDPOINT`
- **List endpoint:** `GET /api/v1/...`
- **Read endpoint:** `GET /api/v1/.../{id}`
- **Create endpoint:** `POST /api/v1/...`
- **Update endpoint:** `PATCH /api/v1/.../{id}` (or N/A if immutable)
- **Delete endpoint:** `DELETE /api/v1/.../{id}`
- **OpenAPI types used:** `storageapi.<TypeName>`, etc.

Note any auth scoping beyond the standard org+project headers.

## Attributes

| Name | Type | Required / Optional / Computed | Plan modifiers | Sensitive | Notes |
| --- | --- | --- | --- | --- | --- |
| `id` | String | Computed | `UseStateForUnknown` | no | |
| ... | | | | | |

Call out nested objects, lists, sets explicitly — diff semantics differ.

## Lifecycle

- **Create:** sync / async (with state watcher)
- **Read:** any quirks (paginated list lookups, etc.)
- **Update:** PATCH-able, or rejected entirely (every field `RequiresReplace`)?
- **Delete:** sync / async (with state watcher)

## Provisioning states (if async)

- API field that signals readiness: `<spec.path>`
- Healthy / provisioned values: `<list>`
- Failure values: `<list>`

## Immutability

List every field that requires replacement on change:

- `<field>` — because <reason>

## Write-once / sensitive fields

List any field that the API returns only on Create (never on Read):

- `<field>` — preservation strategy: ...

For any field marked `Sensitive: true`, list it here with the reason.

## Import

- **Shape:** passthrough ID / composite (`<parent>/<id>`) / unsupported
- **Recoverable attributes:** all / list...
- **Unrecoverable attributes:** list... — these go into `ImportStateVerifyIgnore` in acc tests and need a warning in the resource's `ImportState` handler.

## Known API constraints

- Uniqueness rules (e.g. one-per-project-per-region).
- Region scoping (which regions support this resource?).
- Quotas.
- Anything else that surfaces as a 4xx with a specific code/message — capture the message text so docs can pre-empt support tickets.

## Examples

A representative HCL block that the docs will show. Should be runnable as-is once placeholder IDs are substituted.

```hcl
resource "nscale_<name>" "example" {
  # ...
}
```

## Test plan

- **Unit converters to cover:** ...
- **Acceptance test cases:** create + read, import + verify, update path (or reject), idempotency...
- **Negative cases:** what 4xx errors should surface cleanly?

## Open questions

- ...
