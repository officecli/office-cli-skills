# officecli

[Official website](https://officecli.io/)

`officecli` is an npm wrapper package for the OfficeCLI binary.

It does not reimplement the CLI in JavaScript. During `npm install`, it downloads the matching prebuilt binary from `officecli/officecli-dist`, verifies `checksums.txt`, and exposes the `officecli` command on your `PATH`.

## Install

```bash
npm install -g officecli
```

Run it after install:

```bash
officecli --version
officecli new docx "Product launch brief" --no-publish
```

The installed binary is usable without a local model endpoint or an API key. By default it uses OfficeCLI hosted anonymous trial access on `https://platform.officecli.io`; the one-time free quota is tied to this machine. When the free quota is used up, run `officecli auth set-key <api-key>` after purchasing or creating a hosted key from https://officecli.io/pricing.

To use your own model endpoint instead, switch to External Mode:

```bash
officecli config set-runtime external
officecli config set-generation
```

## Version Mapping

- npm package version `0.2.56` downloads OfficeCLI release `v0.2.56`
- the wrapper installs only the current stable binary that matches the package version

## Supported Platforms

- macOS `x64`
- macOS `arm64`
- Linux `x64`
- Linux `arm64`

Windows is not supported yet because the current public binary release flow only publishes `darwin` and `linux` archives.

## Environment Overrides

- `OFFICECLI_NPM_DIST_REPO`: override the GitHub release repository, default `officecli/officecli-dist`
- `OFFICECLI_NPM_SKIP_DOWNLOAD=1`: skip the postinstall download step

Legacy environment variables `OFFICECLI_NPM_VERSION` and `OFFICECLI_NPM_LATEST_TAG` are no longer supported because public distribution now keeps only the current stable release.

## Local Validation

From this repository:

```bash
cd packages/npm/officecli
npm pack --dry-run
npm install
npm run smoke:version
```
