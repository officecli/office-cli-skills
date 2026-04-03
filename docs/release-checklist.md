# OfficeCLI 正式上线前检查清单

> 适用范围：`officecli` CLI 本体 + `platform/` 授权/支付/控制台服务
>
> 建议使用方式：按 `P0 -> P1 -> P2` 顺序推进；`P0` 未完成前不要正式对外发布。
>
> 第一波建议优先完成：`生产安全配置` + `Session / Cookie 安全`。这两项不依赖 Google / Stripe 账号，可先落地并立即降低上线风险。
>
> 当前仓库内已完成的“代码级收口”包括：生产环境关键 secret 校验、统一安全 Cookie、请求级 `request_id`、基础限流、结构化事件日志、统一访问日志/慢请求日志、基础 CI test/build。下方勾选项仅标记这些已经在仓库中落地的内容；依赖真实账号、部署环境或人工操作的项目仍保持未完成。

## 当前阶段可立即执行的人工验收（不依赖 Google / Stripe）

> 目标：在第三方账号未就绪前，把本地/预发可验证的高风险项先跑掉。建议按 `A -> B -> C -> D` 顺序执行，并把结果直接回填到本清单。

### A. 启动与配置验收
- [ ] 用 `APP_ENV=development` 启动一次，确认本地 fallback 仍可工作
- [ ] 用 `APP_ENV=production` + 默认 `ADMIN_PASSWORD=admin123` 启动一次，确认服务直接启动失败
- [ ] 用 `APP_ENV=production` + 默认 `SESSION_SECRET` 启动一次，确认服务直接启动失败
- [ ] 用 `APP_ENV=production` + 默认 `APP_SESSION_SECRET` 启动一次，确认服务直接启动失败
- [ ] 用 `APP_ENV=production` + 默认 `API_KEY_HASH_SALT=change-me-salt` 启动一次，确认服务直接启动失败
- [ ] 用一组非占位真实值重新启动一次，确认服务可以正常启动
- [ ] 记录本次实际使用的部署变量来源（本地 `.env` / 密钥平台 / 手工导出）

建议命令：
- `cd platform && go test ./internal/app -run TestLoadConfig`
- `cd platform && APP_ENV=production ADMIN_PASSWORD=admin123 make dev`
- `cd platform && APP_ENV=production ADMIN_PASSWORD=real-pass SESSION_SECRET=0123456789abcdef APP_SESSION_SECRET=abcdef0123456789 API_KEY_HASH_SALT=real-salt make dev`

### B. Session / Cookie 安全验收
- [ ] 打开浏览器 DevTools，验证管理员登录后的 `cop_admin_session` 带 `HttpOnly`
- [ ] 在 `APP_ENV=production` 场景下验证 `cop_admin_session` 带 `Secure`
- [ ] 验证管理员 Cookie 带 `SameSite=Lax`
- [ ] 验证管理员登出后 Cookie 被清空，浏览器不再残留可用 session
- [ ] 验证用户侧登出同样会清理 `cop_app_session`
- [ ] 记录“本地 HTTP 开发”和“生产 HTTPS”两种模式下 Cookie 差异，避免联调时误判

参考文件：
- `platform/internal/app/application.go`
- `platform/internal/app/session_cookie_test.go`

### C. API 与额度链路验收
- [ ] 本地创建一个测试 API key，记录 key prefix 与创建时间
- [ ] 用 CLI 或 `curl` 调用一次 `/api/license/check`，确认返回成功并带 `request_id`
- [ ] 用同一组参数调用一次 `/api/license/consume`，确认返回成功并带 `request_id`
- [ ] 重复使用同一个 `request_id` 验证 consume 幂等行为符合预期
- [ ] 构造免费额度耗尽场景，确认返回冲突错误且响应里有 `request_id`
- [ ] 在后台确认 usage event / free quota / api key 额度变化与预期一致
- [ ] 记录一条真实 `request_id`，确认能在服务日志里检索到对应请求

建议命令：
- `cd platform && go test ./internal/app -run 'TestRegisterLicenseRoutes(Check|Consume)'`
- `curl -X POST http://127.0.0.1:8080/api/license/check -H 'Content-Type: application/json' -d '{"fingerprint_hash":"manual-test","action":"generate"}'`
- `curl -X POST http://127.0.0.1:8080/api/license/consume -H 'Content-Type: application/json' -d '{"fingerprint_hash":"manual-test","request_id":"manual-req-1","usage_type":"generate","access_mode":"free"}'`

