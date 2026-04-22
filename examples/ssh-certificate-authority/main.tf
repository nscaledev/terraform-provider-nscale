terraform {
  required_providers {
    nscale = {
      source  = "nscaledev/nscale"
      version = "0.0.8"
    }
  }
}

provider "nscale" {}

resource "nscale_ssh_certificate_authority" "example" {
  name       = "example-ca"
  public_key = file("/tmp/test_ca.pub")
}

data "nscale_ssh_certificate_authority" "lookup" {
  id = nscale_ssh_certificate_authority.example.id
}
