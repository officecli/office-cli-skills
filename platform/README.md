# OfficeCLI Platform

`officecli-platform` 是 OfficeCLI 的独立授权、支付与平台控制台服务。它只位于 `platform/` 子项目中，独立编译、独立前端构建、独立依赖管理，不修改旁边的 `officecli`。

## 技术栈与目录结构

- 后端：Go + ego + GORM + MySQL + Redis
- 前端：React + TypeScript + Vite + Ant Design + TanStack Query
- 本地依赖：Docker Compose 启动 MySQL/Redis
- 静态托管：Go 服务托管 `web/site/dist`、`web/app/dist`、`web/admin/dist`

主要目录：

- `cmd/platform/main.go`：服务入口
- `internal/app`：配置、路由、应用装配
- `internal/license`：license check / consume 业务
- `internal/admin`：2b 后台登录、overview、users、orders、billing-events、api-key、free-quota、usage-events
- `internal/auth`：Google 登录、用户 session
- `internal/appuser`：2c 用户中心 API
- `internal/billing`：Stripe checkout / webhook / 订单
- `internal/model`：实体模型
- `internal/store/mysql`：GORM repository
- `internal/store/redis`：会话、幂等、锁
- `migrations/`：建表 SQL
- `seed/`：演示数据
- `web/site/`：官网 landing、pricing、download、faq、docs、login
- `web/app/`：2c 用户中心
- `web/admin/`：React 管理台源码
- `web/*/dist/`：各前端构建产物
- `deploy/docker-compose.yml`：本地 MySQL/Redis

## 域名与路由分配

- `officecli.io`
  - `/`、`/pricing`、`/download`、`/faq`、`/docs`、`/login`
  - 只承载公开内容与转化入口
- `platform.officecli.io`
  - `/app`、`/app/*`：2c 用户中心
  - `/admin`、`/admin/*`：2b 内部后台
  - `/api/license/*`、`/api/auth/*`、`/api/app/*`、`/api/admin/*`
  - `/api/stripe/webhook`、`/healthz`
- 平台域名根路径会重定向到 `/app`
- 官网域名误入 `/app` 或 `/admin` 时会重定向到 `platform.officecli.io`

## 本地启动方式

1. 启动依赖：

```bash
docker compose -f deploy/docker-compose.yml up -d
```

2. 配置环境变量：

```bash
cp .env.example .env
export $(grep -v '^#' .env | xargs)
```

3. 执行 migration：

```bash
mysql -uroot -proot -h127.0.0.1 cli_office_platform < migrations/001_init.sql
mysql -uroot -proot -h127.0.0.1 cli_office_platform < migrations/002_paid_quota.sql
mysql -uroot -proot -h127.0.0.1 cli_office_platform < migrations/003_auth_billing.sql
mysql -uroot -proot -h127.0.0.1 cli_office_platform < migrations/004_growth_rewards.sql
```

> `platform` 启动时也会自动按文件名顺序执行 `migrations/*.sql`。上面的手工命令主要用于首次建库或排障时显式校验库结构。

4. 安装前端依赖并构建：

```bash
make web-install
make web-build
```

5. 启动服务：

```bash
make dev
```

本地访问路径：

- 官网：`http://127.0.0.1:8080/`
- 2c：`http://127.0.0.1:8080/app`
- 2b：`http://127.0.0.1:8080/admin`

生产环境建议：

- `SITE_BASE_URL=https://officecli.io`
- `PLATFORM_BASE_URL=https://platform.officecli.io`

## 配置项

