# OfficeCLI Closed-Source Distribution Architecture

This document describes the recommended repository layout and release flow for a closed-source codebase with public binary distribution.

## Repository Roles

### Private source repository

The current repository remains private and owns:

- source code
- tests and CI
- official builds
- GoReleaser configuration
- scripts and templates that sync public repositories

### Public distribution repository

Suggested repository: `officecli/officecli-dist`

It should own:

- GitHub Releases
- binary archives
- `checksums.txt`
- the Linux install script
- minimal end-user install instructions

### Public Homebrew tap repository

Suggested repository: `officecli/homebrew-officecli`

It should own:

- `Formula/officecli.rb`
- release URLs that point to the public distribution repository

### Public skills repository

Suggested repository: `officecli/officecli-skills`

It should own:

- synced `skills/` content
- public `SKILL.md` instructions
- examples without exposing proprietary implementation details

### Public npm package

Suggested package: `officecli`

Suggested repository: `officecli/officecli-npm`

It should own:

- a thin Node wrapper only
- package metadata and install scripts
- no proprietary source code beyond the wrapper logic
- runtime downloads that point to the public distribution repository
- public-facing metadata that points to public endpoints only
- the trusted-publishing workflow that actually runs `npm publish`

### Public CI control plane

Suggested repository: `officecli/officecli-ci`

It should own:

- release and operations workflows that check out the private source repository
- the active `CLI Release`, `NPM Publish`, and installed E2E workflows
- the GitHub Actions entrypoints used for public binary and npm publication

## Release Flows

Versioned release flow:

1. Create a `vX.Y.Z` tag in the private source repository
2. Trigger the public `CLI Release` workflow in `officecli/officecli-ci`
3. Build darwin and linux artifacts for amd64 and arm64 with GoReleaser
4. Publish artifacts to the public distribution repository
5. Sync the Homebrew tap formula

Npm wrapper flow:

1. Update `packages/npm/officecli/package.json` to the target version
2. Ensure the matching `vX.Y.Z` binary release already exists in `officecli/officecli-dist`
3. Trigger the public `NPM Publish` workflow in `officecli/officecli-ci`
4. Let the public control plane publish the package with trusted publishing
5. Verify `npm install -g officecli` still resolves the matching binary release

Current stable retention flow:

1. Publish the newest `vX.Y.Z` release to `officecli/officecli-dist`
2. Sync the Homebrew formula and distribution install script
3. Prune older public dist releases and tags so only the newest stable release remains

## Required Repository Variables

- `PUBLIC_DIST_REPO_OWNER=officecli`
- `PUBLIC_DIST_REPO_NAME=officecli-dist`
- `PUBLIC_DIST_REPO=officecli/officecli-dist`
- `PUBLIC_DIST_DEFAULT_BRANCH=main`
- `HOMEBREW_TAP_REPO=officecli/homebrew-officecli`
- `HOMEBREW_TAP_DEFAULT_BRANCH=main`
- `PUBLIC_SKILLS_REPO=officecli/officecli-skills`
- `PUBLIC_SKILLS_DEFAULT_BRANCH=main`
- `PUBLIC_NPM_REPO=officecli/officecli-npm`
- `PUBLIC_NPM_DEFAULT_BRANCH=main`
- `NPM_PACKAGE_NAME=officecli`
- `CLI_EMBEDDED_PUBLISH_BASE_URL=https://platform.officecli.io`
- `CLI_EMBEDDED_PUBLISH_AUTH_KEY_ID=officecli-cli`

## Required Secrets

- `PUBLIC_DIST_REPO_TOKEN`
- `HOMEBREW_TAP_TOKEN`
- `PUBLIC_SKILLS_REPO_TOKEN`
- `PUBLIC_NPM_REPO_TOKEN`
- `CLI_EMBEDDED_PUBLISH_AUTH_KEY`

Each token should grant the minimum write access required for the target public repository.

For npm publication, prefer trusted publishing via GitHub Actions OIDC instead of a long-lived npm token.

## Local Validation

Run at least:

```bash
go test ./...
bash -n scripts/install-officecli.sh
bash -n scripts/prune-public-dist-releases.sh
bash -n scripts/sync-public-dist-repo.sh
bash -n scripts/sync-homebrew-tap.sh
bash -n scripts/sync-public-npm-repo.sh
bash -n scripts/sync-public-skills-repo.sh
(cd packages/npm/officecli && npm pack --dry-run)
```

## Public Boundary

Public repositories must not include:

- private source repository URLs
- private module paths
- business credentials beyond CI secret names
- deployment environment details
- internal docs or commit history

Current policy:

- public dist keeps only the newest stable release
- Homebrew keeps only the single current `Formula/officecli.rb`
- the npm wrapper installs only the package-matched stable release and no longer supports `latest` or historical version overrides
