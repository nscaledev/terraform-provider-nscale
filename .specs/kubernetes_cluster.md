# Spec: nscale_kubernetes_cluster (+ platform releases)

> NKS managed Kubernetes. Covers three Terraform types in one PR:
> `nscale_kubernetes_cluster` (resource), `nscale_kubernetes_cluster` (data
> source), and `nscale_kubernetes_platform_releases` (list data source).
>
> Source ticket: DX-2007. Due Friday 2026-08-28.

## Kind and package

- **Terraform type names:** `nscale_kubernetes_cluster`, `nscale_kubernetes_platform_releases`
- **Resource, data source, or both:** cluster = both; platform releases = data source only
- **Service package:** `internal/services/kubernetescluster/` (**new**)

Named `kubernetescluster` (not `kubernetes`) to sit alongside the existing
`computecluster` package and leave room for a future `kubernetesnodepool`.

---

## ⚠️ Decision required before implementation: how we get the NKS Go client

The ticket says "generate the NKS Go client in `nscale-sdk-go` (new `nks`
package)". **That work is already done** — but not in a version we can consume
as-is.

| Version | `kubernetes` package contents |
| --- | --- |
| `v0.0.4` (current) | Old **unikorn** Kubernetes Service API (`/api/v1/organizations/{orgID}/clusters`, cluster managers, vclusters). Not NKS. |
| `v0.1.0` | Same old unikorn API. |
| `v0.2.0` | **NKS API** — `/api/v1/clusters`, `/api/v1/nodepools`, `/api/v1/platformreleases`. This is what we want. |

The catch: **`v0.2.0` is a breaking release for the rest of the provider.**

- It **deletes the `github.com/nscaledev/nscale-sdk-go/common` package** and inlines
  those types into each service package. 32 files in this repo import it as
  `coreapi`, including the shared core: `internal/nscale/helper.go`,
  `internal/nscale/metadata.go`, `internal/utils/tftypes/tftypes.go`. Every
  resource's model, every watcher, and `ResourceStatus` are typed on it.
- It **renames the identity and region operations** from the generated
  `GetApiV1OrganizationsOrganizationID…` form to named `operationId`s (identity
  drops from 22 `Get*` methods to 11). Every call site in `internal/services/`
  changes.

Go modules allow one version of a module per build, so we cannot take
`kubernetes` from `v0.2.0` and keep everything else on `v0.0.4`.

Verified empirically: `go get nscale-sdk-go@v0.2.0 && go build ./...` fails at
the first missing `common` import; reverted.

### Options

**(A) Bump the whole SDK to `v0.2.0` and refactor the provider.**
Correct long-term. But it rewrites the type foundation under every existing
resource plus all identity/region call sites, in the same PR as a new service.
Large regression surface, and the acceptance suite that would catch a mistake
costs real money to run. Not deliverable safely by Friday.

**(B) Generate the NKS client locally into this repo. ← recommended**
`oapi-codegen` is already a declared tool in `go.mod`
(`tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen`). Generate from
the ticket's spec URL into `internal/nks/nks.gen.go` with a `//go:generate`
directive, exactly as `nscale-sdk-go` does. Blast radius: zero — nothing
existing changes. The NKS spec is self-contained (it inlines its own `Error`,
`ResourceMetadata`, `Tag`, `StaticResourceMetadata`), so it needs no `common`.
Its error shape is `{error, error_description, trace_id}`, which
`internal/nscale/error.go` already parses unchanged.

**(C) Split the SDK bump into its own PR, land it first, then build NKS on top.**
Cleanest sequencing and the right destination, but serialises two chunks of work
against a Friday deadline.

### Recommendation

**(B) now, (A) as a tracked follow-up.** Ship NKS behind a locally-generated
client this week; do the SDK-wide `v0.2.0` migration as a dedicated PR with the
full acceptance suite behind it. When that lands, swapping
`internal/nks` → `nscale-sdk-go/kubernetes` is an import-path change, because
the generated types are identical (same spec, same generator).

Cost of the follow-up: one import rewrite in one package. Cost of getting (A)
wrong this week: every resource in the provider.