- `APP_ENV`：运行环境，支持 `development` / `staging` / `production`，默认 `development`
- `HTTP_ADDR`：HTTP 监听地址
- `MYSQL_DSN`：MySQL 连接串
- `REDIS_ADDR`：Redis 地址
- `ADMIN_LOGIN_RATE_LIMIT_PER_MINUTE`：管理员登录接口每分钟限流阈值
- `LICENSE_RATE_LIMIT_PER_MINUTE`：license check / consume 每分钟限流阈值
- `RATE_LIMIT_VISITOR_TTL`：限流访客记录的内存保留时长
- `DEFAULT_FREE_LIMIT`：CLI 匿名免费额度上限，当前产品定义为“每台机器每天 10 次成功生成”
- `ADMIN_PASSWORD`：后台口令
- `SESSION_SECRET`：后台 session 签名密钥
- `API_KEY_HASH_SALT`：api-key hash salt
- `USAGE_IDEMPOTENCY_TTL`：免费 consume 幂等缓存 TTL
- `ADMIN_SESSION_TTL`：后台会话 TTL
- `SITE_BASE_URL`：官网域名基址
- `PLATFORM_BASE_URL`：平台域名基址
- `APP_SESSION_SECRET`：2c 用户 session 签名密钥
- `APP_SESSION_TTL`：2c 用户会话 TTL
- `GOOGLE_REDIRECT_URL`：Google OAuth 回调地址，默认指向 `https://platform.officecli.io/api/auth/google/callback`
- `STRIPE_SUCCESS_URL`：Stripe 成功回跳地址，默认指向 `https://platform.officecli.io/app/billing?status=success`
- `STRIPE_CANCEL_URL`：Stripe 取消回跳地址，默认指向 `https://platform.officecli.io/app/billing?status=cancel`
- `HOSTED_LLM_BASE_URL`：hosted LLM 上游 API 地址
- `HOSTED_LLM_API_KEY`：hosted LLM 上游 API key
- `HOSTED_LLM_TEXT_MODEL`：hosted 文本模型
- `HOSTED_LLM_IMAGE_MODEL`：hosted 图片模型
- `HOSTED_LLM_PROVIDER`：hosted 上游 provider 标识
- `HOSTED_PRICING_RULES_JSON`：hosted credits 计费规则 JSON
- `ADMIN_STATIC_DIR`：React 构建产物目录
- `APP_STATIC_DIR`：2c 构建产物目录
- `SITE_STATIC_DIR`：官网构建产物目录

修改默认免费额度只需要调整 `DEFAULT_FREE_LIMIT`。

管理员登录当前采用 Google OAuth + allowlist；默认示例配置已收口为仅允许 `luyang950@gmail.com` 进入后台。

### 生产环境配置说明

- 本地开发默认使用 `APP_ENV=development`，允许回退到示例里的默认值，便于快速启动
- 当 `APP_ENV=production` 时，平台会强制校验以下配置，一旦缺失或仍为示例值会直接启动失败：
  - `ADMIN_PASSWORD`
  - `SESSION_SECRET`
  - `APP_SESSION_SECRET`
  - `API_KEY_HASH_SALT`
- 这批配置属于上线前必须完成项，因为它们直接影响后台口令、session 签名和 API key 哈希安全
- `GOOGLE_*` 与 `STRIPE_*` 当前阶段仍可暂缓；在账号未就绪前不会因为它们缺失而阻塞服务启动，但正式联调前仍需补齐
- 生产环境下，用户端和管理员端 session cookie 都会统一启用 `Secure`、`HttpOnly` 和 `SameSite=Lax`
- 限流默认值会按环境区分：
  - `development` / `staging`：管理员登录 `60/min`，license 接口 `300/min`，访客 TTL `1m`
  - `production`：管理员登录 `5/min`，license 接口 `30/min`，访客 TTL `5m`
- 如需调优，可以通过 `ADMIN_LOGIN_RATE_LIMIT_PER_MINUTE`、`LICENSE_RATE_LIMIT_PER_MINUTE`、`RATE_LIMIT_VISITOR_TTL` 覆盖默认值

### 可观测性现状

- 服务默认以 JSON 结构化日志输出到 stdout
- 所有通过统一响应 helper 返回的成功/失败响应都会包含 `request_id`
- 中间件会复用来路 `X-Request-Id`，没有则自动生成，并回写到响应头 `X-Request-Id`
- 已记录的重点事件包括：
  - `admin_login_failed`
  - `auth_callback_failed`
  - `stripe_webhook_read_failed`
  - `stripe_webhook_failed`
  - `rate_limit_exceeded`
- 已接入统一访问日志：
  - 常规请求记录为 `http_request_completed`
  - 超过 1s 的慢请求记录为 `http_request_slow`
  - 日志字段包含 `request_id`、`method`、`path`、`status`、`client_ip`、`latency_ms`
- `/healthz` 默认不写访问日志，避免外部健康检查把业务日志刷满
- 当前仍未接入外部告警平台；登录失败、支付失败、数据库/Redis 异常告警仍是上线前待办

paid 模式当前采用“次数包余额”模型：`check` 只校验是否可用，真正扣减在生成成功后由统一的 `POST /api/license/consume` 完成。

### Hosted pricing 配置

- hosted LLM 的上游地址、模型与 credits 计费规则当前都由服务端配置驱动。
- `HOSTED_PRICING_RULES_JSON` 是当前 hosted pricing 的主配置入口；未提供时会回退到内置默认规则。
- 管理台 `/admin/hosted-pricing` 当前是只读页，只用于展示服务当前加载到的规则。
- 如果需要调整 hosted pricing，请更新部署环境变量或配置文件中的 `HOSTED_PRICING_RULES_JSON`，然后重启 `officecli-platform`。

## 初始化管理员与测试数据

