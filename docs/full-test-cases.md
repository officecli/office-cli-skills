# OfficeCLI 全功能测试用例

本文档整理 `officecli` 当前面向交付与回归的全功能测试用例，统一覆盖以下 3 个测试块：

- 命令行工具（`officecli`）
- `platform-app`（用户中心）
- `platform-admin`（管理后台）

本文档目标是提供一份可直接执行的测试矩阵，供研发自测、QA 回归、联调验收与上线前检查共用。

> 说明：
>
> - 本文档不把 `platform/web/site` 官网营销站拆成独立测试块。
> - `platform` 底层 API 不单独拆第四部分，而是放进 CLI / app / admin 的测试场景里体现。
> - 授权 / 免费额度 / 付费额度专项设计与联调步骤，仍以 `docs/usage-limits-test-cases.md`、`docs/usage-limits-e2e.md` 为专项参考；本总表只做整合，不重复维护两套细节。

## 1. 测试范围与用例字段说明

### 1.1 测试范围

- **命令行工具**：配置初始化、参数解析、文档生成、授权校验、结果输出、发布预览、PPT 自动配图、交互模式
- **platform-app**：登录态、用户概览、API key 管理、Billing、Usage、Downloads、奖励/邀请/Discord 相关展示
- **platform-admin**：管理员登录、Dashboard、Users、Orders、Billing Events、API Keys、Free Quotas、Usage Events、可观测性与风控

### 1.2 优先级定义

- `P0`：主链路可用性与核心业务闭环，阻塞发布
- `P1`：关键异常流、状态同步、管理动作结果、日志与链路可见性
- `P2`：边界、兼容性、空态/文案/弱相关场景

### 1.3 自动化状态定义

- `已自动化`：仓库已有明确自动化测试证据
- `建议自动化`：适合补单测/集成测试，但当前未见直接证据
- `人工 E2E`：依赖真实服务、浏览器或跨系统联调

---

## 2. 命令行工具测试矩阵

### 2.1 `config` 配置管理

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CLI-CONFIG-001 | config | `config set-generation` 首次写入生成服务配置成功 | 本地不存在配置文件 | 执行 `officecli config set-generation` 并输入生成服务地址/访问凭证 | 成功写入配置文件；生成服务配置可被 `new` 使用 | P0 | 已自动化（`internal/cli/app_test.go` `TestAppRun_ConfigSetGenerationWritesConfig`） |
| CLI-CONFIG-002 | config | `config status` 展示当前配置状态 | 已存在配置文件 | 执行 `officecli config status` | 返回生成服务、额度校验、发布状态与默认值摘要 | P0 | 已自动化（`TestAppRun_ConfigStatusShowsProductState`） |
| CLI-CONFIG-003 | config | `config set-license` 使用固定平台地址 | 无需用户填写额度服务地址 | 执行 `officecli config set-license` | 配置文件写入固定平台地址；用户只配置额度开关与付费额度密钥 | P0 | 已自动化（`TestAppRun_ConfigSetLicenseUsesFixedPlatformURL`） |
| CLI-CONFIG-004 | config | `config set-publish` 可单独更新发布配置 | 已存在或不存在配置文件 | 执行 `officecli config set-publish` | 只更新在线预览发布配置，不影响其他配置域 | P1 | 已自动化（`TestAppRun_ConfigSetPublishWritesConfig`） |
| CLI-CONFIG-005 | config | `config set-defaults` 单独更新默认值 | 已存在或不存在配置文件 | 执行 `officecli config set-defaults` | 默认输出目录、默认模式、默认发布行为按新值落盘 | P1 | 已自动化（`TestAppRun_ConfigSetDefaultsWritesConfig`） |
| CLI-CONFIG-006 | config | 缺失生成服务配置时给出 `config` 指引 | 缺少生成服务配置 | 执行 `officecli new ...` | 明确指出缺失项，并引导运行 `officecli config set-generation` | P0 | 已自动化（`TestAppRun_MissingGenerationConfigShowsConfigGuidance`） |


