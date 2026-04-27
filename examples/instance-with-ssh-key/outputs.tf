output "example_instance_id" {
  value = nscale_instance.example.id
}

output "example_instance_public_ip" {
  value = nscale_instance.example.public_ip
}

output "example_ssh_certificate_authority_id" {
  value = nscale_ssh_certificate_authority.example.id
}

output "example_instance_private_key" {
  value     = data.nscale_instance_ssh_key.example.private_key
  sensitive = true
}
