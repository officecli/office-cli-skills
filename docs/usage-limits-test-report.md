# OfficeCLI Usage-Limits Test Report

## Summary

- scope: licensing, free quota, paid quota, consumption timing, blocking behavior, and status visibility
- baseline commit: `25b166919666a3b4e576f611731ae6c189edbe01`
- report date: `2026-03-31`
- method: static review plus existing automated evidence and local execution results

## Result

- The repository already contains strong automated coverage for core usage-limit behavior.
- Critical CLI and platform tests were executed successfully in a corrected shell environment.
- The main remaining gap is cross-surface E2E proof after admin-side quota adjustments.

## Evidence Highlights

- CLI tests cover free-quota blocking, paid-key validation, status output, offline behavior, and consume warnings.
- Platform tests cover free and paid checks, idempotent consume, concurrency safety, and route-level error handling.
- App page tests cover quota visibility on overview and API key pages.

## Environment Note

Earlier failures were caused by a mismatched Go toolchain in an inherited shell environment:

```text
compile: version "go1.25.5" does not match go tool version "go1.26.1"
```

When needed, retry with:

```bash
env -u GOROOT zsh -lc 'go test ...'
```

## Remaining Risk

- CLI and platform behavior is well covered at the unit and package level.
- Cross-system visibility after admin-side quota changes still needs stronger E2E confirmation.