### 2.2 `new` 文档生成主流程与参数组合

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CLI-NEW-001 | new | `pptx` 主链路生成成功 | 生成服务、额度与输出配置有效 | 执行 `officecli new pptx "主题" "简介" --no-publish` | 成功生成本地 `.pptx` 文件；输出文件路径 | P0 | 已自动化（`internal/runtime/service_test.go`、`internal/cli/executor_test.go`） |
| CLI-NEW-002 | new | `docx` 主链路生成成功 | 同上 | 执行 `officecli new docx ... --no-publish` | 成功生成 `.docx` 文件 | P0 | 已自动化（`internal/runtime/service_test.go` `TestServiceGenerateDOCXWithFakeClient`） |
| CLI-NEW-003 | new | `xlsx` 主链路生成成功 | 同上 | 执行 `officecli new xlsx ... --no-publish` | 成功生成 `.xlsx` 文件 | P0 | 建议自动化 |
| CLI-NEW-004 | new | `--prompt` 优先级最高 | 同时提供 positional brief、stdin、`--prompt-file`、`--prompt` | 执行命令并观察实际送入生成器的 prompt | 使用 `--prompt` 内容 | P0 | 已自动化（`TestBuildGenerateJob_PromptPrecedence`） |
| CLI-NEW-005 | new | `--prompt-file` 优先于 stdin 与 positional brief | 提供 prompt-file、stdin、brief | 执行命令 | 使用文件内容作为 prompt | P0 | 已自动化（`TestBuildGenerateJob_UsesPromptFileBeforeStdinAndPositionals`） |
| CLI-NEW-006 | new | 未提供 prompt 时回退到 brief | 提供 topic + brief，无 stdin / `--prompt*` | 执行命令 | 使用 brief 作为 prompt | P1 | 已自动化（同 `BuildGenerateJob` 覆盖） |
| CLI-NEW-007 | new | 未提供 brief 时回退到 topic | 仅提供 topic | 执行命令 | 使用 topic 作为 prompt | P1 | 已自动化（同 `BuildGenerateJob` 覆盖） |
| CLI-NEW-008 | new | `--mode fast` 直接生成 | 配置和依赖有效 | 执行 `officecli new pptx ... --mode fast` | 不进入补问流程，直接生成 | P0 | 已自动化（`engine/plan/workflow_test.go` `TestPrepareExecutionPlan_FastSkipsQuestions`） |
| CLI-NEW-009 | new | `--mode best` 在 TTY 中进入补问 | 运行环境为 TTY | 执行 `officecli new pptx ... --mode best` | 进入补问流程，用户可回答问题后继续生成 | P1 | 建议自动化 / 人工 E2E |
| CLI-NEW-010 | new | `best` 模式在非 TTY 下失败 | 非 TTY 环境 | 执行 `officecli new pptx ... --mode best` | 返回“best 模式需要交互补问，请在 TTY 中运行或改用 --mode fast” | P0 | 建议自动化 |
| CLI-NEW-011 | new | `--json` 输出机器可读结果 | 生成成功 | 执行 `officecli new xlsx ... --json` | stdout 输出 JSON；包含文件路径、发布字段；不输出 spinner/progress 文案 | P0 | 已自动化（`TestAppRun_NewJSONSkipsProgressOutput`、`TestRenderResult_JSONIncludesPublishFields`） |
| CLI-NEW-012 | new | `--out` 指定输出目录 | 指定可写目录 | 执行 `officecli new ... --out ./dist` | 文档落在指定目录；目录不存在时自动创建 | P0 | 建议自动化 |
| CLI-NEW-013 | new | `--publish` 覆盖配置强制发布 | `defaults.publish=false`，publish provider 已配置 | 执行 `officecli new ... --publish` | 仍执行发布流程并返回访问地址/密码 | P1 | 已自动化（`TestBuildGenerateJob_PublishFlagsOverrideConfig` + `TestExecutorGenerateAndPublish`） |
| CLI-NEW-014 | new | `--no-publish` 覆盖配置关闭发布 | `defaults.publish=true` | 执行 `officecli new ... --no-publish` | 只写本地文件，不调用 publisher | P0 | 已自动化（`TestBuildGenerateJob_PublishFlagsOverrideConfig`） |
| CLI-NEW-015 | new | publisher 未配置但开启 publish | `defaults.publish=true`，publisher 为 nil | 执行 `officecli new ...` | 生成成功；warning 提示“未配置发布端，跳过在线预览” | P1 | 建议自动化 |
| CLI-NEW-016 | new | topic 缺失时直接失败 | 参数不完整 | 执行 `officecli new pptx` | 返回 `topic is required` | P0 | 建议自动化 |
| CLI-NEW-017 | new | 文档类型非法时失败 | 参数中 document type 非 `pptx/docx/xlsx` | 执行 `officecli new pdf ...` | 返回 `unsupported document type` | P0 | 建议自动化 |
| CLI-NEW-018 | new | mode 非法时失败 | `--mode ultra` | 执行命令 | 返回 `unsupported mode` | P1 | 建议自动化 |
| CLI-NEW-019 | new | 非法 flag 时失败 | 输入不存在的 flag | 执行命令 | flag 解析失败，返回明确错误 | P2 | 建议自动化 |
| CLI-NEW-020 | new | 输出目录不可写时失败 | `--out` 指向无权限目录 | 执行命令 | 写文件阶段失败；不返回伪成功结果 | P1 | 建议自动化 |

### 2.3 PPT 特性、图片与发布行为

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CLI-PPT-001 | pptx | 默认开启自动配图 | 生成 `pptx` 且未传 `--no-images` | 执行 `officecli new pptx ...` | 生成提示允许输出图片字段；适合配图的页会嵌图 | P0 | 已自动化（`TestBuildGenerateJob_ImagesEnabledByDefault`、`TestBuildPPTXPrompt_ImagesEnabledIncludesImageGuidance`、`TestServiceGeneratePPTX_GeneratesImagesWhenEnabled`） |
| CLI-PPT-002 | pptx | `--no-images` 关闭自动配图 | 同上 | 执行 `officecli new pptx ... --no-images` | prompt 禁止图片字段；最终 PPT 不嵌图 | P0 | 已自动化（`TestBuildGenerateJob_NoImagesDisablesImageGeneration`、`TestBuildPPTXPrompt_ImagesDisabledForbidsImageFields`、`TestServiceGeneratePPTX_SkipsImagesWhenDisabled`） |
| CLI-PPT-003 | pptx | 图片生成失败时降级成功 | 图像接口故障，但文本生成正常 | 执行 `officecli new pptx ...` | 仍返回成功文件；附带 warning；不因配图失败整单失败 | P0 | 已自动化（`TestServiceGeneratePPTX_DegradesGracefullyWhenImageGenerationFails`） |
| CLI-PPT-004 | pptx | 图片素材嵌入到最终文件 | 生成包含图片页的 PPT | 检查生成的 PPTX 压缩包内容或本地打开预览 | 图片以 OOXML 资源形式写入，并在页面可见 | P1 | 已自动化（`pkg/officegen/pptx_generator_test.go` 多个嵌图测试） |
| CLI-PPT-005 | pptx | 图表页与 dashboard 页不强制配图 | PPT 中包含 chart / dashboard layout | 生成文档并检查图片策略 | chart / dashboard 不乱插图，布局符合规则 | P1 | 已自动化（`BuildPPTXPrompt` 规则 + generator 测试） |
| CLI-PPT-006 | pptx | 输出文件名回退到 `topic.pptx` | 生成器未返回文档名 | 执行生成 | 本地文件名回退为 `<topic>.pptx` | P2 | 建议自动化 |
| CLI-PPT-007 | publish | 发布成功返回访问地址和密码 | publish provider 正常 | 执行默认发布命令 | 最终结果包含 `published=true`、`access_url`、`password` | P0 | 已自动化（`TestExecutorGenerateAndPublish`） |
| CLI-PPT-008 | publish | 发布失败导致命令失败且不扣减额度 | 文件本地写入成功，但 publisher 返回错误 | 执行命令 | 返回发布失败；本地文件仍存在；`consume` 不调用 | P0 | 已自动化（`TestExecutorDoesNotConsumeWhenPublishFails`） |

