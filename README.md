# Nscale Terraform Provider

The Nscale Terraform provider allows Terraform to manage Nscale resources, letting you define and provision them using standard Terraform workflows.

**Note**: For now, the provider supports Nscale networks, security groups, compute instances and compute clusters, with more resources planned for future releases.

---

## Documentation

Documentation for this provider is available on the [Terraform Registry](https://registry.terraform.io/providers/nscaledev/nscale/latest/docs), and you can also find [examples](https://github.com/nscaledev/terraform-provider-nscale/tree/main/examples) in this repository.

---

## Quick Start

The provider is distributed via the Terraform Registry as `nscaledev/nscale`. Declare it in your `required_providers`block:

```terraform
terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
      # Check the Terraform Registry or GitHub Releases for the latest version.
      # version = "~> 0.0.2"
    }
  }
}
```

The provider authenticates with Nscale using a service token, which you can generate and rotate in the Nscale Console.

A typical provider configuration looks like this:

```terraform
provider "nscale" {
  # Recommended: supply these values via environment variables, not hard-coded here.

  # organization_id = "<your-organization-id>"
  # project_id      = "<your-project-id>"
  # service_token   = "<your-service-token>"
}
```