# OfficeCLI Security Design Report

This document summarizes the main security design boundaries for the CLI and platform.

## Core Principles

- keep proprietary implementation out of public distribution repositories
- fail fast on missing production secrets
- hash API keys before persistence
- use idempotency keys for quota consumption
- keep admin and app session secrets separate
- avoid exposing internal implementation details in user-visible output

## CLI Security Notes

- the binary should validate quota status before generation begins
- paid mode should require online validation when a paid API key is configured
- publish credentials must not be embedded in source-controlled config
- update checks should not execute arbitrary code; they should only compare trusted release metadata

## Platform Security Notes

- API keys should be stored by hash plus prefix display only
- admin login must be rate-limited
- session cookies should use secure defaults in production
- request ids should be propagated for traceability
- `/healthz` should remain minimal and non-sensitive

## Remaining Risks

- incomplete external OAuth and billing integrations increase launch risk
- reward and referral flows need stronger end-to-end abuse controls
- public wording can create security and trust issues if it overclaims unsupported capabilities
