# OfficeCLI 使用限制测试用例

本文档整理 `officecli` 当前“使用限制”相关的测试用例设计，覆盖 CLI 与 `officecli-platform` 的授权、免费额度、付费额度、扣减、拦截与状态展示。本文档以当前实现为准，不扩展到尚未实现的企业版、组织版或其他商业化能力。

## 1. 测试范围与限制定义

“使用限制”统一指以下行为边界：

- 免费额度限制：未配置 `license.api_key` 时，按 `fingerprint_hash` 使用免费额度；默认额度来自 `DEFAULT_FREE_LIMIT`
- 付费 key 限制：校验 `api_key` 的有效性、禁用状态、过期状态、剩余额度
- 扣减时机限制：`check` 只判定是否可用，`consume` 只在文档成功生成后扣减 1 次
- 幂等与并发限制：相同 `request_id` 不应重复扣减，并发请求不应超扣
- CLI 使用入口限制：额度耗尽或 key 不可用时，应在进入 LLM 生成前失败
- 状态可见性限制：`auth status`、`auth set-key`、生成结果 warning 中应展示当前模式与剩余额度

## 2. 平台侧授权与额度判断

覆盖目标：`platform/internal/license/service.go`

| 编号 | 模块 | 场景 | 前置条件 | 预期结果 |
| --- | --- | --- | --- | --- |
| UL-PLAT-001 | License Check | 免费模式首次检查 | 新 `fingerprint_hash`，无 `api_key` | 自动创建免费额度，返回 `allowed=true`、`access_mode=free`、`free_remaining=DEFAULT_FREE_LIMIT` |
| UL-PLAT-002 | License Check | 免费额度耗尽 | `free_used == free_limit` | 返回 `allowed=false`、`access_mode=blocked`、`reason_code=free_quota_exhausted` |
| UL-PLAT-003 | License Check | 付费 key 有效 | `api_key` 存在、启用、未过期、剩余额度 > 0 | 返回 `allowed=true`、`access_mode=paid`、带 `plan_name` 与 `paid_quota_remaining` |
| UL-PLAT-004 | License Check | 付费 key 无效 | `api_key` 不存在 | 返回 `reason_code=invalid_api_key` |
| UL-PLAT-005 | License Check | 付费 key 禁用 | key `status=disabled` | 返回 `reason_code=disabled_api_key` |
| UL-PLAT-006 | License Check | 付费 key 过期 | `expires_at < now` | 返回 `reason_code=expired_api_key` |
| UL-PLAT-007 | License Check | 付费额度耗尽 | `quota_total - quota_used <= 0` | 返回 `reason_code=paid_quota_exhausted` |
| UL-PLAT-008 | License Consume | 免费扣减成功 | 免费额度剩余至少 1 次 | `free_used + 1`，`free_remaining - 1`，`remaining` 同步更新 |
| UL-PLAT-009 | License Consume | 付费扣减成功 | 付费额度剩余至少 1 次 | `quota_used + 1`，`paid_quota_remaining - 1`，`remaining` 同步更新 |
| UL-PLAT-010 | License Consume | 相同请求幂等 | 同一 `request_id` 重复调用 `consume` | 仅第一次扣减成功，后续返回缓存结果，不重复扣减 |
| UL-PLAT-011 | License Consume | 免费额度并发安全 | 免费剩余仅 1 次，多并发调用 | 最终仅 1 次成功扣减，不出现超扣 |
| UL-PLAT-012 | Free Quota | 后台调整额度生效 | 已存在 `free_quota`，后台提高 `free_limit` | 后续 `check` 返回的新 `free_remaining` 正确 |

## 3. CLI 侧使用入口限制

覆盖目标：`internal/cli/app.go`、`internal/cli/executor.go`

| 编号 | 模块 | 场景 | 前置条件 | 预期结果 |
| --- | --- | --- | --- | --- |
| UL-CLI-001 | `new` | 免费额度耗尽时拦截 | `checkLicense` 返回 `blocked/free_quota_exhausted` | `new` 在进入 LLM 生成前失败，不初始化 LLM |
| UL-CLI-002 | `auth set-key` | 写入付费 key 成功 | `checkLicense` 返回 `allowed/paid` | 新 key 写入配置文件，并输出当前授权模式 |
| UL-CLI-003 | `auth set-key` | 缺少命令参数时交互输入 | 未提供参数，stdin 可读 | 提示输入 API Key，写入用户输入值 |
| UL-CLI-004 | `auth set-key` | 付费 key 校验失败 | 新 key 校验不通过 | 不覆盖旧配置，返回失败 |
| UL-CLI-005 | `auth status` | 免费模式状态展示 | `checkLicense` 返回 `free` | 输出 `当前授权模式：free` 与 `剩余免费次数` |
| UL-CLI-006 | `auth status` | 付费模式状态展示 | `checkLicense` 返回 `paid` | 输出 `当前授权模式：paid` 与 `剩余付费次数` |
| UL-CLI-007 | `checkLicense` | 付费额度耗尽提示文案 | `reason_code=paid_quota_exhausted` | 返回“次数已耗尽”类错误信息 |
| UL-CLI-008 | `executor` | 生成成功但 consume 同步失败 | 文档已生成，`license.Consume` 返回错误 | 不影响文件产出，warning 提示“额度同步失败” |
| UL-CLI-009 | `executor` | 免费模式生成成功 warning | `AccessMode=free` 且 `consume` 成功 | warning 显示“当前为免费模式，剩余 X 次生成额度” |
| UL-CLI-010 | `executor` | 付费模式生成成功 warning | `AccessMode=paid` 且 `consume` 成功 | warning 显示“当前为付费模式，剩余 X 次生成额度” |
| UL-CLI-011 | `checkLicense` | 未启用 license 校验 | `license.Enabled=false` | 视为可用，返回“当前未启用 license 校验” |
| UL-CLI-012 | `checkLicense` | 平台不可达且未配置付费 key | `check` 返回网络错误，未配置 `api_key` | 透传原始错误，便于用户判断平台不可达 |
| UL-CLI-013 | `checkLicense` | 平台不可达且已配置付费 key | `check` 返回网络错误，已配置 `api_key` | 返回“当前付费模式要求在线校验”类错误 |
| UL-CLI-014 | `executor` | 生成失败时不扣减 | 生成阶段返回错误 | 直接失败，`consume` 不应被调用 |
| UL-CLI-015 | `executor` | 发布失败时不扣减 | 本地文件写入成功，发布阶段返回错误 | 直接失败，`consume` 不应被调用 |

