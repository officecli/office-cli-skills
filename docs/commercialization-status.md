# OfficeCLI 商业化现状与规则收口

本文档按当前仓库 `HEAD` 盘点 `officecli` / `platform/` 商业化链路已经落地的部分、仍未闭环的部分，以及对外文档应该坚持的口径。目标是避免 README、平台文档、控制台文案各说各话。

## 1. 当前已经端到端落地的能力

### 1.1 CLI 免费 / 付费额度闭环

当前真正打通的额度来源只有两类：

1. `free`：未配置 `license.api_key` 时，CLI 按 `fingerprint_hash` 走免费额度。
2. `paid`：配置了有效 `license.api_key` 时，平台按 API key 次数包校验和扣减。

已落地的行为边界：

- `check` 在进入 LLM 生成前执行，失败时直接拦截 CLI。
- `consume` 只会在文档成功生成后触发，不会在生成失败或发布失败时扣减。
- CLI 成功生成后会把真实 `access_mode` 与剩余额度写回 warning / JSON 输出。
- 平台 `POST /api/license/check` 与 `POST /api/license/consume` 已具备 request-id、幂等和基础限流保护。

对应代码证据：

- `internal/cli/app.go`
- `internal/cli/executor.go`
- `platform/internal/license/service.go`
- `platform/internal/app/application.go`

### 1.2 平台控制台与官网基本商业化页面

当前仓库已经具备以下前端入口：

- 官网：Pricing / FAQ / Download / Docs
- 用户中心：Overview / API Keys / Billing / Usage / Downloads
- 管理台：Dashboard / API Keys / Free Quotas / Usage Events / Orders / Billing Events / Users

这些页面已经能承接免费额度、付费次数包、调用记录、后台治理等基础信息，但它们展示的是当前 API 真正返回的数据，不应被文案包装成“奖励体系已经上线”。

### 1.3 生产发布与人工验收资料

当前 HEAD 已有可复用的上线前资料：

- `docs/release-checklist.md`
- `docs/platform-production-deploy.md`
- `docs/usage-limits-test-cases.md`
- `docs/usage-limits-test-report.md`
- `docs/usage-limits-e2e.md`

## 2. 当前文档必须统一遵守的商业化规则

### 2.1 对外可宣称的规则

仅以下规则可以在 README、平台文案或人工交付时按“已上线”表述：

- 匿名用户默认拥有按机器指纹统计的免费额度。
- 付费用户通过 API key 消耗次数包额度。
- 成功生成后才扣减额度。
- CLI / 用户中心 / 管理台都能看到当前免费或付费额度信息。

### 2.2 只能表述为“预留 / 规划中”的规则

以下能力虽然在 schema、类型或文案里已有预留，但当前不能按“已上线”对外描述：

- `reward` 奖励额度来源
- 邀请码 / 邀请注册链接闭环
- 邀请注册后首次成功生成发奖
- Discord OAuth 绑定与 guild 校验发奖
- GA4 / UTM / invite 参数归因闭环

原因很明确：当前仓库只有模型、迁移或 CLI 输出字段预留，缺少端到端服务实现与自动化证据。

对应证据：

- 已预留：`platform/migrations/004_growth_rewards.sql`
- 已预留：`platform/internal/model/models.go`
- 已预留：`internal/license/types.go`
- 已预留：`internal/cli/types.go`
- 但平台实际 license 判定仍只有 `free` / `paid`：`platform/internal/license/service.go`

### 2.3 当前额度优先级口径

在奖励链路真正实现前，当前有效口径只能是：

1. 有有效 `license.api_key` 时走 `paid`
2. 无有效 `license.api_key` 时走 `free`
3. `reward` 仍是协议预留值，不应对外承诺可用

## 3. 当前 HEAD 明确存在的未闭环项

### 3.1 代码内已经预留、但没有打通的能力

- `reward_grants` / `user_referrals` / `discord_connections` 相关表结构已存在，但未看到对应 service / store / route 闭环。
- CLI 已支持展示 `reward_remaining`，但平台服务并未产出 reward 模式响应。
- 用户中心和管理台尚未出现邀请进度、Discord 绑定状态、奖励账本等页面。

### 3.2 依赖外部配置或人工操作的能力

- Google OAuth 正式凭证与回调地址联调
- Stripe live key / webhook secret / 支付回跳地址联调
- 告警平台接入
- GitHub release secrets / variables 配置

这些内容必须继续在 blockers 文档与 release checklist 中保留为未完成项，直到真实环境验证完成。

## 4. 本轮 lane-4 结论

基于当前 HEAD，可以确认：

- 免费 / 付费额度闭环已经是仓库里的真实能力。
- 奖励 / 邀请 / Discord / 归因仍属于“结构预留 + 文案待收口”阶段。
- README、平台 README、上线 blocker 清单必须明确区分“已落地能力”和“尚未上线能力”，避免误导。 
