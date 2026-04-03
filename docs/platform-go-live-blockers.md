# platform 上线前 blockers 清单（按当前 HEAD）

本文档只记录当前仓库已经能明确识别的 blockers。凡是依赖第三方账号、生产环境变量或人工发布环境验证的事项，一律保持为未完成，避免假装已上线。

## P0：当前明确阻塞商业化闭环的事项

### 1. Discord 真实校验与发奖闭环仍未真正上线

当前仓库已经具备：

- reward grant 的 service / store / route 闭环
- 邀请注册到首次成功生成发奖闭环
- `/api/app/growth`、`/api/admin/growth`
- app overview/growth 明细与 admin growth 页面
- `/api/app/discord/connect`、`/api/app/discord/status` 最小真实入口

但仍缺少：

- 真实 Discord OAuth callback
- 可信 guild membership 校验
- 可信校验成功后的 Discord 发奖闭环

结论：奖励体系不能再按“只有预留”描述，但 Discord 奖励仍不能按“已正式上线”对外宣称。

### 2. 平台 license 服务仍只有 free / paid 两条真实路径

当前真实判定逻辑仍在：

- `platform/internal/license/service.go`

从代码看：

- `check` 只会进入 `checkPaid` 或 `checkFree`
- `consume` 只会进入 `consumePaid` 或 `consumeFree`
- 没有 reward source 聚合、统一消耗顺序、reward consume 实现

结论：商业化规则文档必须继续以“免费 + 付费”为当前正式能力。

### 3. 外部账号与生产环境配置仍未完成实证

根据现有 README / checklist，以下仍属于真实 blocker：

- Google OAuth 正式 `client_id` / `client_secret` / `redirect_uri`
- Stripe live `secret_key` / `webhook_secret` / success/cancel URL
- 生产环境最终 secrets：`ADMIN_PASSWORD`、`SESSION_SECRET`、`APP_SESSION_SECRET`、`API_KEY_HASH_SALT`
- 外部告警平台
- GitHub release 相关 variables / secrets

对应文档：

- `platform/README.md`
- `docs/release-checklist.md`

结论：这些项未完成前，只能视为“代码准备中”，不能视为“生产已就绪”。

## P1：建议在正式发布前补齐的验证缺口

### 4. 真机 / 公网 E2E 仍需人工回填证据

现有仓库已经有：

- `docs/usage-limits-e2e.md`
- `docs/platform-production-deploy.md`
- `docs/release-checklist.md`

但仍需要真实环境回填：

- CLI 真机调用 `/api/license/check` / `/api/license/consume`
- Google 登录进入 `/app`
- 管理台 Google allowlist 登录
- Stripe checkout + webhook
- Nginx 静态资源更新与公网验证

结论：上线前必须把真实 request-id、截图或执行日志回填到验收记录。

### 5. Analytics / GA4 / UTM / invite 归因仍未完全闭环

当前仓库已经具备：

- site/app 的 GA4 初始化逻辑
- login / pricing / download / checkout / invite carry 的最小事件上报

当前仍缺：

- attribution 落库或后台可视化
- 与真实生产 measurement ID 的联调证据
- 前后台统一 analytics 验收记录

结论：最小可用事件上报已落地，但若本轮目标包含“完整增长归因闭环”，这一项仍是 blocker。

## 当前建议的发布口径

如果必须基于当前 HEAD 对外说明能力边界，建议统一使用下面的口径：

- 已上线：CLI 免费额度、API key 付费额度、基础用户中心、后台治理面板、基础支付与部署文档
- 未上线：真实 Discord 奖励校验闭环、完整增长归因闭环、外部告警自动化
- 待真实环境验证：Google OAuth、Stripe live、生产发布、域名 / TLS / 回调链路
