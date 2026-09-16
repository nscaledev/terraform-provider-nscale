output "nscale_kubernetes_cluster_id" {
  description = "The provisioned cluster's ID. Use it with `terraform import` or the data source."
  value       = nscale_kubernetes_cluster.main.id
}

# project_id and region_id are inherited from the attached network rather than
# configured, so echoing them back shows which scope the cluster landed in.
output "nscale_kubernetes_cluster_project_id" {
  description = "The project the cluster landed in, inherited from its network."
  value       = nscale_kubernetes_cluster.main.project_id
}

output "nscale_kubernetes_cluster_region_id" {
  description = "The region the cluster landed in, inherited from its network."
  value       = nscale_kubernetes_cluster.main.region_id
}

output "nscale_kubernetes_cluster_kubernetes_version" {
  description = "The Kubernetes version the control plane reports."
  value       = nscale_kubernetes_cluster.main.kubernetes_version_observed
}

# The private endpoint is always populated once provisioned; the public one is
# null unless api_server.public_ip is enabled.
output "nscale_kubernetes_cluster_api_server_private_endpoint" {
  description = "The cluster's private Kubernetes API server endpoint."
  value = format(
    "https://%s:%s",
    nscale_kubernetes_cluster.main.api_server_endpoint.private_host,
    nscale_kubernetes_cluster.main.api_server_endpoint.private_port,
  )
}

output "nscale_kubernetes_cluster_api_server_public_endpoint" {
  description = "The cluster's public Kubernetes API server endpoint, or null when public_ip is disabled."
  value = nscale_kubernetes_cluster.main.api_server_endpoint.public_host == null ? null : format(
    "https://%s:%s",
    nscale_kubernetes_cluster.main.api_server_endpoint.public_host,
    nscale_kubernetes_cluster.main.api_server_endpoint.public_port,
  )
}

# Not a secret: this is the cluster's public CA bundle, which is why the
# attribute is not marked sensitive. There is no kubeconfig or token endpoint —
# authentication goes through the `nscale` CLI as a client-go credential plugin.
output "nscale_kubernetes_cluster_certificate_authority_data" {
  description = "Base64-encoded API server CA bundle."
  value       = nscale_kubernetes_cluster.main.api_server_endpoint.certificate_authority_data
}

output "nscale_kubernetes_cluster_upgrade_targets" {
  description = "Platform releases this cluster can upgrade to, in order."
  value       = nscale_kubernetes_cluster.main.eligible_upgrade_target_ids
}
