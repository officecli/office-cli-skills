# AGENTS.md

Agents working in this repository should follow these default constraints:

- Always answer in Chinese.
- For CLI or npm release work, treat the public repository `officecli/officecli-ci` as the active release control plane. Do not use or rely on deprecated private-repo release workflows as the publishing entrypoint.
- For `officecli` `pptx` generation, note that suitable slides will auto-generate and embed images by default. Use `--no-images` for a text-only deck.
- Do not assume production release for `platform/` follows a standard remote image registry workflow. The current production flow is: local image build -> upload to server -> import into k3s/containerd -> update Deployment.
- Read `docs/platform-production-deploy.md` before changing production deployment behavior.
- When releasing `platform/` to production, prefer `scripts/deploy-platform-prod.sh` instead of assembling deployment commands manually.
- Before changing routes for `platform.officecli.io` or `officecli.io`, check the live Nginx site config and current Deployment state so the repository and production stay aligned.
- When releasing `platform/`, verify these endpoints by default:
  - `https://officecli.io/`
  - `https://officecli.io/api/pricing`
  - `https://platform.officecli.io/app/`
  - `https://platform.officecli.io/admin/`
  - `https://platform.officecli.io/healthz`
