---
name: tf-provider-release
description: Cut a new release of the Nscale Terraform provider. Use when the user wants to release, tag, ship, publish, or version this provider — triggers include "release", "cut a release", "tag X", "ship v1.2.0", "publish to the registry", "bump version", or any phrasing about producing a new GitHub Release / Terraform Registry version of this repo. Walks decide-version → changelog → release PR → tag → verify.
---

# Releasing the Nscale Terraform provider

Use this skill to take `main` from "ready to ship" to a published Terraform Registry release. The authoritative process is in [RELEASING.md](../../../RELEASING.md); this skill operationalises it and adds checkpoints so destructive steps are never taken without explicit confirmation.

## Quick orientation

- **Authoritative process:** [RELEASING.md](../../../RELEASING.md). If this skill ever contradicts it, RELEASING.md wins — update the skill.
- **Changelog source of truth:** [CHANGELOG.md](../../../CHANGELOG.md). Keep-a-Changelog format with categories BREAKING CHANGES / FEATURES / ENHANCEMENTS / BUG FIXES / DEPRECATIONS / DOCS.
- **Pipeline:** `.github/workflows/release.yml` triggers on `v*` tag push, runs GoReleaser, signs with the `GPG_PRIVATE_KEY` + `PASSPHRASE` repo secrets, publishes a GitHub Release. The Terraform Registry picks it up via webhook.

## Hard rules

- **Never push a tag without explicit user confirmation in the current turn.** Tag pushes trigger publication; they cannot be cleanly un-done after the Registry has indexed them.
- **Never open the release PR without explicit user confirmation.** Branch and changelog edits are fine; PR creation crosses into "visible to the team".
- **Tag from `main` only**, on the merge commit of the release PR. Never tag a feature branch.
- **Never reuse a tag.** If you have to redo, delete the GitHub Release first, then `git push --delete origin vX.Y.Z`, then re-tag.

## The four phases

Create a TodoWrite list with one entry per phase before starting. Mark each completed as soon as it's done; do not batch.

1. **Decide the bump** — read commits since the last tag, propose MAJOR/MINOR/PATCH with reasoning.
2. **Update the changelog** — promote `[Unreleased]` into a dated section, refresh compare links.
3. **Open the release PR** — branch + commit + PR titled `chore: release vX.Y.Z`. **STOP before opening the PR; confirm with the user.**
4. **Tag and verify** — after merge, tag from `main` and watch the workflow. **STOP before tag-push; confirm with the user.**

---

## Phase 1 — Decide the bump

Goal: pick `vX.Y.Z` with clear reasoning the user can challenge.

1. Find the previous tag and read the commits since:
   ```sh
   git fetch --tags
   git describe --tags --abbrev=0
   git log $(git describe --tags --abbrev=0)..origin/main --no-merges --oneline
   ```
2. For any non-obvious commit, run `git show <sha> --stat` and inspect the diff. PR titles can be misleading (e.g. `add X` without `feat:` prefix is often a feature).
3. Classify each commit against the bump table in RELEASING.md:
   - **MAJOR** — removed/renamed attribute or resource, type change, default behavior change that breaks state, raised min Terraform/Go version.
   - **MINOR** — new resource, data source, attribute, or feature; backward compatible.
   - **PATCH** — bug fix, dependency bump, doc-only.
4. The highest classification wins. Present the bump to the user with one-line reasoning per commit. Wait for confirmation before continuing.
5. Cross-check the existing `## [Unreleased]` section in `CHANGELOG.md`. If entries are missing for visible commits, add them now — the changelog is the source of truth for what users will read.

Anti-patterns:
- Picking PATCH because "it's a small change" when an attribute was added. Schema additions are MINOR even if the diff is one line.
- Bumping MAJOR for an internal refactor. If users can't observe the change, it isn't breaking.

---

## Phase 2 — Update the changelog

