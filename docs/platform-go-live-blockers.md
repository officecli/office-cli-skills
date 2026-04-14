# Platform Go-Live Blockers

This is the current blocker list that should be cleared before declaring the platform production-ready.

## Integration Blockers

- production Google OAuth credentials and callback validation
- production Stripe live key and webhook validation
- Discord OAuth and trusted guild-verification flow
- analytics and attribution configuration validation

## Product and Operations Blockers

- reward ledger detail APIs and operational visibility
- cross-service E2E evidence for reward, referral, and Discord flows
- final copy review for English-only user-facing text
- production secret validation and rollback readiness

## Release Rule

Do not mark the platform as fully launched until each blocker has evidence attached in the release checklist or linked deployment notes.