### 2.4 授权、额度与结果 warning

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CLI-AUTH-001 | auth | `auth status` 展示免费模式与剩余次数 | `checkLicense` 返回 free 模式 | 执行 `officecli auth status` | 输出“当前授权状态：free”与剩余免费次数 | P0 | 已自动化（`TestAppRun_AuthStatusShowsRemainingFreeQuota`） |
| CLI-AUTH-002 | auth | `auth status` 展示付费模式与剩余额度 | `checkLicense` 返回 paid 模式 | 执行 `officecli auth status` | 输出“当前授权状态：paid”与剩余付费次数 | P0 | 已自动化（`TestAppRun_AuthStatusShowsRemainingPaidQuota`） |
| CLI-AUTH-003 | auth | `auth status` 展示奖励模式 | `checkLicense` 返回 reward 模式 | 执行 `officecli auth status` | 输出 reward 模式与剩余奖励额度 | P1 | 已自动化（`TestAppRun_AuthStatusShowsRemainingRewardQuota`） |
| CLI-AUTH-004 | auth | `auth set-key` 直接传参写入成功 | 新 key 校验通过 | 执行 `officecli auth set-key <api-key>` | 配置文件更新为新 key；输出当前模式 | P0 | 已自动化（`TestAppRun_AuthSetKeyWritesConfig`） |
| CLI-AUTH-005 | auth | `auth set-key` 缺参时走交互输入 | stdin 可读 | 执行 `officecli auth set-key` 并输入 key | 使用输入值校验并落盘 | P1 | 已自动化（`TestAppRun_AuthSetKeyPromptsWhenArgMissing`） |
| CLI-AUTH-006 | auth | 新 key 校验失败时保留旧值 | 配置文件已有旧 key，新 key 无效 | 执行 `officecli auth set-key bad-key` | 返回失败；旧 key 不被覆盖 | P0 | 已自动化（`TestAppRun_AuthSetKeyValidationFailureKeepsOldConfig`） |
| CLI-AUTH-007 | license | 免费额度耗尽时在生成前拦截 | `checkLicense` 返回 `blocked/free_quota_exhausted` | 执行 `officecli new ...` | 在进入实际生成阶段前直接失败 | P0 | 已自动化（`TestAppRun_NewStopsBeforeGenerationWhenFreeQuotaExhausted`） |
| CLI-AUTH-008 | license | 付费额度耗尽提示付费文案 | `reason_code=paid_quota_exhausted` | 执行 `officecli auth status` 或 `new` | 返回“次数已耗尽”类提示 | P0 | 已自动化（`TestCheckLicensePaidQuotaExhaustedShowsPaidMessage`） |
| CLI-AUTH-009 | license | 未启用 license 校验时旁路放行 | `license.enabled=false` | 执行 `officecli auth status` / `new` | 返回“当前未启用 license 校验”或直接放行 | P1 | 已自动化（`TestCheckLicenseWhenDisabledReturnsBypassMessage`） |
| CLI-AUTH-010 | license | 平台离线且未配置付费 key | license 平台不可达，无 `api_key` | 执行 `officecli auth status` | 透传原始网络错误，便于判断平台不可达 | P1 | 已自动化（`TestCheckLicenseOfflineWithoutPaidKeyReturnsOriginalError`） |
| CLI-AUTH-011 | license | 平台离线且已配置付费 key | license 平台不可达，已写入 `api_key` | 执行 `officecli auth status` | 提示“当前付费模式要求在线校验” | P1 | 已自动化（`TestCheckLicenseOfflineWithPaidKeyRequiresOnlineValidation`） |
| CLI-AUTH-012 | consume | 生成成功后才扣减额度 | `checkLicense` 返回允许；生成成功 | 执行 `officecli new ...` | `consume` 在发布完成后调用 1 次；结果返回 remaining | P0 | 已自动化（`TestExecutorConsumesUsageAfterSuccess`） |
| CLI-AUTH-013 | consume | 生成成功但 consume 失败时仍返回成功 | 文件已生成，consume 返回错误 | 执行命令 | 命令仍成功；保留本地文件；warning 提示稍后执行 `officecli auth status` 检查 | P0 | 已自动化（`TestExecutorKeepsSuccessWhenConsumeFails`） |
| CLI-AUTH-014 | consume | 免费模式成功后追加剩余 warning | free 模式 consume 成功 | 执行命令 | 结果 warning 包含“当前为免费模式，剩余 X 次生成额度” | P1 | 已自动化（`TestExecutorAddsFreeModeRemainingWarningAfterConsume`） |
| CLI-AUTH-015 | consume | 付费模式成功后追加剩余 warning | paid 模式 consume 成功 | 执行命令 | warning 展示剩余付费额度 | P1 | 已自动化（`TestExecutorAddsPaidModeRemainingWarningAfterConsume`） |
| CLI-AUTH-016 | consume | 奖励模式成功后追加剩余 warning | reward 模式 consume 成功 | 执行命令 | warning 展示剩余奖励额度 | P1 | 已自动化（`TestExecutorAddsRewardModeRemainingWarningAfterConsume`） |
| CLI-AUTH-017 | consume | 生成失败时不扣减 | 生成阶段或组装阶段失败 | 执行命令 | 命令失败；不调用 `consume` | P0 | 已自动化（`TestExecutorDoesNotConsumeWhenGenerationFails`） |

