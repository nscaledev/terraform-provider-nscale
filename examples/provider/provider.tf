# Configure the Nscale Provider
provider "nscale" {
  region_service_api_endpoint  = "<region-service-api-endpoint>"
  compute_service_api_endpoint = "<compute-service-api-endpoint>"
  service_token                = "<your-service-token>"
  region_id                    = "<your-region-id>"
  organization_id              = "<your-organization-id>"
  project_id                   = "<your-project-id>"
}
