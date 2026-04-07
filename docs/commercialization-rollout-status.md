# OfficeCLI 商业化 4-lane 当前状态（HEAD 盘点）

> 盘点日期：2026-04-02
>
> 目标：基于当前仓库 HEAD 判断 4-lane 商业化闭环哪些部分已经落地、哪些仍未达到生产就绪，并把当前 blocker 说清楚，避免误判为“已全面上线”。

## 一句话结论

- lane1 已经从“免费/付费双通道”推进到“reward + free + paid”后端判定与 CLI 展示，但仍缺奖励账本审计视图和真实 E2E 联调证据。
- lane2 已经具备 invite_code 生成、邀请注册、首次成功激活奖励、Discord 绑定 service，以及 app 用户侧最小 connect/status 入口，但仍缺真实 Discord OAuth / guild 校验入口。
- lane3 已经把 reward/referral/Discord 明细接到 app overview 与 admin growth UI，并补了最小 GA4 事件上报，但还没有完整归因落库/分析体系。
- lane4 的测试与文档骨架继续完善中，但外部配置、跨服务 E2E、生产凭据验收仍是发布 blocker。

## Lane 状态总览

| Lane | 当前状态 | 已有证据 | 主要 blocker |
| --- | --- | --- | --- |
| lane1：奖励账本 / 三类额度来源 / CLI 真实额度展示 | 部分完成 | `platform/internal/reward/service.go`、`platform/internal/license/service.go` 已打通 reward balance/consume；`internal/cli/app.go`、`internal/cli/executor.go` 已展示 `reward_remaining`；`platform/internal/license/service_test.go`、`internal/cli/*_test.go` 已覆盖主要路径 | 仍缺 reward ledger 明细 API / 页面；当前返回值只有剩余额度，没有具体 grant 选择轨迹；缺真实 DB + CLI 的 E2E 证据 |
| lane2：邀请码 / 邀请激活奖励 / Discord 绑定与发奖 | 部分完成 | `platform/internal/store/sqlstore/store.go` 已生成 `invite_code`；`platform/internal/auth/service.go` 已注册 referral；`platform/internal/growth/service.go` 已实现 referral / Discord 奖励逻辑；`platform/internal/app/application.go` 已新增 `/api/app/discord/connect` 与 `/api/app/discord/status`；后端测试已覆盖 connect/幂等/发奖约束 | 没有真实 Discord OAuth callback / guild membership 校验；当前只允许 connect，不允许依赖前端参数直接发奖；anti-abuse 仍仅停留在幂等与唯一键 |
| lane3：app/admin/site 权益页、邀请进度、Discord 状态、奖励可见性、归因 | 部分完成 | `/api/app/growth` 与 `/api/admin/growth` 已返回明细；`platform/web/app/src/pages/OverviewPage.tsx` 已展示 reward grants / referrals / discord status；`platform/web/admin/src/pages/GrowthPage.tsx` 已展示 growth 运营视图；site/app 已补最小 GA4 初始化与关键事件上报 | 没有完整 attribution 落库/报表；Discord 仍无可信 guild 校验；跨服务 E2E 仍缺 |
| lane4：测试补齐、README/平台文档、生产 blocker 清单 | 部分完成 | `docs/monetization-lane-verification.md`、`docs/release-checklist.md`、`docs/platform-production-deploy.md`、`platform/README.md` 已记录当前实现与 blocker；Go package tests 已覆盖 reward/growth/auth/app wiring | 缺跨服务 E2E；缺 Discord / analytics 配置；Google OAuth / Stripe / Discord / analytics 生产联调仍需人工验收 |

## 按 lane 的详细盘点

### lane1：reward 已接入后端判定与 consume，但审计面仍不完整

当前 HEAD 已明确落地：

- `platform/internal/reward/service.go`
  - 已实现奖励授予、余额汇总、额度消耗。
- `platform/internal/license/service.go`
  - 当未传 API key 且用户存在 reward balance 时，`Check` 会优先走 reward。
  - `Consume` 已支持 `reward` 模式，并保留幂等恢复逻辑。
- `internal/cli/app.go` / `internal/cli/executor.go`
  - 已对 `reward` 模式输出剩余额度文案。