### 2.5 帮助信息、版本与交互输出

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CLI-UX-001 | help | 根命令帮助输出完整 | 无 | 执行 `officecli --help` | 展示 `config` / `auth` / `new`、常用选项、默认行为、示例 | P1 | 已自动化（`TestAppRun_HelpOutput`、`TestAppRun_HelpIncludesConfigCommand`） |
| CLI-UX-002 | help | 子命令帮助输出完整 | 无 | 执行 `officecli new --help`、`officecli auth --help`、`officecli config --help` | 对应帮助文案准确 | P1 | 已自动化（`TestAppRun_SubcommandHelpOutput`） |
| CLI-UX-003 | version | 版本信息输出 | 无 | 执行 `officecli --version` | 输出 `version/commit/buildDate` | P2 | 已自动化（`TestAppRun_VersionOutput`） |
| CLI-UX-004 | progress | TTY 下显示 spinner 与阶段进度 | 运行环境为 TTY | 执行生成命令 | 有 spinner 动画、阶段更新与收尾状态 | P1 | 已自动化（`TestAppRun_NewTTYShowsSpinnerFrames`、`TestRenderProgress_TTYAnimatesAndFinalizesStage`） |
| CLI-UX-005 | progress | 非 TTY 下按阶段打印日志 | 非 TTY 环境 | 执行生成命令 | 输出分阶段文本，不使用 spinner | P1 | 已自动化（`TestRenderProgress_NonTTYPrintsStageLines`） |
| CLI-UX-006 | progress | 等待补问时暂停 spinner 并展示 waiting 行 | `best` 模式且进入问题阶段 | 执行生成命令 | 明确显示等待回答，不出现进度混乱 | P2 | 已自动化（`TestRenderProgress_PauseStopsSpinnerAndPrintsWaitingLine`） |
| CLI-UX-007 | result | 结果输出先显示进度再显示最终结果 | 生成成功 | 执行命令 | 用户先看到过程，再看到文件路径 / warning / 发布结果 | P2 | 已自动化（`TestAppRun_NewShowsProgressBeforeFinalResult`） |
| CLI-UX-008 | config | 缺失生成服务配置时提示 config 指引 | 缺少生成服务配置 | 执行 `officecli new ...` | 明确指出缺失项，并引导运行 `officecli config set-generation` | P0 | 已自动化（`TestAppRun_MissingGenerationConfigShowsConfigGuidance`） |

---

## 3. platform-app 测试矩阵

### 3.1 登录态、路由与会话

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| APP-AUTH-001 | login | 未登录访问 `/app` 跳转到登录页 | 无有效 session | 打开 `/app` | 进入 `/app/login` 或展示登录页壳子 | P0 | 已自动化（`platform/web/app/src/App.test.tsx`） |
| APP-AUTH-002 | login | 登录页可发起 Google 登录 | 浏览器可访问 app 登录页 | 点击“Google 登录”按钮 | 跳转到 `/api/auth/google/login`；带上 return_to 与 attribution 参数 | P0 | 已自动化（`platform/web/app/src/analytics.test.ts`） |
| APP-AUTH-003 | login | 已登录用户访问 app 根路径进入 Overview | 有有效 app session | 打开 `/app` | 正常渲染 overview shell 与指标卡 | P0 | 已自动化（`platform/web/app/src/App.test.tsx`） |
| APP-AUTH-004 | logout | 登出后回到登录页 | 已登录 | 在顶部点击退出登录 | session 被清理；回到登录页；受保护页面再次访问会被拦截 | P0 | 建议自动化 / 人工 E2E |
| APP-AUTH-005 | session | 生产环境 app cookie 具备安全属性 | `APP_ENV=production` 且 HTTPS | 完成登录后检查 `cop_app_session` Cookie | 具备 `HttpOnly`、`Secure`、`SameSite=Lax` | P0 | 已自动化（`platform/internal/app/session_cookie_test.go`） |
| APP-AUTH-006 | session | app 登出清空 cookie | 已登录 | 调用 `/api/auth/logout` | app cookie 被清理，不残留可用 session | P0 | 已自动化（`TestRegisterAuthRoutesLogoutClearsCookieWithSameSiteLax`） |