### D. 日志与运维可见性验收
- [ ] 请求任意业务接口，确认响应头 `X-Request-Id` 存在
- [ ] 确认成功响应体和错误响应体都包含 `request_id`
- [ ] 制造一次管理员登录失败，确认日志里出现 `admin_login_failed`
- [ ] 制造一次限流命中，确认日志里出现 `rate_limit_exceeded`
- [ ] 检查常规请求日志是否输出 `http_request_completed`
- [ ] 检查超过阈值的请求是否输出 `http_request_slow`
- [ ] 确认 `/healthz` 不会刷访问日志
- [ ] 记录当前日志查看入口（本地 stdout / 容器日志 / 平台日志后台）

参考文件：
- `platform/internal/httpapi/observability.go`
- `platform/internal/httpapi/access_log_test.go`
- `platform/internal/app/observability_test.go`

### 当前阶段建议产出物
- [ ] 一份已填写结果的本清单
- [ ] 一组可复用的启动命令/验收命令
- [ ] 至少 2 条可用于排障示例的 `request_id`
- [ ] 一份“仍阻塞上线的外部依赖项”列表（Google / Stripe / 告警平台 / 域名与证书）
- [ ] 一份已回填的商业化 4-lane blocker 盘点（参考 `docs/commercialization-rollout-status.md`）

## P0：上线前必须完成

### 1. 生产安全配置
- [ ] 为正式环境生成独立的 `ADMIN_PASSWORD`
- [ ] 为正式环境生成独立的 `SESSION_SECRET`
- [ ] 为正式环境生成独立的 `APP_SESSION_SECRET`
- [ ] 为正式环境生成独立的 `API_KEY_HASH_SALT`
- [ ] 确认生产环境不会使用默认配置直接启动
- [ ] 确认 `.env`、部署平台 Secret、CI Secret 中不存在演示口令或占位值
- [ ] 确认本地样例配置不会被误用于正式环境

相关文件：
- `platform/internal/app/config.go`
- `platform/.env.example`
- `platform/configs/config.example.yaml`

### 2. Session / Cookie 安全
- [ ] 校验用户端登录 Cookie 使用 `HttpOnly`
- [ ] 校验用户端登录 Cookie 在 HTTPS 下启用 `Secure`
- [ ] 校验后台管理员 Cookie 在 HTTPS 下启用 `Secure`
- [ ] 明确 Cookie 的 `SameSite` 策略是否符合登录/跳转流程
- [ ] 校验登出后 Cookie 被正确清理
- [ ] 校验 session TTL 符合预期，过期后会强制重新登录

相关文件：
- `platform/internal/app/application.go`
- `platform/internal/admin/cookie.go`
- `platform/internal/auth/cookie.go`

### 3. Google OAuth 正式联调
- [ ] 在 Google 控制台配置正式 `redirect_uri`
- [ ] 校验 `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` 已配置
- [ ] 校验 `GOOGLE_REDIRECT_URL` 与正式域名一致
- [ ] 验证登录成功流程可用
- [ ] 验证用户拒绝授权时的失败提示
- [ ] 验证 OAuth state 校验与回跳地址行为正确
- [ ] 验证登录后 `/app` 能正确获取当前用户信息
- [ ] 验证登出后受保护页面重新进入时会被拦截

相关文件：
- `platform/internal/auth/google.go`
- `platform/internal/auth/service.go`
- `platform/internal/app/application.go`

### 4. Stripe 支付与 Webhook 正式联调
- [ ] 配置正式 `STRIPE_SECRET_KEY`
- [ ] 配置正式 `STRIPE_WEBHOOK_SECRET`
- [ ] 校验 `STRIPE_SUCCESS_URL` / `STRIPE_CANCEL_URL` 正确
- [ ] 验证 checkout 创建成功
- [ ] 验证支付成功后订单状态正确
- [ ] 验证支付成功后额度正确发放
- [ ] 验证支付失败时订单状态正确
- [ ] 验证退款/撤销后的状态处理符合预期
- [ ] 验证重复 webhook 不会重复发放额度
- [ ] 验证 webhook 签名错误时请求被拒绝

