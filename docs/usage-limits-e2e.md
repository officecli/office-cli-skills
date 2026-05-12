# OfficeCLI Usage-Limits E2E Guide

This guide describes practical end-to-end checks for External Mode free-unlimited behavior, Hosted Mode credit behavior, billing visibility, and CLI behavior.

## Goal

Validate that:

- External Mode works without an API key and remains allowed after repeated consumes
- External Mode with an API key still does not consume legacy external quota
- Hosted Mode works with a hosted-enabled key and consumes hosted credits after success
- Hosted blocked states stop generation before the LLM run starts
- app/admin/database views show hosted credits, orders, usage events, and hosted credit grants consistently

## Suggested Scenarios

### Scenario A: External Mode stays free and unlimited

1. perform an external `check` for a fixed machine fingerprint
2. perform `consume` with the returned commit token
3. repeat the check/consume sequence multiple times
4. confirm every external check remains `allowed=true`
5. query `usage_events` and confirm `runtime_mode='external'`, `billed_units=0`, and `settled_credits=0`

### Scenario B: Hosted credits are required and consumed

1. create or select a hosted-enabled API key with a positive `credit_balance`
2. run a Hosted Mode document or IMG generation
3. confirm the key's `credit_balance` decreases and `credit_reserved` returns to zero after settlement
4. reduce hosted credits to zero
5. confirm Hosted Mode is blocked before content generation begins

### Scenario C: Signup and invite hosted credits are idempotent

1. sign in as a new Google user and open the app Overview
2. confirm one Starter key exists and the account receives 30 hosted credits
3. sign in again and confirm no duplicate signup grant is created
4. activate one invited user through first successful generation
5. confirm the inviter receives exactly 20 hosted credits and repeat activation does not duplicate the grant

### Scenario D: Billing sells hosted credits only

1. load `/api/pricing`
2. confirm every returned pack has `pack_kind='hosted_credits'`
3. confirm App Billing only renders hosted credit packs
4. confirm historical external orders remain visible in order history
5. confirm a manual external checkout request is rejected with `external mode is free and no longer sold as a paid pack`

## Manual Checks Still Worth Keeping

- app and admin surfaces display the same hosted credit numbers
- CLI output matches the selected runtime mode
- there is no stale cache after admin-side key or credit changes
