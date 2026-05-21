# Releasing the Nscale Terraform Provider

This provider is published to the [Terraform Registry](https://registry.terraform.io/providers/nscaledev/nscale)
as `nscaledev/nscale`. Releases follow [Semantic Versioning](https://semver.org/).

## Versioning policy

Tags must be of the form `vMAJOR.MINOR.PATCH` (e.g. `v1.2.3`). The Registry
requires the leading `v` and rejects anything else.

| Bump  | When to use                                                                                          |
| ----- | ---------------------------------------------------------------------------------------------------- |
| MAJOR | Any user-visible breaking change: removed/renamed attribute or resource, changed type, changed default behavior that breaks existing state, raising the minimum Terraform / Go version. |
| MINOR | New resource, data source, attribute, or feature — fully backward compatible.                        |
| PATCH | Bug fixes, dependency bumps, doc-only changes. No user-visible behavior change.                      |

Pre-release suffixes are supported: `v1.2.0-beta.1`, `v1.2.0-rc.1`. These are
published as GitHub pre-releases and appear on the Registry but are not the
"latest" version.

The provider is post-1.0, so breaking changes require a major bump (`v2.0.0`).

## Release process

### 1. Decide the version

Review changes since the previous tag:

```sh
git log $(git describe --tags --abbrev=0)..HEAD --no-merges --oneline
```

Pick the next version using the table above. If you're unsure between MINOR
and PATCH, prefer MINOR — the Registry treats them identically but downstream
users use `~>` constraints that differ.

### 2. Update `CHANGELOG.md`

- Move everything under `## [Unreleased]` into a new `## [X.Y.Z] - YYYY-MM-DD`
  section.
- Group entries under `BREAKING CHANGES`, `FEATURES`, `ENHANCEMENTS`,
  `BUG FIXES`, `DEPRECATIONS`, `DOCS` as appropriate.
- Add a fresh empty `## [Unreleased]` section at the top.
- Update the version compare links at the bottom of the file.

### 3. Open a release PR

Title: `chore: release vX.Y.Z`. Body: paste the new changelog section.
Merge once approved.

### 4. Tag and push

From `main`, on the merge commit for the release PR:

```sh
git checkout main && git pull
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

### 5. Watch the release

Pushing the tag triggers `.github/workflows/release.yml`, which runs
GoReleaser to:

- Cross-compile binaries for the OS/arch matrix the Registry expects.
- Produce `terraform-provider-nscale_X.Y.Z_SHA256SUMS` and sign it with the
  release GPG key (stored in the `GPG_PRIVATE_KEY` / `PASSPHRASE` repo
  secrets).
- Publish a GitHub Release with binaries, checksums, signature, and the
  `terraform-registry-manifest.json`.

Verify:

```sh
gh run watch                                              # CI run
gh release view vX.Y.Z                                    # GitHub Release
open https://registry.terraform.io/providers/nscaledev/nscale/X.Y.Z   # Registry
```

The Registry usually picks the release up within a few minutes via webhook.

### 6. Post-release

- Announce in the relevant channel.

## GPG key

The signing key is registered with the Terraform Registry under the
`nscaledev` namespace. Rotating it requires updating the Registry namespace
settings *and* the `GPG_PRIVATE_KEY` / `PASSPHRASE` repo secrets in the same
window — otherwise existing users will see signature verification failures.

## Hotfixes

For a critical fix on an older major series (e.g. patching `v1.x` after
`v2.0.0` ships):

1. Branch from the latest tag of that series: `git checkout -b release/1.x v1.1.0`.
2. Apply the fix, update `CHANGELOG.md` under a new section.
3. Tag `v1.1.1` from that branch and push.
4. Forward-port the fix to `main` if applicable.
