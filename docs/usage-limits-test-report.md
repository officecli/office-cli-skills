# OfficeCLI 使用限制测试报告

## 1. 报告摘要

- 报告对象：`officecli` 使用限制
- 代码基线：`25b166919666a3b4e576f611731ae6c189edbe01`
- 报告日期：`2026-03-31`
- 测试范围：CLI 与 `officecli-platform` 的授权、免费额度、付费额度、扣减、拦截与状态展示
- 结论口径：静态审查 + 已有自动化覆盖分析 + 本地执行结果

结论：

- 当前仓库已经具备一套围绕使用限制的核心自动化测试，重点覆盖了免费额度耗尽拦截、平台不可达错误路径、license 禁用绕过、付费 key 写入校验、授权状态展示、平台侧 `check/consume` 的核心额度逻辑、幂等与并发安全
- 已在修复后的默认 shell 环境下成功执行根模块与 `platform` 子模块的关键测试
- 当前报告已升级为“已执行结果版”测试报告；修复说明保留在环境章节，便于定位旧会话中的遗留问题

## 2. 使用限制规则清单

### 2.1 免费额度

- 未配置 `license.api_key` 时，按 `fingerprint_hash` 消耗免费额度
- 默认额度来自平台配置 `DEFAULT_FREE_LIMIT`
- 免费额度耗尽后，`check` 应返回阻断结果，CLI 不得继续生成

### 2.2 付费 key 状态

- 付费 key 需要校验是否存在、是否启用、是否过期、是否仍有余额
- 任一校验失败时，CLI 或平台应返回明确错误原因

### 2.3 次数包余额

- 付费模式采用次数包余额模型
- `check` 只负责判断是否可用并返回余额信息
- `consume` 成功后才真正扣减 1 次

### 2.4 扣减时机

- 文档生成前先进行 `check`
- 文档生成成功后才调用 `consume`
- 若文档已生成但额度同步失败，不应影响文件产出，但应输出 warning

### 2.5 幂等并发

- 相同 `request_id` 的重复 `consume` 不应重复扣减
- 同时竞争有限额度时，不应出现超扣

## 3. 用例执行结果

