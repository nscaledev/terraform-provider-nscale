#!/usr/bin/env bash
#
# Regenerates the committed provider schema baseline. Run this deliberately
# whenever you intentionally change the provider's schema (added/removed/renamed
# an attribute, resource, or data source), the same way you re-run
# `make generate` after a docs-affecting change. The diff it produces is part
# of your PR and is what reviewers check.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
baseline="$repo_root/testdata/schema/provider-schema.golden.json"

mkdir -p "$(dirname "$baseline")"
"$script_dir/_generate-schema.sh" >"$baseline"

echo "Wrote schema baseline to ${baseline#"$repo_root"/}" >&2
