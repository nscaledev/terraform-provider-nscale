# Changelog

All notable changes to the Nscale Terraform provider are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Categories used: `BREAKING CHANGES`, `FEATURES`, `ENHANCEMENTS`, `BUG FIXES`,
`DEPRECATIONS`, `DOCS`.

## [Unreleased]

### FEATURES

- Added `nscale_ssh_certificate_authority` resource and data source ([#36](https://github.com/nscaledev/terraform-provider-nscale/pull/36)).
- Wired `ssh_certificate_authority_id` into `nscale_instance` (DX-785).
- Added file storage usage refresh control ([#40](https://github.com/nscaledev/terraform-provider-nscale/pull/40)).

### BUG FIXES

- Detect in-use security group at plan time, surfacing the conflict before apply (ADA-12, [#37](https://github.com/nscaledev/terraform-provider-nscale/pull/37)).
- Waiter now surfaces a diagnostic when a resource enters an error state (DX-1025, [#39](https://github.com/nscaledev/terraform-provider-nscale/pull/39)).

### DOCS

- Added `tf-provider-feature` skill and reference guides ([#41](https://github.com/nscaledev/terraform-provider-nscale/pull/41)).
- Tidied resource docs and examples.

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

[Unreleased]: https://github.com/nscaledev/terraform-provider-nscale/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/nscaledev/terraform-provider-nscale/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/nscaledev/terraform-provider-nscale/compare/v0.0.10...v1.0.0
[0.0.10]: https://github.com/nscaledev/terraform-provider-nscale/releases/tag/v0.0.10
