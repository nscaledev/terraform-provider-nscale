terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

provider "nscale" {}

# Accelerator and unit are public capacity shapes offered per region (for
# example GB300 / NVL72). The valid combinations are region-specific and
# enforced by the API. region_id and project_id default to the provider's
# configured region (NSCALE_REGION_ID) and project (NSCALE_PROJECT_ID).
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

variable "image_id" {
  type        = string
  description = "The identifier of an existing image to boot each pinned server with."
}

variable "unit_count" {
  type        = number
  description = "The number of contiguous reservation units to reserve."
  default     = 1
}

# The reservation unit describes the capacity shape offered to this organization
# in this region: how many hosts one unit spans, what each host provides, and
# how much is currently free. Reading it means the placement below never has to
# hardcode a host count.
data "nscale_reservation_unit" "training" {
  accelerator = var.accelerator
  unit        = var.unit
}

# Reserve one or more contiguous accelerator reservation units in a region.
resource "nscale_reservation" "training" {
  name        = "gb300-nvl72"
  description = "Reserved accelerator units for training."
  accelerator = var.accelerator
  unit        = var.unit
  unit_count  = var.unit_count

  tags = {
    workload = "training"
  }

  # Fail during plan with a clear message rather than during apply with a 507.
  # Capacity is shared across the organization, so this narrows the window
  # rather than closing it — an apply can still lose a race for the last block.
  lifecycle {
    precondition {
      condition = var.unit_count <= data.nscale_reservation_unit.training.largest_contiguous_unit_count
      error_message = format(
        "Requested %d contiguous %s %s units but the largest contiguous block currently available is %d.",
        var.unit_count,
        var.accelerator,
        var.unit,
        data.nscale_reservation_unit.training.largest_contiguous_unit_count,
      )
    }
  }
}

# The network determines the InfiniBand partition boundary; all hosts in a
# placement share a single partition key.
resource "nscale_network" "training" {
  name       = "training"
  cidr_block = "192.168.0.0/24"
}

resource "nscale_security_group" "training" {
  name = "training"

  rules = [
    {
      type      = "ingress"
      protocol  = "tcp"
      from_port = 22
    }
  ]

  network_id = nscale_network.training.id
}

# Allocate hosts from the reservation, driving pinned Region server creation for
# each selected host.
resource "nscale_placement" "workers" {
  name           = "training-workers"
  reservation_id = nscale_reservation.training.id
  network_id     = nscale_network.training.id

  # Every host the reservation claimed. hosts_per_unit is the unit's fixed
  # topology (an NVL72 unit is 18 hosts), so this stays correct if unit_count
  # changes or the platform revises the unit shape.
  host_count = (
    nscale_reservation.training.claimed_unit_count *
    data.nscale_reservation_unit.training.hosts_per_unit
  )

  constraints = {
    policy             = "spread"
    max_skew           = 1
    when_unsatisfiable = "fail"
  }

  server_spec = {
    image_id = var.image_id

    networking = {
      security_group_ids = [nscale_security_group.training.id]
    }
  }
}

output "reservation_machine_flavor_id" {
  description = "The Region machine flavor resolved for the reservation."
  value       = nscale_reservation.training.machine_flavor_id
}

output "placement_ready_host_count" {
  description = "The number of hosts whose Region server resources are ready."
  value       = nscale_placement.workers.ready_host_count
}

output "reservation_unit" {
  description = "The shape of one reservation unit, and the capacity available at the last refresh."
  value = {
    hosts_per_unit                = data.nscale_reservation_unit.training.hosts_per_unit
    largest_contiguous_unit_count = data.nscale_reservation_unit.training.largest_contiguous_unit_count
  }
}
