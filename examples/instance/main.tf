terraform {
  required_providers {
    nscale = {
      source  = "nscaledev/nscale"
      version = "0.0.2"
    }
  }
}

provider "nscale" {}

data "nscale_region" "glo1" {
  id = "<glo1-region-id>"
}

resource "nscale_network" "example" {
  name            = "example"
  cidr_block      = "192.168.0.0/24"
  dns_nameservers = ["8.8.8.8", "8.8.4.4"]
  region_id       = data.nscale_region.glo1.id
}

resource "nscale_security_group" "example" {
  name = "example"

  rules = [
    {
      type      = "ingress"
      protocol  = "tcp"
      from_port = 80
    }
  ]

  network_id = nscale_network.example.id
  region_id  = data.nscale_region.glo1.id
}

data "nscale_instance_flavor" "g_4_standard_40s" {
  id        = "<g-4-standard-40s-flavor-id>"
  region_id = data.nscale_region.glo1.id
}

resource "nscale_instance" "example" {
  name = "example"

  network_interface {
    network_id         = nscale_network.example.id
    enable_public_ip   = true
    security_group_ids = [nscale_security_group.example.id]
  }

  image_id  = "2ad391d1-4c7c-4963-b81a-a4fed53c2b00"
  flavor_id = data.nscale_instance_flavor.g_4_standard_40s.id
  region_id = data.nscale_region.glo1.id
}
