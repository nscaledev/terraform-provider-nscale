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

resource "nscale_network" "example" {
  name            = "example"
  cidr_block      = "192.168.0.0/24"
  dns_nameservers = ["8.8.8.8", "8.8.4.4"]
  region_id       = data.nscale_region.glo1.id
}

data "nscale_file_storage_class" "standard" {
  id        = "<standard-file-storage-class-id>"
  region_id = data.nscale_region.glo1.id
}

resource "nscale_file_storage" "example" {
  name             = "example"
  storage_class_id = data.nscale_file_storage_class.standard.id
  capacity         = 20
  root_squash      = true
  region_id        = data.nscale_region.glo1.id

  network {
    id = nscale_network.example.id
  }
}
