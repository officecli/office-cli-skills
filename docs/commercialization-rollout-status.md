# OfficeCLI Monetization Four-Lane Status

Snapshot date: `2026-04-02`

This document summarizes which parts of the current monetization rollout are already implemented at `HEAD`, which parts are only partially implemented, and which parts still block a production-ready launch.

## Executive Summary

- Lane 1: reward, free, and paid quota evaluation is wired into backend checks and CLI output, but reward ledger audit visibility is still incomplete.
- Lane 2: invite codes, referral activation, and Discord connection services exist, but a trusted Discord OAuth and guild-verification loop is still missing.
- Lane 3: app and admin surfaces expose the current growth summaries, but attribution storage and production-grade analytics are still incomplete.
- Lane 4: tests and documentation are stronger than before, but external configuration, cross-service E2E evidence, and production credential validation still block release.

## Lane Overview

| Lane | Current status | Evidence | Main blocker |
| --- | --- | --- | --- |
| Lane 1: reward ledger, multi-source quota, CLI visibility | Partial | `platform/internal/reward/service.go`, `platform/internal/license/service.go`, `internal/cli/app.go`, `internal/cli/executor.go` | No reward ledger detail API or UI; no full DB plus CLI E2E evidence |
| Lane 2: invites, referral rewards, Discord rewards | Partial | `platform/internal/store/sqlstore/store.go`, `platform/internal/auth/service.go`, `platform/internal/growth/service.go` | No real Discord OAuth callback or guild verification |
| Lane 3: app, admin, and site growth visibility | Partial | `/api/app/growth`, `/api/admin/growth`, overview and growth pages | Attribution storage and reporting remain incomplete |
| Lane 4: tests, docs, and release evidence | Partial | release and deploy docs, package tests, monetization verification docs | External integrations and production credentials still require manual validation |

## Release Blockers

The following items should still be treated as blockers:

1. No reward-grant detail API or page for operational auditing
2. No trusted Discord OAuth and guild-membership verification flow
3. No complete cross-service E2E evidence for reward, referral, and Discord flows
4. No complete GA4, UTM, and invite-attribution storage and analytics pipeline
5. Google OAuth, Stripe webhook, Discord, and analytics production credentials are not fully validated

## Recommended External Wording

- Safe to say: OfficeCLI supports free, reward, and paid quota evaluation on the backend, with baseline visibility in the CLI and platform surfaces.
- Do not say: Discord reward automation and full attribution analytics are fully launched.
- For lane 2 and lane 3, describe them as partially implemented with remaining external integration and production-validation work.
