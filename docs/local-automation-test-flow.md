# OfficeCLI Local Automation Test Flow

This document describes a practical local flow for running the most useful automated checks before opening a PR or preparing a release candidate.

## Goals

- catch English-only copy regressions
- validate core Go packages
- validate key frontend tests
- ensure release-related scripts still parse correctly

## Recommended Local Sequence

1. Run the no-Han scanner:

```bash
python3 ./scripts/check-no-han.py
```

2. Run core Go tests:

```bash
go test ./internal/cli ./internal/review ./internal/providers/llm ./internal/runtime ./pkg/ooxmledit ./pkg/officegen ./engine/nonppt ./engine/ppt ./engine/plan
```

3. Run platform Go tests from the submodule:

```bash
cd platform
go test ./internal/license ./internal/app ./internal/auth ./internal/appuser ./internal/growth ./internal/reward
```

4. Run important frontend tests:

```bash
cd platform
pnpm test -- --runInBand src/pages/BillingPage.test.tsx src/pages/OverviewPage.test.tsx src/pages/ApiKeysPage.test.tsx
```

5. Validate release scripts:

```bash
bash -n scripts/install-officecli.sh
bash -n scripts/sync-public-dist-repo.sh
bash -n scripts/sync-homebrew-tap.sh
bash -n scripts/sync-public-skills-repo.sh
```

## Notes

- If an existing shell has a stale `GOROOT`, retry with `env -u GOROOT`.
- Prefer login shells for release verification so the shell picks up the current user environment.
- Use manual E2E only for flows that depend on real OAuth, Stripe, publishing, or hosted runtime credentials.
