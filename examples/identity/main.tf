terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

provider "nscale" {}

# Roles are pre-configured, read-only platform resources. Supply the identifier
# of an existing role; you cannot create or modify roles through Terraform.
variable "role_id" {
  type        = string
  description = "The identifier of a pre-configured role to grant to the group."
}

resource "nscale_identity_group" "engineers" {
  name        = "engineers"
  description = "Engineering staff."

  role_ids = [
    var.role_id,
  ]

  # UUIDs of users provisioned by the external identity platform.
  user_ids = []

  tags = {
    team = "platform"
  }
}

resource "nscale_identity_project" "example" {
  name        = "demo-project"
  description = "Sandbox for Terraform demos."

  group_ids = [
    nscale_identity_group.engineers.id,
  ]

  tags = {
    team = "platform"
  }
}

data "nscale_identity_group" "lookup" {
  id = nscale_identity_group.engineers.id
}

data "nscale_identity_project" "lookup" {
  id = nscale_identity_project.example.id
}
