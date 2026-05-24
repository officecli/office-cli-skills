# Changelog

## 0.2.96 - 2026-05-24

### Removed

- Phase 6 (POINT OF NO RETURN): 永久删除 `credit_reserved` 列与 `reserved_delta` 列。Postgres migration `029_drop_credit_reserved.sql` 一次性对 `user_hosted_credit_accounts` / `fingerprint_credit_accounts` / `api_keys` 三张账户表 `DROP COLUMN credit_reserved`，并对 `user_hosted_credit_ledger` / `fingerprint_credit_ledger` 两张 ledger 表 `DROP COLUMN reserved_delta`。Go model 同步删除 `APIKey.CreditReserved` / `FingerprintCreditAccount.CreditReserved` / `UserHostedCreditAccount.CreditReserved` 字段及三者各自的 `AvailableCredits()` 方法，删除 `UserHostedCreditLedger.ReservedDelta` 与 `FingerprintCreditLedger.ReservedDelta` 字段；删除 `HostedPricingRule.ReservationCredits` 列与字段（0.2.95 已标 deprecated）；删除 `HostedCreditLedgerSourceReserve/SourceSettle/SourceRelease/SourceReservedCutover` 四个 ledger 源常量。`store.ListUserHostedCreditLedger/ListAllUserHostedCreditLedger` 的 source_type 白名单同步去掉 `reserved_cutover`。Cutover 一次性工具 `platform/cmd/cutover-reserved/` 整目录删除。

### Changed

- 所有 `account.AvailableCredits()` / `key.AvailableCredits()` 调用点改为直接访问 `CreditBalance` 字段，覆盖 `appuser` / `license` / `hostedllm` / `redemption` 各 service。`license.CreditAccountSnapshot.Reserved` 字段及外部 API 中 `reserved` 概念全部清除。Admin web UI 的 `credit_reserved` / `reservation_credits` / `reserved_delta` 字段、表格列、表单输入及 mock 数据同步剔除：UsersPage 移除「Key reserved legacy」徽章，QuotaSourcesPage 移除 Reserved/Available 双列只留 Balance，HostedPricingRulesPage 移除 Reservation credits 表单字段和文案，CreditLedgerPage 删除 Reserved 列与文案改写。

### Notes

- 生产数据库已在 server 上完成 pre-Phase-6 备份（`/tmp/officecli-platform-pre-phase6-20260524-145615.sql.gz`）。Cutover 在 0.2.94 灰度后已将所有 `credit_reserved=0`，Phase 6 deploy 后回滚需走 schema 重建+备份恢复路径。

## 0.2.95 - 2026-05-24

### Removed

- Phase 5: 删除遗留 Reserve/Settle/Release 三阶段 credit 代码，charge-only 成为唯一扣费路径。`HOSTED_CHARGE_ONLY_MODE` env / `HostedChargeOnlyMode` 配置 / `ChargeOnlyMode` enum 及分流逻辑全部移除；store 层 `ReserveHostedCreditsByUser/Fingerprint` / `SettleHostedCreditsByUser/Fingerprint` / `ReleaseHostedCreditsByUser/Fingerprint`、API key 三件套 `ReserveCreditsByHash` / `ReleaseReservedCredits` / `SettleReservedCredits`、私有 helper `applyHostedCreditReservation` / `applyFingerprintCreditReservation` 全部删除；service 层 `reserveSubjectCredits` / `releaseSubjectCredits` / `settleSubjectCredits` / `reserveCreditsForModel` / `completeReserveLegacy` / `generateImageReserveLegacy` 及相关 reserve-path 测试一并删除。`model.HostedPricingRule.ReservationCredits` 字段保留并标记 deprecated（Phase 6 drop column 时一并删除）；`HostedCreditLedgerSourceReserve/Settle/Release` 字符串常量保留供历史 ledger 反序列化。生产已在 0.2.93+0.2.94 灰度至 `ChargeOnlyMode=all` ≥24h 并执行 cutover，零行为变更基线。

## 0.2.94 - 2026-05-24

### Fixed

