terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

provider "nscale" {}

resource "nscale_ssh_certificate_authority" "example" {
  name       = "example-ca"
  public_key = file("/tmp/test_ca.pub")
}

data "nscale_network" "example" {
  id = "a3ef33f1-a8fe-409f-a793-867532dd3cd2"
}

resource "nscale_security_group" "example" {
  name = "dx785-example"

  rules = [
    {
      type      = "ingress"
      protocol  = "tcp"
      from_port = 22
    }
  ]

  network_id = data.nscale_network.example.id
}

data "nscale_instance_flavor" "example" {
  id = "1d9e96bf-4f0f-40dd-93ea-205f293eaf77"
}

resource "nscale_instance" "example" {
  name = "dx785-example"

  network_interface {
    network_id         = data.nscale_network.example.id
    enable_public_ip   = true
    security_group_ids = [nscale_security_group.example.id]
  }

  image_id                     = "43f82789-e45d-4dac-a364-26b5040fc47b"
  flavor_id                    = data.nscale_instance_flavor.example.id
  ssh_certificate_authority_id = nscale_ssh_certificate_authority.example.id
}

data "nscale_instance_ssh_key" "example" {
  instance_id = nscale_instance.example.id
}
