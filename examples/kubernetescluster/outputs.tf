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

# The status outputs below read data.nscale_kubernetes_cluster.ready rather than
# the resource. The resource has wait_for_provisioned = false, so its own status
# describes the moment of creation; the data source is gated on the node pools
# and therefore sees the settled cluster.
output "nscale_kubernetes_cluster_kubernetes_version" {
  description = "The Kubernetes version the control plane reports."
  value       = data.nscale_kubernetes_cluster.ready.kubernetes_version_observed
}

# The private endpoint is always populated once provisioned; the public one is
# null unless api_server.public_ip is enabled.
output "nscale_kubernetes_cluster_api_server_private_endpoint" {
  description = "The cluster's private Kubernetes API server endpoint."
  value = format(
    "https://%s:%s",
    data.nscale_kubernetes_cluster.ready.api_server_endpoint.private_host,
    data.nscale_kubernetes_cluster.ready.api_server_endpoint.private_port,
  )
}

output "nscale_kubernetes_cluster_api_server_public_endpoint" {
  description = "The cluster's public Kubernetes API server endpoint, or null when public_ip is disabled."
  value = data.nscale_kubernetes_cluster.ready.api_server_endpoint.public_host == null ? null : format(
    "https://%s:%s",
    data.nscale_kubernetes_cluster.ready.api_server_endpoint.public_host,
    data.nscale_kubernetes_cluster.ready.api_server_endpoint.public_port,
  )
}

# Not a secret: this is the cluster's public CA bundle, which is why the
# attribute is not marked sensitive. There is no kubeconfig or token endpoint —
# authentication goes through the `nscale` CLI as a client-go credential plugin.
output "nscale_kubernetes_cluster_certificate_authority_data" {
  description = "Base64-encoded API server CA bundle."
  value       = data.nscale_kubernetes_cluster.ready.api_server_endpoint.certificate_authority_data
}

output "nscale_kubernetes_cluster_upgrade_targets" {
  description = "Platform releases this cluster can upgrade to, in order."
  value       = data.nscale_kubernetes_cluster.ready.eligible_upgrade_target_ids
}

# Both pools are count-gated, so these use one(...), which yields null when the
# pool is absent rather than erroring the way [0] would.
output "nscale_kubernetes_node_pool_workers_id" {
  description = "The on-demand worker pool's ID, or null when worker_flavor_id was not set."
  value       = one(nscale_kubernetes_node_pool.workers[*].id)
}

# ready_replicas against replicas is the useful health read on a pool.
# up_to_date_replicas is the one to watch during a roll: it drops below replicas
# while workers are being replaced and recovers when the roll completes.
output "nscale_kubernetes_node_pool_workers_ready" {
  description = "Ready versus desired workers in the on-demand pool."
  value = one(nscale_kubernetes_node_pool.workers[*]) == null ? null : format(
    "%s/%s ready, %s up to date",
    one(nscale_kubernetes_node_pool.workers[*].ready_replicas),
    one(nscale_kubernetes_node_pool.workers[*].replicas),
    one(nscale_kubernetes_node_pool.workers[*].up_to_date_replicas),
  )
}

# A pool has no platform_release_id argument — it inherits the cluster's. This
# echoes back which release the workers were actually pinned to, which lags the
# cluster's platform_release_id while an upgrade rolls through.
output "nscale_kubernetes_node_pool_workers_platform_release" {
  description = "The platform release the on-demand pool inherited from the cluster."
  value       = one(nscale_kubernetes_node_pool.workers[*].applied_platform_release_id)
}

output "nscale_kubernetes_node_pool_gpu_id" {
  description = "The reservation-backed pool's ID, or null when gpu_reservation_id was not set."
  value       = one(nscale_kubernetes_node_pool.gpu[*].id)
}

# The placement the pool created inside the reservation. Null on a compute pool,
# which is why only the reservation-backed pool exposes it here.
output "nscale_kubernetes_node_pool_gpu_placement_id" {
  description = "The reservation placement backing the GPU pool."
  value       = one(nscale_kubernetes_node_pool.gpu[*].placement_id)
}