- PPTX `assemble` 阶段在 hosted 模式下进入 `Generating image asset (N/M)` 后，因 `llm.GenerateImage` 是阻塞调用且单张图常态需要几十秒到几分钟，agent-bridge stdout 会出现长达 10 分钟以上的静默，OfficeDex 桌面端 stall 检测（≥5 分钟无 progress）会误把任务标记为卡死。`internal/runtime/progress.go` 新增 `generateImageWithHeartbeat`：每 25 秒发一条 `Still waiting on image provider (asset N/M, elapsed Xs)` 的 `task.progress` 心跳，`GenerateImage` 返回或 ctx 取消时心跳 goroutine 立即结束（无泄漏）。PPTX 的 slide 主图和 visual 子图、独立 IMG 三处 `GenerateImage` 调用点全部接入心跳。PPTX 每张图还补了 `Image asset N/M ready` / `Image asset N/M failed: <reason>` 收尾事件；打包阶段在 `Packaging the PPTX file` 与 `PPTX assembly completed` 之间补一条 `Finalizing PPTX layout and writing output bytes`。同时新增 `SetImageHeartbeatIntervalForTesting` 供测试调短心跳节奏，新增 `TestAgentBridgeForwardsImageHeartbeatProgressUnder60s` 端到端断言任意两条 `task.progress` 间隔 ≤ 60s 且心跳事件确实抵达 JSON-RPC stdout。

### Added

- Platform 新增 OfficeDex 桌面端「用户主动提交问题报告」接收端：`POST /api/issue-reports`（clisession 鉴权，60 req/min/user）与 `POST /api/issue-reports/anonymous`（30 req/min/IP + 100 req/min 全局），数据落入 postgres migration 028 新增的 `issue_reports` 与 `issue_report_request_ids` 两张表。服务端基于 v1 算法（sha256）派生 `client_event_id` 实现幂等重放、RFC3339 时间戳 ±5min 校验并在偏差时退化为 `server_now`；4KB body 上限、`schema_version` 协商（不支持时返回 400 `unsupported_schema`）、长度截断 + 反 script/spam 正则的内容过滤（已规避 ReDoS）；GDPR：`contact_email` 可为 NULL、90 天保留 hook。新增 6 条 slog 指标（received / dropped / anonymous_ratio / clock_skew / truncate 等）和 `httpapi.BearerToken` 助手；rate-limit 中间件扩展了 keyFunc。Reviewer 改动已折叠：token 入 rate-limit key 前先 hash、空 bearer 在调用 resolver 前 401 短路、重放路径在 composite unique index 上以 `OnConflict DoNothing` 仅追加新 request_ids。

## 0.2.93 - 2026-05-24

### Added

- Hosted credits: 新增 charge-only 单阶段扣费路径（Reserve→Settle/Release 三阶段的替代方案），由 `HOSTED_CHARGE_ONLY_MODE` env 控制灰度（默认 `disabled`，可选 `fingerprint` / `user` / `all`）。Store 层新增 9 个对称函数：`ChargeHostedCreditsByUser` / `ChargeHostedCreditsByFingerprint` / `ChargeAPIKeyCredits`、`WriteChargeFailedLedgerFor{User,Fingerprint,APIKey}`、`PrecheckHostedCredits{ByUser,ByFingerprint,APIKey}`。Charge 路径在事务内 `SELECT FOR UPDATE` 做余额准入 + 单行 ledger 写入，幂等键 `charge:{requestID}`，并修复了 settle 行 `usage_event_id` 全为 NULL 的审计断链（每笔 charge 行强制关联 usage_event_id）。新增 `HOSTED_RECONCILE_ENABLED` env 控制上游 200+解码失败场景下是否写 `charge_failed_post_upstream` 兜底 ledger。
- Cutover 一次性工具 `platform/cmd/cutover-reserved`：将三张账户表的 `credit_reserved` 字段清零并写入 `reserved_cutover` 审计 ledger，安全门为 `OFFICECLI_ALLOW_CUTOVER=1`，默认 dry-run、`--commit` 才落库。
- Admin /credit-ledger 页面：新增 ledger 浏览视图，含「显示历史 reserve 记录」开关；用户/管理员 billing list API 新增 `include_zero_delta` 参数，默认 `false` 过滤 `credit_delta=0` 的 reserve/release/settle 噪声行。

### Changed

- 新增的 charge-only 路径在 ADR Trade-off 1 下显式承认 ≤ max(chat, image) credits 的单次透支边界（当前约 6 credits），跨副本严格并发准入交由 DB 行锁实现，**显式不引入** sync.Map 或 Redis；plan 与实现细节见 `.omc/plans/2026-05-24-remove-credit-reserve-v2.md`。本版本所有行为 flag-gated default off，零行为变更基线。

## 0.2.92 - 2026-05-24

### Added

