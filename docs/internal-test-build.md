# Internal Test Build Runbook

This runbook is for private OfficeCLI test binaries only. Internal builds are not public releases.

## Build

1. Open the private source repository in GitHub.
2. Run the `Internal CLI Build` workflow manually.
3. Set `internal_version` to an internal-only label such as `0.2.28-internal.1`, or leave it empty to use `0.2.28-internal.<run_number>`.
4. Download the `officecli-<version>-internal-binaries` artifact from the workflow run.

The artifact contains four binaries and `checksums.txt`:

- `officecli_<version>_darwin_amd64`
- `officecli_<version>_darwin_arm64`
- `officecli_<version>_linux_amd64`
- `officecli_<version>_linux_arm64`

## Verify

Before sharing an internal binary with a tester, verify the version label:

```bash
chmod +x ./officecli_0.2.28-internal.1_linux_amd64
./officecli_0.2.28-internal.1_linux_amd64 --version
```

The output must include the exact internal version label.

## Red Lines

- Internal artifacts must not be uploaded to any public repository, public GitHub Release, public object storage bucket, npm package, Homebrew tap, website download page, or chat channel that includes non-test users.
- Internal artifacts must not be renamed into stable release artifacts.
- Internal artifacts expire from GitHub Actions after 7 days.
- Public releases after testing must be rebuilt through the public `CLI Release` / `NPM Publish` flow. Do not reuse the internal artifact for a public release.

## Public Release After Testing

When testing is complete, release from the intended source commit through the normal public control plane in `officecli/officecli-ci`.

The public release flow must rebuild binaries, publish to `officecli/officecli-dist`, then publish npm through the public `NPM Publish` workflow. The internal workflow is only for private test artifacts and must not be treated as release evidence by itself.