| 用例编号 | 模块 | 场景 | 前置条件 | 预期结果 | 当前状态 | 证据 | 备注 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| UL-PLAT-001 | Platform | 免费模式首次检查 | 新机器、无 `api_key` | 自动创建额度并返回 `free` | 已覆盖 | `platform/internal/license/service_test.go:166` | 对应 `TestCheckCreatesQuotaForNewMachine` |
| UL-PLAT-002 | Platform | 免费额度耗尽 | `free_used == free_limit` | 返回 `blocked/free_quota_exhausted` | 已覆盖 | `platform/internal/license/service_test.go:180` | 对应 `TestCheckBlocksWhenFreeQuotaExhausted` |
| UL-PLAT-003 | Platform | 付费 key 有效 | key 启用、未过期、有余额 | 返回 `paid` 与剩余额度 | 已覆盖 | `platform/internal/license/service_test.go:196` | 同一测试表驱动覆盖多个状态 |
| UL-PLAT-004 | Platform | 付费 key 无效 | key 不存在 | 返回 `invalid_api_key` | 已覆盖 | `platform/internal/license/service_test.go:196` | 表驱动子场景 |
| UL-PLAT-005 | Platform | 付费 key 禁用 | `status=disabled` | 返回 `disabled_api_key` | 已覆盖 | `platform/internal/license/service_test.go:196` | 表驱动子场景 |
| UL-PLAT-006 | Platform | 付费 key 过期 | `expires_at < now` | 返回 `expired_api_key` | 已覆盖 | `platform/internal/license/service_test.go:196` | 表驱动子场景 |
| UL-PLAT-007 | Platform | 付费额度耗尽 | 剩余额度为 0 | 返回 `paid_quota_exhausted` | 已覆盖 | `platform/internal/license/service_test.go:196` | 表驱动子场景 |
| UL-PLAT-008 | Platform | 免费扣减成功 | 免费剩余至少 1 次 | `free_used +1`、`remaining -1` | 已覆盖 | `platform/internal/license/service_test.go:235` | 与幂等场景合并验证 |
| UL-PLAT-009 | Platform | 付费扣减成功 | 付费剩余至少 1 次 | `quota_used +1`、`remaining -1` | 已覆盖 | `platform/internal/license/service_test.go:266` | 对应 `TestConsumePaidIsIdempotent` |
| UL-PLAT-010 | Platform | 相同请求幂等 | 同一 `request_id` 重复调用 | 仅扣减一次 | 已覆盖 | `platform/internal/license/service_test.go:235`, `platform/internal/license/service_test.go:266` | 免费与付费均有证据 |
| UL-PLAT-011 | Platform | 免费额度并发安全 | 免费仅剩 1 次，多并发请求 | 仅 1 次成功扣减 | 已覆盖 | `platform/internal/license/service_test.go:235` | 对应并发子场景 |
| UL-PLAT-012 | Platform | 调整额度生效 | 提高 `free_limit` | 后续 `check` 返回新剩余额度 | 已覆盖 | `platform/internal/license/service_test.go:282` | 对应 `TestAdjustQuotaAffectsCheckRemaining` |
| UL-CLI-001 | CLI | 免费额度耗尽时拦截 | `checkLicense` 返回 blocked | 进入 LLM 前失败 | 已覆盖 | `internal/cli/app_test.go:880` | 对应 `TestAppRun_NewStopsBeforeLLMWhenFreeQuotaExhausted` |
| UL-CLI-002 | CLI | `auth set-key` 成功写入配置 | key 校验通过 | 写入配置并展示授权模式 | 已覆盖 | `internal/cli/app_test.go:930` | 对应 `TestAppRun_AuthSetKeyWritesConfig` |
| UL-CLI-003 | CLI | `auth set-key` 交互输入 | 缺少命令参数 | 进入提示输入 | 已覆盖 | `internal/cli/app_test.go:971` | 对应 `TestAppRun_AuthSetKeyPromptsWhenArgMissing` |
| UL-CLI-004 | CLI | `auth set-key` 校验失败不覆盖旧值 | 新 key 校验失败 | 旧配置保留 | 已覆盖 | `internal/cli/app_test.go:1011` | 对应 `TestAppRun_AuthSetKeyValidationFailureKeepsOldConfig` |
| UL-CLI-005 | CLI | `auth status` 免费模式展示 | 返回 `free` | 展示免费模式与剩余额度 | 已覆盖 | `internal/cli/app_test.go:1094` | 对应 `TestAppRun_AuthStatusShowsRemainingFreeQuota` |
| UL-CLI-006 | CLI | `auth status` 付费模式展示 | 返回 `paid` | 展示付费模式与剩余额度 | 已覆盖 | `internal/cli/app_test.go:1048` | 对应 `TestAppRun_AuthStatusShowsRemainingPaidQuota` |
| UL-CLI-007 | CLI | 付费额度耗尽错误文案 | `reason_code=paid_quota_exhausted` | 返回“次数已耗尽”类提示 | 已覆盖 | `internal/cli/app_test.go:1070` | 对应 `TestCheckLicensePaidQuotaExhaustedShowsPaidMessage` |
| UL-CLI-008 | CLI | 生成成功但额度同步失败 | 文档已生成、`Consume` 失败 | 不影响结果，增加 warning | 已覆盖 | `internal/cli/executor.go:90`, `internal/cli/executor_test.go:112` | 已有测试覆盖 `consume` 路径 |
| UL-CLI-009 | CLI | 免费模式成功后 warning | `AccessMode=free` 且扣减成功 | warning 展示免费剩余次数 | 已覆盖 | `internal/cli/executor.go:97`, `internal/cli/executor_test.go:116` | 通过执行器结果断言覆盖 |
| UL-CLI-010 | CLI | 付费模式成功后 warning | `AccessMode=paid` 且扣减成功 | warning 展示付费剩余次数 | 已覆盖 | `internal/cli/executor.go:97`, `internal/cli/executor_test.go:116` | 通过执行器结果断言覆盖 |
| UL-CLI-011 | CLI | 未启用 license 校验 | `license.Enabled=false` | 视为可用并返回提示 | 已覆盖 | `internal/cli/app_test.go:1125` | 对应 `TestCheckLicenseWhenDisabledReturnsBypassMessage` |
| UL-CLI-012 | CLI | 平台不可达且未配置付费 key | 平台调用失败 | 透传原始错误 | 已覆盖 | `internal/cli/app_test.go:1095` | 对应 `TestCheckLicenseOfflineWithoutPaidKeyReturnsOriginalError` |
| UL-CLI-013 | CLI | 平台不可达且已配置付费 key | 平台调用失败 | 提示“付费模式要求在线校验” | 已覆盖 | `internal/cli/app_test.go:1110` | 对应 `TestCheckLicenseOfflineWithPaidKeyRequiresOnlineValidation` |
| UL-CLI-014 | CLI | 生成失败不扣减 | 生成阶段报错 | 不触发 `consume` | 已覆盖 | `internal/cli/executor_test.go:269` | 对应 `TestExecutorDoesNotConsumeWhenGenerationFails` |
| UL-CLI-015 | CLI | 发布失败不扣减 | 本地文件成功、发布失败 | 不触发 `consume` | 已覆盖 | `internal/cli/executor_test.go:297` | 对应 `TestExecutorDoesNotConsumeWhenPublishFails` |
| UL-API-001 | Platform API | `POST /api/license/check` 非法 JSON | 请求体非法 | 返回 `400` | 已覆盖 | `platform/internal/app/application_license_routes_test.go:108` | 对应路由层测试 |
| UL-API-002 | Platform API | `POST /api/license/consume` 免费额度耗尽 | `consume` 返回 quota exhausted | 返回 `409` | 已覆盖 | `platform/internal/app/application_license_routes_test.go:127` | 对应路由层测试 |
| UL-API-003 | Platform API | `POST /api/license/consume` paid 缺少 `api_key` | paid consume 参数不完整 | 返回 `400` | 已覆盖 | `platform/internal/app/application_license_routes_test.go:152` | 对应路由层测试 |
| UL-WEB-001 | Web App | 概览页展示最新总剩余额度 | 用户中心收到新的 `total_remaining` | 页面显示最新统计值 | 已覆盖 | `platform/web/app/src/pages/OverviewPage.test.tsx` | 页面级测试 |
| UL-WEB-002 | Web App | API Keys 页展示最新额度字段 | 用户中心收到新的 `quota_total/quota_used/quota_remaining` | 页面表格显示最新字段值 | 已覆盖 | `platform/web/app/src/pages/ApiKeysPage.test.tsx` | 页面级测试 |
| UL-GAP-001 | Console / Admin | 后台调额后的跨系统展示同步 | 后台调额后重新进入用户中心 | 后台数据经 API 同步到前台页面 | 未覆盖 | `platform/internal/app/application.go:344` | 仍缺少联调或 E2E 级验证 |

