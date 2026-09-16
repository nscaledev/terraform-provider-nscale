terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

# NKS is the only service with no default endpoint baked into the provider, so
# it must be set explicitly — here, or via NSCALE_NKS_SERVICE_API_ENDPOINT.
# Leaving it unset is not a provider error on its own; it only fails when a
# nscale_kubernetes_* type is actually used.
#
# Everything else comes from the standard NSCALE_* environment variables
# (NSCALE_SERVICE_TOKEN, NSCALE_ORGANIZATION_ID, NSCALE_PROJECT_ID,
# NSCALE_REGION_ID).
provider "nscale" {
  nks_service_api_endpoint = var.nks_service_api_endpoint
}

variable "nks_service_api_endpoint" {
  type        = string
  description = "Base URL of the NKS API. Leave null to use NSCALE_NKS_SERVICE_API_ENDPOINT."
  default     = null
}

variable "name" {
  type        = string
  description = "Name for the cluster and its network. Immutable once the cluster exists."
  default     = "kubernetes-example"
}

# Set this to attach to an existing network instead of creating one. The
# cluster's project, organization and region are all inherited from whichever
# network it attaches to — a cluster takes no project_id of its own.
variable "network_id" {
  type        = string
  description = "Existing region network to attach to. Leave null to create one."
  default     = null
}

variable "network_cidr_block" {
  type        = string
  description = "Prefix for the created network. Ignored when network_id is set."
  default     = "10.0.0.0/16"
}

variable "pod_cidr" {
  type        = string
  description = "IPv4 CIDR for pod addresses. Fixed for the life of the cluster."
  default     = "100.65.0.0/16"
}

variable "service_cidr" {
  type        = string
  description = "IPv4 CIDR for service addresses. Fixed for the life of the cluster."
  default     = "172.20.0.0/16"
}

variable "api_server_public_ip" {
  type        = bool
  description = "Expose the Kubernetes API server on a public endpoint."
  default     = false
}

# The API's own default is ["0.0.0.0/0"], so an omitted allowlist on a public
# API server is reachable from anywhere — hence an explicit, deliberately narrow
# default here. Widen it deliberately, not by accident.
variable "api_server_allowed_cidrs" {
  type        = list(string)
  description = "Source IPv4 CIDR allowlist for the API server endpoint (1-32 entries)."
  default     = ["10.0.0.0/8"]
}

variable "hardware_addon_enabled" {
  type        = bool
  description = "Enable the optional hardware addon profile."
  default     = true
}

locals {
  create_network = var.network_id == null
  network_id     = local.create_network ? nscale_network.main[0].id : data.nscale_network.existing[0].id
  region_id      = local.create_network ? nscale_network.main[0].region_id : data.nscale_network.existing[0].region_id
}

resource "nscale_network" "main" {
  count = local.create_network ? 1 : 0

  name       = var.name
  cidr_block = var.network_cidr_block

  dns_nameservers = ["1.1.1.1"]
}

data "nscale_network" "existing" {
  count = local.create_network ? 0 : 1

  id = var.network_id
}

# Pick a release eligible for a NEW cluster rather than hardcoding an ID, which
# goes stale as the catalogue moves. Creating on a deprecated release fails with
# HTTP 422.
#
# An omitted filter is not the same as false: omitting `deprecated` returns
# current AND deprecated releases. When upgrading rather than creating, filter
# only on `withdrawn` — you may need to re-state a release your cluster is
# already on which has since been deprecated.
#
# The region comes off the network, not from NSCALE_REGION_ID: the cluster's
# region is whatever the network says it is, and a release from any other region
# fails at apply with a 422.
data "nscale_kubernetes_platform_releases" "eligible" {
  region_id  = local.region_id
  deprecated = false
  withdrawn  = false
}

resource "nscale_kubernetes_cluster" "main" {
  # name, network_id and both cluster_network CIDRs are immutable: the API
  # rejects a change to any of them, so Terraform replaces the cluster instead.
  name        = var.name
  description = "Example NKS cluster managed by Terraform"

  network_id          = local.network_id
  platform_release_id = data.nscale_kubernetes_platform_releases.eligible.releases[0].id

  # Nested attributes, not blocks — note the `=`.
  #
  # Pod and service addresses stay clear of the attached network's own prefix.
  # Omit the block to take the API defaults (10.240.0.0/12 and 10.96.0.0/16);
  # either way the values are fixed for the life of the cluster, so a collision
  # discovered later means a rebuild rather than an update.
  cluster_network = {
    pod_cidr     = var.pod_cidr
    service_cidr = var.service_cidr
  }

  # allowed_cidrs is always set, never left null. It is Optional+Computed, so
  # omitting it inside a block that is present leaves it unknown on every plan
  # and the example would never reach a clean "No changes." The allowlist
  # applies to the private endpoint as well as the public one.
  api_server = {
    public_ip     = var.api_server_public_ip
    allowed_cidrs = var.api_server_allowed_cidrs
  }

  # A profile is an object, not a bool — the wrapper leaves room for per-profile
  # settings beyond `enabled`.
  addons = {
    hardware = {
      enabled = var.hardware_addon_enabled
    }
  }

  tags = {
    environment = "example"
  }

  # `timeouts` is a block, so no `=`. Defaults are 60m create / 90m update /
  # 60m delete, sized from a measured 32-minute build. Override only to raise
  # them — a create timeout shorter than the real build time fails an apply on
  # a cluster that was going to come up fine, and leaves it running and billing
  # with Terraform no longer tracking it.
  #
  # Destroying a cluster that has node pools drains every worker first and
  # honours PodDisruptionBudgets, and an unsatisfiable PDB blocks that with no
  # deadline. If a destroy times out, fix the PDB rather than raise this.
  timeouts {
    create = "90m"
  }
}
