# Monetization Lane Verification

Updated: 2026-04-02

This snapshot records what the current `HEAD` already covers for the four commercialization lanes, plus the blockers that still prevent a production-ready closeout.

## Lane 1 - Quota source resolution and CLI feedback

Status: partial

Covered now:

- `platform/internal/reward/service.go` and `platform/internal/store/sqlstore/store.go` provide reward-grant storage, balance aggregation, and single-credit consume behavior.
- `platform/internal/license/service.go` now checks reward balance before free quota when no API key is supplied, and supports reward consumes with idempotent restore semantics.
- `internal/cli/app.go` and `internal/cli/executor.go` already surface `reward_remaining` in auth status and post-generate warnings.
- Tests cover reward path resolution and consume/idempotency in `platform/internal/license/service_test.go`, plus CLI rendering in `internal/cli/app_test.go` and `internal/cli/executor_test.go`.

Blockers:

- `license/check` and `license/consume` still return only summarized remaining counts; they do not expose the concrete reward grant source or ledger row selected for a consume.
- There is still no operator-facing reward ledger page or export path for auditing cross-source consumption decisions.
- End-to-end verification is still package-level; there is no live API-to-CLI reward flow proof with a real database fixture.

## Lane 2 - Invite, referral activation, Discord reward flow

Status: partial

Covered now:

- `platform/internal/store/sqlstore/store.go` now generates deterministic `invite_code` values during Google-user creation/update, matching the unique schema introduced by `platform/migrations/004_growth_rewards.sql`.
- `platform/internal/auth/service.go` preserves invite codes through OAuth state and registers referrals after login callback.
- `platform/internal/growth/service.go` implements idempotent referral registration, invite-activation reward grants, Discord connection linking, and Discord-join reward grants.
- Tests cover invite-code propagation and referral registration in `platform/internal/auth/service_test.go`, plus referral/Discord reward idempotency in `platform/internal/growth/service_test.go`.

Blockers:

- There is still no real Discord OAuth callback, guild-membership verification client, or background sync against Discord itself.
- Referral flows are externally reachable now, and Discord now has dedicated `/api/app/discord/connect` and `/api/app/discord/status` routes, but the Discord route intentionally stays in a blocked-verification state until a trusted guild checker exists.
- Anti-abuse is still limited to unique keys and service idempotency; there is no rate limit, fraud review, or invite-policy enforcement layer yet.

## Lane 3 - App/admin/site reward visibility and attribution

Status: partial

Covered now:

- `platform/internal/appuser/service.go` aggregates `reward_remaining`, `invite_code`, reward grants, referral detail, and Discord status into `/api/app/growth`, plus dedicated Discord connect/status responses.
- `platform/web/app/src/pages/OverviewPage.tsx` now renders reward grant detail, referral progress, and Discord connection state from the real growth payload; `platform/web/app/src/App.test.tsx` covers the shell.
- `platform/web/admin/src/pages/GrowthPage.tsx` now exposes operator-facing reward/referral/Discord lists, and coverage in `platform/web/admin/src/App.test.tsx` confirms the route wiring.
- Route coverage now includes real `/api/app/discord/connect` and `/api/app/discord/status` wiring in `platform/internal/app/application_app_routes_test.go`.
- `platform/web/site/src/analytics.ts` and `platform/web/app/src/analytics.ts` now initialize GA4 conditionally and emit minimal tracked events for login, pricing, download, checkout, and invite parameter carry.

Blockers:

- The current reward/referral/Discord UI is still minimal and does not yet provide exports, filtering, or deeper operational drilldowns.
- GA4 event wiring exists now, but there is still no attribution persistence, reporting surface, or production-measurement validation evidence.

## Lane 4 - End-to-end closeout, docs, release blockers

Status: partial

Covered now:

- Package-level verification now covers reward/growth/auth/app overview wiring via `platform/internal/{reward,growth,license,auth,appuser,app}` tests.
- CLI reward messaging remains covered in `internal/cli/*_test.go`.
- Runbooks and release docs still exist in `docs/release-checklist.md`, `docs/platform-production-deploy.md`, and `platform/README.md`.

Blockers:

- `platform/configs/config.example.yaml` still lacks Discord OAuth / guild settings, GA4 configuration, and invite-attribution configuration.
- There is still no cross-service E2E artifact proving reward/referral/Discord behavior against a live stack.
- Google OAuth, Stripe, Discord, and analytics production credentials still require manual rollout validation outside the repo.

## Verification commands used for this snapshot

```bash
cd platform && go test ./internal/growth ./internal/reward ./internal/license ./internal/appuser ./internal/auth ./internal/app
rg -n "reward|invite|discord|referral|ga4|utm|analytics" platform/internal platform/web docs README.md
```
