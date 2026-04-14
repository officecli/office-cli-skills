# Platform Production Deployment

This repository does not use a standard remote-image promotion workflow for production. The current production path is:

1. build the image locally
2. upload it to the server
3. import it into `k3s` or `containerd`
4. update the Deployment

When changing production deployment behavior, prefer the repository script instead of manually assembling ad hoc commands.

## Preferred Deployment Entry Point

Use:

```bash
scripts/deploy-platform-prod.sh
```

Do not manually rebuild the full deployment sequence unless you are debugging the deploy script itself.

## Required Validation Targets

After a production deploy, validate:

- `https://officecli.io/`
- `https://officecli.io/api/pricing`
- `https://platform.officecli.io/app/`
- `https://platform.officecli.io/admin/`
- `https://platform.officecli.io/healthz`

## Pre-Deploy Checklist

- confirm the current Deployment state on the server
- confirm the active Nginx site configuration for `officecli.io` and `platform.officecli.io`
- confirm the local image tag that will be uploaded
- confirm required secrets and config files are present
- confirm rollback artifacts are still available

## Post-Deploy Checklist

- verify the health endpoint
- verify the public site and pricing API
- verify app and admin entry routes
- verify recent logs for startup, migration, or static-file errors
- verify no unexpected redirect or domain-routing regression exists
