# OfficeCLI 安全设计报告

更新时间：2026-04-04

## 1. 目标与范围

本报告聚焦 `officecli` / `platform` 的商业授权、防盗版、防回放与高成本能力保护，重点覆盖：

- CLI 侧授权校验与本地执行决策
- `platform` 侧 `/api/license/check`、`/api/license/consume`
- API key 默认权限模型
- hosted credits 开通路径
- 本地 DNS / hosts / 代理劫持、抓包回放、key 泄露后的滥用面

不在本轮解决范围内：

- 用户完全修改本地二进制后的不可恢复破解
- 第三方终端环境被完全攻陷后的端点安全
- 全量异常检测 / 风控平台建设

## 2. 威胁模型

本轮采用强对手模型：

- 用户可控制自己的机器、DNS、hosts、代理、系统 CA、环境变量与本地配置
- 用户可抓包、重放、篡改 CLI 请求和响应
- 用户可导出、转卖、共享 API key
- 用户可直接绕过 CLI，调用平台 HTTP 接口

因此，任何“只要请求命中了 `platform.officecli.io` 就可信”的设计都被视为不安全。

## 3. 已确认的核心攻击路径

### 3.1 抓包回放 `license/check`

旧设计下，CLI 直接信任 `/api/license/check` 普通 JSON 返回值。

攻击步骤：

1. 攻击者抓取一次真实 `allowed=true` 响应
2. 在本机劫持 `platform.officecli.io`
3. 让 CLI 命中假服务并回放旧包
4. CLI 被错误放行进入生成阶段
5. `consume` 即使失败，也只给 warning，不阻止文档落地

结论：

- 这是根本性商业授权漏洞
- 只要用户能控制本地网络出口，就可以伪造授权结果

### 3.2 新建 key 默认权限过大

旧设计下，`AppCreateAPIKey` / `AdminCreateAPIKey` 默认直接创建：

- `allowed_modes=hybrid`
- `hosted_enabled=true`
- `default_runtime_mode=hosted`

这与模型层默认值 `external_only + hosted=false` 不一致，导致新 key 天生拥有过大的运行面。

### 3.3 高价值流程闭环不够硬

旧设计下，生成成功后才尝试 `consume`，且失败只 warning。

这意味着：

- 真实计费未完成，用户仍能拿到最终文档
- 商业闭环依赖“事后同步成功”，不具备强约束

## 4. 本轮已落地的安全设计

### 4.1 签名式短时授权证明

`/api/license/check` 不再只返回“客户端可直接相信的普通 JSON 许可结论”，而是返回带签名的 `commit_token`。

proof 绑定字段：

- `fingerprint_hash`
- `user_id`
- `request_id`
- `access_mode`
- `api_key_hint`
- `action`
- `document_type`
- `runtime_mode`
- `request_nonce`
- `issued_at`
- `expires_at`
- `proof_version`

关键性质：

- 服务端签发，CLI 验签
- 短时有效
- 与本次请求绑定
- 旧包回放、篡改字段、过期 proof 都会失败

### 4.2 CLI 侧强校验

CLI 每次 `check` 时会生成随机 `request_nonce`，并在放行前校验：

- proof 版本
- 签名
- 过期时间
- `fingerprint_hash`
- `user_id`
- `action`
- `document_type`
- `runtime_mode`
- `request_nonce`
- `access_mode`

只要本地假服务无法生成合法签名，回放历史响应就无法再次放行。

### 4.3 `consume` 必须带 proof

`/api/license/consume` 现在要求携带并验证 `commit_token`。

这确保：

- 不能只凭 `request_id` / `access_mode` 伪造 consume
- 不能脱离真实 `check` 结果单独补扣
- `check -> consume` 之间形成可验证链路

### 4.4 高价值模式下先扣减再交付

执行顺序已调整为：

1. 生成内存产物
2. 成功 `consume`
3. 写本地文件
4. 发布预览

效果：

- `consume` 失败时不再落地文件
- 商业闭环从“尽力同步”升级为“授权前置条件”

