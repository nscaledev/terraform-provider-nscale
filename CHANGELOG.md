# Changelog

All notable changes to the Nscale Terraform provider are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Categories used: `BREAKING CHANGES`, `FEATURES`, `ENHANCEMENTS`, `BUG FIXES`,
`DEPRECATIONS`, `DOCS`.

## [Unreleased]

## [1.3.0] - 2026-06-29

### FEATURES

- Added `nscale_reservation` and `nscale_placement` resources and data sources ([#53](https://github.com/nscaledev/terraform-provider-nscale/pull/53)).
- Added object storage `nscale_object_storage_endpoint` and `nscale_object_storage_access_key` resources and data sources, plus the `nscale_object_storage_endpoint_class` data source ([#42](https://github.com/nscaledev/terraform-provider-nscale/pull/42)).
- Added `nscale_identity_project` and `nscale_identity_group` resources and data sources ([#50](https://github.com/nscaledev/terraform-provider-nscale/pull/50)).

### ENHANCEMENTS

- `project_id` is now optional at provider `Configure` time, allowing it to be set per-resource (DX-1252, [#58](https://github.com/nscaledev/terraform-provider-nscale/pull/58)).
- Migrated to the public [`nscale-sdk-go`](https://github.com/nscaledev/nscale-sdk-go) SDK for compute (Instance API), region, and common types, and upgraded it to v0.0.4 ([#49](https://github.com/nscaledev/terraform-provider-nscale/pull/49), [#62](https://github.com/nscaledev/terraform-provider-nscale/pull/62)). The deprecated `nscale_compute_cluster` resource stays on the legacy client behind an in-package type-compat shim until its scheduled removal.
- Removed the vendored dependency tree (`vendor/`). All dependencies now resolve via the public Go module proxy.

### BUG FIXES

- Fixed `nscale_placement` `server_spec.networking` list handling: a crash when `security_group_ids` or `allowed_source_addresses` was left unset, and an "inconsistent result after apply" error (`[]` becoming `null`) when either was set to an empty list ([#64](https://github.com/nscaledev/terraform-provider-nscale/pull/64)).

## [1.2.0] - 2026-05-27

### FEATURES

- Added file storage usage refresh control ([#40](https://github.com/nscaledev/terraform-provider-nscale/pull/40)).

### BUG FIXES

- Waiter now surfaces a diagnostic when a resource enters an error state (DX-1025, [#39](https://github.com/nscaledev/terraform-provider-nscale/pull/39)).

### DOCS

- Added `tf-provider-feature` skill and reference guides ([#41](https://github.com/nscaledev/terraform-provider-nscale/pull/41)).

## [1.1.0] - 2026-05-01

### FEATURES

- Added `nscale_ssh_certificate_authority` resource and data source.
- Wired `ssh_certificate_authority_id` into `nscale_instance` (DX-785).

### BUG FIXES

- Detect in-use security group at plan time (ADA-12).
- Aligned `nscale_ssh_certificate_authority` data source to id-based lookup.
- Tightened `ssh_certificate_authority` schema and `tools` build tag.

### DOCS

- Added website docs for `ssh_certificate_authority`.

## [1.0.0] - 2026-04-16

First stable release. The provider schema is now considered stable; future
breaking changes will increment the major version.

### FEATURES

- Stable release of `nscale_network`, `nscale_security_group`,
  `nscale_file_storage`, `nscale_instance`, and `nscale_compute_cluster`
  resources, plus corresponding data sources.
- Configurable resource operation wait timeout.
- `project_id` attribute on `nscale_instance`, `nscale_network`, and
  `nscale_file_storage`.
- Data source for instance SSH key.
- Raw API response body logged at debug level; trace IDs from API errors
  are surfaced in diagnostics.

### ENHANCEMENTS

- Added missing schema plan modifiers across resources to eliminate spurious
  diffs.

### DEPRECATIONS

- `nscale_compute_cluster` is marked deprecated.

## [0.0.10] - 2026-03-18

Final pre-1.0 release. See git history for the full 0.0.x series.

[Unreleased]: https://github.com/nscaledev/terraform-provider-nscale/compare/v1.3.0...HEAD
[1.3.0]: https://github.com/nscaledev/terraform-provider-nscale/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/nscaledev/terraform-provider-nscale/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/nscaledev/terraform-provider-nscale/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/nscaledev/terraform-provider-nscale/compare/v0.0.10...v1.0.0
[0.0.10]: https://github.com/nscaledev/terraform-provider-nscale/releases/tag/v0.0.10
