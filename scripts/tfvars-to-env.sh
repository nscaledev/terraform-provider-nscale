#!/usr/bin/env bash
#
# Maps a gitignored terraform.<profile>.tfvars onto the NSCALE_* environment
# variables the acceptance tests read, and prints them as `export` lines for
# eval. Contains no values, no hostnames and no secret references, so it is safe
# in this public repo — everything sensitive stays in the gitignored tfvars.
#
#   eval "$(./scripts/tfvars-to-env.sh terraform.staging.tfvars)"
#
# Variable names in the tfvars are deliberately descriptive: the prefix is the
# resource's human name and the SUFFIX determines the mapping, so a reader never
# needs a lookup table to know that no_glo1_region_id is the region. Adding a
# region means renaming the prefix, not touching this script.

set -euo pipefail

file="${1:-}"
if [[ -z "$file" ]]; then
	echo "usage: $0 <path/to/terraform.<profile>.tfvars>" >&2
	exit 64
fi
if [[ ! -f "$file" ]]; then
	echo "$0: $file not found" >&2
	exit 66
fi

# suffix -> env var. Order matters only in that the endpoint rule is matched
# first, so region_service_api_endpoint is not mistaken for a *_region_id.
declare -a seen=()

emit() {
	local var="$1" value="$2" key="$3"
	for s in "${seen[@]:-}"; do
		if [[ "$s" == "$var" ]]; then
			echo "$0: $var set twice (second was '$key') — two keys share a suffix; rename one" >&2
			exit 65
		fi
	done
	seen+=("$var")
	printf 'export %s=%q\n' "$var" "$value"
}

while IFS= read -r line || [[ -n "$line" ]]; do
	# strip comments and whitespace; skip blanks
	line="${line%%#*}"
	line="$(printf '%s' "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
	[[ -z "$line" ]] && continue

	key="$(printf '%s' "$line" | sed -n 's/^\([A-Za-z0-9_]*\)[[:space:]]*=.*/\1/p')"
	val="$(printf '%s' "$line" | sed -n 's/^[A-Za-z0-9_]*[[:space:]]*=[[:space:]]*"\(.*\)"[[:space:]]*$/\1/p')"
	[[ -z "$key" ]] && continue
	# An empty value is legitimate only for a not-yet-minted token; skip it so
	# the precheck reports a missing variable rather than an empty one.
	[[ -z "$val" ]] && continue

	case "$key" in
	region_service_api_endpoint) emit NSCALE_REGION_SERVICE_API_ENDPOINT "$val" "$key" ;;
	compute_service_api_endpoint) emit NSCALE_COMPUTE_SERVICE_API_ENDPOINT "$val" "$key" ;;
	identity_service_api_endpoint) emit NSCALE_IDENTITY_SERVICE_API_ENDPOINT "$val" "$key" ;;
	storage_service_api_endpoint) emit NSCALE_STORAGE_SERVICE_API_ENDPOINT "$val" "$key" ;;
	reservation_service_api_endpoint) emit NSCALE_RESERVATION_SERVICE_API_ENDPOINT "$val" "$key" ;;
	service_token) emit NSCALE_SERVICE_TOKEN "$val" "$key" ;;
	*_org_id) emit NSCALE_ORGANIZATION_ID "$val" "$key" ;;
	*_project_id) emit NSCALE_PROJECT_ID "$val" "$key" ;;
	*_region_id) emit NSCALE_REGION_ID "$val" "$key" ;;
	*_role_id) emit NSCALE_TEST_ROLE_ID "$val" "$key" ;;
	*_flavor_id) emit NSCALE_TEST_FLAVOR_ID "$val" "$key" ;;
	*_image_id) emit NSCALE_TEST_IMAGE_ID "$val" "$key" ;;
	*_fs_class_id) emit NSCALE_TEST_FILE_STORAGE_CLASS_ID "$val" "$key" ;;
	*_os_class_id) emit NSCALE_TEST_OBJECT_STORAGE_ENDPOINT_CLASS_ID "$val" "$key" ;;
	*_reservation_accelerator) emit NSCALE_TEST_RESERVATION_ACCELERATOR "$val" "$key" ;;
	*_reservation_unit) emit NSCALE_TEST_RESERVATION_UNIT "$val" "$key" ;;
	*)
		echo "$0: warning: no mapping for '$key' — ignored" >&2
		;;
	esac
done <"$file"

echo 'export TF_ACC=1'
