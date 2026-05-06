Private-repository release workflows are deprecated in this repository.

Active release and operations workflows now live in the public control-plane repository:

- `officecli/officecli-ci`

Use that repository for:

- `CLI Release`
- `NPM Publish`
- `CLI Installed E2E`
- `Platform Deploy`
- `Production Inspection`

Do not add new active workflow YAML files under this private repository unless the control-plane design changes again.

Exception:

- `internal-cli-build.yml` is the only exception. It is a private, manual, internal-only test build workflow.
- It uploads short-lived GitHub Actions artifacts for authorized private-repository members only.
- It must not be used as a release control plane.
- It must not create tags, GitHub Releases, npm packages, Homebrew updates, public download pages, or any artifact in `officecli/officecli-dist`.