## 4. 建议补齐但当前缺失的测试

以下场景建议纳入后续自动化或验收测试，但当前仓库未发现对应证据：

| 编号 | 模块 | 场景 | 预期结果 |
| --- | --- | --- | --- |
| UL-GAP-001 | Console / Admin | 后台修改 `quota_used / quota_total / free_limit` 后前台展示 | 用户中心与后台展示同步正确 |

## 5. 现有自动化证据索引

当前仓库已确认存在的关键测试证据：

- `internal/cli/app_test.go:880` `TestAppRun_NewStopsBeforeLLMWhenFreeQuotaExhausted`
- `internal/cli/app_test.go:930` `TestAppRun_AuthSetKeyWritesConfig`
- `internal/cli/app_test.go:971` `TestAppRun_AuthSetKeyPromptsWhenArgMissing`
- `internal/cli/app_test.go:1011` `TestAppRun_AuthSetKeyValidationFailureKeepsOldConfig`
- `internal/cli/app_test.go:1048` `TestAppRun_AuthStatusShowsRemainingPaidQuota`
- `internal/cli/app_test.go:1070` `TestCheckLicensePaidQuotaExhaustedShowsPaidMessage`
- `internal/cli/app_test.go:1094` `TestAppRun_AuthStatusShowsRemainingFreeQuota`
- `internal/cli/app_test.go:1095` `TestCheckLicenseOfflineWithoutPaidKeyReturnsOriginalError`
- `internal/cli/app_test.go:1110` `TestCheckLicenseOfflineWithPaidKeyRequiresOnlineValidation`
- `internal/cli/app_test.go:1125` `TestCheckLicenseWhenDisabledReturnsBypassMessage`
- `internal/cli/executor_test.go:162` `TestExecutorKeepsSuccessWhenConsumeFails`
- `internal/cli/executor_test.go:197` `TestExecutorAddsFreeModeRemainingWarningAfterConsume`
- `internal/cli/executor_test.go:233` `TestExecutorAddsPaidModeRemainingWarningAfterConsume`
- `internal/cli/executor_test.go:269` `TestExecutorDoesNotConsumeWhenGenerationFails`
- `internal/cli/executor_test.go:297` `TestExecutorDoesNotConsumeWhenPublishFails`
- `platform/internal/license/service_test.go:166` `TestCheckCreatesQuotaForNewMachine`
- `platform/internal/license/service_test.go:180` `TestCheckBlocksWhenFreeQuotaExhausted`
- `platform/internal/license/service_test.go:196` `TestCheckPaidKeyStatuses`
- `platform/internal/license/service_test.go:235` `TestConsumeFreeIsIdempotentAndConcurrentSafe`
- `platform/internal/license/service_test.go:266` `TestConsumePaidIsIdempotent`
- `platform/internal/license/service_test.go:282` `TestAdjustQuotaAffectsCheckRemaining`
- `platform/internal/license/service_test.go:311` `TestConsumePaidRequiresAPIKey`
- `platform/internal/license/service_test.go:327` `TestConsumeRestoreExistingPaidUsageWithoutAPIKey`
- `platform/internal/license/service_test.go:347` `TestConsumeRestoreExistingFreeUsageReturnsCurrentQuota`
- `platform/internal/app/application_license_routes_test.go:108` `TestRegisterLicenseRoutesCheckReturnsBadRequestOnInvalidBody`
- `platform/internal/app/application_license_routes_test.go:127` `TestRegisterLicenseRoutesConsumeReturnsConflictOnFreeQuotaExhausted`
- `platform/internal/app/application_license_routes_test.go:152` `TestRegisterLicenseRoutesConsumeReturnsBadRequestOnPaidConsumeWithoutAPIKey`
- `platform/web/app/src/pages/OverviewPage.test.tsx` 页面统计额度展示测试
- `platform/web/app/src/pages/ApiKeysPage.test.tsx` key 列表额度字段展示测试

## 6. 使用说明

- 该文档用于研发自测、测试设计评审和上线前验收补充
- 若后续补齐自动化，请优先覆盖“当前缺失的测试”章节
- 若环境恢复可执行 `go test`，应同步更新测试报告中的结论与执行结果
