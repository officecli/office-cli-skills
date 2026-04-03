# OfficeCLI 生产巡检说明

本文档说明 `officecli` 当前的生产巡检自动化设计与落地方式。

目标：

- 自动巡检 `officecli.io` / `platform.officecli.io` 核心可用性
- 保留一层完全只读的生产检查
- 对必须写入才能验证的链路，仅允许使用**隔离测试数据**
- 巡检失败后通过 GitHub Actions + 钉钉告警输出结果

## 1. 入口与触发方式

巡检 workflow：

- `/Users/luyang/workspace/shimo/void-oversea/officecli/.github/workflows/production-inspection.yml`

触发方式：

- `schedule`
  - 每 30 分钟执行一次只读巡检
  - 每天 1 次执行隔离写入巡检
- `workflow_dispatch`
  - 人工补跑
  - 可选择是否执行隔离写入巡检

## 2. 巡检分层

### 2.1 只读生产巡检

只读层不会触发任何用户态写入，当前覆盖：

- `https://officecli.io/`
- `https://officecli.io/api/pricing`
- `https://officecli.io/app`
- `https://platform.officecli.io/`
- `https://platform.officecli.io/app/`
- `https://platform.officecli.io/admin/`
- `https://platform.officecli.io/healthz`

断言内容包括：

- 路由跳转方向是否正确
- `pricing` 响应是否可解析
- `healthz` 是否返回正常 JSON
- app/admin 页面是否可达且不是错误页 / 5xx

### 2.2 隔离写入巡检

写入层只验证 license 最小闭环，当前覆盖：

- `POST /api/license/check`
- `POST /api/license/consume`
- 专用测试 key 的 allowed / consume / idempotency
- 专用 blocked key 的 blocked / reason_code

这层**必须**满足以下隔离约束：

- `INSPECTION_FINGERPRINT_HASH` 必须以 `inspection-` 或 `cron-` 开头
- `request_id` 统一以 `inspection-` 开头
- 使用的 API key 必须是开发者专用 key，不能属于真实用户
- blocked key 也必须是开发者专用的 disabled / expired / exhausted key
- 巡检产生的数据只允许出现在开发者专用工作区，不得出现在真实用户可见数据中

如果这些前提不满足，workflow 应显式失败，而不是回退到真实用户数据。

## 3. 相关脚本

新增脚本：

- `/Users/luyang/workspace/shimo/void-oversea/officecli/scripts/run-production-inspection.sh`
- `/Users/luyang/workspace/shimo/void-oversea/officecli/scripts/production-readonly-checks.sh`
- `/Users/luyang/workspace/shimo/void-oversea/officecli/scripts/production-isolated-write-checks.sh`
- `/Users/luyang/workspace/shimo/void-oversea/officecli/scripts/inspection-common.sh`
- `/Users/luyang/workspace/shimo/void-oversea/officecli/scripts/notify-dingtalk.sh`

执行示例：

```bash
bash ./scripts/run-production-inspection.sh readonly
bash ./scripts/run-production-inspection.sh isolated
```

报告输出：

- `checks.tsv`
- `summary.md`
- `meta.env`

GitHub Actions 会把这些文件上传成 artifact，并追加到 job summary。

## 4. 必需的 GitHub Secrets / Variables

### 4.1 Secrets

| 名称 | 用途 |
| --- | --- |
| `DINGTALK_WEBHOOK` | 巡检失败时发送钉钉告警 |
| `INSPECTION_FINGERPRINT_HASH` | 隔离写入巡检专用 fingerprint |
| `INSPECTION_API_KEY` | 可用的开发者专用测试 key |
| `INSPECTION_BLOCKED_API_KEY` | blocked 的开发者专用测试 key |
| `INSPECTION_USER_ID` | 可选，开发者专用 user_id |

### 4.2 Variables

| 名称 | 默认值 | 用途 |
| --- | --- | --- |
| `INSPECTION_SITE_BASE_URL` | `https://officecli.io` | 官网巡检基址 |
| `INSPECTION_PLATFORM_BASE_URL` | `https://platform.officecli.io` | 平台巡检基址 |

## 5. 钉钉告警策略

告警策略为：

- 成功：只写 GitHub Actions summary / artifact，不发钉钉
- 失败：发钉钉

消息至少包含：

- 失败 workflow run 链接
- 失败 job / phase / check 名称
- 失败 URL / 接口
- HTTP 状态码
- `request_id`（如果有）
- detail 说明

## 6. 当前边界

当前巡检**已自动化**：

- 公网首页、pricing、app/admin/healthz 只读检查
- license 最小写入闭环
- 失败 artifact 汇总
- 钉钉失败告警

当前巡检**暂未自动化**：

- Google OAuth 浏览器级登录
- Stripe checkout / webhook 真正支付闭环
- app/admin 浏览器交互级 UI 断言
- CLI / app / admin 三端一致性端到端巡检

这些部分仍建议保留人工验收或在后续单独扩展。
