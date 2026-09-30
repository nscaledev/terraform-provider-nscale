# Stack 2 of 2: cluster.
#
# An NKS cluster whose workers come from the reservation made by ../capacity.
# The reservation is passed in by ID rather than read from the other stack's
# state, so this stack works with any backend and never holds the capacity
# stack's state lock. (terraform_remote_state would work too.)
#
# Nothing here waits on the reservation: it must already be provisioned. A
# cluster apply is therefore ~10-35 minutes, never the reservation's hour, and
# destroying this stack leaves the reservation in place.

terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

provider "nscale" {
  nks_service_api_endpoint = var.nks_service_api_endpoint
}

variable "nks_service_api_endpoint" {
  type        = string
  description = "Base URL of the NKS API. Leave null to use NSCALE_NKS_SERVICE_API_ENDPOINT."
  default     = null
}

variable "reservation_id" {
  type        = string
  description = "The reservation_id output of the capacity stack."
}

variable "name" {
  type        = string
  description = "Name for the cluster, its network and its node pool. Immutable once the cluster exists."
  default     = "nks-gpu"
}

variable "network_cidr_block" {
  type        = string
  description = "Prefix for the cluster's network."
  default     = "10.0.0.0/16"
}

variable "gpu_replicas" {
  type        = number
  description = "Workers to claim from the reservation. Changing this REBUILDS the pool: a placement never resizes."
  default     = 1
}

variable "placement_policy" {
  type        = string
  description = "How workers are placed across topology domains: pack or spread. Fixed at creation."
  default     = "spread"
}

# Read the reservation so the network, and therefore the cluster and pool, land
# in the same region and project. A pool can only draw from a reservation in its
# cluster's region.
data "nscale_reservation" "capacity" {
  id = var.reservation_id

  lifecycle {
    postcondition {
      condition     = self.provisioning_status == "provisioned"
      error_message = "The reservation is still ${self.provisioning_status}. Wait for the capacity stack's apply to finish, then plan again."
    }
  }
}

data "nscale_reservation_unit" "capacity" {
  accelerator = data.nscale_reservation.capacity.accelerator
  unit        = data.nscale_reservation.capacity.unit
  region_id   = data.nscale_reservation.capacity.region_id
}

# Advisory, not a precondition: other pools may share the reservation, so the
# free host count is not knowable from here. This catches asking for more hosts
# than the reservation holds at all.
check "enough_reserved_hosts" {
  assert {
    condition = var.gpu_replicas <= data.nscale_reservation.capacity.claimed_unit_count * data.nscale_reservation_unit.capacity.hosts_per_unit
    error_message = format(
      "gpu_replicas is %d but the reservation holds %d hosts.",
      var.gpu_replicas,
      data.nscale_reservation.capacity.claimed_unit_count * data.nscale_reservation_unit.capacity.hosts_per_unit,
    )
  }
}

resource "nscale_network" "main" {
  name       = var.name
  cidr_block = var.network_cidr_block
  region_id  = data.nscale_reservation.capacity.region_id
  project_id = data.nscale_reservation.capacity.project_id
}

# A release eligible for a new cluster in the reservation's region. Creating
# on a deprecated release, or one not offered in the region, fails with a 422.
data "nscale_kubernetes_platform_releases" "eligible" {
  region_id  = data.nscale_reservation.capacity.region_id
  deprecated = false
  withdrawn  = false
  prerelease = false

  lifecycle {
    postcondition {
      condition     = length(self.releases) > 0
      error_message = "NKS offers no platform release in the reservation's region, so no cluster can be created there."
    }
  }
}

resource "nscale_kubernetes_cluster" "main" {
  name        = var.name
  description = "NKS cluster with reservation-backed workers"

  network_id          = nscale_network.main.id
  platform_release_id = data.nscale_kubernetes_platform_releases.eligible.releases[0].id

  # releases[0] moves when a new release ships, which would plan an upgrade with
  # no config change. To upgrade, remove this and set the ID explicitly.
  lifecycle {
    ignore_changes = [platform_release_id]
  }

  # The GPU operator. Enabled by default; shown because this cluster exists for GPUs.
  addons = {
    hardware = { enabled = true }
  }
}

resource "nscale_kubernetes_node_pool" "gpu" {
  name              = "${var.name}-gpu"
  cluster_id        = nscale_kubernetes_cluster.main.id
  provisioning_mode = "reservation"
  replicas          = var.gpu_replicas

  # NKS claims `replicas` hosts from the reservation as a placement, reported
  # back as placement_id. There is no nscale_placement resource here: that is
  # for using reservation hosts without Kubernetes.
  reservation = {
    reservation_id = data.nscale_reservation.capacity.id

    # Fixed at creation: changing, adding or removing it rebuilds the pool.
    constraints = {
      policy = var.placement_policy
    }
  }

  # A placement never rolls, so replicas, taints and labels all force a rebuild
  # here. Only description and tags change in place.
  taints = [{
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NoSchedule"
  }]

  # The pool waits for its workers to join, after the control plane is up.
  timeouts {
    create = "60m"
  }
}

output "cluster_id" {
  value = nscale_kubernetes_cluster.main.id
}

output "placement_id" {
  description = "The placement NKS created inside the reservation for this pool."
  value       = nscale_kubernetes_node_pool.gpu.placement_id
}

output "api_server_endpoint" {
  description = "Connection data for kubectl; authenticate with `nscale kubernetes token`."
  value       = nscale_kubernetes_cluster.main.api_server_endpoint
}
