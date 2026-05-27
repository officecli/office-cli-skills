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

## License Block Investigation

When a user reports recent OfficeCLI operations were blocked, inspect usage policy events by user id before changing quota data. `invalid_runtime_mode` means the client sent an unsupported `runtime_mode`; current clients should only send `external` or `hosted`.

```sql
SELECT
  ue.result,
  COALESCE(ue.reason_code, '[NULL]') AS reason_code,
  ue.mode,
  COALESCE(ue.runtime_mode, '[NULL]') AS runtime_mode,
  ue.action,
  COALESCE(ue.cli_version, '[NULL]') AS cli_version,
  COUNT(*) AS event_count,
  MIN(ue.created_at) AS first_at,
  MAX(ue.created_at) AS last_at
FROM usage_events ue
LEFT JOIN api_keys ak ON ak.id = ue.api_key_id
WHERE ue.created_at >= now() - interval '14 days'
  AND (ue.user_id = :user_id OR ak.owner_user_id = :user_id)
GROUP BY
  ue.result,
  COALESCE(ue.reason_code, '[NULL]'),
  ue.mode,
  COALESCE(ue.runtime_mode, '[NULL]'),
  ue.action,
  COALESCE(ue.cli_version, '[NULL]')
ORDER BY last_at DESC;
```

## Copy Rule

Any user-visible message observed during inspection must remain English-only. Treat unexpected Han output as a release regression.
