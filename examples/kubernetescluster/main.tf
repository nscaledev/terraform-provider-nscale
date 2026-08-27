terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

# NKS has no default endpoint baked into the provider yet, so it must be set
# explicitly. Everything else comes from the standard NSCALE_* environment
# variables (NSCALE_SERVICE_TOKEN, NSCALE_ORGANIZATION_ID, NSCALE_PROJECT_ID,
# NSCALE_REGION_ID).
provider "nscale" {
  # nks_service_api_endpoint = "https://nks.example.nscale.com"
  # or export NSCALE_NKS_SERVICE_API_ENDPOINT
}

# The cluster's project, organization and region are all inherited from this
# network — a cluster takes no project_id of its own.
resource "nscale_network" "main" {
  name       = "kubernetes-example"
  cidr_block = "10.0.0.0/16"

  dns_nameservers = ["1.1.1.1"]
}

# Pick the newest release eligible for a new cluster rather than hardcoding an
# ID, which goes stale as the catalogue moves.
data "nscale_kubernetes_platform_releases" "eligible" {
  region_id  = nscale_network.main.region_id
  deprecated = false
  withdrawn  = false
}

resource "nscale_kubernetes_cluster" "main" {
  name        = "kubernetes-example"
  description = "Example NKS cluster managed by Terraform"

  network_id          = nscale_network.main.id
  platform_release_id = data.nscale_kubernetes_platform_releases.eligible.releases[0].id

  # Nested attributes, not blocks — note the `=`.
  api_server = {
    public_ip = false
  }

  addons = {
    hardware = true
  }

  tags = {
    environment = "example"
  }

  # `timeouts` is a block, so no `=`. Defaults are 60m create / 90m update /
  # 30m delete, sized from a measured 32-minute build. Override only to raise
  # them — a create timeout shorter than the real build time fails an apply on
  # a cluster that was going to come up fine, and leaves it running.
  timeouts {
    create = "90m"
  }
}

output "cluster_id" {
  description = "The provisioned cluster's ID. Use it with `terraform import` or the data source."
  value       = nscale_kubernetes_cluster.main.id
}

output "kubernetes_version" {
  description = "The Kubernetes version the control plane is running."
  value       = nscale_kubernetes_cluster.main.kubernetes_version_observed
}

# public_ip is false above, so only the private endpoint is populated.
output "api_server_private_endpoint" {
  description = "The cluster's private Kubernetes API server endpoint."
  value = format(
    "https://%s:%s",
    nscale_kubernetes_cluster.main.api_server_endpoint.private_host,
    nscale_kubernetes_cluster.main.api_server_endpoint.private_port,
  )
}

output "upgrade_targets" {
  description = "Platform releases this cluster can upgrade to, in order."
  value       = nscale_kubernetes_cluster.main.eligible_upgrade_target_ids
}
