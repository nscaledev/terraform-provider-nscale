# Stack 1 of 2: capacity.
#
# Owns the reservation and nothing else. It is split from the cluster stack
# because the two have different lifecycles: a reservation is a long-lived
# capacity commitment that can take up to an hour to provision, while clusters
# and node pools come and go. In separate states, a failed or retried cluster
# apply never waits on, or risks releasing, the reservation.
#
# Apply this first, then pass its reservation_id output to ../cluster.

terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

# region_id and project_id default to NSCALE_REGION_ID and NSCALE_PROJECT_ID.
# The cluster stack builds its network from this reservation's region and
# project, so pick a region NKS offers a platform release in.
provider "nscale" {}

variable "name" {
  type        = string
  description = "Name for the reservation."
  default     = "nks-gpu"
}

variable "accelerator" {
  type        = string
  description = "The public accelerator model or family to reserve, e.g. GB300."
  default     = "GB300"
}

variable "unit" {
  type        = string
  description = "The public reservation granularity to reserve, e.g. NVL72."
  default     = "NVL72"
}

variable "unit_count" {
  type        = number
  description = "The number of contiguous reservation units to reserve."
  default     = 1
}

# The unit's fixed shape (hosts per unit) and current availability.
data "nscale_reservation_unit" "gpu" {
  accelerator = var.accelerator
  unit        = var.unit
}

resource "nscale_reservation" "gpu" {
  name        = var.name
  description = "Capacity for NKS reservation-backed node pools."
  accelerator = var.accelerator
  unit        = var.unit
  unit_count  = var.unit_count

  # Provisioning can take up to an hour. 90m is the default, set here to make
  # it visible: a create that times out leaves the reservation tainted, and the
  # next apply would destroy it and start the hour again, so raise this rather
  # than retry.
  timeouts {
    create = "90m"
  }

  # Destroying a reservation releases the capacity and deletes every placement
  # taken from it, including the ones backing node pools. Remove this
  # deliberately when you mean to give the capacity back.
  lifecycle {
    prevent_destroy = true
  }
}

# Advisory: warn at plan time rather than failing with a 507 at apply. See
# examples/reservation for why this is a check and not a precondition.
check "reservation_capacity" {
  assert {
    condition = var.unit_count <= data.nscale_reservation_unit.gpu.largest_contiguous_unit_count
    error_message = format(
      "Requested %d contiguous %s %s units but the largest contiguous block currently available is %d.",
      var.unit_count, var.accelerator, var.unit,
      data.nscale_reservation_unit.gpu.largest_contiguous_unit_count,
    )
  }
}

output "reservation_id" {
  description = "Pass to the cluster stack as reservation_id."
  value       = nscale_reservation.gpu.id
}

output "host_count" {
  description = "Hosts the reservation holds: the most workers its node pools can claim in total."
  value       = nscale_reservation.gpu.claimed_unit_count * data.nscale_reservation_unit.gpu.hosts_per_unit
}
