# Platform Testing Environment Deployment

The phrase "发布到测试环境" means deploying `platform/` to:

- SSH target: `root@172.17.9.196`
- k3s namespace: `officecli`
- Ingress host: `officecli.shimodev.com`

The testing environment is separate from production. Do not use the production `officecli/officecli-ci` control plane for this path unless that workflow is explicitly introduced later.

## Required Services

The testing namespace owns the full stack:

- `officecli-platform`: Go backend plus built site/app/admin assets
- `officecli-platform-postgres`: PostgreSQL StatefulSet with PVC
- `officecli-platform-redis`: Redis Deployment with PVC
- `officecli-minio`: MinIO Deployment with PVC for preview/object storage
- `officecli-platform` Ingress for `officecli.shimodev.com`

## Real Integration Policy

Testing uses real integrations except Stripe:

- OAuth2: company-standard OAuth2/OIDC with real testing client credentials
- Hosted LLM: real testing upstream key and models
- Object storage: real MinIO inside namespace `officecli`
- Stripe: test or placeholder config is acceptable; no real checkout/webhook verification is required

Prepare a local env file from `platform/.env.test.example`. Keep it out of git.

Required real keys include:

- `OAUTH2_AUTH_URL`
- `OAUTH2_TOKEN_URL`
- `OAUTH2_USERINFO_URL`
- `OAUTH2_CLIENT_ID`
- `OAUTH2_CLIENT_SECRET`
- `HOSTED_LLM_BASE_URL`
- `HOSTED_LLM_API_KEY`
- `HOSTED_LLM_TEXT_MODEL`
- `HOSTED_LLM_IMAGE_MODEL`

If the OAuth2 server allows only one callback URL per client, create a second OAuth2 client for admin and set:

- `ADMIN_OAUTH2_CLIENT_ID`
- `ADMIN_OAUTH2_CLIENT_SECRET`

When those two values are empty, admin OAuth2 automatically reuses `OAUTH2_CLIENT_ID` and `OAUTH2_CLIENT_SECRET`.

The deploy script fills the in-cluster values for `POSTGRES_DSN`, `REDIS_ADDR`, and `PREVIEW_OBJECT_*`.

## Deploy

```bash
PLATFORM_ENV_FILE=/secure/path/officecli-platform-test.env \
  bash scripts/deploy-platform-test.sh
```

Useful overrides:

```bash
TEST_SERVER_HOST=172.17.9.196
TEST_SERVER_USER=root
TEST_DOMAIN=officecli.shimodev.com
TEST_NAMESPACE=officecli
TEST_TLS_SECRET=officecli-shimodev-com-tls
```

If `TEST_TLS_SECRET` is empty, the script creates an HTTP Ingress only. Add a TLS Secret or cert-manager flow before declaring HTTPS validation complete.

## DNS

`officecli.shimodev.com` must resolve to `172.17.9.196` for normal validation. Before DNS is switched, use:

```bash
TEST_RESOLVE_IP=172.17.9.196 bash scripts/run-test-inspection.sh
```

## Validation

After deployment, run:

```bash
bash scripts/run-test-inspection.sh
```

Default endpoints:

- `https://officecli.shimodev.com/`
- `https://officecli.shimodev.com/api/pricing`
- `https://officecli.shimodev.com/app/`
- `https://officecli.shimodev.com/admin/`
- `https://officecli.shimodev.com/healthz`

Also verify in-cluster rollout:

```bash
ssh root@172.17.9.196 'kubectl -n officecli get all,ingress,pvc'
ssh root@172.17.9.196 'kubectl -n officecli rollout status statefulset/officecli-platform-postgres'
ssh root@172.17.9.196 'kubectl -n officecli rollout status deployment/officecli-platform-redis'
ssh root@172.17.9.196 'kubectl -n officecli rollout status deployment/officecli-minio'
ssh root@172.17.9.196 'kubectl -n officecli rollout status deployment/officecli-platform'
```
