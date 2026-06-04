terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

provider "nscale" {}

variable "ssh_ca_public_key" {
  type        = string
  description = "The SSH CA public key in OpenSSH format (e.g. ssh-ed25519 AAAA...)."
}

resource "nscale_ssh_certificate_authority" "example" {
  name       = "example-ca"
  public_key = var.ssh_ca_public_key
}

data "nscale_ssh_certificate_authority" "lookup" {
  id = nscale_ssh_certificate_authority.example.id
}
