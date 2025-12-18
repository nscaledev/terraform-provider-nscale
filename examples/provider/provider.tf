# Configure the Nscale Provider
provider "nscale" {
  region_service_api_endpoint  = "<region-service-api-endpoint>"
  compute_service_api_endpoint = "<compute-service-api-endpoint>"
  organization_id              = "<your-organization-id>"
  project_id                   = "<your-project-id>"
  service_token                = "<your-service-token>"
}