相关文件：
- `platform/internal/billing/stripe.go`
- `platform/internal/billing/service.go`
- `platform/internal/app/application.go`

### 5. 数据库迁移与回滚
- [ ] 确认正式库执行 migration 顺序：`001 -> 002 -> 003 -> 004`
- [ ] 在预发环境完整跑一遍 migration
- [ ] 确认 migration 失败时的回滚方式
- [ ] 确认上线前数据库备份已完成
- [ ] 确认 Redis 清理策略与 session/key 缓存策略可接受
- [ ] 明确 `004_growth_rewards.sql` 进入生产前的处理策略：邀请 / 奖励 schema 是否已配套服务逻辑，尤其是 `users.invite_code` 唯一约束是否已被注册流程正确填充
- [ ] 明确 Discord 当前发布策略：本版只发布 connect/status + blocker 提示，还是同时发布真实 guild 校验器

相关目录：
- `platform/migrations`
- `platform/seed`

### 6. 域名、环境变量与静态资源
- [ ] 确认 `SITE_BASE_URL` 指向正式官网域名
- [ ] 确认 `PLATFORM_BASE_URL` 指向正式平台域名
- [ ] 确认 `ADMIN_STATIC_DIR`、`APP_STATIC_DIR`、`SITE_STATIC_DIR` 指向正确构建目录
- [ ] 确认反向代理、TLS、域名跳转策略符合 README 设计
- [ ] 确认官网误入 `/app`、`/admin` 的重定向符合预期

相关文件：
- `platform/README.md`
- `platform/internal/app/application.go`
- `platform/.env.example`

## P1：强烈建议上线前完成

### 7. 自动化质量保障
- [ ] 根目录 `go test ./...` 全量通过
- [ ] `platform` 下 `go test ./...` 全量通过
- [ ] `platform/web/admin` 前端测试通过
- [ ] `platform/web/app` 前端测试通过
- [ ] `platform/web/site` 前端测试通过
- [ ] 根目录 `make build` 通过
- [ ] `platform` 下 `make build && make web-build` 通过
- [x] 已补齐基础 CI 流水线：platform / CLI 的 test + build
- [x] 已补齐 release 流水线骨架（私有源码仓构建 -> public dist / tap / skills 同步）
- [ ] 在 GitHub 仓库中配置 release 相关 variables / secrets
- [ ] PR 合并前自动阻断失败构建

当前 release 相关仓库配置应为：

- 私有源码仓：`officecli/officecli`
- 公共分发仓：`officecli/officecli-dist`
- 公共 Homebrew tap 仓：`officecli/homebrew-officecli`
- 公共 skills 仓：`officecli/officecli-skills`

建议命令：
- `go test ./...`
- `cd platform && go test ./...`
- `cd platform/web/admin && npm test -- --run`
- `cd platform/web/app && npm test -- --run`
- `cd platform/web/site && npm test -- --run`
- `make build`
- `cd platform && make build && make web-build`

### 8. 端到端验收（E2E / Smoke）
- [ ] 官网首页、Pricing、Docs、Download 页面可访问
- [ ] Google 登录后能进入用户中心
- [ ] 用户中心可查看 overview / api keys / usage / billing / downloads
- [ ] 用户中心可查看 reward grants / referrals / Discord status 明细
- [ ] 后台管理员可登录并查看 dashboard
- [ ] 后台管理员可查看 growth（reward grants / referrals / discord connections）
- [ ] 后台可创建/编辑 API key
- [ ] CLI 可调用 `/api/license/check`
- [ ] CLI 成功生成文档后才调用 `/api/license/consume`
- [ ] 免费额度扣减正确
- [ ] 付费额度扣减正确
- [ ] 后台与用户中心能看到正确的 quota/usage/order 数据
- [ ] Discord connect 可创建连接记录，且在未接可信 guild 校验时明确返回 blocker 状态而不是发奖
- [ ] site/app 的最小 GA4 事件在真实 measurement ID 下可见

参考文档：
- `docs/usage-limits-e2e.md`
- `docs/usage-limits-test-cases.md`
- `docs/usage-limits-test-report.md`

