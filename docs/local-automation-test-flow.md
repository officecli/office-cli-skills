# OfficeCLI 本地自动化测试流程

本文档定义 `officecli` 当前本地自动化测试编排方式，目标是把 `/Users/luyang/workspace/shimo/void-oversea/officecli/docs/full-test-cases.md` 中已经自动化、且最关键的 `P0 / P1` 场景收敛成一套开发者可直接复用的本地回归路径。

本流程只做**本地编排**，不修改 GitHub Actions；重点复用现有：

- `go test ./...`
- `platform` 下的 `go test ./...`
- `platform/web/*` 下的 `npm test -- --run`
- CLI / platform build
- `scripts/usage-limits-smoke.sh`

## 1. 目标与原则

- 让开发者只需要记住 3 个主命令：
  - `make test-fast`
  - `make test-full`
  - `make test-smoke`
- 所有流程默认串行执行，前一步失败立即退出，避免噪音
- 不替换已有命令，只新增统一编排入口
- 不引入 Playwright / Cypress 等新依赖
- 浏览器级 app/admin 联调暂不自动化，仍保留在人工 E2E 范畴

## 2. 命令入口

根目录 `Makefile` 新增以下目标：

```bash
make test-fast
make test-full
make test-smoke
make test-local
```

对应脚本入口：

```bash
bash ./scripts/run-local-test-flow.sh fast
bash ./scripts/run-local-test-flow.sh full
bash ./scripts/run-local-test-flow.sh smoke
bash ./scripts/run-local-test-flow.sh local
```

## 3. 三条主流程

### 3.1 `make test-fast`

定位：最快速本地回归，不依赖已启动的 platform 服务，也不做构建。

执行内容：

1. 根目录 CLI Go 测试：`go test ./...`
2. `platform` Go 测试：`cd platform && go test ./...`
3. `platform-app` 前端测试：`cd platform/web/app && npm test -- --run`
4. `platform-admin` 前端测试：`cd platform/web/admin && npm test -- --run`

覆盖风险：

- CLI 的 `init` / `auth` / `new` / `--json` / `--no-images` / warning / 离线校验
- platform 授权、限流、request_id、session cookie、可观测性
- app/admin 已存在 Vitest 覆盖的登录壳层、overview、API key/free quota 展示

对应 `/Users/luyang/workspace/shimo/void-oversea/officecli/docs/full-test-cases.md` 中的主要覆盖：

- CLI 已自动化 P0/P1
- APP-AUTH-001 ~ APP-AUTH-003
- APP-OVR-001 / APP-OVR-002 / APP-KEY-001
- ADM-AUTH-001 ~ ADM-AUTH-007
- ADM-USAGE-002 / ADM-FREE-001 / ADM-FREE-002
- ADM-OBS-001 ~ ADM-OBS-009
- ADM-LIC-001 ~ ADM-LIC-004

### 3.2 `make test-full`

定位：完整本地回归，适合提交前或较大改动后执行。

执行内容：

1. 完整执行 `test-fast`
2. CLI build：`make build`
3. platform build：`cd platform && make build`
4. `platform-app` build：`cd platform/web/app && npm run build`
5. `platform-admin` build：`cd platform/web/admin && npm run build`
6. `platform-site` test：`cd platform/web/site && npm test -- --run`
7. `platform-site` build：`cd platform/web/site && npm run build`

说明：

- `platform-site` 不属于 `/Users/luyang/workspace/shimo/void-oversea/officecli/docs/full-test-cases.md` 的主测试块，但这里保留为“附带质量门”，确保 `platform` 子项目整体可构建。
- 如果你只想聚焦 app/admin，可通过环境变量跳过 site：

```bash
LOCAL_TEST_SKIP_SITE=1 make test-full
```

如果只想跑 full 里的测试，不跑前端 build：

```bash
LOCAL_TEST_SKIP_BUILD=1 make test-full
```

### 3.3 `make test-smoke`

