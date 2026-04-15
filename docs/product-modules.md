# OfficeCLI 产品模块总结

本文从产品视角总结当前仓库所对应的具体产品模块，目标是把代码目录、平台页面和商业化能力，整理成便于沟通的产品结构。

## 一、产品整体定位

OfficeCLI 不是单一的命令行工具，而是一套围绕 Office 文档生成构建的完整产品体系，主要由三层组成：

1. 核心生成产品
2. 平台与商业化产品
3. 生态分发与集成产品

## 二、核心产品模块

### 1. OfficeCLI 核心生成模块

这是产品的核心使用入口，负责把自然语言需求转换为 Office 文档或本地报告文件。

当前支持的主要能力：

- 生成 `PPTX`
- 生成 `DOCX`
- 生成 `XLSX`
- 生成 `Report`
- 通过本地命令行完成配置、执行、输出和结果展示

关联入口：

- [README.md](/home/ubuntu/workspace/officecli/README.md)
- [cmd/officecli/main.go](/home/ubuntu/workspace/officecli/cmd/officecli/main.go)
- [internal/cli/app.go](/home/ubuntu/workspace/officecli/internal/cli/app.go)

### 2. 文档质量评分与审查模块

该模块用于对生成后的 `PPTX` 做质量检查与评分，不属于自动生成流程的一部分，而是独立的质量评估能力。

当前能力包括：

- 结构检查
- 可选的视觉审查
- 分数输出与阈值校验
- `score` / `review` 双命令入口

关联入口：

- [README.md](/home/ubuntu/workspace/officecli/README.md)
- [internal/cli/app.go](/home/ubuntu/workspace/officecli/internal/cli/app.go)

### 3. 文档发布与在线预览模块

该模块负责在生成完成后进行可选发布，并返回在线预览链接。它让产品不只停留在“本地导出文档”，而是具备“生成后即可访问和分享”的能力。

当前能力包括：

- 生成后自动或手动触发发布
- 返回在线访问地址
- 支持本地输出与发布并存

关联入口：

- [README.md](/home/ubuntu/workspace/officecli/README.md)
- [internal/providers](/home/ubuntu/workspace/officecli/internal/providers)

## 三、平台产品模块

### 4. 官网与营销站模块

这是面向外部用户的公开站点，承担品牌展示、功能介绍、定价说明、下载引导和文档入口等职责。

当前页面包括：

- Home
- Pricing
- Download
- Docs
- FAQ
- Login

关联入口：

- [platform/README.md](/home/ubuntu/workspace/officecli/platform/README.md)
- [platform/web/site/src/App.tsx](/home/ubuntu/workspace/officecli/platform/web/site/src/App.tsx)
- [platform/web/site/src/pages/HomePage.tsx](/home/ubuntu/workspace/officecli/platform/web/site/src/pages/HomePage.tsx)

### 5. 用户控制台模块

这是面向注册/登录用户的自助工作台，用来查看额度、管理密钥、查看账单和下载资源，属于客户侧控制面。

当前页面包括：

- Overview
- API Keys
- Billing
- Usage
- Downloads
- Login
- Access Denied

关联入口：

- [platform/web/app/src/App.tsx](/home/ubuntu/workspace/officecli/platform/web/app/src/App.tsx)
- [platform/web/app/src/pages/OverviewPage.tsx](/home/ubuntu/workspace/officecli/platform/web/app/src/pages/OverviewPage.tsx)

### 6. 管理后台模块

这是面向内部运营和管理员的管理系统，用于维护平台运行状态、用户、密钥、订单、计费和配额数据。

当前页面包括：

- Dashboard
- Growth
- Hosted Pricing
- API Keys
- Users
- Orders
- Billing Events
- Free Quotas
- Usage Events

关联入口：

- [platform/web/admin/src/App.tsx](/home/ubuntu/workspace/officecli/platform/web/admin/src/App.tsx)
- [platform/web/admin/src/pages/DashboardPage.tsx](/home/ubuntu/workspace/officecli/platform/web/admin/src/pages/DashboardPage.tsx)

### 7. 平台后端与商业化控制平面模块