- `officecli whoami` 在 `logged_in` 模式下新增一行 `Email: <user-email>`，紧跟在 `User ID:` 之后、`Session:` 之前，方便 OfficeDex 桌面端「设置 / 账户」页用 `(?i)email:\s*(\S+)` 正则解析展示当前登录邮箱。Platform `/api/cli/session` 响应同步新增 `user_email` 字段（`SessionResponse.UserEmail`），由 `clisession.Service.Session` 通过 `GetUserByID` 取出 `model.User.Email` 后回填；用户未绑定邮箱时该字段为空，CLI 同时省略 `Email:` 行（不输出空值）。`anonymous` / `api_key` 模式输出维持原状。新增覆盖四种场景的单测：logged_in 带 email、logged_in 无 email、anonymous、api_key。

## 0.2.91 - 2026-05-24

### Added

- App `/app/billing` 的 "Recent billing activity" 列表合并展示 Stripe 订单与兑换码兑换记录：BillingPage 新增对 `/api/app/redemption-codes/my` 的 query（queryKey `app-redemption-history`，与 `/app/redeem` 共享缓存），把订单与 redemption 合并为按时间倒序排列的统一活动流。Redemption 行展示兑换码、`Redemption code` 标签、来源（app/cli/tui/desktop）、`+N credits` 与绿色的 `Redeemed` 状态徽标；订单行布局保持不变。空状态文案同步更新，只有订单和兑换记录都为空时才显示。

### Fixed

- BillingPage 测试用例中残留的旧 "Continue to Stripe Checkout" 按钮匹配（实际按钮文案早已改为 "Buy {N} credits"）以及 `getByText(/300 credits/i)` 在 pack 卡和按钮上同时命中导致的多元素错误一并修复，全部 BillingPage 测试现在通过。

## 0.2.90 - 2026-05-24

### Changed

- Redemption code 输入框统一回归"强制大写"：兑换码是大小写不敏感的（后端 `redemption.normalizeCode` 与匹配都走 `strings.ToUpper`），auto-generated code 的字母表 `codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"` 本来就只产大写字符。Admin 在 `RedemptionCodesPage` 新增 code 的输入框之前没有任何大小写限制，会出现一会儿大写、一会儿小写或混写的运营创建记录；App `/app/redeem` 在 0.2.89 误移除了 `onChange.toUpperCase()` 之后允许用户输入小写，造成"用户看到的 code"和"admin 列表里的 code"视觉不一致。现在两端的 Code 输入框都重新加回 `onChange={(e) => form.setFieldValue('code', e.target.value.toUpperCase())}`，保持视觉与存储完全一致。

## 0.2.89 - 2026-05-24

### Fixed

- App `/app/redeem`：Code 输入框之前在 `onChange` 强行 `toUpperCase()`，导致用户无法输入小写字符（即使粘贴小写也立刻被覆盖成大写）。后端 `redemption/service.go` 本来就用 `strings.ToUpper(strings.TrimSpace(code))` 归一化，因此前端不需要再做大小写转换。移除该 `onChange` 让输入框保留用户实际按键。

## 0.2.88 - 2026-05-24

### Changed

- Hosted credits 套餐重定档：`hosted-100` ($1 / 100 credits)、`hosted-500` ($5 / 500 credits)、`hosted-2000` ($19 / 2000 credits)，替代旧的 hosted-100/300/1200 ($1/$29/$99)。`billing.Service.Pricing()` 对 hosted_credits 加 code 白名单，DB 里残留的旧 pack 不再返给前端；migration 022 把残留旧 pack 的 `enabled` 翻为 0 让 admin UI 与目录一致。
- 营销站 `/pricing`：按钮原来 popular badge 与 best-value badge 同位重叠、卡片按钮高度不对齐；现在合并/移位 badge、用 `flex flex-col flex-1` wrapper 让三张卡按钮基线对齐，文案改为动态 `Buy {credit_amount} credits`。点击按钮新走跨域 auth-aware 流程：`fetch ${platformBaseURL}/api/auth/me` → 200 直接 `POST /api/app/checkout` 拿 `checkout_url` 跳转 Stripe；401 跳 `${platformBaseURL}/api/auth/oauth2/login?return_to=/app/billing?pack=…&autostart=1`，登录回来由 BillingPage 现有 autostart 兜底；网络/CORS 异常 fallback 到旧链接 `${platformBillingURL}?pack=…&autostart=1`。`motion.a` 保留 `href` 以兼容右键打开新窗口。
- App `/app/billing`：按钮文案与 site 同步为 `Buy {credit_amount} credits`，"Billing flow" 步骤 2 文案对齐到新叙事；checkout/autostart/reconcile 逻辑保持不变。
- 新增 `siteCORSMiddleware` (`platform/internal/app/cors.go`)，跨子域允许 `officecli.io` / `www.officecli.io` 携 cookie 访问 `/api/auth/me` 和 `/api/app/checkout` 两条路径（其它 `/api/*` 路径不受影响、OPTIONS 不会被短路）。允许 origin 列表可通过 `SITE_CORS_ALLOWED_ORIGINS` 环境变量覆盖；未设置时 production 默认 `https://officecli.io` + `https://www.officecli.io`，非 production 额外允许 `http://localhost:5173/4173`。

