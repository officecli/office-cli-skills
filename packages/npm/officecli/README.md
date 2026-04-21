# officecli

`officecli` is an npm wrapper package for the OfficeCLI binary.

It does not reimplement the CLI in JavaScript. During `npm install`, it downloads the matching prebuilt binary from `officecli/officecli-dist`, verifies `checksums.txt`, and exposes the `officecli` command on your `PATH`.

## Install

```bash
npm install -g officecli
```

Run it after install:

```bash
officecli --version
```

## Version Mapping

- npm package version `0.2.20` downloads OfficeCLI release `v0.2.20`
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
