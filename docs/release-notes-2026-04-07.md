# Release Notes - 2026-04-07

## Highlights

- Added a GitHub Actions installed E2E workflow for `officecli` that installs the released CLI in a clean Linux container, runs PPT generation and review, uploads artifacts, and sends email plus DingTalk notifications.
- Added skill-level environment self-heal for both Codex and OpenClaw `officecli` skills so agents can detect missing `officecli`, repair installation, and fill required generation/license config before continuing.
- Added automated tests and CI checks for the new skill environment repair flow.

## Installed E2E

- New workflow: `CLI Installed E2E`
- Supports scheduled runs plus `workflow_dispatch`
- Installs released `officecli` from `officecli/officecli-dist`
- Runs PPT evaluation with score summary and preview URL reporting
- Uploads artifacts and notifies by email and DingTalk
- Added repo-variable controls for enablement and default suite/version
- Hardened summary rendering and installed-version parsing

## Skill Environment Self-Heal

- New scripts in `skills/officecli/` and `skills/openclaw-officecli/`:
  - `check-officecli-env.sh`
  - `fix-officecli-env.sh`
  - `env-common.sh`
- Agents now follow a standard flow:
  - check environment first
  - auto-install `officecli` when missing
  - repair missing generation/license config
  - optionally repair publish config when preview is required
  - re-check before generating documents or starting `agent-bridge`
- OpenClaw repair also rewrites `config.yaml` with the detected `officecli` path and bridge command.

## CI / Docs

- `CLI CI` now validates the new env scripts and runs `scripts/test-skill-officecli-env.sh`
- OpenClaw quickstart, skill README, and install script now document the new self-check/self-repair flow
- Updated GitHub Actions versions to current major releases and opted workflows into Node 24 runtime

## Known Issue

- The installed E2E workflow currently exposes a real compatibility problem between the latest public release and the current license service contract: released binaries fail license validation with `request_nonce is required`. The workflow now reports that failure more clearly, but a new compatible CLI release is still needed for the installed-release path to pass.
