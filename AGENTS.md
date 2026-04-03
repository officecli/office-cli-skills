# AGENTS.md

本仓库内工作的 agent 默认遵循以下约束：

- 永远用中文回答。
- 涉及 `officecli` 的 `pptx` 生成时，注意当前默认会自动生图并将图片嵌入 PPT；如需纯文本版，请使用 `--no-images`。
- 不要假设 `platform/` 的生产发布是标准远程镜像仓库流程；当前生产环境依赖“本地构建镜像 -> 传到服务器 -> 导入 k3s/containerd -> 更新 Deployment”。
- 修改生产部署相关内容前，先阅读 `docs/platform-production-deploy.md`。
- 发布 `platform/` 到生产环境时，优先使用 `scripts/deploy-platform-prod.sh`，不要手工重新拼发布命令。
- 修改 `platform.officecli.io` 或 `officecli.io` 路由前，先检查服务器上的 Nginx 站点配置与当前 Deployment 状态，避免只改仓库、不改现网。
- 如果要发布 `platform/`，默认同时验证：
  - `https://officecli.io/`
  - `https://officecli.io/api/pricing`
  - `https://platform.officecli.io/app/`
  - `https://platform.officecli.io/admin/`
  - `https://platform.officecli.io/healthz`
