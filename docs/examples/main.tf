terraform {
  required_providers {
    nscale = {
      source = "hashicorp.com/nscale/nscale"
    }
  }
}

resource "nscale_compute_cluster" "test" {
  name = "terraform-test-cluster"
  // no-glo1
  region_id = "62f35744-1abd-47b8-a850-cfaa03fd3ca6"

  workload_pools = [
    {
      name     = "default"
      replicas = 1
      // Ubuntu 24.04 with Nvidia drivers
      image_id = "a086833e-9d39-4989-88cc-1b0d3d8da395"
      // g.4.standard.40s, 4 vCPU 16GB RAM 40GB disk
      flavor_id        = "6c57396b-dedb-4492-a7e1-9c50c3c74581"
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

