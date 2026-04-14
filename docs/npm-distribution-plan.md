# NPM Distribution Plan

This document records the initial npm packaging direction for `officecli`.

## Goal

Publish `officecli` to npm without rewriting the CLI in JavaScript.

The npm package should act as a thin wrapper that:

- downloads the matching prebuilt binary from `officecli/officecli-dist`
- verifies `checksums.txt`
- exposes the `officecli` command through the standard npm global install flow

The publish workflow should run from a dedicated public repository so npm trusted publishing, package metadata, and the visible repository URL all point to the same public source.

## Current Package Layout

The initial package lives at `packages/npm/officecli` and contains:

- `package.json`
- `scripts/postinstall.js`
- `lib/install.js`
- `bin/officecli.js`

## Current Behavior

- supported OS: `darwin`, `linux`
- supported CPU: `x64`, `arm64`
- default dist repo: `officecli/officecli-dist`
- default release version: npm package version
- optional override to `latest` via `OFFICECLI_NPM_VERSION=latest`

## Remaining Work Before Publish

1. Create the public npm repository, for example `officecli/officecli-npm`
2. Configure `PUBLIC_NPM_REPO` and `PUBLIC_NPM_REPO_TOKEN` in the private source repository
3. Configure npm trusted publishing against the public npm repository
4. Decide whether npm publish should track stable tags only or also rolling prereleases
5. Decide whether Windows support should be added to the binary release flow

## Validation

Local:

```bash
cd packages/npm/officecli
npm pack
npm install -g ./officecli-*.tgz
officecli --version
```

CI:

- `.github/workflows/npm-wrapper-ci.yml`
- `.github/workflows/npm-publish.yml`
- `.github/workflows/sync-public-npm.yml`

## Release Guardrails

- `packages/npm/officecli/scripts/check-release-version.js` enforces that the npm package version matches the release tag
- `NPM Publish` verifies that the matching GitHub release already exists in `officecli/officecli-dist`
- `NPM Publish` uses GitHub OIDC trusted publishing instead of a long-lived `NPM_TOKEN`
- the npm package metadata should not expose the private source repository URL
- the public npm repository should add its own public `repository.url` during sync