代价：

- 少数场景下会出现“扣减成功，但写文件/发布失败”
- 这是为了防止盗版而有意做出的安全取舍

### 4.5 新建 key 改为最小权限默认值

新的默认值统一收口为：

- `allowed_modes=external_only`
- `hosted_enabled=false`
- `default_runtime_mode=external`

含义：

- 新 key 默认只能跑 external
- hosted 必须通过显式开通或购买 credits 才能启用
- 默认权限与模型层默认值保持一致

### 4.6 购买 hosted credits 才显式解锁 hosted

当 key 获得 hosted credits 时，存储层会自动把该 key 升级为：

- `hosted_enabled=true`
- `allowed_modes=hybrid`（若此前仅 external）
- `default_runtime_mode=hosted`（若此前为空或 external）

这样 hosted entitlement 的开通路径从“创建即开启”收口为“有 hosted 价值输入时显式开启”。

### 4.7 发布服务拒绝零权益 key

`publish` 现在不再接受“刚创建、未充值、零 quota、零 hosted credits”的 key。

当前允许进入发布服务的 key 至少要满足以下任一条件：

- 拥有付费 quota 总量
- 已经产生过付费 quota 消耗
- 当前持有 hosted credits
- 当前存在 hosted credits 预留

效果：

- 新注册用户自助创建的空白 key 不能直接白嫖在线预览发布
- key 泄露后的能力面从“所有 active key 都能 publish”收缩为“至少已获得某种付费权益的 key 才能 publish”

## 5. 当前剩余风险

### 5.1 用户完全 patch 本地二进制

如果攻击者直接修改 CLI 二进制、移除 proof 校验逻辑，则客户端本身无法阻止破解。

这类风险只能靠：

- 服务端 entitlement
- key 吊销
- 审计与异常检测
- 更小权限的 key 模型

### 5.2 `publish` 仍是 key 直接鉴权

当前 `publish` 仍主要基于 API key 自身鉴权，虽然已经拒绝零权益 key。

风险含义：

- 如果一个已付费 key 泄露，攻击者仍可能直接使用发布能力
- 这不是“回放 check 响应”漏洞，但仍是“key 泄露后能力面过大”的问题

建议下一步：

- 给 publish 增加独立 entitlement
- 或要求 publish 也携带近期签发的 proof

### 5.3 hosted / publish 的运维风控仍较轻

目前还缺：

- key 级速率限制
- 异常地区 / 异常 IP / 异常用量检测
- 批量吊销与轮换流程文档
- 告警阈值与安全运营面板

## 6. 运维与配置要求

生产环境必须配置：

- `LICENSE_PROOF_SEED`
- `LICENSE_PROOF_TTL`
- 非默认 `SESSION_SECRET`
- 非默认 `APP_SESSION_SECRET`
- 非默认 `API_KEY_HASH_SALT`

要求：

- `LICENSE_PROOF_SEED` 必须是 32 字节 base64url seed
- 不允许继续使用仓库示例中的默认值
- 轮换 proof seed 时，要同步发布 CLI 侧内置公钥或走双 key 兼容窗口

## 7. 已有验证证据

本轮相关自动化已覆盖：

- CLI proof 验签成功
- CLI 对篡改 / 过期 proof 的拒绝
- 平台 check/consume proof 链路
- consume 失败不再产出文件
- 新建 key 最小权限默认值
- hosted credits 开通 hosted entitlement

全仓 Go 测试已通过：

- `go test ./...`（仓库根目录）
- `go test ./...`（`platform/` 目录）

## 8. 建议的下一阶段

优先级建议如下：

1. 为 `publish` 增加独立 entitlement 或 proof 绑定
2. 为 key 增加更细粒度策略：每日限额、来源限制、能力白名单
3. 建立 key 泄露后的吊销/轮换/审计手册
4. 增加安全运营指标：异常消费、异常发布、异常 hosted 调用
5. 设计 proof key rotation 机制，避免长期单 key 模式
