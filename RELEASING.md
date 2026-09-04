# Releasing the Nscale Terraform Provider

This provider is published to the [Terraform Registry](https://registry.terraform.io/providers/nscaledev/nscale)
as `nscaledev/nscale`. Releases follow [Semantic Versioning](https://semver.org/)
and are automated with [release-please](https://github.com/googleapis/release-please).

**There is no manual release procedure.** You do not pick a version, edit the
changelog, or push a tag. You merge a PR.

## How it works

```
PR merged to main  ──►  release-please.yml  ──►  "chore: release X.Y.Z" PR
                                                        │
                                            (you review + merge it)
                                                        │
                                                        ▼
                                        tag vX.Y.Z + GitHub Release
                                                        │
                                                        ▼
                                       release.yml → GoReleaser → Registry
```

1. Every PR title is validated as a [conventional commit](https://www.conventionalcommits.org/)
   by `.github/workflows/pr-commit-validation.yml`. The repo is configured to
   squash-merge using the PR title, so that validated title becomes the commit
   subject on `main` — and that subject is release-please's only input.
2. On each push to `main`, `.github/workflows/release-please.yml` opens or
   updates a PR titled `chore: release X.Y.Z`. It contains the generated
   `CHANGELOG.md` section, the bumped `.release-please-manifest.json`, and the
   bumped `version/version.go`.
3. Merging that PR makes the same workflow create the `vX.Y.Z` tag and the
   GitHub Release, using the changelog section as the release body.
4. It then calls `.github/workflows/release.yml`, which runs GoReleaser to
   cross-compile the binaries, sign the checksums with the release GPG key, and
   attach everything to the release.
5. The Registry indexes the release via webhook, usually within a few minutes.

## Versioning policy

release-please derives the bump from the commit subjects since the last tag.
The highest classification wins.

| Commit subject             | Bump  |
| -------------------------- | ----- |
| `feat!:` / `BREAKING CHANGE:` footer | MAJOR |
| `feat:`                    | MINOR |
| `fix:` / `perf:` / `refactor:` / `docs:` | PATCH |
| `chore:` / `ci:` / `test:` / `style:` / `build:` | none (hidden) |

Tags are `vMAJOR.MINOR.PATCH`. The Registry requires the leading `v`;
`include-v-in-tag` in `release-please-config.json` supplies it.

**Getting the bump right is a review-time concern, not a release-time one.**
By the time the release PR exists the subjects are already on `main`. So when
reviewing any PR, check the title's type — a new resource, data source, or
attribute is `feat:` even when the diff is one line, and anything that breaks
an existing configuration needs `feat!:` or a `BREAKING CHANGE:` footer. The
provider is post-1.0, so a breaking change means `v2.0.0`.

### What the title check does and does not catch

`pr-commit-validation.yml` checks that the title parses as a conventional
commit and that its type is one of the eleven above. It accepts a `!`
breaking-change marker (`feat(scope)!: ...`). It does not inspect the subject,
so reviewers still carry two things:

- **A mistyped bump.** `feat:` on a breaking change is a valid title; it just
  releases the wrong version. This is the one to watch for.
- **The subject's wording.** It is published as a changelog bullet verbatim, so
  read it as one: lower-case, no trailing period, no longer than a line.

`Revert "feat: ..."`, GitHub's default revert title, is not a conventional
commit and will fail the check. Retitle it `revert: ...`.

### Overriding the version

To force a specific version, merge a commit whose body contains a
`Release-As:` footer:

```
chore: release as 2.0.0

Release-As: 2.0.0
```

## Reviewing a release PR

**Merging the release PR is the publish.** It creates the tag, the GitHub
Release and the Registry version, and cannot be cleanly undone once the
Registry has indexed it. Review it like a release, not like a chore.

The release PR touches exactly three files. Check:

- The version matches the bump the commits justify.
- The changelog section reads sensibly. Entries are commit subjects, so a
  poorly-titled PR produces a poor entry — fix the title before merge next
  time rather than editing the changelog here (release-please regenerates the
  file on every push to `main` and will overwrite manual edits).
- Nothing else is bundled in.

Note that release PRs do **not** get a CI run — see the comment in
`release-please.yml` for why that is deliberate and safe here.

## Verifying a release

```sh
gh run watch                                                       # the workflow
gh release view vX.Y.Z                                             # binaries + notes
open https://registry.terraform.io/providers/nscaledev/nscale/X.Y.Z  # the Registry
```

If GoReleaser fails after the tag and Release already exist, fix the problem on
`main` and re-run the build against the existing tag — do not delete the tag:

```sh
gh workflow run release.yml -f tag=vX.Y.Z
```

`release.yml` has no `push: tags` trigger, so `workflow_dispatch` is the only
way to rebuild a tag. `.goreleaser.yml` sets `release.mode: keep-existing`, so
a rebuild attaches artifacts without clobbering the release notes.

## Configuration reference

| File | Purpose |
| ---- | ------- |
| `release-please-config.json` | Bump rules, changelog sections, `extra-files` |
| `.release-please-manifest.json` | The current released version — release-please's source of truth |
| `.github/workflows/release-please.yml` | Opens the release PR; cuts tag + Release |
| `.github/workflows/release.yml` | GoReleaser build, called by the above |
| `.github/workflows/pr-commit-validation.yml` | Validates the PR title as a conventional commit |
| `.goreleaser.yml` | Build matrix, signing, artifacts |
| `version/version.go` | Bumped in place via the `x-release-please-version` annotation |

## GPG key

The signing key is registered with the Terraform Registry under the
`nscaledev` namespace. Rotating it requires updating the Registry namespace
settings *and* the `GPG_PRIVATE_KEY` / `PASSPHRASE` repo secrets in the same
window — otherwise existing users will see signature verification failures.

## Hotfixes

For a critical fix on an older major series (e.g. patching `v1.x` after
`v2.0.0` ships), release-please's automation does not apply — it only tracks
`main`. Do it by hand:

1. Branch from the latest tag of that series: `git checkout -b release/1.x v1.4.0`.
2. Apply the fix and merge it to that branch.
3. Tag and push `v1.4.1` from that branch. This creates the tag but no
   Release, because `release.yml` has no tag trigger — create the Release with
   `gh release create v1.4.1 --notes "..."`, then build it with
   `gh workflow run release.yml -f tag=v1.4.1`.
4. Forward-port the fix to `main` if applicable, where it goes through the
   normal release-please flow.