当前 blocker：

- `license/check` / `consume` 仍只返回汇总剩余额度，没有具体 reward grant 账本明细。
- 还没有运营视角的 reward ledger 页面或导出能力。
- 仍缺真实数据库场景下的 CLI ↔ platform reward E2E 证明。

### lane2：邀请与 Discord 奖励已到 service 层，但外部闭环未完成

当前 HEAD 已明确落地：

- `platform/internal/store/sqlstore/store.go`
  - 新/老 Google 用户都会被补齐确定性的 `invite_code`。
- `platform/internal/auth/service.go`
  - OAuth state 会保留 `invite_code`，callback 后会注册 referral。
- `platform/internal/growth/service.go`
  - 已实现 referral 注册、首次成功激活奖励、Discord 绑定、Discord 入群奖励的幂等逻辑。

当前 blocker：

- 还没有真实 Discord OAuth callback、guild membership 校验客户端或同步任务。
- app 用户侧已有 connect/status 路由，但当前是明确 blocker 语义：未接可信 guild 校验前不会发 Discord 奖励。
- 还没有速率限制、风控审查、邀请策略等 anti-abuse 机制。

### lane3：汇总态已进入 app/admin，但运营明细面仍缺

当前 HEAD 已明确落地：

- `platform/internal/appuser/service.go`
  - `/api/app/overview` 已聚合 reward、invite、referral、Discord 状态。
- `platform/web/app/src/pages/OverviewPage.tsx`
  - 已展示 Reward Credits、Referral Progress、Discord Status。
- `platform/web/admin/src/pages/UsersPage.tsx`
  - 已展示 `invite_code`。
- `platform/web/admin/src/pages/UsageEventsPage.tsx`
  - 已允许筛选 `reward` usage mode。

当前 blocker：

- 已有最小奖励账本页、邀请进度页、Discord 状态页与运营视图。
- 已有 per-grant ledger API 与 referral timeline 的最小返回口径。
- 已有最小 GA4 / UTM / invite 事件链路，但还没有 attribution 落库、报表与生产联调证据。

### lane4：验证资产比之前更完整，但生产闭环仍缺外部条件

当前 HEAD 已明确落地：

- `platform/internal/{reward,growth,license,auth,appuser,app}` 已具备针对商业化新增能力的 Go 测试。
- `internal/cli/*_test.go` 已覆盖 reward 文案输出。
- `docs/monetization-lane-verification.md`、`docs/release-checklist.md`、`platform/README.md` 已同步当前状态与 blocker。

当前 blocker：

- 仍缺跨服务 E2E（真实 DB / OAuth / webhook / session / CLI）的完整验证。
- `platform/configs/config.example.yaml` 仍缺 Discord OAuth / guild / analytics / attribution 相关配置。
- Google OAuth、Stripe、Discord、analytics 的生产联调和 secrets 验收仍需人工完成。

## 生产发布前 blocker 清单（当前 HEAD 视角）

以下项目在 2026-04-02 这次盘点时仍应视为 blocker：

1. 没有 reward grant 明细 API / 页面，reward consume 缺审计可见性
2. 没有真实 Discord OAuth / guild membership 校验闭环
3. 没有 reward / referral / Discord 的跨服务 E2E 资产
4. 没有完整 GA4 / UTM / invite attribution 落库与分析实现
6. Google OAuth / Stripe webhook / Discord / analytics 生产凭据验收仍待完成

## 对外口径建议

当前对外文档与验收说明建议使用下面的口径，避免过度承诺：

- 可以说：`officecli` 已具备免费、奖励、付费三类额度的后端判定基础，以及 app/admin 的基础可见性。
- 不要说：真实 Discord 奖励联调、完整 GA4/UTM 归因已经正式上线。
- 如果描述 lane2/lane3，请明确写成：`service 层与部分 UI/汇总态已落地，但外部联调、运营入口与生产验收尚未完成。`

## 建议下一步

- 先补最小 reward ledger / referral timeline / Discord status API，再补对应 app/admin 页面。
- 引入真实 Discord OAuth / guild membership 校验入口，再谈 Discord 奖励上线。
- 以 `docs/release-checklist.md` 为主清单，继续回填外部联调与 E2E 证据。
