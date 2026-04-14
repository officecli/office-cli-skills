# OfficeCLI Production Inspection Guide

Use this guide for lightweight production inspection after deployment or during incident review.

## Checkpoints

- site homepage responds correctly
- pricing API responds correctly
- platform app route loads
- platform admin route loads
- health endpoint responds
- no obvious redirect or TLS regressions

## Suggested Commands

```bash
curl -I https://officecli.io/
curl -s https://officecli.io/api/pricing | jq .
curl -I https://platform.officecli.io/app/
curl -I https://platform.officecli.io/admin/
curl -I https://platform.officecli.io/healthz
```

## Logs and Runtime State

- inspect service logs for startup failures, migration failures, billing webhook issues, and auth failures
- inspect recent deployment events in the cluster
- confirm the expected image tag is running
- confirm Nginx routing still matches the deployed services

## Copy Rule

Any user-visible message observed during inspection must remain English-only. Treat unexpected Han output as a release regression.
