# OfficeCLI Monetization Status and Messaging Rules

This document records the monetization capabilities that are actually implemented at the current repository `HEAD`, the areas that remain incomplete, and the wording that public-facing docs should use.

## Capabilities Implemented In This Rollout

- Anonymous users can consume Hosted trial quota tracked by machine fingerprint.
- Authenticated CLI sessions can consume account hosted credits through `officecli login`.
- API keys are account access credentials; they share the owning account hosted credits instead of carrying independent Hosted balances.
- Hosted credits are consumed only after a document or image is generated successfully.
- Checkout, signup bonus, invite activation bonus, and Discord join bonus write account hosted credit ledger rows.
- Invite activation and Discord verification each grant 100 account hosted credits through idempotency keys.
- The CLI, app, and admin views can display anonymous trial, account hosted credits, orders, usage events, and credential state.

Reference areas:

- `internal/cli/app.go`
- `internal/cli/executor.go`
- `platform/internal/license/service.go`
- `platform/internal/app/application.go`

## Capabilities That Must Still Be Described as Planned or Reserved

The following items may have schema placeholders, route stubs, or UI wording, but they must not be described as fully launched unless the end-to-end implementation and evidence exist:

- GA4, UTM, and invite attribution loops
- Deeper operator exports, fraud review, and invite-policy tooling

## Current Priority Rule for Quota Sources

The safe priority rule is:

1. Use API key mode when a valid local API key is configured; the key resolves to `owner_user_id` and spends that account's hosted credits.
2. Otherwise use CLI session mode when `officecli login` has stored a valid session token.
3. Otherwise use anonymous Hosted trial by fingerprint.
4. External Mode remains free and unlimited when explicitly configured.

## Documentation Rule

README files, platform docs, and console copy must clearly distinguish between:

- Implemented capabilities that exist in repository code today
- Reserved or planned capabilities that still depend on external integrations, additional services, or missing evidence
- API keys are advanced credentials, not standalone credit wallets
