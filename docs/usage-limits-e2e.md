# OfficeCLI Usage-Limits E2E Guide

This guide describes practical end-to-end checks for free quota, paid quota, quota synchronization, and CLI behavior.

## Goal

Validate that:

- free mode works without an API key
- paid mode works with a valid API key
- quota changes in the platform are reflected in CLI status
- blocked states stop generation before the LLM run starts

## Suggested Scenarios

### Scenario A: paid quota update is reflected in the app

1. create or select an API key in admin
2. record the current remaining quota
3. purchase or manually add quota
4. reload the app billing and API key views
5. confirm the remaining quota matches the new value

### Scenario B: free-limit adjustment changes CLI status

1. perform a free `check` for a fixed machine fingerprint
2. raise `free_limit` in admin
3. run:

```bash
officecli auth status
```

4. confirm the reported remaining free quota reflects the new limit

### Scenario C: CLI blocks after free quota is exhausted

1. reduce `free_limit` close to `free_used`
2. run generation until the quota is exhausted
3. confirm the final attempt fails before content generation begins

### Scenario D: CLI blocks on invalid paid key state

1. disable a key or reduce its remaining quota to zero
2. run `officecli auth status`
3. confirm the CLI reports the correct blocked reason

## Manual Checks Still Worth Keeping

- app and admin surfaces display the same quota numbers
- CLI warnings match the actual access mode and remaining quota
- there is no stale cache after admin-side quota changes