### 3.2 Overview / API Keys / Usage / Billing / Downloads

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| APP-OVR-001 | overview | 概览页展示基础指标 | 已登录，后端有 overview 数据 | 打开 `/app` | 展示 API Keys、Remaining Credits、Recent Usage、Orders 等指标 | P0 | 已自动化（`App.test.tsx`、`platform/web/app/src/pages/OverviewPage.tsx`） |
| APP-OVR-002 | overview | 概览页展示奖励/邀请/Discord 状态 | 后端返回 `reward_remaining`、`invite_code`、`referral_count`、`discord_*` | 打开 `/app` | 展示 Reward Credits、Referral Progress、Discord Status | P1 | 已自动化（`App.test.tsx`、`platform/internal/appuser/service_test.go`） |
| APP-OVR-003 | overview | 无 key / 无 usage 时展示空态 | 新账号或空数据 | 打开 `/app` | 展示空态文案，不报错、不白屏 | P1 | 建议自动化 |
| APP-KEY-001 | api-keys | 列表展示 key 前缀、状态、配额字段 | 已登录且存在 API keys | 打开 `/app/api-keys` | 正确展示 `key_prefix/status/plan/quota_total/quota_used/quota_remaining` | P0 | 已自动化（`platform/web/app/src/pages/ApiKeysPage.test.tsx`） |
| APP-KEY-002 | api-keys | 用户只能编辑 note 与启停状态 | 已登录且有可编辑 key | 在页面尝试编辑 key | 仅允许改 `note/status`；总额、已用、过期时间只读 | P0 | 建议自动化 / 人工 E2E |
| APP-KEY-003 | api-keys | key 状态变更后页面刷新同步 | 有 active key | 在页面禁用或重新启用 key | 列表状态即时更新；后续 CLI check 结果同步变化 | P1 | 人工 E2E |
| APP-KEY-004 | api-keys | key 列表空态 | 当前用户无任何 key | 打开 `/app/api-keys` | 展示空态与引导文案 | P2 | 建议自动化 |
| APP-USAGE-001 | usage | Usage 页面展示最近调用记录 | 已登录且存在 usage event | 打开 `/app/usage` | 展示 mode、action、result、reason_code、created_at | P0 | 建议自动化 |
| APP-USAGE-002 | usage | 无 usage 数据时展示空态 | usage 为空 | 打开 `/app/usage` | 显示 `No usage events recorded` 类空态 | P2 | 建议自动化 |
| APP-BILL-001 | billing | Billing 页面展示 pricing pack 与订单历史 | 已登录，overview/pricing/order 数据可取 | 打开 `/app/billing` | 展示可购买的次数包、目标 key 选择、Recent billing activity | P0 | 建议自动化 |
| APP-BILL-002 | billing | 选择目标 key 创建 checkout | 已登录，有至少 1 个 key，Stripe mock/测试环境可用 | 选择 pack 与目标 key，发起购买 | 成功调用 checkout；跳转到 Stripe 或返回 checkout URL | P0 | 人工 E2E |
| APP-BILL-003 | billing | 无订单时展示空态 | 无 billing events | 打开 `/app/billing` | 显示 `No billing events yet` 类空态 | P2 | 建议自动化 |
| APP-DL-001 | downloads | Downloads 页面提供 CLI 获取与安装指引 | 已登录 | 打开 `/app/downloads` | 展示可下载版本、安装或接入说明 | P1 | 建议自动化 |

### 3.3 奖励、邀请、Discord 与额度一致性

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| APP-GROW-001 | growth | 概览页展示 invite code 与奖励余额 | 用户存在奖励账本 / invite code | 打开 `/app` | `invite_code` 与 `reward_remaining` 展示正确 | P1 | 已自动化（`platform/internal/appuser/service_test.go`） |
| APP-GROW-002 | growth | app growth 数据结构可返回 referrals / reward grants / discord connection | 后端准备对应数据 | 调用 `/api/app/growth` 或打开相关页面/模块 | 数据结构完整；无字段缺失导致的前端异常 | P1 | 已自动化（`platform/internal/appuser/service_test.go`） |
| APP-GROW-003 | growth | Discord 未绑定时状态正确 | 用户无 Discord 连接 | 打开 `/app` | 显示 `NOT LINKED` 或等价文案 | P2 | 建议自动化 |
| APP-GROW-004 | growth | Discord 已绑定但未入 guild 的状态正确 | `discord_connected=true`，`discord_guild_member=false` | 打开 `/app` | 显示 `CONNECTED` 而非 `VERIFIED` | P2 | 已自动化（`App.test.tsx` 的 overview 数据覆盖） |
| APP-QUOTA-001 | quota | 免费 / 付费 / 奖励额度在 app 展示一致 | 后端分别构造 free / paid / reward 数据 | 打开 overview、api keys、usage | 指标、列表、event 模式一致，不互相冲突 | P0 | 建议自动化 |
| APP-QUOTA-002 | quota | 后台调高 key 配额后 app 同步展示 | admin 已把 key `quota_total` 提高 | 刷新 `/app/api-keys` 与 `/app` | `quota_total`、`remaining`、总剩余额度同步增加 | P0 | 人工 E2E（参考 `docs/usage-limits-e2e.md`） |
| APP-QUOTA-003 | quota | 后台调高 free quota 后 app / CLI 状态同步 | admin 已调整某 fingerprint free limit | 先后查看 app 相关概览与 CLI `auth status` / `new` | 展示/校验逻辑与后台一致 | P1 | 人工 E2E |
| APP-QUOTA-004 | linkage | CLI 成功生成后，usage 与额度变化反映到 app | CLI 与 platform 指向同一套服务 | 执行一次 `officecli new ...`，再刷新 `/app` | `recent_usage_count`、Usage 列表、remaining 指标同步更新 | P0 | 人工 E2E |
| APP-QUOTA-005 | linkage | 被禁用 key 不再可用且 app 状态同步 | admin 禁用目标 key | 用该 key 再执行 CLI check/new，并刷新 `/app/api-keys` | CLI 被拦截；app 页面显示 disabled 状态 | P0 | 人工 E2E |

### 3.4 API / 错误态 / 加载态

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| APP-ERR-001 | api | `/api/app/overview` 失败时前端不白屏 | mock overview 返回错误 | 打开 `/app` | 路由安全回退或展示错误态，不出现白屏循环 | P1 | 建议自动化 |
| APP-ERR-002 | api | `/api/app/api-keys` 失败时页面可恢复 | mock key 列表请求失败 | 打开 `/app/api-keys` | 展示错误反馈或空态，不崩溃 | P1 | 建议自动化 |
| APP-ERR-003 | api | `/api/app/usage-events` 空列表正确渲染 | usage 返回空数组 | 打开 `/app/usage` | 空态可见 | P2 | 建议自动化 |
| APP-ERR-004 | loading | 受保护壳层在 session 拉取期间显示 loading | 首屏 session 请求未完成 | 打开任意受保护路由 | 展示 LoadingScreen，不闪白、不提前跳错 | P2 | 已自动化（`platform/web/app/src/App.tsx` 路由逻辑） |

