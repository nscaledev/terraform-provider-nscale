terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

# NKS is the only service with no default endpoint baked into the provider, so
# it must be set explicitly — here, or via NSCALE_NKS_SERVICE_API_ENDPOINT.
# Leaving it unset is not a provider error on its own; it only fails when a
# nscale_kubernetes_* type is actually used.
#
# Everything else comes from the standard NSCALE_* environment variables
# (NSCALE_SERVICE_TOKEN, NSCALE_ORGANIZATION_ID, NSCALE_PROJECT_ID,
# NSCALE_REGION_ID).
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

# Set this to attach to an existing network instead of creating one. The
# cluster's project, organization and region are all inherited from whichever
# network it attaches to — a cluster takes no project_id of its own.
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
# API server is reachable from anywhere — hence an explicit, deliberately narrow
# default here. Widen it deliberately, not by accident.
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
# goes stale as the catalogue moves. Creating on a deprecated release fails with
# HTTP 422.
#
# An omitted filter is not the same as false: omitting `deprecated` returns
# current AND deprecated releases. When upgrading rather than creating, filter
# only on `withdrawn` — you may need to re-state a release your cluster is
# already on which has since been deprecated.
#
# The region comes off the network, not from NSCALE_REGION_ID: the cluster's
# region is whatever the network says it is, and a release from any other region
# fails at apply with a 422.
data "nscale_kubernetes_platform_releases" "eligible" {
  region_id  = local.region_id
  deprecated = false
  withdrawn  = false
}

resource "nscale_kubernetes_cluster" "main" {
  # name, network_id and both cluster_network CIDRs are immutable: the API
  # rejects a change to any of them, so Terraform replaces the cluster instead.
  name        = var.name
  description = "Example NKS cluster managed by Terraform"

  network_id          = local.network_id
  platform_release_id = data.nscale_kubernetes_platform_releases.eligible.releases[0].id

  # Nested attributes, not blocks — note the `=`.
  #
  # Pod and service addresses stay clear of the attached network's own prefix.
  # Omit the block to take the API defaults (10.240.0.0/12 and 10.96.0.0/16);
  # either way the values are fixed for the life of the cluster, so a collision
  # discovered later means a rebuild rather than an update.
  cluster_network = {
    pod_cidr     = var.pod_cidr
    service_cidr = var.service_cidr
  }

  # allowed_cidrs is always set, never left null. It is Optional+Computed, so
  # omitting it inside a block that is present leaves it unknown on every plan
  # and the example would never reach a clean "No changes." The allowlist
  # applies to the private endpoint as well as the public one.
  api_server = {
    public_ip     = var.api_server_public_ip
    allowed_cidrs = var.api_server_allowed_cidrs
  }

  # `addons` is omitted so the API applies its own defaults (hardware is enabled
  # on create). It is Optional+Computed, so whatever the server chooses is read
  # back into state — set the block explicitly only to override that.

  tags = {
    environment = "example"
  }

  # Return as soon as the cluster exists, so the node pool below is POSTed while
  # the cluster is still provisioning — which is what the console does, and what
  # the API permits. Ordering becomes: create cluster, create pool, then read the
  # settled cluster back through the data source at the bottom of this file.
  #
  # Nothing is unwaited-for. The pool's own wait covers the control plane, since
  # workers cannot reach ready before the API server is up, so a control plane
  # that fails still fails the apply. What this resource's status attributes lose
  # is freshness: they describe the moment of creation until the next refresh,
  # which is why the outputs read the data source instead.
  #
  # Set wait_for_provisioned = true (the default) if you would rather this
  # resource block and have its own status be authoritative — the cost is that
  # the pool then waits for the whole control-plane build before it starts.
  wait_for_provisioned = false

  # `timeouts` is a block, so no `=`. Defaults are 60m create / 90m update /
  # 60m delete, sized from a measured 32-minute build. Override only to raise
  # them — a create timeout shorter than the real build time fails an apply on
  # a cluster that was going to come up fine, and leaves it running and billing
  # with Terraform no longer tracking it.
  #
  # Destroying a cluster that has node pools drains every worker first and
  # honours PodDisruptionBudgets, and an unsatisfiable PDB blocks that with no
  # deadline. If a destroy times out, fix the PDB rather than raise this.
  timeouts {
    create = "90m"
  }
}

