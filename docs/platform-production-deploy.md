# Platform Production Deployment

The `platform/` production release path does not use a standard remote image registry promotion flow.

The underlying deployment implementation is still:

1. build a linux/amd64 image
2. upload it to the production host
3. import it into `k3s` or `containerd`
4. sync static assets
5. update the Deployment

## Control Plane

The operator entry point is the public GitHub Actions control-plane repository:

- `officecli/officecli-ci`

Use these workflows there:

- `Platform Deploy`: code + static asset production deploys
- `Platform Sync Prod Secret`: production secret or config-only syncs

Do not add new active workflow YAML files under this private repository unless the control-plane design changes.

The private-repository script [`scripts/deploy-platform-prod.sh`](/home/ubuntu/workspace/officecli/scripts/deploy-platform-prod.sh) remains the deployment implementation used by `Platform Deploy`. Run it directly only when debugging the deploy script itself or when performing an emergency recovery.

## Recommended Release Flow

1. merge the target platform change into `officecli/officecli-internal`
2. create and push a production deploy tag such as `v0.2.0-prod-20260417-1`
3. dispatch `Platform Deploy` in `officecli/officecli-ci`
4. let the workflow check out the tagged private source tree and run [`scripts/deploy-platform-prod.sh`](/home/ubuntu/workspace/officecli/scripts/deploy-platform-prod.sh)
5. validate the production endpoints and any route-specific regressions

You can trigger the workflow from GitHub Actions UI, or from this repository with:

```bash
./scripts/dispatch-platform-prod-workflow.sh deploy v0.2.0-prod-20260417-1
```

## Config-Only Changes

When only production secret values change, use `Platform Sync Prod Secret` instead of a full code deploy.

Example:

```bash
./scripts/dispatch-platform-prod-workflow.sh sync-secret --restart-deployment
```

## Required Validation Targets

After a production deploy, validate:

- `https://officecli.io/`
- `https://officecli.io/api/pricing`
- `https://platform.officecli.io/app/`
- `https://platform.officecli.io/admin/`
- `https://platform.officecli.io/healthz`

For preview or route changes, also validate:

- a signed-out visit to `https://officecli.io/p/<share-token>` stays on the `/p/<share-token>` URL
- the browser shows a sign-in prompt instead of redirecting to `/officesdk/page`
- no `43052` callback error is surfaced during preview open

## Pre-Deploy Checklist

- confirm the current Deployment state on the server
- confirm the active Nginx site configuration for `officecli.io` and `platform.officecli.io`
- confirm the target production tag exists in `officecli/officecli-internal`
- confirm required `officecli/officecli-ci` secrets and variables are present
- confirm rollback artifacts are still available

## Post-Deploy Checklist

- verify the health endpoint
- verify the public site and pricing API
- verify app and admin entry routes
- verify recent logs for startup, migration, or static-file errors
- verify no unexpected redirect or domain-routing regression exists
