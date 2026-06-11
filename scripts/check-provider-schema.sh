#!/usr/bin/env bash
#
# Diffs the provider's live schema against the committed baseline at
# testdata/schema/provider-schema.golden.json and fails (non-zero exit) on any
# drift. Run by `make schemacheck` and in CI on every PR.
#
# If this fails because you intentionally changed the schema, run
# `./scripts/regenerate-schema.sh` and commit the updated baseline.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
baseline="$repo_root/testdata/schema/provider-schema.golden.json"

if [[ ! -f "$baseline" ]]; then
	echo "error: no schema baseline at ${baseline#"$repo_root"/}" >&2
	echo "       run ./scripts/regenerate-schema.sh to create it" >&2
	exit 1
fi

actual="$(mktemp)"
trap 'rm -f "$actual"' EXIT
"$script_dir/_generate-schema.sh" >"$actual"

if ! diff -u "$baseline" "$actual"; then
	echo >&2
	echo "error: provider schema differs from the committed baseline." >&2
	echo "       if this change is intentional, run:" >&2
	echo "           make schema-update   # (or ./scripts/regenerate-schema.sh)" >&2
	echo "       and commit testdata/schema/provider-schema.golden.json" >&2
	exit 1
fi

echo "provider schema matches the committed baseline" >&2