定位：本地联调 smoke；当前只覆盖 license 闭环。

前提条件：

1. 本地已启动 `platform` 与依赖（PostgreSQL / Redis）
2. `PLATFORM_BASE_URL` 指向可访问的服务，默认 `http://127.0.0.1:8080`
3. 目标 fingerprint 已按需要准备 free quota（脚本默认按 `FREE_LIMIT=2` 预期执行）

执行内容：

1. 预检查 `GET /healthz`
2. 调用 `scripts/usage-limits-smoke.sh`
3. 验证：
   - 免费首次 `check`
   - 免费 `consume` 成功
   - 免费额度耗尽返回 `409`
   - 非法 JSON 返回 `400`

示例：

```bash
make test-smoke
```

或：

```bash
PLATFORM_BASE_URL=http://127.0.0.1:8080 \
FINGERPRINT_HASH=local-smoke-machine \
FREE_LIMIT=2 \
make test-smoke
```

## 4. 总入口

`make test-local` 会按顺序执行：

```text
test-fast -> test-full -> test-smoke
```

适用场景：

- 本地做一次完整提交前回归
- 需要同时验证自动化单测、构建质量门和基础联调 smoke

注意：

- `test-local` 最后会进入 smoke，因此要求 platform 已启动；如果你当前只想做纯离线回归，请直接用 `make test-fast` 或 `make test-full`

## 5. 环境变量

支持以下环境变量控制流程：

| 变量名 | 默认值 | 作用 |
| --- | --- | --- |
| `LOCAL_TEST_SKIP_SITE` | `0` | 设为 `1` 时，`test-full` 跳过 `platform-site` test/build |
| `LOCAL_TEST_SKIP_BUILD` | `0` | 设为 `1` 时，`test-full` 跳过前端 build |
| `PLATFORM_BASE_URL` | `http://127.0.0.1:8080` | `test-smoke` 使用的 platform 地址 |
| `FINGERPRINT_HASH` | `usage-limits-smoke-machine` | 透传给 `scripts/usage-limits-smoke.sh` |
| `FREE_LIMIT` | `2` | 透传给 `scripts/usage-limits-smoke.sh`，控制 smoke 断言预期 |

## 6. 失败时先看哪里

- `test-fast` 失败：
  - CLI / platform Go 逻辑改动，先看 `go test` 报错文件
  - 前端组件测试失败，先看 `platform/web/app` 或 `platform/web/admin` 的 Vitest 输出
- `test-full` 失败：
  - 若测试通过但 build 失败，优先看 TypeScript / Vite / Go build 输出
  - 若仅 `site` 失败，而本次改动只涉及 app/admin，可临时用 `LOCAL_TEST_SKIP_SITE=1`
- `test-smoke` 失败：
  - 先确认 `PLATFORM_BASE_URL/healthz` 可访问
  - 再确认本地数据库、Redis、migration、seed/free quota 准备是否正确
  - 若是响应码不符，优先对照 `/Users/luyang/workspace/shimo/void-oversea/officecli/docs/usage-limits-e2e.md`

## 7. 当前覆盖边界

当前流程**已自动化覆盖**：

- CLI 主体单测与运行时测试
- platform 授权、额度、幂等、限流、request_id、session cookie、observability
- app/admin 现有 Vitest 覆盖到的页面与壳层
- 基础 license smoke

当前流程**仍未自动化覆盖**：

- CLI / app / admin 三端一致性联调
- app 的 API key 编辑交互、usage/billing/downloads 页面细节
- admin 的 Dashboard / Users / Orders / Billing Events / ApiKeys 主页面交互
- 浏览器级 OAuth / Stripe / 真实 UI E2E

这些未覆盖项仍以 `/Users/luyang/workspace/shimo/void-oversea/officecli/docs/full-test-cases.md` 中的 `建议自动化` / `人工 E2E` 为准，后续可继续扩展到新脚本中。
