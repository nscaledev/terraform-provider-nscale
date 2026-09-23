terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

# NKS is the only service with no default endpoint baked into the provider, so
# it must be set explicitly — here, or via NSCALE_NKS_SERVICE_API_ENDPOINT.
# Everything else comes from the standard NSCALE_* environment variables.
provider "nscale" {
  nks_service_api_endpoint = var.nks_service_api_endpoint
}

variable "nks_service_api_endpoint" {
  type        = string
  description = "Base URL of the NKS API. Leave null to use NSCALE_NKS_SERVICE_API_ENDPOINT."
  default     = null
}

variable "name" {
  type        = string
  description = "Name for the cluster and its network. Immutable once the cluster exists."
  default     = "kubernetes-example"
}

variable "network_id" {
  type        = string
  description = "Existing region network to attach to. Leave null to create one."
  default     = null
}

variable "network_cidr_block" {
  type        = string
  description = "Prefix for the created network. Ignored when network_id is set."
  default     = "10.0.0.0/16"
}

variable "pod_cidr" {
  type        = string
  description = "IPv4 CIDR for pod addresses. Fixed for the life of the cluster."
  default     = "100.65.0.0/16"
}

variable "service_cidr" {
  type        = string
  description = "IPv4 CIDR for service addresses. Fixed for the life of the cluster."
  default     = "172.20.0.0/16"
}

variable "api_server_public_ip" {
  type        = bool
  description = "Expose the Kubernetes API server on a public endpoint."
  default     = false
}

# The API's own default is ["0.0.0.0/0"], so an omitted allowlist on a public
# API server is reachable from anywhere — hence a deliberately narrow default.
variable "api_server_allowed_cidrs" {
  type        = list(string)
  description = "Source IPv4 CIDR allowlist for the API server endpoint (1-32 entries)."
  default     = ["10.0.0.0/8"]
}

locals {
  create_network = var.network_id == null
  network_id     = local.create_network ? nscale_network.main[0].id : data.nscale_network.existing[0].id
  region_id      = local.create_network ? nscale_network.main[0].region_id : data.nscale_network.existing[0].region_id
}

resource "nscale_network" "main" {
  count = local.create_network ? 1 : 0

  name       = var.name
  cidr_block = var.network_cidr_block

  dns_nameservers = ["1.1.1.1"]
}

data "nscale_network" "existing" {
  count = local.create_network ? 0 : 1

  id = var.network_id
}

# Pick a release eligible for a NEW cluster rather than hardcoding an ID, which
# goes stale as the catalogue moves. Creating on a deprecated release fails 422.
# When upgrading rather than creating, filter only on `withdrawn` — you may need
# to re-state a release your cluster is on which has since been deprecated.
#
# The region comes off the network, not NSCALE_REGION_ID: a release from any
# other region fails at apply with a 422.
data "nscale_kubernetes_platform_releases" "eligible" {
  region_id  = local.region_id
  deprecated = false
  withdrawn  = false
}

# The cluster's project, organization and region are all inherited from the
# network it attaches to — it takes no project_id of its own.
resource "nscale_kubernetes_cluster" "main" {
  # name, network_id and both cluster_network CIDRs are immutable: the API
  # rejects a change to any of them, so Terraform replaces the cluster instead.
  name        = var.name
  description = "Example NKS cluster managed by Terraform"

  network_id          = local.network_id
  platform_release_id = data.nscale_kubernetes_platform_releases.eligible.releases[0].id

  # Nested attributes, not blocks — note the `=`.
  #
  # Omit the block to take the API defaults (10.240.0.0/12 and 10.96.0.0/16).
  # Either way the values are fixed for the life of the cluster, so a collision
  # discovered later means a rebuild rather than an update.
  cluster_network = {
    pod_cidr     = var.pod_cidr
    service_cidr = var.service_cidr
  }

  # allowed_cidrs is always set, never left null. It is Optional+Computed, so
  # omitting it inside a block that IS present leaves it unknown on every plan
  # and the example would never reach a clean "No changes." The allowlist
  # applies to the private endpoint as well as the public one.
  api_server = {
    public_ip     = var.api_server_public_ip
    allowed_cidrs = var.api_server_allowed_cidrs
  }

  # `addons` is omitted so the API applies its own defaults (hardware enabled on
  # create) and they are read back into state.

  tags = {
    environment = "example"
  }

  # Return as soon as the cluster exists, so the node pools below are POSTed
  # while it is still provisioning rather than after — which is what the console
  # does and what the API permits. Nothing goes unwaited-for: a pool's own wait
  # covers the control plane, since workers cannot reach ready before the API
  # server is up. What this resource loses is status freshness, which is why the
  # outputs read the data source at the bottom of this file instead.
  wait_for_provisioned = false

  # `timeouts` is a block, so no `=`. Defaults are 60m/90m/60m, sized from a
  # measured 32-minute build. Override only to raise them: a create timeout
  # shorter than the real build leaves a running, billing cluster that Terraform
  # is no longer tracking.
  timeouts {
    create = "90m"
  }
}

