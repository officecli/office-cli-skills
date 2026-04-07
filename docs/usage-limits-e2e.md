# OfficeCLI 使用限制联调 / E2E 指南

本文档补充“使用限制”在多系统协作场景下的联调步骤，重点覆盖：

- `officecli-platform` 授权 API 的 smoke 验证
- 后台调额后，用户中心页面展示是否同步
- CLI 与 platform 的关键限制行为是否一致

适用对象：

- 研发联调
- QA 冒烟验证
- 上线前回归

## 1. 前置条件

### 1.1 启动 platform

在 `platform/` 子项目目录准备本地依赖并启动服务：

```bash
docker compose -f deploy/docker-compose.yml up -d
cp .env.example .env
export $(grep -v '^#' .env | xargs)
go run ./cmd/platform db migrate
make web-install
make web-build
make dev
```

默认本地访问：

- 官网：`http://127.0.0.1:8080/`
- 用户中心：`http://127.0.0.1:8080/app`
- 管理后台：`http://127.0.0.1:8080/admin`

### 1.2 准备 CLI 配置

确保 CLI 配置中的 `license.base_url` 指向本地平台，例如：

```json
{
  "license": {
    "base_url": "http://127.0.0.1:8080",
    "enabled": true
  }
}
```

## 2. 自动 smoke 脚本

仓库内提供脚本：

`scripts/usage-limits-smoke.sh`

默认验证：

- `POST /api/license/check` 免费首次检查
- `POST /api/license/consume` 免费扣减
- 免费额度耗尽后的 `409`
- 非法 JSON 请求返回 `400`

执行示例：

```bash
bash ./scripts/usage-limits-smoke.sh
```

如果本地平台地址不同，可覆盖：

```bash
PLATFORM_BASE_URL=http://127.0.0.1:8080 \
FINGERPRINT_HASH=e2e-machine-1 \
bash ./scripts/usage-limits-smoke.sh
```

## 3. 后台调额 -> 用户中心展示同步联调

这部分是当前仍建议保留的人工 E2E 验证。

### 场景 A：调高付费 key 次数后，用户中心同步显示

1. 登录后台 `http://127.0.0.1:8080/admin`
2. 进入 `API Keys`
3. 找到目标 key，记录当前：
   - `quota_total`
   - `quota_used`
   - `quota_remaining`
4. 将该 key 的 `quota_total` 提高，例如从 `100` 调到 `150`
5. 打开用户中心 `http://127.0.0.1:8080/app/api-keys`
6. 刷新页面，确认：
   - 该 key 的 `总次数` 显示为 `150`
   - `已用` 保持原值
   - `剩余` 等于 `quota_total - quota_used`
7. 打开用户中心概览页 `http://127.0.0.1:8080/app`
8. 确认 `总剩余次数` 已同步增加

预期结果：

- API Keys 页面与概览页数据一致
- 前后端展示无旧缓存残留

### 场景 B：调高免费额度后，CLI 状态同步变化

1. 用一个固定 `fingerprint_hash` 先执行一次免费 `check`
2. 在后台 `Free Quotas` 页搜索该指纹
3. 记录当前：
   - `free_limit`
   - `free_used`
4. 将 `free_limit` 提高，例如从 `5` 调到 `8`
5. 重新执行：

```bash
officecli auth status
```

或直接调用：

```bash
curl -X POST http://127.0.0.1:8080/api/license/check \
  -H 'Content-Type: application/json' \
  -d '{
    "fingerprint_hash":"<same-fingerprint>",
    "action":"status"
  }'
```

预期结果：

- `free_remaining` 按新额度计算
- CLI 状态文案与平台返回一致

## 4. CLI 联调建议

### 场景 C：免费额度耗尽后 CLI 拦截

1. 将某个 `fingerprint_hash` 的 `free_limit` 调到接近 `free_used`
2. 连续执行 CLI 生成命令直到耗尽
3. 最后一次执行应在进入 LLM 前失败

预期结果：

- 报错为免费额度耗尽
- 不继续生成内容

### 场景 D：付费 key 不可用时 CLI 拦截

1. 在后台禁用某个 key 或将其余额调为 0
2. 执行：

```bash
officecli auth set-key <api-key>
```

或：

```bash
officecli auth status
```

预期结果：

- 返回明确错误：禁用 / 过期 / 次数耗尽
- 已存在旧配置时，不应被坏 key 覆盖

## 5. 当前建议保留的人工检查点

即便自动化已覆盖，以下项仍建议在发布前人工看一遍：

- 用户中心页面数字刷新后是否和后台一致
- CLI 文案是否和产品预期一致
- 后台调额后是否存在短时缓存导致的旧值展示
- 免费 / 付费模式切换时，warning 文案是否正确
