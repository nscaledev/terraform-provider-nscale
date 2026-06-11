---
name: tf-provider-feature
description: Add a new resource or data source to the Nscale Terraform provider. Use when the user wants to add, implement, plan, or design a new resource or data source for this provider — triggers include "add a resource", "new data source", "implement nscale_<X>", "wire up <something>", "extend the provider", or any phrasing about extending this codebase with a new Terraform-exposed object. Guides spec → implementation → tests → manual run → docs.
---

# Adding a feature to the Nscale Terraform provider

Use this skill for any change that introduces a new `nscale_<something>` resource or data source. It walks you through a five-phase flow and points to reference docs you should consult at each step.

## Quick orientation

- **Codebase conventions:** see [reference/conventions.md](reference/conventions.md) (distilled from the repo's `CLAUDE.md`).
- **Best-practice playbook:** see [reference/playbook.md](reference/playbook.md). The team's opinionated guide on schema design, state/lifecycle, error handling, documentation, release, and code organisation. Cited inline below with `§` markers.
- **Testing strategy:** see [reference/testing.md](reference/testing.md). Four-layer model, minimum bar for a new resource, the canonical acceptance-test skeleton, and the bool round-trip test.
- **Manual test runbook:** see [reference/manual-test-runbook.md](reference/manual-test-runbook.md).
- **Spec template:** see [templates/spec-template.md](templates/spec-template.md).

The playbook and testing guide are authoritative for this codebase. Prefer them over your prior knowledge of generic Terraform providers — they encode failure modes we have already hit.

## The five phases

Create a TodoWrite list with one entry per phase before starting. Mark each completed as soon as it's done; do not batch.

1. **Spec** — Write `.specs/<resource_or_data_source_name>.md`. No code yet.
2. **Implementation** — Create the per-service Go files and register them.
3. **Tests** — Unit + acceptance, against the minimum bar in [reference/testing.md](reference/testing.md).
4. **Manual test** — Run the full plan → apply → state rm → import → destroy loop against staging.
5. **Docs** — `website/docs/{r,d}/<name>.html.markdown` + a runnable example under `examples/<service>/`.

Do not skip ahead. Each phase produces inputs the next phase needs.

---

## Phase 1 — Spec

Goal: produce `.specs/<name>.md` capturing intent, API shape, and constraints so implementation has no ambiguity.

1. Create `.specs/` if it does not exist.
2. Copy [templates/spec-template.md](templates/spec-template.md) to `.specs/<name>.md`.
3. Fill it in with the user. Sections to drive a conversation around:
   - **Name and kind** — `nscale_<name>` and whether it is a resource, a data source, or both.
   - **Service package** — which `internal/services/<service>/` it lives in (existing or new).
   - **Backing API** — exact endpoint(s), HTTP verbs, request/response shapes from the openapi spec.
   - **Attributes** — required / optional / computed, with the type for each. Note any nested objects.
   - **Lifecycle** — sync vs. async create/delete; what "provisioned" looks like in the API; what state-watcher pattern (if any) is needed.
   - **Immutability** — which fields are `RequiresReplace`. Default to "everything except `description` and `tags` is immutable" unless the API explicitly supports PATCH.
   - **Sensitive / write-once fields** — secrets the API returns only on Create and never on Read. Plan how state preserves them across reads (see [playbook §2.3](reference/playbook.md)).
   - **Import shape** — passthrough ID, composite ID (e.g. `<parent_id>/<id>`), or unsupported. Call out attributes that cannot round-trip (see [playbook §2.4](reference/playbook.md)).
   - **Known API constraints** — uniqueness rules (e.g. one-per-project-per-region), region scoping, quotas. These belong in the spec because they shape both error UX (see [playbook §3.3](reference/playbook.md)) and tests.
   - **Open questions** — anything you cannot answer from the openapi spec alone.
4. Get the user to sign off on the spec before writing any Go. If open questions remain, surface them now, not at implementation time.

Anti-patterns to avoid:
- Drafting attributes from the openapi field list without asking which ones to expose. Some fields are internal bookkeeping (e.g. `terraform.nscale.com/...` tags) and must be stripped — see [reference/conventions.md](reference/conventions.md).
- Treating every optional API field as `Optional` in the schema. Many are server-set and should be `Computed`.

---

## Phase 2 — Implementation

Goal: working resource/data source code following the codebase's existing patterns.

1. **Locate the template.** Pick the closest existing resource as a model (e.g. `internal/services/sshca/` for ID-keyed simple resources; `internal/services/objectstorage/access_key_*.go` for resources with composite imports and write-once secrets).
2. **Create files in `internal/services/<service>/`** following the layout in [reference/conventions.md](reference/conventions.md):
   - `<name>_model.go` — Terraform struct with `tfsdk` tags + API↔model converters.
   - `<name>_helper.go` — only if conversion logic doesn't fit cleanly in `_model.go`.
   - `<name>_resource.go` and/or `<name>_data_source.go`.
3. **Schema rules** ([playbook §1](reference/playbook.md) has the full version):
   - Every attribute needs a `MarkdownDescription`. No exceptions — it flows into generated docs.
   - Pick attribute kind by source-of-truth ([§1.1](reference/playbook.md)). Avoid `Optional+Computed` unless the API has a stable default it returns exactly on Read.
   - Apply `UseStateForUnknown` to every computed ID/timestamp; missing plan modifiers cause spurious diffs ([§1.2](reference/playbook.md)).
   - Apply `RequiresReplace` to every immutable field; reject `Update` explicitly when every field is `RequiresReplace` (see `access_key_resource.go` for the pattern, [§2.5](reference/playbook.md)).
   - Configure timeouts via `timeouts.Block` and **pick a resource-class-appropriate default** per the table in [playbook §2.2](reference/playbook.md). The framework has no implicit default.
   - For new async resources, prefer `retry.StateChangeConf` ([§2.2](reference/playbook.md)) over hand-rolled polling.
   - For write-once secret fields, use the stash-on-Read pattern in [playbook §2.3](reference/playbook.md) and add an import warning.
   - **If the API uses upstream openapi types where `omitempty` is applied to a non-pointer `bool`/primitive that you need to round-trip, wrap at the service boundary** ([§1.6](reference/playbook.md)). This is the canonical "Provider produced inconsistent result after apply" bug.
4. **Register** the resource/data source factory in `internal/provider/provider.go`. **Without this step the type compiles but is invisible to users.** It is the single most common oversight — `make schemacheck` now catches it, since an unregistered type never appears in the schema snapshot.
5. **Build:**
   ```sh
   make install
   ```
   Verify the binary at `$(go env GOPATH)/bin/terraform-provider-nscale` is newer than your edits.
6. **Lint and unit test:**
   ```sh
   make fmt lint test
   ```

---

## Phase 3 — Tests

Goal: unit coverage on conversion logic; acceptance coverage on the live API path.

Full strategy + minimum bar in [reference/testing.md](reference/testing.md). For this phase, do the following at a minimum:

1. **Unit tests** — `<name>_model_test.go` covering converters in both directions: happy path, nil/optional fields, JSON round-trip. **For every `Required` bool, include a test that sets it to `false` and asserts the JSON contains the field explicitly.** This is the cheapest catch for the `omitempty` class.
2. **Acceptance tests** — `acc_test.go` + `<name>_resource_test.go` (+ `<name>_data_source_test.go` if applicable). Use the canonical skeleton in [testing.md](reference/testing.md) "Minimum bar for a new resource". Required steps:
   - `_basic`: apply + check key fields + a `PlanOnly: true, ExpectNonEmptyPlan: false` follow-up step (the regression guard).
   - `ImportState: true, ImportStateVerify: true`. Use `ImportStateVerifyIgnore` **only** for legitimately unrecoverable fields (write-once secrets, floating timestamps) and add a comment naming why.
   - `_update` test for any in-place mutation path; ends with a `PlanOnly` step.
   - **For every `Required` bool**: an acceptance step that sets it to `false`, followed by a `PlanOnly` step. If `PlanOnly` fails with "Provider produced inconsistent result after apply", fix the model converter — see [playbook §1.6](reference/playbook.md).
3. **Naming**: `acctest.RandomWithPrefix("tf-acc-test")` for every nameable resource. No hardcoded names.
4. Run locally:
   ```sh
   make test       # unit + lint
   make testacc    # acceptance; needs live creds; see testing.md for the env vars
   ```

---

## Phase 4 — Manual test

Goal: end-to-end exercise against staging, including state operations that acceptance tests don't cover.

1. Add or update `examples/<service>/main.tf` with a runnable example for the new resource/data source. This is the same file the docs reference, so make it realistic, not throwaway.
2. Follow [reference/manual-test-runbook.md](reference/manual-test-runbook.md). The full loop is: `plan → apply → terraform state rm → terraform import → plan (expect no changes) → destroy`.
3. **Critical signal:** the post-import `terraform plan` must say "No changes." Anything else is a round-trip bug — investigate before declaring done. The `omitempty`-on-`bool` class is the canonical one; see [playbook §1.6](reference/playbook.md).
4. **Sensitive outputs:** if the resource exposes a write-once secret, document the safe extraction patterns (`terraform output -raw … | pbcopy`, restricted-perm files, secret-manager piping) in the resource's `website/docs/r/` markdown. See `website/docs/r/object_storage_access_key.html.markdown` "Handling the secret" for the canonical shape, and [playbook §2.3](reference/playbook.md) / [§5.5](reference/playbook.md) for the rationale.

---

## Phase 5 — Docs

The repo uses the **legacy `website/docs/{r,d}/*.html.markdown`** layout, not the Registry `docs/` layout. Do not commit `docs/`.

1. **Generate the schema block:**
   ```sh
   make generate
   ```
   This writes to `./docs/` — discard everything except the `<!-- schema generated by tfplugindocs -->` section, which you'll paste into the hand-authored markdown.
2. **Create `website/docs/r/<name>.html.markdown`** (and `website/docs/d/<name>.html.markdown` if a data source). Follow the doc template in [playbook §5.3](reference/playbook.md). Use the existing access-key/endpoint markdown as a working model. Required sections:
   - Front matter: `page_title: "Nscale: nscale_<name>"`, `subcategory: ""`, `description: Nscale <Title Case>`.
   - `# Resource: nscale_<name>` heading (not the generator's `# nscale_<name> (Resource)`).
   - Short prose description.
   - `## Example Usage` — a complete, runnable HCL block. At least one example is **mandatory**.
   - Pasted schema block from step 1.
   - **Topic-specific guidance subsections in the order listed in [playbook §5.5](reference/playbook.md)**: `Async behaviour`, `Timeouts`, `Import`, `Handling the secret` (write-once cases), `Notes` (API constraints not in schema). The point of these sections is to short-circuit support questions — anything you had to figure out the hard way during this implementation goes here.
3. **Cross-check with the example file.** The HCL block in the markdown does not have to be identical to `examples/<service>/main.tf`, but the two should not contradict each other (same attribute names, plausible values).
4. **Discard `docs/`** — it is gitignored and not tracked.
5. **Regenerate the schema baseline:**
   ```sh
   make schema-update
   ```
   A new resource/data source (or any attribute or `MarkdownDescription` change) moves the provider's public schema, so the committed snapshot at `testdata/schema/provider-schema.golden.json` must be regenerated and committed. Unlike `docs/`, **this file IS tracked** — its diff is the user-facing API change and is what reviewers read. CI's `schema` job fails if you skip it.

---

## Verification before declaring done

Run all of these and confirm they pass:

```sh
make fmt lint test schemacheck
```

Plus the manual test loop from Phase 4 with a clean exit (destroy leaves no resources behind).

If any of the following are true, the feature is **not done**:

- `make lint` reports anything.
- `make test` (unit) has no test for any non-trivial converter, or no `false` round-trip test for any `Required` bool.
- `make testacc` (acceptance) has no `PlanOnly` step after the basic apply.
- `terraform plan` after manual-test import shows changes.
- The resource exposes a write-once secret but the markdown has no "Handling the secret" section.
- The resource is registered in `provider.go` but not documented in `website/docs/`.
- The example in `examples/<service>/main.tf` does not actually apply.
- `ImportStateVerifyIgnore` is set on any field without a comment naming why.
- `make schemacheck` fails — you changed the schema but did not commit the regenerated `testdata/schema/provider-schema.golden.json`.

---

## When this skill does not fit

- **Heavier feature spanning multiple resources, milestones, or a research phase.** Use the `feature-plan` skill (writes to `.feature-plans/`) instead. This skill is for the single-resource loop.
- **Refactors of existing resources.** Use the `code-simplifier` skill or work in plain edit mode.
- **Provider plumbing changes** (auth, service URL handling, generic helpers in `internal/nscale/`). Those touch every resource and need wider review than this skill scopes for.
