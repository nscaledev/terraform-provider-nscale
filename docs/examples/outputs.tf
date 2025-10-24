output "test_cluster_id" {
  value = nscale_compute_cluster.test.id
}
output "test_cluster_name" {
  value = nscale_compute_cluster.test.name
}
output "test_cluster_workload_pool_name" {
  value = nscale_compute_cluster.test.workload_pools[0].name
}
output "test_cluster_workload_pool_ip_address" {
  value = nscale_compute_cluster.test.workload_pools[0].machines[0].public_ip
}