### Removed

- `registerOfficeSDKProxy` 不再使用已弃用的 `httputil.ReverseProxy.Director`；改为直接构造 `&httputil.ReverseProxy{Rewrite: …, ModifyResponse: …}`，行为等价。

## 0.2.87 - 2026-05-24

### Fixed

- `/api/app/orders` (user-facing) was returning ONLY `external_generation` legacy orders and silently dropping every `hosted_credits` order — so paid hosted-100 purchases (#17, #18, #19, #21) never appeared in the BillingPage "Recent billing activity" list, only the old External 100 rows from April. The filter at platform/internal/app/application.go:2089 was originally written `if order.PackKind != PackKindExternalGeneration { continue }` back when external_generation was the only product (Apr 14, before hosted_credits launched). When the product line flipped to hosted_credits the condition was never inverted. Filter now correctly skips legacy external_generation and surfaces hosted_credits orders, matching the sidebar split (Billing = hosted, Legacy quota = external).

## 0.2.86 - 2026-05-24

### Changed

- App `BillingPage` now confirms successful checkout returns: when reconcile finalizes a paid hosted-credits order, a green success alert reads `Payment received — order #N paid` with the credit count added to the account, and the page also surfaces the current hosted credit balance prominently in the "Hosted credit balance" card (fetched from `/api/app/overview`, invalidated together with orders on reconcile). Previously a user returning from Stripe saw no top-of-page indication that payment succeeded — only the orders list at the bottom reflected the transition, which felt like nothing happened.

## 0.2.85 - 2026-05-24

### Fixed

- Webhook audit gap: `billing.Service.HandleWebhook` now records an `ignored` billing event when a `checkout.session.completed` delivery lands on an order that was already paid (e.g., reconcile beat the webhook). Previously `FinalizeOrderPayment` rolled back the transaction on `order.Status != pending` and the route returned 200 silently with no row in `billing_events`, so a resent webhook (visible at 200 in Stripe Dashboard) looked like nothing happened from the admin side. Now every accepted webhook event lands an idempotent row tagged `ignored` so the audit trail matches Stripe's delivery log.

## 0.2.84 - 2026-05-24

### Fixed

- Stripe webhook 400s due to API version mismatch: `billing.StripeGateway.ParseWebhook` now calls `webhook.ConstructEventWithOptions(..., IgnoreAPIVersionMismatch: true)`. Stripe Dashboard configures webhook endpoints with the current API version (e.g. `2026-03-25.dahlia`) while the embedded `stripe-go v82` SDK ships with `2025-08-27.basil`, causing every `checkout.session.completed` delivery to fail with `Received event with API version 2026-03-25.dahlia, but stripe-go 82.5.1 expects API version 2025-08-27.basil`. Order finalization still worked because the success-URL reconcile path covered it, but webhooks (and the failure / refund events that rely on them) were dropped. The fields we read off the event payload — id, type, data.id, payment_intent, customer, metadata — are stable across these API versions.

## 0.2.83 - 2026-05-24

### Fixed

- Hosted-credits pricing precedence: `billing.Service.Pricing()` and `pricingPack()` now dedupe by pack code with config-defined defaults winning over `hosted_credit_packs` DB rows. A stale `hosted-100` row at $5 in the DB was shadowing the config $1 SKU added in 0.2.81, so both the marketing pricing API surfaced duplicate cards and `CreateCheckout` snapshotted the wrong $5 amount onto new orders. Config defaults are git-tracked and now authoritative; admin DB rows continue to provide codes that config does not define.
- Stripe checkout return UX: `/api/app/orders/reconcile` now returns `{order, awaiting_confirmation}`. When Stripe's `GetCheckoutSession` reports `payment_status != "paid"` (e.g., async payment method, 3DS still resolving, or the buyer landed before Stripe finalized), the app `BillingPage` renders a "Payment still confirming" info alert instead of silently leaving the order in `pending` without feedback.

### Changed

- Site `Pricing` cards now lead with the dollar headline (e.g. `$1`, `$5`, `$29`) and place credit count + per-credit math as supporting subtitles; per-credit values render via `Intl.NumberFormat` at a stable 2-decimal precision (so 0.0967 → `$0.10 / credit`).
- `Best value` badge ties now break toward the largest pack (Hosted 500/1200 over Hosted 100) and the badge is suppressed on the `POPULAR` middle card to avoid double-badging.
- Removed the redundant `{N} hosted credits` checkmark row from each card — the headline already carries that information.

## 0.2.82 - 2026-05-24

### Changed

- Web paywall funnel: site `Pricing` cards now deep-link to `platform.officecli.io/app/billing?pack=<code>&autostart=1`; on landing the app reads the query and auto-launches Stripe Checkout once (idempotent via `hasAutoStartedRef`). Marketing-site CTA → Stripe is now effectively 2 clicks for signed-in users (was 6 hops with pack-selection loss).
- Pricing labels rewritten: removed the misleading "100 credits = $1 USD" auxiliary line. Each pack now shows real `$/credit` (3 sig figs) and an `≈ N images @ 10 credits each` business-value conversion, computed from the same pack data on both `site/Pricing` and `app/BillingPage`.
- Site `Pricing` renders up to 3 hosted-credits packs (was hardcoded to 2). The "Best value" badge is data-driven (lowest `$/credit`); "POPULAR" stays on the middle card.
- App `Overview` now shows a low-balance warning banner when hosted credits fall below 20, plus a "Buy more credits →" link on the Hosted Credits metric card.
- App `Billing` page adds a "Have a redeem code?" link to `/redeem`.
- Redeem page localized to English (form labels, errors, history table, success toast).
- App sidebar: Quota nav entry demoted to "Legacy quota" with the `Archive` icon, moved below Billing — it remains routable but no longer competes for primary attention.
- Site `/login` route removed (Navbar Login already jumps to `platform.officecli.io/app`); the orphan stub page file is left on disk for future reuse.

### Added

- `siteApi`: new helpers `pricePerCreditLabel`, `imagesPerPack`, `bestValueCode` for consistent pricing math.
- Tests: 3 new autostart cases in `BillingPage.test.tsx` (autostart fires once, idempotent on re-render, ignored when pack unknown); new `Pricing.test.tsx` covering 3-card render, deep-link href shape, and Best value badge selection.

## 0.2.81 - 2026-05-23

### Added

- New `hosted-100` SKU on the hosted pricing tier: 100 credits for $1.00 USD. Lands as the entry-level pack alongside the existing `hosted-300` ($29) and `hosted-1200` ($99) packs and surfaces on `/pricing` as a third card (entry / popular / bulk) when 3+ hosted packs are returned.

### Changed

- Site `Pricing` component now renders a responsive 1/2/3-column grid driven by the number of hosted packs returned by `/api/pricing`. The 2-pack fallback path is preserved for compatibility.

## 0.2.80 - 2026-05-23

### Added

- Redemption code system: admins can mint, edit, enable/disable codes from the admin web (`/redemption-codes`) and audit every claim from `/redemption-records`. Each code carries `credit_amount`, optional `max_redemptions`, optional `expires_at`, and a `per_user_limit` (default 1). A partial UNIQUE index on `(redemption_code_id, user_id) WHERE per_user_limit_at_claim = 1` enforces single-claim codes even if the application-layer `FOR UPDATE` serialization is bypassed.
- Users can redeem codes from three entry points sharing one platform service:
  - CLI: `officecli redeem <code>` (with `--code`, `--json`, `--source`).
  - TUI: new `/redeem <code>` slash command — runs the same code path as the CLI and refreshes credit status inline.
  - Web app: new `/redeem` page (sidebar entry "Redeem") with claim form and personal redemption history.
- New backend endpoints: `POST /api/app/redemption-codes/redeem` (cookie session), `GET /api/app/redemption-codes/my`, `POST /api/cli/redemption-codes/redeem` (Bearer auth), and admin CRUD under `/api/admin/redemption-codes` plus `/api/admin/redemption-codes/redemptions`.
- Hosted credit ledger gains a `redemption_code` source; each successful redeem writes a single ledger entry with an idempotency key tied to the redemption row so retries are safe.
- Stable machine-readable error codes returned to clients (`code_not_found`, `code_disabled`, `code_expired`, `code_exhausted`, `code_already_claimed`, `code_required`) mapped to 404/403/410/410/409/400 respectively.

### Migration

- Postgres `025_grant_existing_users_100_credit_bonus.sql` retroactively grants 100 hosted credits to every existing user (idempotent via `signup-bonus-bump-100:<userID>`), completing the 30→100 signup-bonus bump from 0.2.78.
- Postgres `026_redemption_codes.sql` creates `redemption_codes` and `redemption_code_redemptions`.
- Postgres `027_redemption_code_singleton_unique.sql` adds `per_user_limit_at_claim` snapshot column and the partial UNIQUE index that enforces single-claim codes.

## 0.2.79 - 2026-05-23

### Changed

- Anonymous trial replaced by per-device hosted credits. The legacy lifetime `free_quotas` and daily `daily_free_quotas` tables are dropped; each new device fingerprint now lands a `fingerprint_credit_accounts` row seeded with 100 starter credits the first time it calls license `check`. Anonymous and registered users share the same hosted billing path (10 credits per image, token-priced text, etc.) — the only difference is that registered users can top up.
- `officecli login` exchange now carries `fingerprint_hash`. On successful login the server merges the available (non-reserved) anonymous balance into the user's hosted credit account via a single idempotent transfer (`anonymous_transfer_in`/`anonymous_transfer_out` ledger entries, keyed by `anonymous-transfer:{fp}:{user}`); reserved credits remain on the fingerprint account so in-flight settlements can finish.
- `license check` now returns `quota_snapshot.credit_account` (`owner_kind` / `balance` / `reserved` / `available`) instead of `free_trial` / `free_trial_daily`. `CheckResponse.FreeLimit/FreeUsed/FreeRemaining` and `ConsumeResponse.FreeUsed/FreeRemaining` are gone.
- CLI status, whoami, and TUI footers print the credit-account view: `Anonymous credit balance (this device): X available / Y reserved / Z total` instead of "Free trial quota (this machine, lifetime)". Logout copy is now "back to anonymous credit mode."
- Admin web removes the "Free Trial Devices" page and the `/free-quotas` route; QuotaSources surfaces only reward / paid / hosted credentials. Admin backend removes `ListFreeQuotas` / `UpdateFreeQuota`. Dashboard "Total Machines" stat now counts distinct rows in `fingerprint_credit_accounts`.

### Migration

- Postgres migrations `023_fingerprint_credit_accounts.sql` (create) and `024_drop_free_quotas.sql` (drop legacy `daily_free_quotas` / `free_quotas`). Historical anonymous trial counts are not backfilled — they are deprecated.

## 0.2.78 - 2026-05-23

### Changed

- New-user signup hosted credit bonus increased from 30 to 100 credits. Existing users are unaffected (the grant is idempotent per `signup-hosted-credits:<userID>`); the overview UI and web fallback both reflect the new amount.
- TUI footer no longer shows the anonymous "Trial: N generations" counter while running in External Mode once the device's free quota is exhausted or the user is signed in — Trial state only makes sense for fresh hosted-trial sessions.
- Image edit (`new img --reference-image`) now accepts OpenAI responses that return a `url` field in addition to `b64_json`, and surfaces the response body in error messages when decoding fails.

## 0.2.77 - 2026-05-23

### Fixed

- `officecli upgrade` no longer auto-runs `npm install -g officecli` without consent. In an interactive shell it asks before applying; in a non-interactive context it prints the suggested command and exits. Pass `--apply` (or `-y`) to keep the previous one-shot behavior.
- Generation commands no longer fail with "missing account login" when the binary has a valid CLI session but the locally installed `~/.codex/skills/officecli/env-common.sh` is an older copy that doesn't recognize the binary's `Mode: logged in` output. The preflight now double-checks the binary's own config and overrides the stale shell verdict when an active session or API key is present.
- `officecli new ...` rejects the combination of `--prompt` and `--prompt-file` with a clear error instead of silently ignoring the file.
- The local dev-build license-proof skip warning is now emitted at most once per process instead of repeating on every access check.

## 0.2.76 - 2026-05-22

### Added

- TUI interactive mode: `/mode [hosted|external]` command to display the current runtime mode or switch between hosted and external without restarting the session.

### Changed

- Hosted AI image generation now charges a flat 10 credits per image (~$0.10), replacing the previous tiered formula. Default hosted pricing rule (`gpt-image-2 / image_default`) and the runtime billing path are updated in lockstep; minimum charge stays at 10 credits.
- Billing page surfaces the new per-image usage rate so customers can estimate cost before generation.
- Marketing site: officedex hero CTA copy updated to "Coming soon" for consistency with other coming-soon surfaces.

### Fixed

- Admin dashboard `joinList` helper now handles `null`/`undefined` arrays without throwing, preventing dashboard render crashes when an upstream field is missing.

## 0.2.75 - 2026-05-22

### Changed

- npm Repository link now points at the URL declared in `packages/npm/officecli/package.json` (currently `github.com/officecli/officecli`). The sync script no longer overwrites `repository`/`bugs` with the internal `officecli-npm` mirror.

## 0.2.74 - 2026-05-22

### Added

- Windows (amd64/arm64) binary release targets. The npm wrapper now installs the matching `officecli.exe` on `win32` x64/arm64 hosts.

### Changed

- npm package metadata: add `repository` and `bugs` fields and expand social links in the README.
- README "Supported Platforms" section now lists Windows x64 and arm64.

## 0.2.73 - 2026-05-21

### Fixed

- Fix hosted-mode PPTX standard-quality images sending wrong model name (`hosted/text` instead of `hosted/image`) to the platform, causing image generation to fail and credits not being tracked.

### Changed

- Default generation mode changed from `fast` to `best` for all document types except `img`. Best mode asks clarifying questions before generating, producing higher-quality output. Use `--mode fast` to skip questions.
- Admin dashboard: add fingerprint quality CSV export and enhanced store operations.

## 0.2.72 - 2026-05-21

### Fixed

- Fix preflight misidentifying session-logged-in users as anonymous, causing `officecli new img` and other generation commands to fail with a misleading "account login or API key required" error.
- Handle whoami network failures gracefully by distinguishing network errors from genuinely invalid sessions, preventing transient connectivity issues from blocking authenticated users.
- Make skill bundle refresh non-fatal when the officecli binary is already installed, so transient GitHub connectivity issues no longer block generation.
- Allow session-authenticated users to pass the license check for image generation without a paid API key (hosted credits are sufficient).
- Add `OFFICECLI_SKIP_SKILL_PREFLIGHT=1` hint to network-related preflight error messages.

## 0.2.71 - 2026-05-21

### Fixed

- Partial fix for preflight auth detection (superseded by 0.2.72).

## 0.2.70 - 2026-05-21

### Changed

- Anonymous users running image generation are now guided to `officecli login` instead of being prompted for a raw API key.
- Environment check and fix scripts detect authentication mode via `officecli whoami` and surface login recommendations when anonymous.
- Skill documentation adds an Authentication section covering login, whoami, doctor, and set-key commands.
- Preflight error messages include actionable login guidance for the `account_login` missing item.

## 0.2.69 - 2026-05-20

### Fixed

- Check for updates before launching the interactive TUI from empty input or a natural-language prompt while keeping explicit command entrypoints unchanged.

## 0.2.68 - 2026-05-20

### Added

- Added operational event tracking and an admin operations funnel view for acquisition, activation, usage, and revenue health.

### Fixed

- Retry transient internal platform transport failures up to three times so standalone image generation can recover from temporary EOF or connection reset errors.

## 0.2.67 - 2026-05-15

### Fixed

- Treat online preview publishing failures as warnings so document generation still succeeds and reports the local file path.
- Reuse active CLI login sessions for platform preview publishing when generation and publish target the same OfficeCLI platform endpoint.

## 0.2.66 - 2026-05-15

### Changed

- Added `/login` to the interactive TUI so users can complete browser-based account login without leaving the session.
- Removed `/clear` from the interactive TUI command set and report it as an unknown command.
- Wrapped TUI help, status, and footer text to prevent long help lines from being truncated.

## 0.2.65 - 2026-05-15

### Changed

- Enabled online preview publishing by default for new installs with no existing config file.
- Updated OpenClaw OfficeCLI skill templates so newly generated skill config also defaults to publishing previews.

## 0.2.64 - 2026-05-15

### Fixed

- Fixed hosted document generation requests so account-credit billing always receives a request id before reserving credits.
- Fixed the TUI prompt router so Chinese requests such as `画一个图，关于长江` use standalone image generation instead of PPTX generation.

## 0.2.63 - 2026-05-15

### Changed

- Changed `officecli login` success output to show the account email when the platform returns it.

## 0.2.62 - 2026-05-15

### Added

- Added browser-based `officecli login`, `officecli logout`, and `officecli whoami` for account hosted credits.
- Added account-level hosted credit accounts, ledgers, CLI sessions, and migrations for MySQL and Postgres.

### Changed

- Changed Hosted Mode billing so CLI sessions and API keys consume the same account hosted credits.
- Changed Billing, Overview, API Keys, Invite, Discord, Docs, and Download copy to the account hosted credits model.
- Changed invite activation and Discord verification rewards to grant 100 account hosted credits each.

## 0.2.61 - 2026-05-14

### Changed

- Removed the previously added non-interactive namespace and kept `officecli new ...` as the only generation command path.
- External OpenAI-compatible generation now retries `/v1/chat/completions` when a root base URL returns an HTML app shell, matching New API-style gateways configured without `/v1`.

## 0.2.60 - 2026-05-14

### Changed

- Added the Bubble Tea based Codex-style `officecli` TUI for continuous natural-language document generation, including `--no-alt-screen` for scrollback-friendly sessions.
- Added `officecli exec ...` as the recommended non-interactive command namespace while keeping `officecli new ...` compatible.
- Added a local MIT-licensed `go-localereader` replacement for darwin/linux TUI builds so dependency scans do not rely on an upstream module without a standalone LICENSE file.

## 0.2.59 - 2026-05-14

### Changed

- Lowered the default PPT quality evaluation pass threshold to 60 for installed CLI E2E runs.

## 0.2.58 - 2026-05-14

### Changed

- Simplified `officecli --help` and `officecli new --help` around hosted-first copy-paste examples for first-time users.
- Added post-install next steps to the npm wrapper and shell installer.
- Reworked README, npm README, download page, and docs quickstart so hosted trial generation is the default first-run path.

## 0.2.57 - 2026-05-14

### Changed

- Made the official website URL visible in the npm package README link text.

## 0.2.56 - 2026-05-14

### Changed

- Added the official website link to the top of the npm package README.

## 0.2.55 - 2026-05-14

### Changed

- The npm-installed CLI now defaults to hosted anonymous trial access, so first-run generation works without a local model endpoint or hosted API key.
- Anonymous hosted trial quota now uses the lifetime machine fingerprint quota and reports `quota_snapshot.free_trial` with `scope=lifetime`.
- Hosted text, JSON, and structured requests accept valid anonymous commit tokens, while final quota is consumed only after a successful artifact is written.

## 0.2.54 - 2026-05-12

### Changed

- External Mode is now free and unlimited for document and standalone image generation, while Hosted Mode continues to use hosted credits.
- Billing and pricing now sell hosted credit packs only, with historical external orders preserved for reconciliation.
- The marketing site, app, admin, docs, and quickstart copy now present External and Hosted as the two primary runtime modes.

### Added

- New users receive 30 hosted credits, and each activated referral grants the inviter 20 hosted credits with idempotent grant tracking.

## 0.2.53 - 2026-05-12

### Changed

- Hosted pricing profiles are now limited to `text` and `image`: document text generation uses `hosted/text`, while standalone images and PPT image assets use `hosted/image`.

## 0.2.52 - 2026-05-11

### Fixed

- `officecli config set-license` now syncs the platform publish credential when publishing uses the default OfficeCLI platform endpoint, preventing stale preview-publish keys after rotating a platform API key.

## 0.2.35 - 2026-05-07

### Added

- Added `officecli new img --reference-image <path-or-url>` for a single local or remote reference image.
- Added `agent-bridge` `office.generate` support for `reference_image` and capability metadata under standalone image generation.
- Added platform hosted image support for OpenAI image edits when a parsed `reference_image` payload is present.
- Added default online publishing for standalone `new img` outputs, including protected platform image preview links.

## 0.1.0 - 2026-03-31

First usable CLI release, focused on turning the repository from a reusable library into a tool that end users can run directly.

### Added

- Added the `officecli new <pptx|docx|xlsx> <topic> [brief]` command entrypoint
- Added `--prompt`, `--prompt-file`, `--mode`, `--lang`, `--style`, `--audience`, `--out`, `--publish`, `--no-publish`, and `--json`
- Added default human-readable output and structured `--json` output
- Added `--help`, `--version`, and build-time version injection
- Added `internal/providers/llm` with OpenAI-compatible and internal HTTP providers
- Added `internal/providers/publish` to publish generated files and return URLs/passwords
- Added sample configuration and prompt files under `examples/`
- Added a `Makefile` covering `build/test/install/run-help/demo/release`
- Added `scripts/demo.sh` for a full local CLI demo flow

### Changed

- Rewrote the README as a human-user-facing usage guide
- Added a runtime wiring layer on top of the engine libraries so `pptx/docx/xlsx` can all run through one unified CLI

### Notes

- Current release targets output `darwin` and `linux` `amd64/arm64` binaries into `dist/`
- The default version string is `dev`; inject a real version with `make build VERSION=...` or `make release VERSION=...`
