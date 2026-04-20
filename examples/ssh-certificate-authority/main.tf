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

# Look up the CA we just created by name
data "nscale_ssh_certificate_authority" "lookup" {
  name = nscale_ssh_certificate_authority.example.name
}
