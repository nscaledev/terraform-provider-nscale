# NKS in Terraform — the user-facing surface

What this adds to the provider, and what the published documentation will say.
Everything here is what a practitioner sees; nothing about how it is built
internally.

| Type | Status | Page |
| --- | --- | --- |
| Resource: `nscale_kubernetes_cluster` | agreed | `website/docs/r/kubernetes_cluster.html.markdown` |
| Data Source: `nscale_kubernetes_cluster` | agreed | `website/docs/d/kubernetes_cluster.html.markdown` |
| Data Source: `nscale_kubernetes_platform_releases` | agreed | `website/docs/d/kubernetes_platform_releases.html.markdown` |
| Resource: `nscale_kubernetes_node_pool` | **proposed** | not yet written |
| Data Source: `nscale_kubernetes_node_pool` | **proposed** | not yet written |
| Runnable example | | `examples/kubernetescluster/main.tf` |

The node pool sections are a proposal and still changeable; everything else
describes behaviour that has been built and exercised against a live API.

---

## Provider configuration

One new provider attribute:

| Attribute | Env var | Default |
| --- | --- | --- |
| `nks_service_api_endpoint` | `NSCALE_NKS_SERVICE_API_ENDPOINT` | **none** |

NKS is the only service without a default endpoint, because the service is
pending a migration and its production hostname is not settled. Leaving it unset
is **not** a provider error — it only fails if you actually use an
`nscale_kubernetes_*` type, and the message names both the attribute and the
environment variable. Configurations that touch no Kubernetes resources are
unaffected.

```hcl
provider "nscale" {
  nks_service_api_endpoint = "https://nks.example.nscale.com"
}
```

---

## Resource: `nscale_kubernetes_cluster`

A managed Kubernetes control plane attached to an `nscale_network`.

### Example

```hcl
resource "nscale_network" "main" {
  name       = "kubernetes"
  cidr_block = "10.0.0.0/16"

  dns_nameservers = ["1.1.1.1"]
}

# Select a release rather than hardcoding an ID — the catalogue moves.
data "nscale_kubernetes_platform_releases" "eligible" {
  region_id  = nscale_network.main.region_id
  deprecated = false
  withdrawn  = false
}

resource "nscale_kubernetes_cluster" "main" {
  name                = "production"
  network_id          = nscale_network.main.id
  platform_release_id = data.nscale_kubernetes_platform_releases.eligible.releases[0].id

  # Nested attributes, not blocks — note the `=`.
  api_server = {
    public_ip     = true
    allowed_cidrs = ["203.0.113.0/24"]
  }

  addons = {
    hardware = true
  }

  # `timeouts` is a block — no `=`.
  timeouts {
    create = "90m"
  }
}
```

### Arguments

| Argument | Type | Required | Changing it |
| --- | --- | --- | --- |
| `name` | String | yes | updates in place |
| `network_id` | String | yes | **forces replacement** |
| `platform_release_id` | String | yes | in-place cluster upgrade |
| `description` | String | no | updates in place |
| `tags` | Map(String) | no | updates in place |
| `api_server` | Object | no | updates in place |
| `cluster_network` | Object | no | updates in place |
| `addons` | Object | no | updates in place |

`api_server`:

| Field | Type | Default |
| --- | --- | --- |
| `public_ip` | Bool | `false` |
| `allowed_cidrs` | Set(String) | `["0.0.0.0/0"]` — 1–32 IPv4 CIDRs |

`cluster_network`:

| Field | Type | Default |
| --- | --- | --- |
| `pod_cidr` | String | `10.240.0.0/12` |
| `service_cidr` | String | `10.96.0.0/16` |

`addons`:

| Field | Type | Default |
| --- | --- | --- |
| `hardware` | Bool | **`true`** |

All defaults are applied by the API, not the provider, and are read back into
state. Omit a block to accept them.

If `public_ip` is `true` and `allowed_cidrs` is omitted, the API server is
reachable from anywhere — the default allowlist is `0.0.0.0/0`.

### Attributes