## 4. 当前已覆盖结论

已确认存在的自动化证据如下：

- `TestAppRun_NewStopsBeforeLLMWhenFreeQuotaExhausted`
- `TestAppRun_AuthSetKeyWritesConfig`
- `TestAppRun_AuthSetKeyPromptsWhenArgMissing`
- `TestAppRun_AuthSetKeyValidationFailureKeepsOldConfig`
- `TestAppRun_AuthStatusShowsRemainingPaidQuota`
- `TestAppRun_AuthStatusShowsRemainingFreeQuota`
- `TestCheckLicensePaidQuotaExhaustedShowsPaidMessage`
- `TestCheckCreatesQuotaForNewMachine`
- `TestCheckBlocksWhenFreeQuotaExhausted`
- `TestCheckPaidKeyStatuses`
- `TestConsumeFreeIsIdempotentAndConcurrentSafe`
- `TestConsumePaidIsIdempotent`
- `TestAdjustQuotaAffectsCheckRemaining`
- `TestCheckLicenseOfflineWithoutPaidKeyReturnsOriginalError`
- `TestCheckLicenseOfflineWithPaidKeyRequiresOnlineValidation`
- `TestCheckLicenseWhenDisabledReturnsBypassMessage`
- `TestExecutorKeepsSuccessWhenConsumeFails`
- `TestExecutorAddsFreeModeRemainingWarningAfterConsume`
- `TestExecutorAddsPaidModeRemainingWarningAfterConsume`
- `TestExecutorDoesNotConsumeWhenGenerationFails`
- `TestExecutorDoesNotConsumeWhenPublishFails`
- `TestConsumePaidRequiresAPIKey`
- `TestConsumeRestoreExistingPaidUsageWithoutAPIKey`
- `TestConsumeRestoreExistingFreeUsageReturnsCurrentQuota`
- `TestRegisterLicenseRoutesCheckReturnsBadRequestOnInvalidBody`
- `TestRegisterLicenseRoutesConsumeReturnsConflictOnFreeQuotaExhausted`
- `TestRegisterLicenseRoutesConsumeReturnsBadRequestOnPaidConsumeWithoutAPIKey`
- `OverviewPage.test.tsx`
- `ApiKeysPage.test.tsx`

