terraform {
  required_providers {
    nscale = {
      source  = "nscaledev/nscale"
      version = "0.0.6"
    }
  }
}

provider "nscale" {}

data "nscale_region" "glo1" {
  id = "<glo1-region-id>"
}

data "nscale_instance_flavor" "g_4_standard_40s" {
  id        = "<g-4-standard-40s-flavor-id>"
  region_id = data.nscale_region.glo1.id
}

resource "nscale_compute_cluster" "example" {
  name      = "example"
  region_id = data.nscale_region.glo1.id

  workload_pools = [
    {
      name             = "default"
      replicas         = 1
      image_id         = "<image-id>"
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