---

## 4. platform-admin 测试矩阵

### 4.1 管理员登录、权限与安全会话

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ADM-AUTH-001 | login | 未登录访问受保护页面自动跳转 Google 登录 | 无管理员 session | 打开 `/admin/users?tab=ops#team` | 前端调用 `/api/admin/auth/google/login?return_to=...` | P0 | 已自动化（`platform/web/admin/src/App.test.tsx`） |
| ADM-AUTH-002 | login | `/admin/login` 不暴露真实后台登录页 | 打开 `/admin/login` | 页面仅渲染通用 404，不暴露内部后台细节 | P0 | 已自动化（`App.test.tsx`） |
| ADM-AUTH-003 | access-denied | 非 allowlist 邮箱进入 access denied | 管理员邮箱不在 allowlist | 打开 `/admin/access-denied` 或触发拒绝流程 | 展示 blocked email 上下文与拒绝页 | P0 | 已自动化（`App.test.tsx`） |
| ADM-AUTH-004 | session | 生产环境管理员 Cookie 具备安全属性 | `APP_ENV=production` 且 HTTPS | 完成管理员登录 | `cop_admin_session` 带 `HttpOnly`、`Secure`、`SameSite=Lax` | P0 | 已自动化（`platform/internal/app/session_cookie_test.go`） |
| ADM-AUTH-005 | logout | 管理员登出清理 cookie | 已登录 | 调用 `/api/admin/logout` | 管理员 cookie 被清理；再次访问受保护页会重新走登录 | P0 | 已自动化（`TestRegisterAdminRoutesLogoutClearsCookieWithSameSiteLax`） |
| ADM-AUTH-006 | auth | Google 回调拒绝用户时不种 cookie | access denied 用户执行回调 | 完成 OAuth callback | 重定向到 access denied；不写管理员 session | P1 | 已自动化（`TestRegisterAdminRoutesGoogleCallbackRedirectsDeniedUsersWithoutCookie`） |
| ADM-AUTH-007 | auth | 管理员 session 接口返回当前身份 | 已登录 | 调用 `/api/admin/session` | 返回 email / name / auth_method 等信息 | P1 | 已自动化（`TestRegisterAdminRoutesSessionReturnsCurrentIdentity`） |

### 4.2 Dashboard / Users / Orders / Billing Events / Usage Events

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ADM-DASH-001 | dashboard | Dashboard 展示平台总览指标 | 已登录，overview 接口返回数据 | 打开 `/admin` | 展示 Total API Keys、Active Keys、Free Machines、Blocked 24H 等指标 | P0 | 建议自动化 |
| ADM-DASH-002 | dashboard | Dashboard 对 24H 检查/扣减/过期 key 展示正确 | overview 数据完整 | 打开 `/admin` | `checks_last_24h`、`consumes_last_24h`、`expired_api_keys`、`remaining_paid_quota` 等指标正确 | P1 | 建议自动化 |
| ADM-USER-001 | users | Users 页面可查看用户列表与状态 | 已登录，有用户数据 | 打开 `/admin/users` | 展示用户 email / name / status / invite_code / created_at | P0 | 建议自动化 |
| ADM-ORDER-001 | orders | Orders 页面展示订单记录 | 已登录，有订单数据 | 打开 `/admin/orders` | 展示订单状态、金额、pack、target key、创建时间 | P1 | 建议自动化 |
| ADM-BILL-001 | billing-events | Billing Events 页面展示 webhook / 支付事件 | 已登录，有 billing event 数据 | 打开 `/admin/billing-events` | 展示 event_id、event_type、status、processed_at、error_message | P1 | 建议自动化 |
| ADM-USAGE-001 | usage-events | Usage Events 页面展示最近调用记录 | 已登录，有 usage 数据 | 打开 `/admin/usage-events` | 展示 `mode/action/result/reason_code/fingerprint_hash` | P0 | 建议自动化 |
| ADM-USAGE-002 | usage-events | Usage Events 支持 reward 模式过滤 | usage events 中存在 reward | 打开页面并检查筛选项 | mode filter 中包含 `reward` | P1 | 已自动化（`platform/web/admin/src/App.test.tsx`） |
| ADM-USAGE-003 | usage-events | 空列表/无匹配筛选时展示空态 | 数据为空或筛选后为空 | 打开页面 | 不报错；显示空态 | P2 | 建议自动化 |

