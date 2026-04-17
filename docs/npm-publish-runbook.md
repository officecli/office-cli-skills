# NPM Publish Runbook

This runbook describes the intended publish flow for the `officecli` npm wrapper.

The active publish control plane is the public repository `officecli/officecli-ci`. Do not use the deprecated private-repository workflows as the npm publishing entrypoint.

## Preconditions

Before publishing the npm package:

1. the matching CLI release tag already exists in the private source repository
2. the matching public binary release already exists in `officecli/officecli-dist`
3. npm package settings already trust the public control-plane workflow as a trusted publisher

## Recommended Flow

1. update `packages/npm/officecli/package.json` to the target version in the private source repository
2. create and push the matching tag such as `v0.2.6`
3. let `officecli/officecli-ci` run `CLI Release` to publish binaries to `officecli/officecli-dist`
4. let `officecli/officecli-ci` prune older public dist releases so only the newest stable release remains
5. let `officecli/officecli-ci` run `NPM Publish` after the dist release exists
6. let the public control plane publish `officecli` to npm

## Trusted Publisher Setup

Configure npm trusted publishing for this exact GitHub Actions workflow:

1. go to npm package settings
2. open `Publishing access`
3. add a trusted publisher for GitHub Actions
4. set the repository to `officecli/officecli-ci`
5. set the workflow filename to `npm-publish.yml`
6. save the trusted publisher at the package level in npm settings

Important notes:

- npm treats the workflow filename as case-sensitive and must match exactly
- trusted publishing uses GitHub OIDC, so no long-lived `NPM_TOKEN` is required
- the public-facing npm metadata can still point to the public repositories used for distribution
- the package `homepage` can still point to `https://officecli.io/`
- historical npm version deprecation is not part of the trusted-publishing flow and still requires separate npm registry credentials if you choose to enforce it

## Manual Dispatch

You can also dispatch `npm-publish.yml` in `officecli/officecli-ci` manually with:

- `version`: release tag, for example `v0.2.6`
- `dist_repo`: optional override for the public dist repository

## Local Verification

```bash
cd officecli-npm
npm run check:release-version
npm pack
npm install -g ./officecli-*.tgz
officecli --version
```

## Failure Modes

- if the npm version and tag differ, the workflow fails before publish
- if the matching public dist release is missing, the workflow fails before publish
- if the trusted publisher configuration is wrong, `npm publish` fails with authentication errors
- if someone attempts to rely on `OFFICECLI_NPM_VERSION` or `OFFICECLI_NPM_LATEST_TAG`, install now fails because the public package only supports the current stable release
