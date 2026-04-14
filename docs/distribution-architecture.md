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

## Release Flows

Versioned release flow:

1. Create a `vX.Y.Z` tag in the private source repository
2. Trigger the `CLI Release` GitHub Action
3. Build darwin and linux artifacts for amd64 and arm64 with GoReleaser
4. Publish artifacts to the public distribution repository
5. Sync the Homebrew tap formula

Rolling latest flow:

1. Push to `main`
2. Trigger the `CLI Publish Latest` GitHub Action
3. Build `officecli_latest_*` artifacts
4. Replace the public `latest` release assets
5. Sync the install script and distribution metadata

## Required Repository Variables

- `PUBLIC_DIST_REPO_OWNER=officecli`
- `PUBLIC_DIST_REPO_NAME=officecli-dist`
- `PUBLIC_DIST_REPO=officecli/officecli-dist`
- `PUBLIC_DIST_DEFAULT_BRANCH=main`
- `HOMEBREW_TAP_REPO=officecli/homebrew-officecli`
- `HOMEBREW_TAP_DEFAULT_BRANCH=main`
- `PUBLIC_SKILLS_REPO=officecli/officecli-skills`
- `PUBLIC_SKILLS_DEFAULT_BRANCH=main`
- `CLI_EMBEDDED_PUBLISH_BASE_URL=https://claudeoffice.com`
- `CLI_EMBEDDED_PUBLISH_AUTH_KEY_ID=officecli-cli`

## Required Secrets

- `PUBLIC_DIST_REPO_TOKEN`
- `HOMEBREW_TAP_TOKEN`
- `PUBLIC_SKILLS_REPO_TOKEN`
- `CLI_EMBEDDED_PUBLISH_AUTH_KEY`

Each token should grant the minimum write access required for the target public repository.

## Local Validation

Run at least:

```bash
go test ./...
bash -n scripts/install-officecli.sh
bash -n scripts/sync-public-dist-repo.sh
bash -n scripts/sync-homebrew-tap.sh
bash -n scripts/sync-public-skills-repo.sh
```

## Public Boundary

Public repositories must not include:

- private source repository URLs
- private module paths
- business credentials beyond CI secret names
- deployment environment details
- internal docs or commit history
