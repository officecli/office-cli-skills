# AGENTS.md

Agents working in this repository should follow these default constraints:

- Always answer in Chinese.
- For CLI or npm release work, treat the public repository `officecli/officecli-ci` as the active release control plane. Do not use or rely on deprecated private-repo release workflows as the publishing entrypoint.
- For `officecli` `pptx` generation, note that suitable slides will auto-generate and embed images by default. Use `--no-images` for a text-only deck.
- Do not assume production release for `platform/` follows a standard remote image registry workflow. The current production flow is: local image build -> upload to server -> import into k3s/containerd -> update Deployment.
- Read `docs/platform-production-deploy.md` before changing production deployment behavior.
- When releasing `platform/` to production, prefer `scripts/deploy-platform-prod.sh` instead of assembling deployment commands manually.
- Before changing routes for `platform.officecli.io` or `officecli.io`, check the live Nginx site config and current Deployment state so the repository and production stay aligned.
- "发布到测试环境" means deploying the full `platform/` stack to `root@172.17.9.196` in that server's k3s cluster, namespace `officecli`, with ingress host `officecli.shimodev.com`.
- The testing environment must run its own k3s resources in namespace `officecli`: platform service, PostgreSQL, Redis, and MinIO. Do not point testing preview/object storage at production buckets.
- Testing environment integrations are real except Stripe: company-standard OAuth2, Hosted LLM, and MinIO must be configured with real staging/test credentials. Stripe may stay as test or placeholder config and does not need a real checkout/webhook verification.
- OAuth2 defaults to one app client via `OAUTH2_CLIENT_ID`/`OAUTH2_CLIENT_SECRET`; if the provider only allows one callback per client, configure `ADMIN_OAUTH2_CLIENT_ID`/`ADMIN_OAUTH2_CLIENT_SECRET` for the admin callback without changing the app client.
- For local CLI testing against the testing environment, use `officecli-dev`. It must keep an isolated config/session under the `officecli-dev` dev profile and point license, hosted generation, and preview publishing at `https://officecli.shimodev.com`; do not overwrite the user's production `officecli` config for this workflow.
- For testing environment deployment, prefer `scripts/deploy-platform-test.sh` and read `docs/platform-test-deploy.md`. This is separate from the production release path and does not use `officecli/officecli-ci` as the control plane unless that is explicitly added later.
- When releasing `platform/` to the testing environment, verify these endpoints by default:
  - `https://officecli.shimodev.com/`
  - `https://officecli.shimodev.com/api/pricing`
  - `https://officecli.shimodev.com/app/`
  - `https://officecli.shimodev.com/admin/`
  - `https://officecli.shimodev.com/healthz`
- When releasing `platform/`, verify these endpoints by default:
  - `https://officecli.io/`
  - `https://officecli.io/api/pricing`
  - `https://platform.officecli.io/app/`
  - `https://platform.officecli.io/admin/`
  - `https://platform.officecli.io/healthz`
