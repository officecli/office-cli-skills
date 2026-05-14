# OfficeCLI

OfficeCLI 是一个命令行工具，可以用自然语言 prompt 生成 Office 文件和独立图片。直接使用 `officecli` 二进制即可在终端、脚本、CI 或本地自动化流程中生成 `PPTX`、`DOCX`、`XLSX`、基于工作簿的 `Report` 和 `img` 输出。

这个仓库是 OfficeCLI 二进制的公开安装和文档入口，同时也包含 Claude Code、Codex 风格本地 Agent 和 OpenClaw 的可选 skill 封装。AI Agent 使用 OfficeCLI 是非重点功能；主入口仍然是 `officecli` 命令本身。

主 README 仍然使用英文：[README.md](./README.md)

## 使用 npm 安装

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

示例目录见 [demos/README.md](./demos/README.md)。每个示例都包含预览图、输出文件、`prompt.md`、`metadata.json` 和可复现命令，方便确认实际生成效果。

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