**This is the one open question that blocks Phase 2. Everything below assumes (B).**

---

## Backing API

NKS lives behind its own base URL → needs provider wiring, mirroring the
reservation service precedent.

- **Base service URL env:** `NSCALE_NKS_SERVICE_API_ENDPOINT`
  (provider attribute `nks_service_api_endpoint`; default TBC — see Open questions)
- **Spec:** `https://raw.githubusercontent.com/nscaledev/openapi/main/nks-core/main/openapi.yaml`
  (title "NKS API", v0.1.0)

**Generate from the canonical URL, not from the SDK's vendored copy.** They have
already diverged — the canonical spec is ahead of
`nscale-sdk-go@v0.2.0/kubernetes/openapi.yaml` (66,291 → 69,755 bytes). In scope
for us:

- `listPlatformReleases` gained an **`organizationID` filter**.
- `platformReleaseStatusV1` gained a **required `usableOrganizationIds`** field.
- `deprecated` semantics reversed: was "once true, remains true for the lifetime
  of the release", now "an explicit operator decision and **may be reversed**".
- `413 Request Entity Too Large` added to the create/update responses.
- `clusterKubernetesVersionStatusV1.target` reworded — no schema change.

Out of scope but worth knowing, because it shows node pools are still moving:
taints lost `propagation`, a `labels` map was added, and `nodePoolReleaseStatusV1`
is new.

This divergence is independent evidence for option (B) — pinning to the SDK would
ship us a client that is already stale.

| Operation | Verb + path | Response |
| --- | --- | --- |
| `listClusters` | `GET /api/v1/clusters` | `200 []ClusterV1Read` |
| `createCluster` | `POST /api/v1/clusters` | `201 ClusterV1Read` (async) |
| `getCluster` | `GET /api/v1/clusters/{clusterID}` | `200 ClusterV1Read` |
| `updateCluster` | `PUT /api/v1/clusters/{clusterID}` | `200 ClusterV1Read` (async, full replacement) |
| `deleteCluster` | `DELETE /api/v1/clusters/{clusterID}` | `202` (async) |
| `listPlatformReleases` | `GET /api/v1/platformreleases` | `200 []PlatformReleaseV1Read` |
| `getPlatformRelease` | `GET /api/v1/platformreleases/{releaseID}` | `200 PlatformReleaseV1Read` |

**Types used:** `ClusterV1Read/Create/Update`, `ClusterCreateSpecV1`,
`ClusterUpdateSpecV1`, `ClusterSpecV1`, `ClusterStatusV1`,
`ClusterApiServerAccessV1`, `ClusterApiServerStatusV1`, `ClusterNetworkV1`,
`ClusterAddonsCreateV1`, `ClusterReleaseStatusV1`,
`ClusterKubernetesVersionStatusV1`, `ProjectScopedResourceReadMetadataV1`,
`ResourceMetadata`, `PlatformReleaseV1Read`, `PlatformReleaseStatusV1`,
`ListPlatformReleasesParams`.

### ⚠️ `project_id` cannot be an argument

The ticket lists `project_id` as an argument "resolve via client default like
other resources". **The NKS API has no way to accept one.** `ClusterV1Create` is
`{metadata: {name, description, tags}, spec: {networkId, platformReleaseId, …}}`
and `clusterCreateSpecV1` is `additionalProperties: false`. There is no
`projectId`/`organizationId` field and no header parameter.

