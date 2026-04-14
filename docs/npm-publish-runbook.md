# NPM Publish Runbook

This runbook describes the intended publish flow for the `officecli` npm wrapper.

## Preconditions

Before publishing the npm package:

1. the matching CLI release tag already exists in the source repository
2. the matching public binary release already exists in `officecli/officecli-dist`
3. `packages/npm/officecli/package.json` version matches the release tag
4. npm package settings already trust this GitHub Actions workflow as a trusted publisher

## Recommended Flow

1. update `packages/npm/officecli/package.json` to the target version
2. create and push the matching tag such as `v0.2.5`
3. let `CLI Release` publish binaries to `officecli/officecli-dist`
4. let `NPM Publish` verify the public dist release exists
5. let `NPM Publish` run `npm pack`
6. let `NPM Publish` publish `officecli` to npm

## Trusted Publisher Setup

Configure npm trusted publishing for this exact GitHub Actions workflow:

1. go to npm package settings
2. open `Publishing access`
3. add a trusted publisher for GitHub Actions
4. set the repository to `officecli/officecli`
5. set the workflow filename to `npm-publish.yml`
6. save the trusted publisher at the package level in npm settings

Important notes:

- npm treats the workflow filename as case-sensitive and must match exactly
- trusted publishing uses GitHub OIDC, so no long-lived `NPM_TOKEN` is required
- if this repository remains private, avoid exposing the private repository URL through public package metadata
- this package intentionally points `homepage` to `https://officecli.io/` instead of the private source repository

## Manual Dispatch

You can also dispatch `.github/workflows/npm-publish.yml` manually with:

- `version`: release tag, for example `v0.2.5`
- `dist_repo`: optional override for the public dist repository

## Local Verification

```bash
cd packages/npm/officecli
npm run check:release-version
npm pack
npm install -g ./officecli-*.tgz
officecli --version
```

## Failure Modes

- if the npm version and tag differ, the workflow fails before publish
- if the matching public dist release is missing, the workflow fails before publish
- if the trusted publisher configuration is wrong, `npm publish` fails with authentication errors