| Attribute | Type | Notes |
| --- | --- | --- |
| `id` | String | |
| `project_id` | String | inherited from `network_id` |
| `organization_id` | String | inherited from `network_id` |
| `region_id` | String | inherited from `network_id` |
| `creation_time` | String | |
| `provisioning_status` | String | `pending`, `provisioning`, `provisioned`, `deprovisioning`, `error` |
| `health_status` | String | `healthy`, `degraded`, `error`, `unknown` |
| `kubernetes_version_target` | String | version applied to the control plane |
| `kubernetes_version_observed` | String | version the control plane reports |
| `api_server_endpoint` | Object | null until the control plane is reachable |
| `applied_platform_release_id` | String | lags `platform_release_id` during an upgrade |
| `platform_release_deprecated` | Bool | |
| `platform_release_withdrawn` | Bool | |
| `upgrade_available` | Bool | |
| `eligible_upgrade_target_ids` | List(String) | valid upgrade targets, in order |

`api_server_endpoint`:

| Field | Type | Notes |
| --- | --- | --- |
| `certificate_authority_data` | String | base64 CA bundle. Public key material, **not** a secret |
| `private_host` / `private_port` | String / Number | always present once provisioned |
| `public_host` / `public_port` | String / Number | null unless `api_server.public_ip` is enabled |

### You cannot set the project

Unlike every other project-scoped resource in this provider,
`nscale_kubernetes_cluster` takes **no `project_id`**. The NKS API has no field
for it: a cluster's project, organization and region are all derived from the
network you attach to.

You choose scope by choosing the network. Setting `project_id`,
`organization_id` or `region_id` is an error.

This is also part of why `network_id` forces replacement — changing it is a move
between projects, and the API rejects it on an existing cluster.

### Timeouts

| | Default |
| --- | --- |
| `create` | 60m |
| `update` | 90m |
| `delete` | 30m |

Sized from measurement: cluster builds have been observed anywhere from ~6 to
~32 minutes in the same region on the same release, so the defaults allow
roughly twice the slowest observation. Deletes take around 2 minutes.

Raise these rather than lower them. A create timeout shorter than the real build
time fails the apply on a cluster that was going to come up fine — and leaves it
running and billing, with Terraform no longer tracking it.

### Provisioning behaviour

Creates, updates and deletes are asynchronous, and Terraform waits for each.

An apply returns only once the cluster's status genuinely reflects the change
you made. That matters because NKS reports status asynchronously and separately
from the write itself, so for a window after any apply the status describes the
*previous* configuration. Waiting past that window is what guarantees the
computed values written to state — endpoints, versions, applied release —
describe the cluster you actually asked for.

A cluster reporting `provisioned` but `health_status = "error"` is treated as a
failure, and the API's own explanation is surfaced. Transitional health
(`degraded`, `unknown`) is not a failure — addons and node registration settle
after the control plane first reports provisioned.

### Import

```sh
terraform import nscale_kubernetes_cluster.main <cluster-id>
```

Every attribute round-trips; there are no unrecoverable fields. NKS returns no
secrets, and every argument is readable back from the cluster.

The one wrinkle: `timeouts` is configuration-only and no API returns it, so if
your configuration has a `timeouts` block the first plan after import shows an
update to add it. Applying is harmless. This affects every resource with
timeouts, not just this one.

### Connecting to the cluster

There is no kubeconfig endpoint and no token endpoint. `api_server_endpoint`
gives you the CA bundle and addresses; authentication goes through the `nscale`
CLI acting as a client-go credential plugin:

```hcl
provider "kubernetes" {
  host = format(
    "https://%s:%s",
    nscale_kubernetes_cluster.main.api_server_endpoint.public_host,
    nscale_kubernetes_cluster.main.api_server_endpoint.public_port,
  )
  cluster_ca_certificate = base64decode(
    nscale_kubernetes_cluster.main.api_server_endpoint.certificate_authority_data
  )

  exec {
    api_version = "client.authentication.k8s.io/v1"
    command     = "nscale"
    args        = ["kubernetes", "token"]
  }
}
```