综合判断：

- 使用限制链路的核心业务规则已有较好单元测试基础
- 平台侧的授权、额度、幂等和并发风险点覆盖较完整
- CLI 侧的授权拦截、配置写入和状态展示已有直接证据
- 仍缺少部分错误路径、HTTP 层和跨页面展示层的补充验证

## 5. 缺口与风险

### 5.1 未覆盖场景

- 后台调额后的跨系统展示同步

### 5.2 环境阻塞

- 旧的 Codex 会话进程可能继承了修复前的 `GOROOT`
- 根模块与 `platform/` 子模块分离，`platform/internal/license` 需在子模块上下文中执行测试

### 5.3 风险等级

- 中：平台不可达时的提示一致性虽然已有测试，但仍建议在 CLI 真机联调中再确认一次
- 低：后台到用户中心页面的跨系统展示链路仍缺少 E2E 级验证，影响运营可见性

### 5.4 建议优先级

1. 补后台调额到用户中心的联调或 E2E 测试
2. 在真实控制台环境补一轮 CLI 联调，确认提示文案与单测一致

## 6. 执行环境与阻塞项

本次实际执行命令：

```bash
env -u GOROOT zsh -lc 'cd <repo-root> && go test ./internal/cli ./internal/license -count=1'
env -u GOROOT zsh -lc 'cd <repo-root>/platform && go test ./internal/license ./internal/app -count=1'
```

结果：

- 根模块测试通过：`ok github.com/officecli/officecli/internal/cli`
- 根模块 `internal/license` 无测试文件：`[no test files]`
- `platform` 子模块测试通过：`ok github.com/officecli/officecli/platform/internal/license`
- `platform/internal/app` 路由层测试通过：`ok github.com/officecli/officecli/platform/internal/app`

修复前默认环境下的关键错误信息：

```text
compile: version "go1.25.5" does not match go tool version "go1.26.1"
```

因此本报告结论为：

- 当前关键使用限制测试已在修复后的默认 shell 环境下执行通过
- 已修复 shell 中的 Go 环境指向，使新的 login shell 默认使用用户本地安装的 Go 工具链
- 对当前 Codex 会话这种已注入旧 `GOROOT` 的进程，验证时仍需临时 `env -u GOROOT`；新开的终端或新 shell 不再需要额外处理

## 7. 附录

- 测试用例清单：`docs/usage-limits-test-cases.md`
- CLI 关键证据：`internal/cli/app_test.go`、`internal/cli/executor_test.go`
- 平台关键证据：`platform/internal/license/service_test.go`