- 管理员：当前采用单口令方案，直接设置 `ADMIN_PASSWORD`
- 演示免费额度数据：

```bash
mysql -uroot -proot -h127.0.0.1 cli_office_platform < seed/demo.sql
```

## 创建测试 api-key

登录后台 `http://127.0.0.1:8080/admin` 后，在 API Keys 页面点击创建即可。创建成功后会返回一次完整明文 key；之后后台只显示 prefix。用户中心只允许用户管理 `status` / `note` 与查看额度；`quota_total` / `quota_used` / `expires_at` 仍由后台运营面控制。

如果要从 2c 侧验证完整链路，可以在 `http://127.0.0.1:8080/app` 登录后自行创建属于当前用户的 key。

## CLI 契约接口示例

### `POST /api/license/check`

```bash
curl -X POST https://platform.officecli.io/api/license/check \
  -H 'Content-Type: application/json' \
  -d '{
    "fingerprint_hash":"machine-1",
    "action":"generate"
  }'
```

### `POST /api/license/consume`

```bash
curl -X POST https://platform.officecli.io/api/license/consume \
  -H 'Content-Type: application/json' \
  -d '{
    "fingerprint_hash":"machine-1",
    "request_id":"req-1",
    "usage_type":"generate",
    "access_mode":"paid",
    "api_key":"cop_live_xxx"
  }'
```

## 商业化闭环当前状态（2026-04-02 HEAD）

- 当前可确认已落地能力：免费额度、付费次数包、CLI check/consume 闭环、reward/referral/Discord 明细 API、app/admin 的 growth 页面与最小 GA4 事件上报。
- 当前不能宣称已上线：真实 Discord OAuth callback、真实 guild 校验、基于可信 Discord 校验后的自动发奖闭环、完整 GA4 / UTM / invite attribution。
- `reward_grants` / `user_referrals` / `discord_connections` 已不再只是 migration + model；当前可经 `/api/app/growth`、`/api/admin/growth` 与对应前端页面消费。
- Discord 当前策略已明确隔离：`/api/app/discord/connect` 与 `/api/app/discord/status` 可真实调用，但未接可信 guild 校验器前只会返回 `verification_blocked`，不会假装已完成发奖上线。
- 详细盘点、对外口径和生产 blocker 清单见：`../docs/commercialization-rollout-status.md`

## 页面职责说明

- `officecli.io`
  - Landing / Pricing / Download / FAQ / Docs / Login redirect
- `platform.officecli.io/app`
  - 用户概览、我的 API key、订单、购买次数包、使用记录、下载入口
- `platform.officecli.io/admin`
  - 内部后台：用户、订单、支付事件、API key、免费额度、调用记录
  - 当前默认仅允许 `luyang950@gmail.com` 通过 Google 登录进入
  - 已包含最小 growth 运营页，可查看 reward grants / referrals / discord connections

## 管理台功能说明

- Dashboard：总 key 数、状态分布、免费机器数、最近 24h 检查/消耗/阻断次数
- Growth：reward grants、referrals、discord connections 最小运营视图
- API Keys：创建 key、启用/禁用、更新套餐名/过期时间/备注、查看和调整 paid 次数包总量/已用/剩余
- Free Quotas：按 fingerprint 搜索、调整 `free_limit`
- Usage Events：按 mode/result/reason_code/fingerprint/api key/时间范围过滤
- Users / Orders / Billing Events：查看用户状态、订单状态和 Stripe webhook 处理结果
- 真实 Discord guild 校验 / 发奖闭环：当前仍是 blocker，未接可信校验器前不可按“已上线”宣称

## 测试命令

```bash
make test
make web-test
make web-build
make build
```

## CI 现状

- 仓库已新增 GitHub Actions：
  - `.github/workflows/platform-ci.yml`
  - `.github/workflows/cli-ci.yml`
- `platform-ci` 会执行：
  - `go test ./...`
  - `make build`
  - `platform/web/admin` / `platform/web/app` / `platform/web/site` 的前端 test + build
- `cli-ci` 会执行根目录 CLI 的 `go test ./...` 与 `make build`
- 当前仓库内已经具备 test/build 级别的 CI，但 release 流水线、外部告警联动、分支保护规则仍需在正式上线前补齐

## 当前阶段怎么验收

- 如果 Google / Stripe 账号暂时还没下来，建议先执行 `docs/release-checklist.md` 里的“当前阶段可立即执行的人工验收（不依赖 Google / Stripe）”
- 这部分已经把配置安全、Cookie、安全响应、license 链路、日志可见性拆成可直接照着跑的步骤，适合当前阶段先把最容易线上翻车的部分压住