Requires the `nscale` CLI on the machine running `terraform apply`; in CI, set
`NSCALE_SERVICE_TOKEN`. A stock released CLI works — the `token` subcommand is
not feature-gated.

The advantage over a data source returning a token is that no credential is
written to Terraform state. Use `private_host` instead when running from inside
the network.

### Notes

- **Updates replace the whole spec.** NKS exposes `PUT`, not `PATCH`. Removing an
  argument from your configuration clears it rather than leaving it alone.
- **Choose a current release for a new cluster** — neither deprecated nor
  withdrawn. Creating on a deprecated release fails with HTTP 422. When
  *upgrading*, a release that has since been deprecated is still selectable
  (you must be able to re-state the one you are on); a withdrawn one is not.
- Selecting a release unavailable in the network's region also fails with 422.

---

## Data Source: `nscale_kubernetes_cluster`

Reads a cluster by ID — for referencing one Terraform does not manage.

```hcl
data "nscale_kubernetes_cluster" "existing" {
  id = "b2c8d4e6-1a3f-4c7b-9e2d-5f8a0c1b3d7e"
}
```

`id` is required; every other attribute matches the resource and is computed.

Lookup is by ID only. The NKS list API matches names exactly and
case-sensitively, and display names are not unique, so name lookup would be
ambiguous.

---

## Data Source: `nscale_kubernetes_platform_releases`

Lists the platform releases a cluster can run. Use this instead of hardcoding a
`platform_release_id` — release IDs change as the catalogue moves, and a pinned
one goes stale the moment it is deprecated.

This is the provider's first plural data source: it returns a list rather than
looking one object up by ID.

```hcl
data "nscale_kubernetes_platform_releases" "eligible" {
  region_id  = nscale_network.main.region_id
  deprecated = false
  withdrawn  = false
}

# releases[0] is the newest match — the API returns catalogue order.
output "newest" {
  value = data.nscale_kubernetes_platform_releases.eligible.releases[0].id
}
```

### Filters — all optional

| Filter | Type | Notes |
| --- | --- | --- |
| `organization_id` | String | defaults to the provider's organization |
| `region_id` | String | |
| `architecture` | String | `x86_64` or `aarch64` |
| `prerelease` | Bool | |
| `deprecated` | Bool | |
| `withdrawn` | Bool | |

**An omitted filter is different from `false`.** `deprecated = false` returns
only current releases; omitting `deprecated` returns current *and* deprecated
ones. Unset filters are not sent to the API at all.

### `releases` — computed list

| Field | Type |
| --- | --- |
| `id` | String |
| `name` | String |
| `kubernetes_version` | String |
| `prerelease` | Bool |
| `deprecated` | Bool |
| `withdrawn` | Bool |
| `withdrawal_reason` | String |
| `withdrawal_message` | String |
| `supported_architectures` | List(String) |
| `available_region_ids` | List(String) |
| `usable_organization_ids` | List(String) |

### Which filters to use

- **Creating a cluster** — `deprecated = false` and `withdrawn = false`.
- **Upgrading** — `withdrawn = false` only. You may need to re-state a release
  your cluster is already on which has since been deprecated, and filtering it
  out would make that configuration un-appliable.
- **Browsing** — omit the filters; each release carries the flags as attributes
  so you can filter in HCL.

When planning an upgrade, prefer `eligible_upgrade_target_ids` on the cluster
itself: the API computes eligibility for that specific cluster, which is
stricter than anything derivable from the catalogue.

`deprecated` means clusters should upgrade away from a release; it is an
operator decision and can be reversed. `withdrawn` is stronger — pulled from
selection, with a reason and message. `prerelease` versions are selectable but
are superseded quickly; avoid them in production.

---

---

## Resource: `nscale_kubernetes_node_pool` — PROPOSED

> **Not yet implemented.** This section is the proposed surface, circulated for
> agreement. Everything above describes shipped behaviour; everything here is
> still changeable. Detail and open questions in
> [`kubernetes_node_pool.md`](kubernetes_node_pool.md).

The workers for a cluster. NKS models node pools as a separate top-level
resource rather than a field on the cluster, so they are a separate Terraform
resource that references the cluster by ID.

