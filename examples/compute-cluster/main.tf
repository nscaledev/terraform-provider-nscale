terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

provider "nscale" {}

# Flavor and image are pre-configured platform resources; supply existing IDs.
# region_id defaults to the provider's configured region (NSCALE_REGION_ID).
variable "flavor_id" {
  type        = string
  description = "The identifier of an existing instance flavor."
}

variable "image_id" {
  type        = string
  description = "The identifier of an existing image."
}

data "nscale_instance_flavor" "g_4_standard_40s" {
  id = var.flavor_id
}

resource "nscale_compute_cluster" "example" {
  name = "example"

  workload_pools = [
    {
      name             = "default"
      replicas         = 1
      image_id         = var.image_id
      flavor_id        = data.nscale_instance_flavor.g_4_standard_40s.id
      enable_public_ip = true

      firewall_rules = [
        {
          direction = "ingress"
          protocol  = "tcp"
          ports     = 22
          prefixes  = ["0.0.0.0/0"]
        }
      ]
    }
  ]
}
