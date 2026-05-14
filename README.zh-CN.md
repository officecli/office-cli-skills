# OfficeCLI：面向 Claude Code、Codex 和 AI Agent 的 Office 技能

`officecli` 是 OfficeCLI 的公开 GitHub 仓库，用来分发技能、插件封装、安装脚本和示例，方便 Claude Code、Codex、OpenClaw 以及其他本地 Agent 调用 OfficeCLI 生成 Office 文件。

主 README 仍然使用英文：[README.md](./README.md)

## 这个仓库提供什么

- Claude Code marketplace 元数据和插件封装
- 面向通用 Office 文档工作流的 `officecli` skill
- 面向 OpenClaw 集成的 `openclaw-officecli` skill
- Codex 等本地 Agent 可直接使用的安装脚本
- 可复现的生成示例，包括预览图、prompt、元数据和输出文件

这个仓库不是托管式 SaaS 插件后端，也不包含私有的 OfficeCLI 引擎源码。最终的 `pptx`、`docx`、`xlsx`、`report` 和 `img` 输出仍然由本机安装的 `officecli` runtime 生成。

## 快速链接

官网页面：

- [概览](https://officecli.io/officecli)
- [安装](https://officecli.io/officecli/install)
- [Claude Code](https://officecli.io/officecli/claude-code)
- [Codex](https://officecli.io/officecli/codex)
- [OpenClaw](https://officecli.io/officecli/openclaw)
- [FAQ](https://officecli.io/officecli/faq)

仓库内文档：

- [安装指南](./install/README.md)
- [生成示例](./demos/README.md)
- [Claude Code 安装](./claude-code/README.md)
- [Codex 安装](./codex/README.md)
- [OpenClaw 安装](./openclaw/README.md)
- [FAQ](./faq/README.md)

## 支持的工作流

公开的 `officecli` skill 面向这些 Agent 工作流：

- 生成 PPTX：演示文稿、方案、汇报和管理层简报
- 生成 DOCX：复盘、备忘录、客户文档和结构化草稿
- 生成 XLSX：表格、跟踪器、分析工作簿和数据模板
- 生成 report：基于工作簿生成报告类产物
- 生成 img：通过 `office.generate` 生成独立图片
- 执行前读取能力信息，让 Agent 判断 OfficeCLI 是否支持当前请求

## Claude Code 安装

```text
/plugin marketplace add officecli/officecli
/plugin install officecli@officecli
```

如需安装 OpenClaw 变体：

```text
/plugin install openclaw-officecli@officecli
```

## Codex 和其他本地 Agent 安装

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli/main/scripts/install-skill.sh | bash -s -- officecli
```

如果只安装 skill，不自动安装 `officecli` 二进制：

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli/main/scripts/install-skill.sh | AUTO_INSTALL_BINARY=0 bash -s -- officecli
```

## OpenClaw 安装

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli/main/scripts/install-openclaw-skill.sh | bash
```

安装后，把 `openclaw-officecli` 绑定到目标 OpenClaw agent，并确认 agent 有本地命令执行和文件读取权限。

## 本机要求

- 已安装本地 `officecli` 二进制
- 已完成 OfficeCLI generation 和 license 配置
- Agent 客户端有权限在同一台机器上调用本地命令
- OpenClaw 用户需要配置 `officecli agent-bridge`

安装后可以这样验证：

```bash
officecli --version
officecli config status
officecli agent-bridge
```

## 生成示例

示例目录见 [demos/README.md](./demos/README.md)。每个示例都包含预览图、输出文件、`prompt.md`、`metadata.json` 和可复现命令，方便确认实际生成效果。

## 常见问题

### 这个仓库是不是 OfficeCLI 的完整源码？

不是。这个公开仓库只分发 skill、插件封装、安装脚本和示例。OfficeCLI 的本地 runtime 负责实际生成文件。

### 安装这个仓库后就能直接生成文件吗？

需要本机存在可用的 `officecli` runtime，并完成必要配置。安装脚本会尝试安装二进制，但最终是否能生成取决于本机 runtime 和配置状态。

### Claude Code marketplace 和直接安装有什么区别？

Claude Code marketplace 是插件安装路径；Codex 等本地 Agent 可以直接使用脚本安装 skill 文件。两者最终都把 Office 任务路由到本机 OfficeCLI。
