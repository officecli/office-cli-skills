# OfficeCLI Full Test Matrix

This file is the case inventory for OfficeCLI regression coverage. For the overall harness model, gate design, evidence rules, and journey-based testing strategy, see [`docs/harness-testing-strategy.md`](/home/ubuntu/workspace/officecli/docs/harness-testing-strategy.md).

This document provides a consolidated regression and delivery test matrix for three primary surfaces:

- the `officecli` CLI
- `platform-app`
- `platform-admin`

It is intended for developer self-testing, QA regression, integration sign-off, and pre-release checks.

## Scope

- CLI: configuration, argument parsing, generation, licensing, result output, publishing, PPT image generation, standalone image generation, interactive modes
- Platform app: login, overview, API key management, billing, usage, downloads, and growth visibility
- Platform admin: admin login, dashboard, users, orders, billing events, API keys, free quotas, usage events, and risk-control visibility

## Priority Levels

- `P0`: core path and release blocker
- `P1`: important failure paths and state consistency
- `P2`: edge cases, copy, empty states, and compatibility

## Core CLI Cases

- `CLI-CONFIG-001`: `config set-generation` writes generation settings on first run
- `CLI-CONFIG-002`: `config status` shows the current configuration summary
- `CLI-CONFIG-003`: `config set-license` uses the fixed platform base URL
- `CLI-CONFIG-004`: `config set-publish` updates publish settings independently
- `CLI-CONFIG-005`: `config set-defaults` updates default output settings independently
- `CLI-CONFIG-006`: missing generation settings should guide the user to `config set-generation`
- `CLI-NEW-001`: `new pptx` succeeds with valid config and writes a local file
- `CLI-NEW-002`: `new docx` succeeds
- `CLI-NEW-003`: `new xlsx` succeeds
- `CLI-NEW-004`: `--prompt` has the highest prompt-precedence priority
- `CLI-NEW-005`: `--prompt-file` overrides stdin and positional brief text
- `CLI-NEW-008`: `--mode fast` skips follow-up questions
- `CLI-NEW-009`: `--mode best` requires a TTY and enters interactive follow-up flow
- `CLI-NEW-011`: `--json` returns machine-readable output without progress copy
- `CLI-NEW-013`: `--publish` overrides config and forces publishing
- `CLI-NEW-014`: `--no-publish` overrides config and disables publishing
- `CLI-NEW-015`: publishing enabled without a publisher should return a clear warning
- `CLI-NEW-016`: `new img` uses the OfficeCLI server image route, writes one local image, publishes by default when configured, and exposes server quota metadata
- `CLI-NEW-017`: `new img --ratio square|landscape|portrait` maps to supported image ratios and rejects unsupported values
- `CLI-NEW-018`: `new img` rejects `--mode best`, `--file`, `--local-preview`, and `--no-images`; `--no-publish` keeps output local-only
- `CLI-REVIEW-001`: structural review succeeds for a valid local deck
- `CLI-REVIEW-002`: `--no-visual` skips LibreOffice-driven visual review

## Core App Cases

- `APP-OVERVIEW-001`: overview displays the latest quota and plan summary
- `APP-APIKEY-001`: API keys page shows current quota fields correctly
- `APP-BILLING-001`: billing page renders English-only pack names and descriptions
- `APP-BILLING-002`: checkout posts to the expected app endpoint
- `APP-DOWNLOADS-001`: downloads page exposes current binary release links
- `APP-GROWTH-001`: growth surfaces show reward, referral, and Discord summary state without overclaiming unsupported flows

## Core Admin Cases

- `ADMIN-LOGIN-001`: admin login succeeds only for allowed accounts
- `ADMIN-USERS-001`: users page shows invite code and account metadata
- `ADMIN-APIKEY-001`: API key creation exposes the full key exactly once
- `ADMIN-QUOTA-001`: free-quota adjustments affect subsequent license checks
- `ADMIN-USAGE-001`: usage events can be filtered by access mode, including reward events when available
- `ADMIN-GROWTH-001`: growth view shows current summary metrics and does not imply unsupported attribution guarantees

## Recommended Evidence

Prefer automated evidence where possible:

- Go package tests for CLI, runtime, and platform services
- page-level tests for React app and admin surfaces
- manual E2E only for flows that depend on real OAuth, Stripe, or external publishing services

For detailed quota and licensing scenarios, also see:

- [`docs/usage-limits-test-cases.md`](/home/ubuntu/workspace/officecli/docs/usage-limits-test-cases.md)
- [`docs/usage-limits-e2e.md`](/home/ubuntu/workspace/officecli/docs/usage-limits-e2e.md)
