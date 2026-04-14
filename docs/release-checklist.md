# OfficeCLI Release Checklist

Use this checklist before declaring a release or production rollout complete.

## CLI

- version metadata is correct
- all user-visible output is English-only
- update detection behaves correctly
- `officecli version` reports the expected commit and build time
- `officecli new` works for at least one PPTX, DOCX, and XLSX case
- `officecli review` or `score` still works

## Platform

- pricing API returns the expected pack data
- app billing page uses English-only pack names and descriptions
- app and admin pages load successfully
- quota checks and quota consumption still work
- reward and referral wording does not overclaim unsupported flows

## Distribution

- tagged release artifacts exist where expected
- rolling latest artifacts are updated where expected
- public distribution repository is in sync
- Homebrew tap formula points to the expected asset and checksum
- skills repository, if updated, does not change binary semantics

## Operations

- production secrets and variables are present
- deployment script and rollback path are validated
- health checks pass
- post-deploy inspection passes

## Evidence

Attach links or command output for:

- the release tag
- the GitHub Action run
- the public release assets
- the Homebrew formula update
- the production inspection result