# ---------------------------------------------------------------------------
# Node pools
#
# Creating a cluster and adding a pool are separate actions, and a pool can be
# added at any time — including before the cluster is provisioned and healthy.
# NKS models pools as a separate top-level resource, so they reference the
# cluster by ID rather than being a block on it, which is also what makes
# Terraform create the cluster first and destroy the pools first.
# ---------------------------------------------------------------------------

# Set this and you get a worker pool, which is what you normally want. Left
# null the pool is skipped: a cluster with no node pools is permitted by the API
# and a legitimate thing to run, so the example does not force one.
#
# Flavor IDs are region-specific UUIDs, so there is no honest default to offer —
# look one up with the region service, or `nscale flavor list`.
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

# Set this to add a reservation-backed pool alongside the on-demand one. Left
# null the pool is skipped, because a reservation-backed pool cannot be created
# without reserved capacity to draw on.
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

  # name, cluster_id, provisioning_mode and compute.flavor_id are all immutable:
  # the API rejects a change to any of them, so Terraform replaces the pool
  # instead. A flavour change in particular is a full rebuild — to move a
  # workload onto a different flavour without an outage, add a second pool and
  # drain this one.
  name        = "${var.name}-workers"
  description = "On-demand workers managed by Terraform"

  cluster_id        = nscale_kubernetes_cluster.main.id
  provisioning_mode = "compute"
  replicas          = var.worker_replicas

  # Nested attribute, not a block — note the `=`. Exactly the block matching
  # provisioning_mode must be present; supplying the other one, or both, fails
  # at plan time rather than at apply.
  compute = {
    flavor_id = var.worker_flavor_id
  }

  # Editing taints or labels ROLLS THE POOL. NKS applies both during node
  # registration and never reconciles them onto running nodes, so the only way a
  # change takes effect is by replacing every worker — one at a time, draining
  # each through the Eviction API. Terraform shows it as an ordinary in-place
  # update, because at the API level that is exactly what it is.
  #
  # Watch up_to_date_replicas to follow a roll: it drops while workers are being
  # replaced and returns to replicas when the roll finishes.
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

  # Defaults are 30m create / 60m update / 60m delete. Update and delete are
  # double create because both walk the pool one worker at a time: an update may
  # roll every worker, and a delete drains every worker on the way out. Raise
  # update for a large pool — cost is roughly replicas x per-node time.
  #
  # No timeout can be made safe against a PodDisruptionBudget that cannot be
  # satisfied: there is no drain timeout upstream, so the block is indefinite and
  # this deadline is the only bound. A timeout mid-roll does not mean the roll
  # failed; the next apply resumes it.
  timeouts {
    update = "90m"
  }
}

# A reservation-backed pool, drawing on capacity reserved through
# nscale_reservation. This is the cross-resource story the cluster example does
# not otherwise show.
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

  # A reservation pool is close to immutable, and this is the sharp edge worth
  # knowing before you write it: the placement backing the pool NEVER ROLLS, so
  # the API refuses any edit that could only take effect by recycling workers.
  # replicas, taints and labels therefore all force REPLACEMENT here, not an
  # update — `replicas 1 -> 2` is a rebuild that releases and re-claims the
  # placement, not a scale. Only description and tags change in place.
  #
  # Terraform does warn about this one, because the plan shows a replacement.
  taints = [{
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NoSchedule"
  }]

  tags = {
    environment = "example"
  }
}

# Read the cluster back once the pool exists. This is the "then wait for the
# cluster" half of the ordering: by the time a pool is provisioned and healthy
# the control plane is necessarily up, so this read returns settled values that
# the resource above does not yet have.
#
# depends_on is what forces it to run last. Without it Terraform is free to read
# the cluster at the same time it creates the pool, which would defeat the point.
data "nscale_kubernetes_cluster" "ready" {
  id = nscale_kubernetes_cluster.main.id

  depends_on = [
    nscale_kubernetes_node_pool.workers,
    nscale_kubernetes_node_pool.gpu,
  ]
}