A cluster with no node pools has nowhere to schedule work, so in practice every
cluster has at least one.

### Example

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
    workload = "general"
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

  # Editing taints rolls every worker in this pool. See below.
  taints = [{
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NoSchedule"
  }]
}
```

### Arguments

| Argument | Type | Required | Changing it |
| --- | --- | --- | --- |
| `name` | String | yes | updates in place |
| `cluster_id` | String | yes | **forces replacement** |
| `provisioning_mode` | String | yes | **forces replacement** |
| `replicas` | Number | yes | scales in place |
| `description` | String | no | updates in place |
| `tags` | Map(String) | no | updates in place |
| `compute` | Object | if mode is `compute` | updates in place |
| `reservation` | Object | if mode is `reservation` | updates in place |
| `taints` | List(Object) | no | in place, **rolls the pool** |
| `labels` | Map(String) | no | in place, **rolls the pool** |

`provisioning_mode` is `compute` or `reservation` and selects which capacity
block applies:

| Block | Field | Notes |
| --- | --- | --- |
| `compute` | `flavor_id` | required when mode is `compute` |
| `reservation` | `reservation_id` | required when mode is `reservation`; pairs with `nscale_reservation` |

Supplying the wrong block for the mode, or both, fails at **plan** time rather
than apply.

`replicas` has a minimum of **0** — scaling a pool to zero is legal and keeps
the pool definition without any workers.

`taints[]` takes `key` (required), `value` (optional) and `effect` — one of
`NoSchedule`, `PreferNoSchedule`, `NoExecute`. Maximum 64 taints; `labels` is
capped at 64 entries.

### Editing taints or labels replaces every node in the pool

The API does not reconcile taints or labels onto running nodes. A change applies
to newly created workers, and the pool's existing workers are **rolled** so it
takes effect.

Terraform will show this as an ordinary in-place update, because that is what it
is at the API level — the plan cannot warn you. Treat a taint or label edit as a
rolling replacement of the pool, and size `replicas` and disruption budgets
accordingly.

### Attributes

| Attribute | Type | Notes |
| --- | --- | --- |
| `id` | String | |
| `project_id` / `organization_id` / `region_id` | String | inherited via the cluster |
| `creation_time` | String | |
| `provisioning_status` / `health_status` | String | as the cluster |
| `current_replicas` | Number | workers that exist |
| `ready_replicas` | Number | workers ready to schedule |
| `up_to_date_replicas` | Number | workers on the current pool template |
| `kubernetes_version` | String | version the workers report |
| `applied_platform_release_id` | String | release pinned to this pool |
| `platform_release_kubernetes_version` | String | |
| `platform_release_deprecated` / `_withdrawn` | Bool | |
| `placement_id` | String | reservation mode only |

The three replica counts are what tell you whether a scale or a roll has
finished: `current` is how many exist, `ready` how many can take work, and
`up_to_date` how many are on the latest template. During a taint-driven roll,
`up_to_date` climbs as workers are replaced.

### Timeouts, provisioning and import

Same model as the cluster: asynchronous create, update and delete, each waited
on, with the same settledness rule so an apply never returns while the reported
status still describes the previous configuration.

Proposed defaults are 30m across create, update and delete — worker VMs joining
an existing cluster should be quicker than a control plane build. **These are
assumptions and will be corrected by measurement**, exactly as the cluster's
were: its 30m create default turned out to be too short on the first real apply.

Update shares the create timeout because a taint or label edit rolls the whole
pool, which costs about as much as building it.

Import is passthrough on the pool ID.

### Notes

- Updates replace the whole spec, as with the cluster. Scaling `replicas` must
  not disturb `taints` or `labels`, so the provider rebuilds the full spec from
  configuration on every update.
- A pool pins its own platform release, reported separately from the cluster's.
- Deleting a cluster removes its node pools. Terraform normally destroys the
  pools first, because they depend on `cluster_id`.

---

## Not included

**A production endpoint default.** Pending the NKS service migration;
`nks_service_api_endpoint` must be set explicitly until then.