### 4.3 API Keys / Free Quotas 管理动作

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ADM-KEY-001 | api-keys | 后台可创建新 key | 已登录，有目标用户 | 在 `/admin/api-keys` 创建 key | 返回一次性明文 key；列表新增记录 | P0 | 人工 E2E |
| ADM-KEY-002 | api-keys | 后台可编辑 key 的 note / status / quota | 已存在 key | 在列表或编辑面板修改字段 | 保存成功；列表与后端状态一致 | P0 | 建议自动化 / 人工 E2E |
| ADM-KEY-003 | api-keys | 禁用 key 后 CLI 被立即拦截 | 已存在可用 key，CLI 使用该 key | 在 admin 禁用 key，再执行 `officecli new` 或 `auth status` | CLI 返回 disabled/blocked；不再继续生成 | P0 | 人工 E2E |
| ADM-KEY-004 | api-keys | 提高 key 配额后 app 与 CLI 同步 | key 当前 remaining 有值 | 在 admin 把 `quota_total` 提高 | app remaining 增加；CLI status 与 consume 后 remaining 正确 | P0 | 人工 E2E |
| ADM-KEY-005 | api-keys | app 用户无权修改后台控制字段 | 用户端尝试改 `quota_total` / `expires_at` | 在 app 页面尝试编辑 | 仅 admin 可以改这些字段 | P1 | 已自动化（`platform/internal/appuser/service_test.go` `TestUpdateAPIKeyOnlyPersistsStatusAndNote`） |
| ADM-FREE-001 | free-quotas | 后台查看免费额度列表 | 已登录，有 free quota 数据 | 打开 `/admin/free-quotas` | 展示 fingerprint、free_limit、free_used、更新时间 | P0 | 已自动化（`platform/web/admin/src/pages/FreeQuotasPage.test.tsx`） |
| ADM-FREE-002 | free-quotas | 调高 free quota 后界面显示最新值 | 已存在 fingerprint 记录 | 在后台调整 `free_limit` | 列表立即反映最新 `free_limit/free_used` | P1 | 已自动化（`FreeQuotasPage.test.tsx`） |
| ADM-FREE-003 | free-quotas | 调高 free quota 后 CLI 状态同步 | admin 已把目标 fingerprint free limit 提高 | 执行 `officecli auth status` 或 `new` | 剩余免费次数按新值计算 | P0 | 人工 E2E（参考 `docs/usage-limits-e2e.md`） |
| ADM-FREE-004 | free-quotas | 免费额度跨天重置正确 | 有昨日已耗尽 quota 的 fingerprint | 跨天后执行 check/status | `free_used` 按新日期重置，remaining 恢复 | P1 | 已自动化（`platform/internal/license/service_test.go` `TestFreeQuotaResetsAcrossDays`） |

### 4.4 License / 可观测性 / 限流 / 错误响应

| 编号 | 模块 | 场景 | 前置条件 | 步骤 | 预期结果 | 优先级 | 自动化状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ADM-OBS-001 | request_id | 所有统一响应都包含 `request_id` | 服务已启动 | 分别请求成功与失败 API | 响应头 `X-Request-Id` 和响应体 `request_id` 一致存在 | P0 | 已自动化（`platform/internal/httpapi/request_id_test.go`、`platform/internal/app/application_license_routes_test.go`） |
| ADM-OBS-002 | healthz | `/healthz` 返回 request_id 且不刷访问日志 | 服务已启动 | 请求 `/healthz` | 响应里有 `request_id`；访问日志跳过该路径 | P1 | 已自动化（`healthz_test.go`、`access_log_test.go`） |
| ADM-OBS-003 | access-log | 正常请求写 `http_request_completed` 日志 | 服务日志可读 | 请求业务接口 | 结构化日志含 `request_id/method/path/status/latency_ms` | P1 | 已自动化（`platform/internal/httpapi/access_log_test.go`） |
| ADM-OBS-004 | slow-log | 慢请求写 `http_request_slow` 日志 | 人为制造慢请求 | 请求接口 | 输出慢请求日志 | P2 | 已自动化（`access_log_test.go`） |
| ADM-OBS-005 | rate-limit | 管理员登录限流生效并写日志 | 高并发或快速重复请求 `/api/admin/login` | 超阈值请求 | 返回限流错误；日志出现 `rate_limit_exceeded` | P0 | 已自动化（`platform/internal/app/rate_limit_test.go`、`observability_test.go`） |
| ADM-OBS-006 | rate-limit | license check / consume 限流生效 | 快速重复请求 `/api/license/check` 与 `/api/license/consume` | 超阈值请求 | 返回限流错误，不影响系统稳定性 | P0 | 已自动化（`rate_limit_test.go`） |
| ADM-OBS-007 | auth-log | 管理员登录失败写关键日志 | 使用错误密码登录 | 调用 `/api/admin/login` | 日志包含 `admin_login_failed` 与 `request_id` | P1 | 已自动化（`observability_test.go`） |
| ADM-OBS-008 | oauth-log | OAuth 回调失败写关键日志 | 模拟 OAuth 失败 | 调用 Google callback | 日志包含 `auth_callback_failed` 与 `request_id` | P1 | 已自动化（`observability_test.go`） |
| ADM-OBS-009 | stripe-log | Stripe webhook 失败写关键日志 | 模拟无效 webhook | 调用 `/api/stripe/webhook` | 日志包含 `stripe_webhook_failed` 与 `request_id` | P1 | 已自动化（`observability_test.go`） |
| ADM-LIC-001 | license | 免费模式首次 check 自动建额度 | 新 fingerprint，无 API key | 调用 `/api/license/check` | 返回 `allowed=true`、`access_mode=free`、自动创建 daily quota | P0 | 已自动化（`platform/internal/license/service_test.go`） |
| ADM-LIC-002 | license | 免费额度耗尽后被阻塞 | `free_used == free_limit` | 调用 `/api/license/check` / `/consume` | 返回 blocked / conflict，reason 为 `free_quota_exhausted` | P0 | 已自动化（`service_test.go`、`application_license_routes_test.go`） |
| ADM-LIC-003 | license | paid / reward / free 优先级正确 | 用户同时具备 reward 与 free / 或 paid key | 调用 check / consume | `access_mode` 与 remaining 符合设计 | P0 | 已自动化（`platform/internal/license/service_test.go`） |
| ADM-LIC-004 | license | consume 幂等且并发安全 | 同一 `request_id` 重复 consume；并发环境 | 重复/并发请求 `/api/license/consume` | 仅 1 次真实扣减；不超扣 | P0 | 已自动化（`platform/internal/license/service_test.go`） |
| ADM-LIC-005 | license | app/admin/CLI 对额度变化的一致性 | 同一 key / fingerprint 在三端可见 | 后台调额、CLI 生成、刷新 app/admin | 三端 remaining、状态、usage 结果一致 | P0 | 人工 E2E |

---

## 5. 现有自动化证据索引

以下为本次全功能总表对应的高价值现有自动化证据，便于后续更新文档时快速定位：

