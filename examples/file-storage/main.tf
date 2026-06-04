terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

provider "nscale" {}

# File storage classes are pre-configured, read-only platform resources. Supply
# the identifier of an existing one. The region defaults to the region ID
# configured on the provider (NSCALE_REGION_ID), so it does not need to be set
# on each resource below.
variable "file_storage_class_id" {
  type        = string
  description = "The identifier of an existing file storage class to provision against."
}

resource "nscale_network" "example" {
  name            = "example"
  cidr_block      = "192.168.0.0/24"
  dns_nameservers = ["8.8.8.8", "8.8.4.4"]
}

data "nscale_file_storage_class" "standard" {
  id = var.file_storage_class_id
}

resource "nscale_file_storage" "example" {
  name             = "example"
  storage_class_id = data.nscale_file_storage_class.standard.id
  capacity         = 20
  root_squash      = true

  network {
    id = nscale_network.example.id
  }
}
