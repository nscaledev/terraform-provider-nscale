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

# Reserve one or more contiguous accelerator reservation units in a region.
resource "nscale_reservation" "training" {
  name        = "gb300-nvl72"
  description = "Reserved accelerator units for training."
  accelerator = var.accelerator
  unit        = var.unit
  unit_count  = 1

  tags = {
    workload = "training"
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
  host_count     = 1

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