### 5.1 CLI / 运行时 / 生成器

- `internal/cli/app_test.go`
  - help / version / init / auth / progress / JSON 输出
  - prompt 优先级、publish 覆盖、`--no-images`
  - license 拦截、offline 差异、reward/free/paid 状态展示
- `internal/cli/executor_test.go`
  - 生成成功后扣减
  - consume 失败时成功返回 + warning
  - 免费 / 付费 / 奖励 warning
  - 生成失败 / 发布失败不扣减
- `internal/runtime/service_test.go`
  - `pptx/docx` 生成主链路
  - `pptx` 图片启用 / 禁用 / 降级
  - progress event 发射
- `pkg/officegen/pptx_generator_test.go`
  - 图表、嵌图、JPEG、图片布局、裁剪、Office 兼容格式
- `pkg/ooxmledit/pptx_modify_test.go`
  - PPT OOXML 文本替换、表格、段落与 slide XML 修改

### 5.2 platform-app / platform-admin 前端

- `platform/web/app/src/App.test.tsx`
  - 登录页与 overview 壳层渲染
- `platform/web/app/src/pages/ApiKeysPage.test.tsx`
  - app API key 配额字段展示
- `platform/web/app/src/analytics.test.ts`
  - Google 登录 attribution 参数透传
- `platform/web/admin/src/App.test.tsx`
  - `/admin/login` 404、access denied、未登录跳转 Google 登录、reward filter
- `platform/web/admin/src/pages/FreeQuotasPage.test.tsx`
  - free quota 最新值展示

### 5.3 platform 后端 / 授权 / 安全 / 可观测性

- `platform/internal/license/service_test.go`
  - 免费 / 付费 / 奖励 check/consume、并发幂等、跨天重置
- `platform/internal/app/application_license_routes_test.go`
  - license 路由 bad request / conflict / reward / request_id
- `platform/internal/app/session_cookie_test.go`
  - app/admin session cookie 安全属性、登出清理、invite 透传
- `platform/internal/app/rate_limit_test.go`
  - admin login 与 license 接口限流
- `platform/internal/httpapi/request_id_test.go`
  - 统一 `request_id` 响应头 / envelope
- `platform/internal/httpapi/access_log_test.go`
  - completed / slow / healthz skip
- `platform/internal/app/observability_test.go`
  - admin login failed、OAuth callback failed、Stripe webhook failed、rate limit 日志
- `platform/internal/appuser/service_test.go`
  - overview / growth / API key 可编辑字段限制
- `platform/internal/admin/service_test.go`
  - key 创建与配额更新
- `platform/internal/billing/service_test.go`
  - checkout 权限与目标 key 校验
- `platform/internal/auth/service_test.go`
  - invite code 写入 OAuth state 与 referral 注册

---

## 6. 建议补齐的测试缺口

以下场景建议优先补齐，以把本总表中的 `建议自动化` / `人工 E2E` 持续转为自动化：

| 编号 | 测试块 | 缺口场景 | 建议补齐方式 |
| --- | --- | --- | --- |
| GAP-001 | CLI | `xlsx` 端到端成功生成的 CLI 级自动化证据不足 | 增加 `internal/cli/executor_test.go` 或 `internal/runtime/service_test.go` 对 `xlsx` 完整链路覆盖 |
| GAP-002 | CLI | `best` 模式非 TTY 失败提示缺直接自动化 | 在 `internal/cli/app_test.go` 增加非 TTY + `--mode best` 覆盖 |
| GAP-003 | CLI | publisher=nil 时 warning 缺直接测试 | 在 `executor_test.go` 增加“无 publisher 但 publish=true”的成功场景 |
| GAP-004 | platform-app | `/app/api-keys` 编辑 note/status 的前端交互缺自动化 | 增加页面交互测试或 API mock 集成测试 |
| GAP-005 | platform-app | `usage`、`billing`、`downloads` 页面的 UI 主流程缺自动化 | 为各页面补 `*.test.tsx` 覆盖正常态 / 空态 / 错误态 |
| GAP-006 | platform-app | CLI 生成后 app usage/overview 同步缺联调验证脚本 | 在 `docs/usage-limits-e2e.md` 基础上补完整三端联调脚本 |
| GAP-007 | platform-admin | Dashboard、Users、Orders、BillingEvents、ApiKeys 页面缺前端自动化 | 逐页增加组件测试，至少覆盖表格渲染与关键操作入口 |
| GAP-008 | platform-admin | 创建 / 禁用 key 后 CLI 即时拦截缺自动化 | 做后端集成测试或本地 smoke 脚本串联 admin + CLI + license |
| GAP-009 | 全链路 | 后台调额后 app / admin / CLI 三端一致性缺固定回归脚本 | 新增端到端 smoke 脚本，覆盖 key quota 与 free quota 两类同步 |
| GAP-010 | 全链路 | Stripe checkout 成功后 app Billing / admin Orders / CLI status 一致性缺自动化 | 在测试环境引入 billing mock 或 webhook 回放测试 |

---

## 7. 执行建议

- 日常回归建议先跑 `P0`，再按改动范围补跑相关 `P1`
- 发布前建议至少覆盖：
  - 一条 `officecli new pptx ...` 默认配图链路
  - 一条 `officecli new pptx ... --no-images` 无图链路
  - 一条 `officecli new docx ...`
  - 一条 `officecli new xlsx ...`
  - 一条 `platform-app` 登录 + overview + api keys
  - 一条 `platform-admin` 登录 + key 调整 + free quota 调整
  - 一条 CLI / app / admin 三端额度一致性联调
- 若只变更授权/额度逻辑，应同时参考：
  - `docs/usage-limits-test-cases.md`
  - `docs/usage-limits-e2e.md`
  - `docs/usage-limits-test-report.md`