# ---------------------------------------------------------------------------
# Node pools. NKS models these as a separate top-level resource, so they
# reference the cluster by ID rather than being a block on it — which is what
# makes Terraform create the cluster first and destroy the pools first.
# ---------------------------------------------------------------------------

# Left null the pool is skipped: a cluster with no node pools is permitted by
# the API, so the example does not force one. Flavor IDs are region-specific
# UUIDs, so there is no honest default — use `nscale flavor list`.
variable "worker_flavor_id" {
  type        = string
  description = "Compute flavor each worker runs on. Must exist in the cluster's region. Null skips the pool."
  default     = null
}

variable "worker_replicas" {
  type        = number
  description = "Workers in the on-demand pool. 0 is valid and keeps the pool definition without running workers."
  default     = 2

  validation {
    condition     = var.worker_replicas >= 0
    error_message = "worker_replicas cannot be negative."
  }
}

variable "gpu_reservation_id" {
  type        = string
  description = "Reservation to draw GPU capacity from. Leave null to skip the reservation-backed pool."
  default     = null
}

variable "gpu_replicas" {
  type        = number
  description = "Workers in the reservation-backed pool. Changing this REBUILDS the pool — see the comment below."
  default     = 1
}

# An on-demand pool. This is the one that behaves the way a Terraform user
# expects: replicas, taints and labels are all in-place updates.
resource "nscale_kubernetes_node_pool" "workers" {
  count = var.worker_flavor_id == null ? 0 : 1

  # name, cluster_id, provisioning_mode and compute.flavor_id are all immutable,
  # so Terraform replaces the pool instead. To move a workload onto a different
  # flavour without an outage, add a second pool and drain this one.
  name        = "${var.name}-workers"
  description = "On-demand workers managed by Terraform"

  cluster_id        = nscale_kubernetes_cluster.main.id
  provisioning_mode = "compute"
  replicas          = var.worker_replicas

  # Exactly the block matching provisioning_mode must be present; supplying the
  # other one, or both, fails at plan time rather than at apply.
  compute = {
    flavor_id = var.worker_flavor_id
  }

  # Editing taints or labels ROLLS THE POOL. NKS applies both during node
  # registration and never reconciles them onto running nodes, so the only way a
  # change takes effect is by replacing every worker — one at a time, draining
  # each through the Eviction API. Terraform shows it as an ordinary in-place
  # update, because at the API level that is exactly what it is. Watch
  # up_to_date_replicas to follow the roll.
  taints = [{
    key    = "workload"
    value  = "general"
    effect = "PreferNoSchedule"
  }]

  labels = {
    tier = "standard"
  }

  tags = {
    environment = "example"
  }

  # Defaults are 30m/60m/60m. Update and delete are double create because both
  # walk the pool one worker at a time; cost is roughly replicas x per-node. No
  # timeout is safe against an unsatisfiable PodDisruptionBudget — there is no
  # drain timeout upstream — but a timeout mid-roll does not mean the roll
  # failed, and the next apply resumes it.
  timeouts {
    update = "90m"
  }
}

# A reservation-backed pool, drawing on capacity reserved through
# nscale_reservation.
resource "nscale_kubernetes_node_pool" "gpu" {
  count = var.gpu_reservation_id == null ? 0 : 1

  name        = "${var.name}-gpu"
  description = "Reservation-backed GPU workers managed by Terraform"

  cluster_id        = nscale_kubernetes_cluster.main.id
  provisioning_mode = "reservation"
  replicas          = var.gpu_replicas

  reservation = {
    reservation_id = var.gpu_reservation_id
  }

  # The placement backing this pool NEVER ROLLS, so the API refuses any edit
  # that could only take effect by recycling workers. replicas, taints and
  # labels therefore all force REPLACEMENT here — `replicas 1 -> 2` is a rebuild
  # that releases and re-claims the placement, not a scale. Only description and
  # tags change in place. The plan does show the replacement.
  taints = [{
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NoSchedule"
  }]

  tags = {
    environment = "example"
  }
}

# Read the cluster back once the pools exist. By then the control plane is
# necessarily up, so this returns settled values the resource above does not yet
# have. depends_on is what forces it to run last — without it Terraform is free
# to read the cluster while the pools are still being created.
data "nscale_kubernetes_cluster" "ready" {
  id = nscale_kubernetes_cluster.main.id

  depends_on = [
    nscale_kubernetes_node_pool.workers,
    nscale_kubernetes_node_pool.gpu,
  ]
}