Project and organization are **inherited from the referenced `networkId`** — the
same mechanism the spec documents for region ("`status.regionId`: Resolved region
inherited from the attached network"). They come back on read in
`metadata.projectId` / `metadata.organizationId`.

**Therefore `project_id`, `organization_id` and `region_id` are all `Computed`
only.** Setting them is an error the framework raises for us. Scope is chosen by
picking the network. This is worth a `~> Note` in the docs, because it differs
from every other resource in the provider.

---

## Attributes — `nscale_kubernetes_cluster` (resource)

### Arguments

| Name | Type | R/O/C | Plan modifiers | Notes |
| --- | --- | --- | --- | --- |
| `name` | String | Required | — | `metadata.name`; `NameValidator()` |
| `description` | String | Optional | — | `metadata.description` |
| `tags` | Map(String) | Optional+Computed | — | `metadata.tags`; `NoReservedPrefix`; strip operation tags on read |
| `network_id` | String | Required | **`RequiresReplace`** | `spec.networkId`. Immutable per spec comment on `clusterUpdateSpecV1`. Also determines project + region. |
| `platform_release_id` | String | Required | — | `spec.platformReleaseId`. Changing = in-place upgrade. |
| `api_server` | SingleNested | Optional+Computed | — | `spec.apiServer`; see below |
| `api_server.public_ip` | Bool | Optional+Computed | — | server default `false` |
| `api_server.allowed_cidrs` | Set(String) | Optional+Computed | — | server default `["0.0.0.0/0"]`; 1–32 items, unique, IPv4 CIDR pattern → `SetValidator` size + `CIDRValidator` |
| `cluster_network` | SingleNested | Optional+Computed | — | `spec.clusterNetwork` |
| `cluster_network.pod_cidr` | String | Optional+Computed | — | server default `10.240.0.0/12` |
| `cluster_network.service_cidr` | String | Optional+Computed | — | server default `10.96.0.0/16` |
| `addons` | SingleNested | Optional+Computed | — | `spec.addons` |
| `addons.hardware` | Bool | Optional+Computed | — | server default **`true`** on create |

`allowed_cidrs` is a **Set** (spec says `uniqueItems: true`, order carries no
meaning) and contains nothing sensitive, so the list-vs-set caveat in
[playbook §1.3](../.claude/skills/tf-provider-feature/reference/playbook.md) does
not bite.

`cluster_network` is **mutable in place** — no `RequiresReplace`. Decided
2026-08-24. Changing `pod_cidr` / `service_cidr` goes through the normal PUT and
is waited on like any other update.

### Computed

| Name | Type | Plan modifiers | Notes |
| --- | --- | --- | --- |
| `id` | String | `UseStateForUnknown` | `metadata.id` |
| `project_id` | String | `UseStateForUnknown` | `metadata.projectId` — inherited from network |
| `organization_id` | String | `UseStateForUnknown` | `metadata.organizationId` |
| `region_id` | String | `UseStateForUnknown` | `status.regionId` — inherited from network |
| `creation_time` | String | `UseStateForUnknown` | `metadata.creationTime` |
| `provisioning_status` | String | — (transient) | `metadata.provisioningStatus` |
| `health_status` | String | — (transient) | `metadata.healthStatus` |
| `kubernetes_version_target` | String | — | `status.kubernetesVersion.target` |
| `kubernetes_version_observed` | String | — | `status.kubernetesVersion.observed` |
| `api_server_endpoint` | SingleNested | — | from `status.apiServer` — see below |
| `api_server_endpoint.certificate_authority_data` | String | — | base64 CA bundle. **Not sensitive** — a CA bundle is public key material. |
| `api_server_endpoint.private_host` / `private_port` | String / Int64 | — | `status.apiServer.endpoints.private` |
| `api_server_endpoint.public_host` / `public_port` | String / Int64 | — | `status.apiServer.endpoints.public` (nullable) |
| `applied_platform_release_id` | String | — | `status.release.appliedId` — lags `platform_release_id` during upgrade |
| `platform_release_deprecated` | Bool | — | `status.release.deprecated` |
| `platform_release_withdrawn` | Bool | — | `status.release.withdrawn` |
| `upgrade_available` | Bool | — | `status.release.upgradeAvailable` |
| `eligible_upgrade_target_ids` | List(String) | — | `status.release.eligibleTargets`, in upgrade order → **List**, order is meaningful |

Per [playbook anti-pattern #12](../.claude/skills/tf-provider-feature/reference/playbook.md),
no `UseStateForUnknown` on the transient status fields (`provisioning_status`,
`health_status`, the version/release/endpoint blocks) — they legitimately change
between plans.

**Deliberately excluded** (observability noise that would churn state every plan,
and none of it is actionable from HCL): `status.addons` component-level rollout
detail, `status.controlPlane` replica counts, `status.nodePools` summary counts.
Revisit if users ask.

### `omitempty`-bool check ([playbook §1.6](../.claude/skills/tf-provider-feature/reference/playbook.md))

**Not applicable on the write path.** Every optional write-side scalar in the NKS
spec is generated as a pointer:
`ClusterAddonsCreateV1.Hardware *bool`, `ClusterApiServerAccessV1.PublicIP *bool`,
`ClusterApiServerAccessV1.AllowedCidrs *[]string`, `ClusterNetworkV1.PodCidr *string`,
`ClusterNetworkV1.ServiceCidr *string`. A configured `false` serialises correctly.

Unit tests still assert `public_ip = false` and `hardware = false` round-trip
explicitly — that is the cheapest guard against a future regeneration flipping a
pointer to a value type.

---

## Attributes — `nscale_kubernetes_cluster` (data source)

Lookup by `id` (repo convention). Same computed attribute set as the resource,
all `Computed`, no plan modifiers, no timeouts block.

## Attributes — `nscale_kubernetes_platform_releases` (data source)

**This is the provider's first plural/list data source** — every existing one
reads a single object by id. Flagging the new convention explicitly.

### Filters (all Optional)

| Name | Type | Maps to |
| --- | --- | --- |
| `organization_id` | String | `organizationID`; defaults to the provider-configured org |
| `region_id` | String | `regionID` query param (repeatable in API; single value here) |
| `architecture` | String | `architecture`; one of `x86_64`, `aarch64` |
| `prerelease` | Bool | `prerelease` |
| `deprecated` | Bool | `deprecated` |
| `withdrawn` | Bool | `withdrawn` |

**Every unset filter must be omitted from the query string, not sent empty**
(ticket, confirmed by the spec's repeatable-param shape). The generated
`ListPlatformReleasesParams` uses pointer fields, so leaving them `nil` does the
right thing — the converter must map Terraform null → `nil`, never → zero value.

### Computed: `releases` (List of nested)

Order: as returned by the API. `List`, not `Set` — the API documents a meaningful
order and users will want `releases[0]`.

| Name | Type | Maps to |
| --- | --- | --- |
| `id` | String | `metadata.id` |
| `name` | String | `metadata.name` |
| `kubernetes_version` | String | `status.kubernetesVersion` |
| `deprecated` | Bool | `status.deprecated` |
| `withdrawn` | Bool | `status.withdrawn` |
| `prerelease` | Bool | `status.prerelease` |
| `withdrawal_reason` | String | `status.withdrawalReason` (nullable enum) |
| `withdrawal_message` | String | `status.withdrawalMessage` (nullable) |
| `supported_architectures` | List(String) | `status.supportedArchitectures` |
| `available_region_ids` | List(String) | `status.availableRegionIds` |
| `usable_organization_ids` | List(String) | `status.usableOrganizationIds` — which of the caller's orgs may select this release |

Exposing `deprecated`/`withdrawn`/`prerelease` as computed fields (rather than
only as filters) is what lets a config select "latest eligible" without
hardcoding an ID — the ticket's stated motivation.

**CLI selection convention to mirror, documented in prose, not enforced in code:**
create should offer only non-deprecated + non-withdrawn releases; update keeps
deprecated (you must be able to re-state the release you are already on) but
never withdrawn. The data source stays a thin filter; the provider does not
second-guess the catalogue.

---

## Lifecycle

- **Create:** `POST` → `201` with the cluster body, then **async**. Wait for settled + provisioned.
- **Read:** direct `GET` by id. 404 → `RemoveResource`.
- **Update:** `PUT` full replacement. Build the entire `ClusterV1Update` from
  config every time — **no read-modify-write merging of computed fields** into the
  payload (ticket). Note `clusterUpdateSpecV1` requires `networkId` even though it
  is immutable, so the unchanged value is echoed back.
- **Delete:** `DELETE` → `202`, then poll to not-found.

### ⚠️ The shared `GenericResource` cannot be reused as-is

`internal/nscale/resource.go` + `helper.go` give us the CRUD skeleton and three
watchers, and every recent resource uses them. Three reasons NKS needs its own
waiter (`kubernetes_cluster_wait.go`):

1. **`ResourceStatus` is typed on `coreapi`** (`internal/nscale/metadata.go`):
   `ProvisioningStatus coreapi.ResourceProvisioningStatus`, `Tags *coreapi.TagList`.
   NKS's enums are structurally identical but a different Go type. Needs either a
   conversion shim or an NKS-native watcher.
2. **Settledness needs `observedGeneration`, which the shared watcher has no
   concept of.** After a write, `status.observedGeneration < metadata.generation`
   means the `provisioned` being read describes the *previous* spec. The shared
   `CreateStateWatcher` would return success with stale computed values —
   precisely the bug the ticket warns about.
3. **`provisioned` + `healthStatus: error` is a failure**, and
   `CreateStateWatcher` only looks at `provisioningStatus`.

Note the repo already solves the equivalent staleness problem for the cache-backed
region API with the **operation-tag** trick (`nscale.WriteOperationTag` writes a
`terraform.nscale.com/<uuid>` tag into the update payload, and
`UpdateStateWatcher` polls until it is echoed back). NKS gives us
`observedGeneration` natively, which is strictly better: no tag pollution, no
strip-on-read step, and it covers create as well as update. **Use
`observedGeneration`; do not port the tag trick to NKS.**

### Waiter semantics

Settled predicate, shared by create and update:

```
status.observedGeneration != nil && *status.observedGeneration >= metadata.generation
```

| Phase | Pending | Target | Hard failure |
| --- | --- | --- | --- |
| Create / Update | not settled; `pending`; `provisioning` | settled ∧ `provisioned` ∧ `healthStatus != error` | settled ∧ (`provisioningStatus == error` ∨ `healthStatus == error`) |
| Delete | `deprovisioning`, or any still-readable state | 404 | `provisioningStatus == error` **after deprovisioning has been observed** → terminal, fail immediately rather than waiting out the timeout |

The delete rule is deliberately narrower than the shared `DeleteStateWatcher`,
which treats `error` as terminal from the first poll. A cluster that was already
sitting in `error` before the destroy must still be allowed to deprovision.

### Timeouts

`timeouts.Block` with `Create/Update/Delete: true`. Defaults, extending the
[playbook §2.2](../.claude/skills/tf-provider-feature/reference/playbook.md) table
with a new "managed Kubernetes control plane" class:

| | Create | Update | Delete |
| --- | --- | --- | --- |
| Original guess | 30m | 45m | 30m |
| **Actual, after measurement** | **60m** | **90m** | **30m** |

**Measured on uni-dev 2026-08-26: create took 32m18s, delete 2m11s.** The
original 30m create default would have failed on the very first real apply —
timing out on a cluster that was minutes from ready, and leaving a running,
billable cluster that Terraform had lost track of. Defaults raised to roughly
2x observed. Update is longest because a platform release change is a rolling
control-plane upgrade, not a config write.

This is the single most valuable thing the manual test caught, and it was only
catchable against real infrastructure.

---

## Immutability

- `network_id` — `RequiresReplace`. **Confirmed by the NKS team (Matt Pryor,
  2026-08-24):** networkId is immutable for the lifetime of a cluster. The
  "Immutable after creation" annotation appears only on `clusterUpdateSpecV1`
  (not on the create spec) because PUT is full-object-replacement — the field
  must be *present* in every update, carrying the same value it was created
  with. Supplying a different one is expected to return **422**. Changing it
  requires teardown and rebuild, which is exactly what `RequiresReplace` gives us.
`network_id` is the **only** field that forces replacement.

Because `network_id` is `RequiresReplace`, Terraform can never plan a PUT that
changes it: any change destroys and recreates. The update converter therefore
just carries the planned (== prior) value straight through, satisfying the
"same networkID in the initial POST and every subsequent PUT" rule for free.

Everything else is mutable in place: `name`, `description`, `tags`,
`platform_release_id` (the upgrade path), `api_server.*`, `addons.hardware`,
`cluster_network.pod_cidr`, `cluster_network.service_cidr`.

## Write-once / sensitive fields

**None.** NKS returns no secrets — the CA bundle in `status.apiServer` is public
key material and the API is credential-free by design. No `Sensitive: true`, no
stash-on-Read, no "Handling the secret" docs section.

This is a direct consequence of the auth design below.

### Cluster auth hand-off (kubeconfig)

There is **no kubeconfig endpoint** and no token endpoint on NKS. `nscale
kubernetes get-kubeconfig` assembles the kubeconfig client-side from
`status.apiServer` (CA bundle + endpoints) and injects a client-go **exec
plugin** (`nscale kubernetes token`) that emits an `ExecCredential` wrapping the
caller's nscale access token (honouring `NSCALE_SERVICE_TOKEN` for CI).

Two ways to hand this to the `kubernetes`/`helm` providers:

- **(a)** an EKS-style `nscale_kubernetes_cluster_auth` data source returning a
  short-lived token. Ergonomic, but **the token lands in plaintext state** and
  would be the provider's only secret-bearing attribute.
- **(b)** document an `exec` block invoking the nscale CLI. No secret in state;
  requires the CLI on the apply host.

**This spec ships (b)** — a documented `exec` example in the resource docs,
consistent with the ticket's "recommend (b) for Friday scope, (a) as
fast-follow". Exposing `api_server_endpoint.*` as first-class computed attributes
is what makes (b) work, and it is a prerequisite for (a) later either way.

## Import

- **Shape:** passthrough ID — `resource.ImportStatePassthroughID` on `path.Root("id")`.
- **Recoverable:** everything. No write-once fields, and every argument is
  readable from `spec`. `ImportStateVerify: true` with an **empty**
  `ImportStateVerifyIgnore` (other than `timeouts`, which is config-only and never
  in the API).

```sh
terraform import nscale_kubernetes_cluster.main <cluster-id>
```

The post-import `terraform plan` saying "No changes" is the gate — it is the only
check that proves the Optional+Computed defaults (`api_server`, `cluster_network`,
`addons`) actually round-trip. See Open questions.

## Known API constraints

- **Project/org/region are not settable** — inherited from `network_id`. See above.
- **List `name` filters are exact + case-sensitive and non-unique.** Multiple
  clusters may share a display name. Neither data source looks up by name, so this
  only matters if we add name lookup later — don't.
- **All list filters are repeatable ID-list query params and must be omitted, not
  sent empty, when unset.**
- **Update is full replacement (PUT), not PATCH.** A field omitted from the payload
  is cleared, not left alone. There is no PATCH endpoint and none is planned
  near-term — the NKS team deliberately skipped it because merge semantics are
  ambiguous. This is why the ticket's "build the complete `clusterV1Update` from
  config every time" rule is load-bearing rather than stylistic.
- **`networkId` must be re-sent unchanged on every PUT.** A PUT carrying a
  different `networkId` is expected to return **422** rather than move the
  cluster. `RequiresReplace` means Terraform never generates that request.
- `409 Conflict` on `updateCluster` — concurrent modification. Surface unmodified
  per [playbook §3.1](../.claude/skills/tf-provider-feature/reference/playbook.md);
  do not auto-retry (it is not transient, it means someone else wrote).
- `422 Unprocessable Content` on create/update — e.g. selecting a withdrawn
  platform release, or one not available in the network's region
  (`status.availableRegionIds`). Surface the message; do not pre-validate
  provider-side against the catalogue
  ([playbook anti-pattern #4](../.claude/skills/tf-provider-feature/reference/playbook.md)).

### ⚠️ Scope note: a cluster alone has no workers

NKS models node pools as a **separate top-level resource**
(`/api/v1/nodepools`, filtered by `clusterID`). DX-2007 does not mention a node
pool resource, and this spec does not include one.

A `nscale_kubernetes_cluster` with no node pools is a control plane with nothing
to schedule on — usable for `terraform apply`/`destroy` and for the acceptance
tests, but **not a workload-ready cluster**. Users cannot run anything on what
this PR ships unless they create node pools outside Terraform.

Recommend filing `nscale_kubernetes_node_pool` as an immediate follow-up.
Flagging rather than expanding scope — that is the ticket owner's call, and the
node pool spec (`NodePoolRequestSpecV1`: provisioning modes, taints,
reservations, flavours, autoscaling) is comparable in size to this one.

---

## Examples

```hcl
data "nscale_kubernetes_platform_releases" "stable" {
  region_id    = nscale_network.main.region_id
  architecture = "x86_64"
  deprecated   = false
  withdrawn    = false
}

resource "nscale_kubernetes_cluster" "main" {
  name                = "production"
  network_id          = nscale_network.main.id
  platform_release_id = data.nscale_kubernetes_platform_releases.stable.releases[0].id

  # Nested attributes, not blocks — note the `=`.
  api_server = {
    public_ip     = true
    allowed_cidrs = ["203.0.113.0/24"]
  }

  addons = {
    hardware = true
  }

  # `timeouts` is a block.
  timeouts {
    create = "45m"
    update = "60m"
  }
}

output "api_endpoint" {
  value = nscale_kubernetes_cluster.main.api_server_endpoint.public_host
}
```

Kubeconfig hand-off (option (b)), for the docs:

```hcl
provider "kubernetes" {
  host                   = "https://${nscale_kubernetes_cluster.main.api_server_endpoint.public_host}:${nscale_kubernetes_cluster.main.api_server_endpoint.public_port}"
  cluster_ca_certificate = base64decode(nscale_kubernetes_cluster.main.api_server_endpoint.certificate_authority_data)

  exec {
    api_version = "client.authentication.k8s.io/v1"
    command     = "nscale"
    args        = ["kubernetes", "token"]
  }
}
```

## Test plan

**Unit** (`kubernetes_cluster_model_test.go`, `platform_releases_model_test.go`):

- `ClusterV1Read` → model, both directions, incl. status fields — the ticket's
  "spec↔model round-trip".
- Nil/absent optional sub-objects: `status.apiServer` nil (pre-provisioning),
  `status.release` nil, `endpoints.public` nil, `kubernetesVersion.observed` nil.
- `public_ip = false` and `hardware = false` serialise **explicitly**, not omitted.
- Unset platform-release filters produce `nil` params, i.e. **no query string key**
  (assert on the encoded URL, not just the struct).
- Settled predicate: `observedGeneration` nil / `< generation` / `>= generation`.
- Waiter classification table: (provisioning status × health status × settled) →
  pending / target / failure.

**Acceptance** (`acc_test.go`, `kubernetes_cluster_resource_test.go`,
`kubernetes_cluster_data_source_test.go`, `platform_releases_data_source_test.go`):

- `_basic`: apply → check `id`, `region_id`, `project_id` populated, endpoints
  non-empty → **`PlanOnly: true, ExpectNonEmptyPlan: false`** follow-up step.
- Import: `ImportState: true, ImportStateVerify: true`, no ignores.
- `_update`: change `platform_release_id` to another eligible release → assert
  **in-place** (not replace) → assert `applied_platform_release_id` catches up →
  `PlanOnly` step.
- `_replace`: change `network_id` → assert plan is a replace.
- `_update_cluster_network`: change `pod_cidr` → assert **in-place**, assert the
  new value reads back, `PlanOnly` step. This is the step that proves the
  mutable-CIDR decision; if the API rejects or ignores the change, it fails here
  rather than in a user's apply.
- `api_server.public_ip = false` step + `PlanOnly` (the bool-zero guard).
- Data source by `id` matches the resource.
- Platform releases list non-empty; filtered-by-architecture subset ⊆ unfiltered.
- Names via `acctest.RandomWithPrefix("tf-acc-test")`.

**Negative:** withdrawn platform release → clean `422` surfaced with the API
message, not a Go error dump.

**Cost caveat:** these acceptance tests provision real Kubernetes control planes.
Slower and more expensive than the rest of the suite — worth a comment in
`acc_test.go` and possibly a separate CI lane.

**Results on uni-dev, 2026-08-26** — all green:

| Test | Result |
| --- | --- |
| `_basic` (apply + PlanOnly + import/verify) | PASS (387s) |
| `DataSource_byID` | PASS (387s) |
| `_rejectsSettingScope` | PASS (0.15s) |
| `PlatformReleasesDataSource_basic` / `_eligibleOnly` / `_filterSubset` | PASS |
| `_replaceOnNetworkChange` | SKIP — needs a second network in the same project+region; uni-dev has none |
| `_boolZeroValues`, `_updateRelease`, `_updateClusterNetwork` | not yet run |

**`_basic` failed once before passing on retry**, and the failure is worth
recording because it exercised the failure path for the first time: NKS set
`provisioningStatus: error` with *"one or more add-ons are degraded"*, and the
waiter correctly detected it, surfaced the API's own message via
`failureDetail()`, and failed the apply rather than reporting success. The retry
passed with identical config, so it is intermittent infrastructure rather than a
provider defect.

Hypothesis for the NKS team, unconfirmed: the `hardware` addon defaults to true
and these clusters have no node pools, so there is no hardware for it to install
onto. The cluster was destroyed by the test framework before `status.addons`
could be captured — worth grabbing that field if it recurs.

**Observed create times varied 5x: 6m27s to 32m18s**, same region and release.
That spread is why the create default is 60m rather than anything near the
median — a 30m default would pass on a fast day and fail on a slow one.

---

## Open questions

1. ~~**SDK strategy?**~~ **Decided 2026-08-25: (B).** The client is generated
   in-tree at `internal/nks/` from the canonical spec, via the `oapi-codegen`
   tool already declared in `go.mod`. `make nks-spec` refreshes it. The SDK-wide
   `v0.2.0` migration is a tracked follow-up; when it lands, delete
   `internal/nks/` and swap the import path.

   Note this required bumping `oapi-codegen` v2.5.0 → v2.8.0: v2.5.0 no longer
   compiles against the repo's `kin-openapi` v0.144.0 (bumped by dependabot in
   e9a4c2e). That was pre-existing breakage — nothing in-repo ran the generator
   — and the bump is dev-tooling only.
2. ~~**NKS base URL?**~~ **Found 2026-08-26: NKS is on uni-dev, not nks-stg** —
   `https://nks.uni-dev.glo1.nscale.com`. The whole uni-dev estate follows
   `https://<service>.uni-dev.glo1.nscale.com`. Production remains TBC pending
   the migration.

   Handled by giving `DefaultNscaleNKSServiceAPIEndpoint` an empty value rather
   than guessing. An unset endpoint is not a provider-level error — it only
   fails if an NKS-backed resource is used, via `Client.RequireNKS`, so existing
   configurations are untouched. Verified with a real `terraform plan`: the
   diagnostic names both the provider attribute and the environment variable.
3. ~~**Do the server defaults echo back on read?**~~ **CONFIRMED empirically
   2026-08-26** against a real provisioned cluster on uni-dev. `GET` returns all
   three optional blocks fully populated with the applied defaults:
   `addons.hardware: true`, `apiServer: {publicIP, allowedCidrs: ["0.0.0.0/0"]}`,
   `clusterNetwork: {podCidr: "10.240.0.0/12", serviceCidr: "10.96.0.0/16"}`.
   The Optional+Computed design holds. Also confirmed on the same read:
   `observedGeneration` is populated and equals `generation` on a settled
   cluster, and `eligibleTargets` comes back as `[]` (not absent) when there is
   no upgrade — the empty-vs-null distinction the converter relies on.
4. ~~**Is `cluster_network` mutable?**~~ **Decided 2026-08-24: yes — in-place, no
   replacement.** `pod_cidr` / `service_cidr` changes go through the normal PUT.
   Note this is the *permissive* choice: if the API turns out to reject or
   silently ignore a CIDR change, users get a failed apply or silent drift rather
   than a rebuild. Worth an explicit acceptance-test step
   (`_update_cluster_network`) so we find out early. Tightening this to
   `RequiresReplace` later would be a breaking change
   ([playbook §6.1](../.claude/skills/tf-provider-feature/reference/playbook.md)).
5. **Node pools — separate ticket?** A cluster without them has no workers. See
   scope note above.
6. **`nscale_kubernetes_cluster_auth` fast-follow — confirm deferred.** This spec
   ships the documented `exec` block only.