Goal: leave `CHANGELOG.md` ready for the release PR.

1. Rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` using today's date.
2. Insert a fresh empty `## [Unreleased]` section above it.
3. Update the compare links at the bottom:
   - Change the old `[Unreleased]: .../vPREV...HEAD` to `[Unreleased]: .../vX.Y.Z...HEAD`.
   - Add `[X.Y.Z]: .../compare/vPREV...vX.Y.Z`.
4. If any category under the new section is empty, delete it. Keep order: BREAKING CHANGES, FEATURES, ENHANCEMENTS, BUG FIXES, DEPRECATIONS, DOCS.
5. Re-read the section as a user would. Each entry should describe a change in plain English with a PR link, not a commit subject.

---

## Phase 3 — Open the release PR

Goal: produce a one-file, reviewable PR titled exactly `chore: release vX.Y.Z`.

1. Branch:
   ```sh
   git checkout main && git pull
   git checkout -b chore/release-vX.Y.Z
   ```
2. Stage the CHANGELOG edit only. Do not bundle unrelated changes into a release PR — if you find unrelated fixes needed, ship them separately first.
3. Commit:
   ```sh
   git commit -m "chore: release vX.Y.Z"
   ```
4. **STOP. Confirm with the user before pushing the branch or opening the PR.** Present the diff and the PR body you intend to use (the new changelog section, verbatim).
5. On confirmation, push and open the PR with `gh pr create --base main --title "chore: release vX.Y.Z" --body "<new changelog section>"`.

Anti-patterns:
- Combining "ship v1.2.0" with "also fix the lint config". The release PR must be a single-file CHANGELOG edit so reviewers can approve it on sight.
- Including the `[X.Y.Z]` compare link but forgetting to update `[Unreleased]` — the link checker won't catch this.

---

## Phase 4 — Tag and verify

Goal: cut the tag, watch the pipeline, confirm the Registry has the version.

1. Wait for the release PR to merge. Pull `main`:
   ```sh
   git checkout main && git pull
   ```
2. Confirm `HEAD` is the merge commit of the release PR and that `CHANGELOG.md` reflects the new release.
3. **STOP. Confirm with the user before tagging.** Show the commit you're about to tag (`git log -1`).
4. On confirmation:
   ```sh
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```
5. Watch the workflow:
   ```sh
   gh run watch
   gh release view vX.Y.Z
   ```
6. After the workflow succeeds, verify the Registry has indexed it (may take a few minutes):
   ```
   https://registry.terraform.io/providers/nscaledev/nscale/X.Y.Z
   ```
7. Announce in the relevant channel (see RELEASING.md §6).

If the workflow fails after the tag is pushed:
- The tag is now live but no Release was created. Investigate the failure first.
- If the fix needs a code change, you must delete the tag (`git push --delete origin vX.Y.Z`), merge the fix, and re-tag with the same version. This is safe **only if no Release was published** for that tag.
- If a Release was partially published, treat the version as burned and bump the PATCH (e.g. `vX.Y.Z+1`) rather than reusing.

---

## Hotfix variant

When patching an older major series (e.g. `v1.x` after `v2.0.0` has shipped):

1. Branch from the latest tag of that series: `git checkout -b release/1.x v1.1.0`.
2. Cherry-pick or re-implement the fix. Update `CHANGELOG.md` under the new patch section.
3. Open a PR targeting the release branch, not `main`.
4. Tag from the release branch (`vX.Y.Z+1`) after merge.
5. Forward-port the fix to `main` in a separate PR if applicable.

The same Phase 3 / Phase 4 confirmation checkpoints apply.

---

## When NOT to use this skill

- "Bump the dep in `go.mod`" — that's a regular PR, not a release.
- "Update the example to pin v1.1.0" — also a regular PR.
- "Generate release notes for v1.1.0 retroactively" — edit `CHANGELOG.md` directly via a normal `docs:` PR; don't re-cut the tag.
