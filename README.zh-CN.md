# OfficeCLI 是面向 PPTX、DOCX、XLSX、报告和图片的 AI 文档生成 CLI

[![GitHub Release](https://img.shields.io/github/v/release/officecli/officecli-dist?label=release)](https://github.com/officecli/officecli-dist/releases)
[![npm](https://img.shields.io/npm/v/officecli?label=npm)](https://www.npmjs.com/package/officecli)
[![License](https://img.shields.io/github/license/officecli/officecli)](./LICENSE)
[![Website](https://img.shields.io/badge/website-officecli.io-0f766e)](https://officecli.io/officecli)
[![OfficeDex](https://img.shields.io/badge/桌面端-OfficeDex.ai-5645d4)](https://officedex.ai)
[![Discord](https://img.shields.io/badge/community-Discord-5865F2)](https://discord.gg/ezAHMkdG)
[![X](https://img.shields.io/badge/follow-%40officecli-000000?logo=x)](https://x.com/officecli)

OfficeCLI 是一款命令行工具，可以把自然语言 prompt 生成可编辑的 Office 文件和独立图片。你可以在终端、脚本、CI 或本地自动化流程中直接使用一个 `officecli` 二进制生成 `PPTX`、`DOCX`、`XLSX`、基于工作簿的 `Report` 和 `img` 输出。首次运行默认可用 hosted trial；如果要接入自己的模型 endpoint，可以切换到 External Mode。

- 官网：[officecli.io/officecli](https://officecli.io/officecli) · [officedex.ai](https://officedex.ai)
- 桌面端：[OfficeDex — AI 原生文档工作台](https://officedex.ai)
- 英文 README：[README.md](./README.md)
- 生成示例：[demos/README.md](./demos/README.md)
- 可选 Agent 集成：[Claude Code](./claude-code/README.md)、[Codex](./codex/README.md)、[OpenClaw](./openclaw/README.md)
- 社区：[Discord](https://discord.gg/ezAHMkdG)

## 一行安装

npm 是大多数用户的推荐安装方式：

```bash
npm install -g officecli
```

验证二进制：

```bash
officecli --version
officecli auth status
```

如果不适合使用 npm，可以选择其它安装方式。

直接安装最新 release 二进制：

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-dist/main/scripts/install-officecli.sh | bash
```

或者使用 Homebrew：

```bash
brew tap officecli/officecli
brew install officecli
```

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

## 用 CLI 生成文件

默认提供 Hosted anonymous trial access，首次运行不需要本地模型 endpoint 或 API key。

生成 PPTX：

```bash
officecli new pptx "Q3 Business Review" --prompt "Create a six-slide executive deck for a SaaS quarterly business review. Cover growth, retention, risks, and next-quarter actions."
```

生成 DOCX：

```bash
officecli new docx "Product Launch Brief" --prompt "Write a concise launch brief with audience, positioning, timeline, risks, and next steps."
```

生成 XLSX：

```bash
officecli new xlsx "Sales Pipeline" --prompt "Create a sales pipeline workbook with stages, owners, deal values, probability, and next action columns."
```

基于工作簿生成 Report：

```bash
officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --prompt "Summarize regional revenue shifts and the board-level decision points."
```

生成独立图片：

```bash
officecli new img "Launch Visual" --prompt "Create a polished product launch hero image for an enterprise collaboration platform." --ratio landscape --reference-image ./reference.png
```

生成文件默认写入 `./output`。可以用 `--out <dir>` 指定输出目录，用 `--json` 输出机器可读结果，用 `--no-publish` 关闭在线发布。

生成 `PPTX` 时，OfficeCLI 会默认生成并嵌入合适的图片；如果只想要纯文本 deck，使用 `--no-images`。

## 适合哪些搜索和场景

| 搜索意图 | OfficeCLI 能力 |
| --- | --- |
| AI PPTX generator | 从 prompt 生成可编辑 PowerPoint，默认支持图片，也支持 `--no-images` 纯文本 deck。 |
| DOCX generator CLI | 生成可编辑 Word 文档，适合 brief、memo、proposal 和客户文档。 |
| XLSX automation | 生成 workbook、tracker、dashboard 和分析表格。 |
| Report generation | 以现有 workbook 为数据来源，生成可阅读的工作簿报告。 |
| Image generation CLI | 使用 `--ratio square\|landscape\|portrait` 和可选 reference image 生成独立图片。 |
| 本地优先文档自动化 | 一个二进制即可在终端、脚本、CI 或 Agent workflow 中运行。 |

## Runtime 和访问模式

查看当前访问模式：

```bash
officecli auth status
```

Hosted trial 用完后，可以从 https://officecli.io/pricing 创建或购买 hosted key，然后保存：

```bash
officecli auth set-key <api-key>
```

如果要使用自己的模型 endpoint，可以切换到 External Mode 并初始化生成配置：

```bash
officecli config set-runtime external
officecli config set-generation
```

配置可选在线发布：

```bash
officecli config set-publish
```

查看当前配置：

```bash
officecli config status
```

## 二进制支持的能力

- 生成 PPTX：演示文稿、方案、汇报和管理层简报
- 生成 DOCX：复盘、备忘录、客户文档和结构化草稿
- 生成 XLSX：表格、跟踪器、分析工作簿和数据模板
- 生成 report：基于工作簿生成报告类产物
- 生成 img：支持 `--ratio square|landscape|portrait`
- 独立图片支持 `--reference-image`
- 使用 `--no-publish` 只保留本地输出
- 使用 `officecli score pptx <file>` 对本地 PPTX 做结构评分

兼容别名仍然可用：

```bash
officecli review pptx ./output/Enterprise-Collaboration-Platform.pptx
```

## 生成示例

这些示例把可视预览、生成文件、`prompt.md`、`metadata.json` 和可复现命令放在一起，方便确认 OfficeCLI 的实际输出效果。

| AI PPTX、DOCX、XLSX、report 和 image 示例 | 预览 | 文件 |
| --- | --- | --- |
| 带图 AI PPTX generator | ![AI PPTX generator preview created by OfficeCLI](./demos/pptx-image-rich/preview.png) | [PPTX](./demos/pptx-image-rich/image-rich-strategy-deck.pptx) · [Prompt](./demos/pptx-image-rich/prompt.md) · [Metadata](./demos/pptx-image-rich/metadata.json) |
| 纯文本 AI PPTX generator | ![Text-only AI PPTX generator preview created by OfficeCLI](./demos/pptx-text-only/preview.png) | [PPTX](./demos/pptx-text-only/text-only-executive-briefing.pptx) · [Prompt](./demos/pptx-text-only/prompt.md) · [Metadata](./demos/pptx-text-only/metadata.json) |
| DOCX generator CLI | ![DOCX generator CLI preview created by OfficeCLI](./demos/docx-brief/preview.png) | [DOCX](./demos/docx-brief/officecli-customer-brief.docx) · [Prompt](./demos/docx-brief/prompt.md) · [Metadata](./demos/docx-brief/metadata.json) |
| XLSX automation dashboard | ![XLSX automation preview created by OfficeCLI](./demos/xlsx-dashboard/preview.png) | [XLSX](./demos/xlsx-dashboard/demo-adoption-dashboard.xlsx) · [Prompt](./demos/xlsx-dashboard/prompt.md) · [Metadata](./demos/xlsx-dashboard/metadata.json) |
| workbook-backed report generation | ![Report generation preview created by OfficeCLI](./demos/report-workbook/preview.png) | [HTML report](./demos/report-workbook/demo-program-readiness-report.html) · [Source XLSX](./demos/report-workbook/demo-program-source-workbook.xlsx) · [Prompt](./demos/report-workbook/prompt.md) |
| image generation CLI | ![Image generation CLI preview created by OfficeCLI](./demos/standalone-img/preview.png) | [PNG](./demos/standalone-img/officecli-hero-image.png) · [Prompt](./demos/standalone-img/prompt.md) · [Metadata](./demos/standalone-img/metadata.json) |

完整可复现命令见 [demos/README.md](./demos/README.md)。

## 可选 AI Agent 集成

当你希望 AI Agent 自动把 Office 任务路由到本地 OfficeCLI 二进制时，可以安装 public skills。这是可选能力；主使用方式仍然是直接执行 `officecli` 命令。

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
- 通过 hosted trial、hosted key 或 External Mode 获得 OfficeCLI 访问能力
- 如果需要在线预览，配置 publish
- 如果使用 Agent 集成，Agent 客户端需要有权限在同一台机器上调用本地命令

安装后可以这样验证：

```bash
officecli --version
officecli config status
officecli agent-bridge
```

## 常见问题

### 必须通过 AI Agent 才能用 OfficeCLI 吗？

不需要。主流程是直接使用 `officecli new pptx`、`officecli new docx`、`officecli new xlsx`、`officecli new report` 和 `officecli new img`。

### 安装这个仓库后就能直接生成文件吗？

需要本机存在可用的 `officecli` 二进制，并具备 hosted trial、hosted key 或 External Mode 配置。Agent skills 只是把任务路由到本地二进制。

### Claude Code marketplace 和直接安装有什么区别？

Claude Code marketplace 是可选插件安装路径；Codex 等本地 Agent 可以直接使用脚本安装 skill 文件。两者最终都把 Office 任务路由到本机 OfficeCLI。

### 这个仓库是不是 OfficeCLI 的完整源码？

不是。这个公开仓库包含安装文档、示例、脚本、skill 定义和插件封装。
