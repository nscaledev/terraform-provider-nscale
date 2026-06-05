terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}

provider "nscale" {}

# An endpoint is the S3-compatible access surface; buckets are created and
# managed by an S3 client (aws-cli, mc, etc.) against this endpoint, not by
# Terraform. One endpoint can host many buckets.
locals {
  endpoint_name = "ml-platform"
  bucket_names  = ["ml-artifacts", "ml-datasets"]
  bucket_arns = flatten([
    for b in local.bucket_names : [
      "arn:aws:s3:::${b}",
      "arn:aws:s3:::${b}/*",
    ]
  ])
}

data "nscale_region" "glo1" {
  id = "62f35744-1abd-47b8-a850-cfaa03fd3ca6"
}

data "nscale_object_storage_endpoint_class" "standard" {
  id        = "d7bf78c2-eaa4-4e73-946e-9271bd15471a"
  region_id = data.nscale_region.glo1.id
}

resource "nscale_object_storage_endpoint" "example" {
  name              = local.endpoint_name
  description       = "Object storage endpoint for ML workloads. Buckets are created via the S3 client, not Terraform."
  endpoint_class_id = data.nscale_object_storage_endpoint_class.standard.id
  region_id         = data.nscale_region.glo1.id

  identity_policies = [
    {
      name = "bucket-admin"
      document = jsonencode({
        Version = "2012-10-17"
        Statement = [{
          Effect   = "Allow"
          Action   = ["s3:*"]
          Resource = local.bucket_arns
        }]
      })
    },
    {
      name = "bucket-readonly"
      document = jsonencode({
        Version = "2012-10-17"
        Statement = [{
          Effect   = "Allow"
          Action   = ["s3:GetObject", "s3:ListBucket"]
          Resource = local.bucket_arns
        }]
      })
    },
  ]
}

resource "nscale_object_storage_access_key" "writer" {
  endpoint_id     = nscale_object_storage_endpoint.example.id
  name            = "writer"
  description     = "Read-write credential for the ingest pipeline."
  identity_policy = "bucket-admin"
}

# The data sources demonstrate how to look up an existing endpoint or access
# key from another module. Both are read-only — see the resources above for
# the canonical creation flow.
data "nscale_object_storage_endpoint" "lookup" {
  id = nscale_object_storage_endpoint.example.id
}

data "nscale_object_storage_access_key" "lookup" {
  endpoint_id = nscale_object_storage_endpoint.example.id
  id          = nscale_object_storage_access_key.writer.id
}

# DNS hostname S3 clients should use to reach the endpoint. `try(...)` returns
# null while the endpoint is still provisioning or if the class only supports
# private exposure.
output "s3_endpoint_dns" {
  value = try(nscale_object_storage_endpoint.example.exposure.public.dns_name, null)
}

output "s3_access_key_id" {
  value = nscale_object_storage_access_key.writer.access_key_id
}

# Sensitive: the secret is returned only once on creation. Pipe it into a
# secret manager on first apply, or use `terraform output -raw s3_secret`
# to pull it programmatically. If state is lost, replace the access key
# resource with `terraform apply -replace=nscale_object_storage_access_key.writer`
# to mint a fresh credential.
output "s3_secret" {
  value     = nscale_object_storage_access_key.writer.secret
  sensitive = true
}
