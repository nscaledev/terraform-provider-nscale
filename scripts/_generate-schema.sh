#!/usr/bin/env bash
#
# Builds and installs the provider, then prints its schema as deterministic
# (jq -S, sorted-keys) JSON to stdout. This is the shared generator used by
# both check-provider-schema.sh and regenerate-schema.sh — do not call it
# directly, use one of those.
#
# It uses a dev_overrides .terraformrc so no `terraform init` (and no network
# or credentials) is required.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

bindir="$(mktemp -d)"
workdir="$(mktemp -d)"
trap 'rm -rf "$bindir" "$workdir"' EXIT

# Install the provider binary (terraform-provider-nscale) into a dir we own.
GOBIN="$bindir" go install "$repo_root" >&2

cat >"$workdir/main.tf" <<'EOF'
terraform {
  required_providers {
    nscale = {
      source = "nscaledev/nscale"
    }
  }
}
EOF

cat >"$workdir/dev.tfrc" <<EOF
provider_installation {
  dev_overrides {
    "nscaledev/nscale" = "$bindir"
  }
  direct {}
}
EOF

# dev_overrides emits a warning to stderr and lets us skip `terraform init`.
cd "$workdir"
TF_CLI_CONFIG_FILE="$workdir/dev.tfrc" terraform providers schema -json 2>/dev/null | jq -S .
