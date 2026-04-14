# OfficeCLI Monetization Status and Messaging Rules

This document records the monetization capabilities that are actually implemented at the current repository `HEAD`, the areas that remain incomplete, and the wording that public-facing docs should use.

## Capabilities Already Implemented

- Anonymous users can consume free quota tracked by machine fingerprint.
- Paid users can consume quota packs through API keys.
- Quota is consumed only after a document is generated successfully.
- The CLI, app, and admin views can display current free or paid quota information.

Reference areas:

- `internal/cli/app.go`
- `internal/cli/executor.go`
- `platform/internal/license/service.go`
- `platform/internal/app/application.go`

## Capabilities That Must Still Be Described as Planned or Reserved

The following items may have schema placeholders, route stubs, or UI wording, but they must not be described as fully launched unless the end-to-end implementation and evidence exist:

- Reward-based quota grants
- Invite-code registration loops
- Referral rewards after first successful generation
- Discord OAuth connection and guild-verification rewards
- GA4, UTM, and invite attribution loops

## Current Priority Rule for Quota Sources

Until the reward flow is fully completed, the safe priority rule is:

1. Use `paid` when a valid `license.api_key` is present
2. Otherwise use `free`
3. Treat `reward` as reserved protocol space unless the platform actually returns it

## Documentation Rule

README files, platform docs, and console copy must clearly distinguish between:

- Implemented capabilities that exist in production code today
- Reserved or planned capabilities that still depend on external integrations, additional services, or missing evidence