这是 `platform/` 的后端核心，负责承接授权、鉴权、计费、订单、用量、用户和后台管理等能力，是平台产品真正的控制中心。

当前主要子能力包括：

- License 校验与消耗
- Google 登录与会话管理
- 用户应用侧 API
- 管理后台 API
- Stripe 结账与 webhook
- API Key 管理
- Usage Events 记录
- PostgreSQL / Redis 存储

关联入口：

- [platform/README.md](/home/ubuntu/workspace/officecli/platform/README.md)
- [platform/internal/app](/home/ubuntu/workspace/officecli/platform/internal/app)
- [platform/internal/license](/home/ubuntu/workspace/officecli/platform/internal/license)
- [platform/internal/auth](/home/ubuntu/workspace/officecli/platform/internal/auth)
- [platform/internal/billing](/home/ubuntu/workspace/officecli/platform/internal/billing)
- [platform/internal/appuser](/home/ubuntu/workspace/officecli/platform/internal/appuser)
- [platform/internal/admin](/home/ubuntu/workspace/officecli/platform/internal/admin)

## 四、增长与生态模块

### 8. 增长激励模块

该模块围绕邀请、推荐、奖励额度和 Discord 连接展开，目标是把“用户增长”纳入平台能力中。

当前可见能力包括：

- 邀请码
- 推荐进度展示
- 奖励额度展示
- Discord 连接入口
- Growth 数据在用户端和管理端可见

需要注意：

- 这部分能力在仓库中属于“部分实现”
- 部分流程已有后端和页面，但仍不应视为完全商业化上线

关联入口：

- [docs/commercialization-status.md](/home/ubuntu/workspace/officecli/docs/commercialization-status.md)
- [docs/commercialization-rollout-status.md](/home/ubuntu/workspace/officecli/docs/commercialization-rollout-status.md)
- [platform/internal/growth](/home/ubuntu/workspace/officecli/platform/internal/growth)
- [platform/internal/reward](/home/ubuntu/workspace/officecli/platform/internal/reward)

### 9. Agent / 插件集成模块

该模块用于把 OfficeCLI 接入 Claude Code、OpenClaw 等智能体工作流，让产品不仅能被人直接使用，也能被其他 Agent 作为文档能力调用。

当前形式包括：

- Claude 插件封装
- OpenClaw 插件封装
- `agent-bridge` JSON-RPC 接口
- 本地 skill / plugin 工作流接入

关联入口：

- [plugins/officecli/README.md](/home/ubuntu/workspace/officecli/plugins/officecli/README.md)
- [plugins/openclaw-officecli/README.md](/home/ubuntu/workspace/officecli/plugins/openclaw-officecli/README.md)
- [docs/openclaw-user-quickstart.md](/home/ubuntu/workspace/officecli/docs/openclaw-user-quickstart.md)
- [internal/cli/app.go](/home/ubuntu/workspace/officecli/internal/cli/app.go)

### 10. 分发与安装渠道模块

该模块负责把产品分发到不同使用场景中，解决用户“如何安装、如何拿到可执行版本”的问题。

当前渠道包括：

- GitHub 二进制发布
- npm 包装器
- 公共 skills / plugin 仓库
- Homebrew 规划

关联入口：

- [docs/distribution-architecture.md](/home/ubuntu/workspace/officecli/docs/distribution-architecture.md)
- [packages/npm/officecli/README.md](/home/ubuntu/workspace/officecli/packages/npm/officecli/README.md)

## 五、推荐的产品分层表达

如果需要对外或对内统一描述，建议把 OfficeCLI 概括为以下三层：

### A. 核心生成层

- OfficeCLI 文档生成
- 文档评分与审查
- 发布与在线预览

### B. 平台控制层

- 官网与营销站
- 用户控制台
- 管理后台
- 授权、计费与控制平面后端

### C. 增长与生态层

- 邀请、推荐、奖励增长体系
- Agent / 插件接入
- npm / skills / 二进制分发

## 六、一句话总结

OfficeCLI 当前已经形成一个完整产品组合：前台是 AI Office 文档生成工具，中台是授权计费与控制平台，外围是增长机制与多渠道生态分发。
