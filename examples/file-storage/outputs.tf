output "example_network_id" {
  value = nscale_network.example.id
}

output "example_file_storage_id" {
  value = nscale_file_storage.example.id
}

output "example_file_storage_mount_sources" {
  value = nscale_file_storage.example.network[*].mount_source
}