### 9. 监控、日志与告警
- [x] 为服务接入结构化日志
- [x] 请求与统一响应已关联 `request_id`
- [x] 已记录管理员登录失败、OAuth 回调失败、Stripe webhook 失败、限流命中等关键失败事件
- [x] 已接入统一访问日志与慢请求日志（默认跳过 `/healthz`）
- [ ] 对登录失败、支付失败、webhook 失败增加告警
- [ ] 对 MySQL/Redis 连接异常增加告警
- [ ] 将 `/healthz` 接入外部监控
- [ ] 明确日志保留周期与排障入口
- [ ] 明确谁负责 7x24 或工作时间告警响应

相关文件：
- `platform/internal/app/application.go`
- `platform/internal/billing/service.go`
- `platform/internal/license/service.go`

### 10. 风控与接口保护
- [ ] 为管理员登录接口增加限流或额外保护
- [ ] 为 license check / consume 接口增加限流策略
- [ ] 为 OAuth 登录接口增加滥用防护
- [ ] 为 Stripe webhook 增加来源校验和重放防护确认
- [ ] 明确免费额度是否需要更强的设备/指纹防刷策略

重点接口：
- `/api/admin/login`
- `/api/auth/google/login`
- `/api/auth/google/callback`
- `/api/license/check`
- `/api/license/consume`
- `/api/stripe/webhook`

### 11. 部署与运维预案
- [ ] 明确生产部署方式（容器 / 主机进程 / 平台托管）
- [ ] 明确上线步骤文档
- [ ] 明确回滚步骤文档
- [ ] 明确数据库备份恢复步骤
- [ ] 明确 Redis 故障处理方式
- [ ] 明确 Stripe webhook 重放处理方式
- [ ] 明确 Google OAuth 配置失效时的排障方式

相关文件：
- `platform/deploy/docker-compose.yml`
- `platform/README.md`

## P2：建议上线后尽快补齐

### 12. 前端性能优化
- [ ] 对 `web/app` 做路由级拆包
- [ ] 对 `web/site` 做路由级拆包
- [ ] 拆分 vendor chunk，降低首屏包体积
- [ ] 检查海外访问场景下首页首屏性能
- [ ] 检查静态资源缓存策略

相关目录：
- `platform/web/app`
- `platform/web/site`
- `platform/web/admin`

### 13. 后台权限模型增强
- [ ] 从单口令后台升级为管理员账号体系
- [ ] 为管理员操作补充更清晰的审计日志
- [ ] 考虑增加 2FA、IP 白名单或 VPN 限制
- [ ] 区分只读与可写后台权限

相关文件：
- `platform/internal/admin/service.go`
- `platform/internal/model/models.go`

### 14. 官网与对外信息完善
- [ ] Pricing 文案、套餐额度、价格与实际支付配置一致
- [ ] FAQ 与 Docs 内容可支撑外部用户自助使用
- [ ] Download 页面提供准确下载说明
- [ ] 支持邮箱、反馈渠道、故障通知方式可用
- [ ] 补充隐私政策、服务条款、退款说明

相关目录：
- `platform/web/site/src`

### 15. 商业与运营准备
- [ ] 明确免费额度策略
- [ ] 明确付费额度包策略
- [ ] 明确客户支持 SLA
- [ ] 明确发版节奏、问题升级流程与值班方式
- [ ] 准备首批用户 onboarding 文档

## 上线前最后一次总验收
- [ ] 预发环境完成一次完整回归
- [ ] 生产环境变量逐项复核
- [ ] 生产数据库备份完成
- [ ] 正式域名、证书、回调地址全部复核
- [ ] 首次支付成功链路实测通过
- [ ] CLI 真机生成一份 `pptx` 实测通过
- [ ] CLI 真机生成一份 `docx` 实测通过
- [ ] CLI 真机生成一份 `xlsx` 实测通过
- [ ] 后台管理、用户中心、官网三端可用
- [ ] 已指定上线负责人、回滚负责人、告警响应负责人

## 建议责任分组
- 产品/运营：官网内容、FAQ、Docs、价格、支持渠道
- 后端：配置安全、支付、OAuth、license、migration、监控
- 前端：官网/用户中心/后台联调、性能优化、异常提示
- 运维：部署、证书、域名、告警、备份、回滚
- 测试：E2E、回归、支付链路、额度链路、异常路径
