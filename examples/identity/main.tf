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

# Users are provisioned by the external identity platform; there is no
# nscale_identity_user resource. To add an existing user to a group, reference
# them by their user-record UUID in user_ids. Defaults to empty so the example
# runs without it.
variable "user_ids" {
  type        = list(string)
  description = "UUIDs of existing users to add to the group."
  default     = []
}

resource "nscale_identity_group" "engineers" {
  name        = "engineers"
  description = "Engineering staff."

  role_ids = [
    var.role_id,
  ]

  # Add existing users to the group by UUID. The group's read-only `subjects`
  # attribute is derived from this by the identity service.
  user_ids = var.user_ids

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
