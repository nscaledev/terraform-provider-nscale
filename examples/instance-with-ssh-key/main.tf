terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

provider "nscale" {}

# Flavor and image are pre-configured platform resources; supply existing IDs.
# region_id defaults to the provider's configured region (NSCALE_REGION_ID), so
# it does not need to be set on the resources below.
variable "flavor_id" {
  type        = string
  description = "The identifier of an existing instance flavor."
}

variable "image_id" {
  type        = string
  description = "The identifier of an existing image."
}

variable "ssh_ca_public_key" {
  type        = string
  description = "The SSH CA public key in OpenSSH format (e.g. ssh-ed25519 AAAA...)."
}

resource "nscale_network" "example" {
  name            = "example"
  cidr_block      = "192.168.0.0/24"
  dns_nameservers = ["8.8.8.8", "8.8.4.4"]
}

resource "nscale_security_group" "example" {
  name = "example"

  rules = [
    {
      type      = "ingress"
      protocol  = "tcp"
      from_port = 22
    }
  ]

  network_id = nscale_network.example.id
}

data "nscale_instance_flavor" "g_4_standard_40s" {
  id = var.flavor_id
}

resource "nscale_ssh_certificate_authority" "example" {
  name       = "example-ca"
  public_key = var.ssh_ca_public_key
}

resource "nscale_instance" "example" {
  name = "example"

  network_interface {
    network_id         = nscale_network.example.id
    enable_public_ip   = true
    security_group_ids = [nscale_security_group.example.id]
  }

  image_id                     = var.image_id
  flavor_id                    = data.nscale_instance_flavor.g_4_standard_40s.id
  ssh_certificate_authority_id = nscale_ssh_certificate_authority.example.id
}

data "nscale_instance_ssh_key" "example" {
  instance_id = nscale_instance.example.id
}
