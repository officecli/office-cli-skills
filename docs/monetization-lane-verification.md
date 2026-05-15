# Monetization Lane Verification

Updated: 2026-04-02

This snapshot records what the current `HEAD` already covers for the four commercialization lanes, plus the blockers that still prevent a production-ready closeout.

## Lane 1 - Quota source resolution and CLI feedback

Status: partial

Covered now:

- `platform/internal/store/sqlstore/store.go` provides account hosted credit accounts, ledger rows, reserve/settle/release idempotency, and API-key-to-owner-account credit resolution.
- `platform/internal/license/service.go` resolves anonymous trial, CLI session, API key owner, and External Mode access without treating API keys as independent Hosted wallets.
- `internal/cli/app.go` now exposes top-level `login`, `logout`, `whoami`, and `set-key`; `auth` remains compatibility-only rather than the public entrypoint.
- Tests cover account hosted credit grants, API key owner-account reservation, license checks, Hosted LLM settlement, growth rewards, billing, and CLI rendering.

Blockers:

- `license/check` and Hosted LLM responses still return summarized remaining counts; they do not expose a full account ledger cursor to the CLI.
- Operator-facing hosted credit ledger export is still minimal.
- End-to-end verification is still package-level; live API-to-CLI proof with a real production-like database fixture remains a release task.

## Lane 2 - Invite, referral activation, Discord reward flow

Status: partial

Covered now:

- `platform/internal/store/sqlstore/store.go` now generates deterministic `invite_code` values during Google-user creation/update, matching the unique schema introduced by `platform/migrations/004_growth_rewards.sql`.
- `platform/internal/auth/service.go` preserves invite codes through OAuth state and registers referrals after login callback.
- `platform/internal/growth/service.go` implements idempotent referral registration, invite-activation hosted credit grants, Discord connection linking, and Discord-join hosted credit grants.
- Tests cover invite-code propagation and referral registration in `platform/internal/auth/service_test.go`, plus referral/Discord reward idempotency in `platform/internal/growth/service_test.go`.

Blockers:

- Discord OAuth callback exists for CLI/app flow wiring, but production-grade guild-membership verification and background sync still need live operational validation.
- Referral flows are externally reachable now, and Discord has dedicated app routes; reward grants must still depend on trusted verification signals.
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
