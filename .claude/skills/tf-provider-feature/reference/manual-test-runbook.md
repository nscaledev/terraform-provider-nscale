# Manual test runbook

End-to-end exercise against staging. Run this after acceptance tests pass and before opening the PR.

## Prereqs

In the shell you will run terraform from:

```sh
export NSCALE_SERVICE_TOKEN=<staging-bearer-token>
export NSCALE_ORGANIZATION_ID=<uuid>
export NSCALE_PROJECT_ID=<uuid-of-throwaway-project>
export NSCALE_REGION_ID=<uuid>
# Required for staging — defaults are prod URLs:
export NSCALE_REGION_SERVICE_API_ENDPOINT=https://region.nks-stg.europe-west2.nscale.com
export NSCALE_STORAGE_SERVICE_API_ENDPOINT=https://storage.nks-stg.europe-west2.nscale.com
export NSCALE_COMPUTE_SERVICE_API_ENDPOINT=https://compute.nks-stg.europe-west2.nscale.com
```

Dev override (`~/.terraformrc`) so Terraform picks the locally built provider:

```hcl
provider_installation {
  dev_overrides {
    "nscaledev/nscale" = "/Users/you/go/bin"
  }
  direct {}
}
```

Build and install:

```sh
make install
ls -la "$(go env GOPATH)/bin/terraform-provider-nscale"   # confirm fresh binary
```

> With a dev override, `terraform init` is unnecessary and emits a warning. Skip straight to `plan`.

## Verify the staging project is clean

Some resources have uniqueness constraints (e.g. one object-storage endpoint per project per region). Confirm the project does not already hold a conflicting resource:

```sh
curl -sH "Authorization: Bearer $NSCALE_SERVICE_TOKEN" \
  "$NSCALE_STORAGE_SERVICE_API_ENDPOINT/api/v1/organizations/$NSCALE_ORGANIZATION_ID/projects/$NSCALE_PROJECT_ID/<list-endpoint>" \
  | jq
```

Adjust the path to whichever list endpoint matches the new resource. Expect `[]` (empty).

## The loop

```sh
cd examples/<service>

terraform plan
```

Sanity-check the plan output:
- Sensitive fields render as `(sensitive value)`.
- Computed-known-after-apply fields are not unexpectedly set in config.
- No spurious `(known after apply)` on fields that should be derivable from the provider config.

```sh
terraform apply
```

Capture outputs. For sensitive outputs, **do not run `terraform output -raw <secret>` bare** — pipe it:

```sh
terraform output -raw <secret_name> | pbcopy
# or
( umask 077 && terraform output -raw <secret_name> > ./secret )
```

## State delete + re-import

This is the real test that the resource round-trips cleanly.

```sh
# Grab IDs from current state — adjust paths per resource:
EP_ID=$(terraform state show <addr> | awk '/^    id /{gsub(/"/,"",$3); print $3; exit}')

# Drop from state; objects stay alive server-side:
terraform state rm <addr>
terraform state list   # confirm

# Re-import. For composite IDs (e.g. nested resources), use the documented format:
terraform import <addr> "<id-or-composite-id>"

# THE TEST: plan must be clean.
terraform plan
```

**Pass:** `terraform plan` says `No changes. Your infrastructure matches the configuration.`

**Fail:** any diff. Investigate before declaring done. Common causes:
- Write-once fields like secrets — expected; use `ImportStateVerifyIgnore` in acc tests and document.
- `omitempty` on bool in upstream openapi types causing nil → false drift. Upstream bug — see playbook.
- Computed fields populated only on Create — likely need a `UseStateForUnknown` plan modifier or a Read-side hydration fix.
- Internal `terraform.nscale.com/...` tags surfacing through. Should be stripped by `nscale.RemoveOperationTags`.

## Destroy

```sh
terraform destroy
```

Confirm no remnants:

```sh
curl -sH "Authorization: Bearer $NSCALE_SERVICE_TOKEN" \
  "$NSCALE_STORAGE_SERVICE_API_ENDPOINT/api/v1/organizations/$NSCALE_ORGANIZATION_ID/projects/$NSCALE_PROJECT_ID/<list-endpoint>" \
  | jq
```

Expect `[]`.

## Sensitive data hygiene

- The state file is plaintext JSON. After a manual test against staging, delete it: `rm examples/<service>/terraform.tfstate*`.
- Do not commit any state files. They are gitignored but verify with `git status` before committing.
- If the resource exposes a write-once secret, the resource's `website/docs/r/` markdown must contain a "Handling the secret" section. See `website/docs/r/object_storage_access_key.html.markdown` for the canonical shape (state-at-rest warning + `pbcopy` / `umask` / secret-manager piping examples).
