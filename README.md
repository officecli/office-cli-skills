# OfficeCLI

OfficeCLI 是一个面向人类用户的命令行工具：你用自然语言描述你想要的内容，它会帮你生成 `PPTX / DOCX / XLSX` 文件，并在配置了发布端时自动返回在线预览地址。

其中 `PPTX` 生成默认会为合适页面自动配图并把图片嵌进最终文件；如果你只想生成纯文本版 PPT，可以显式加 `--no-images`。

## Claude marketplace 安装

本仓库现在同时包含可发布到 Claude Code plugin marketplace 的 skills 包装层：

- `officecli`：面向 Claude Code / agent 的通用 Office 文件技能
- `openclaw-officecli`：面向 OpenClaw 集成场景的 Office 文件技能

如果你把当前仓库发布到 GitHub，例如 `officecli/officecli-skills`，Claude Code 用户可以直接这样安装：

```text
/plugin marketplace add officecli/officecli-skills
/plugin install officecli@officecli-skills
```

如需安装 OpenClaw 版本：

```text
/plugin install openclaw-officecli@officecli-skills
```

本仓库里的 Claude marketplace 关键文件如下：

- `/.claude-plugin/marketplace.json`
- `/plugins/officecli/.claude-plugin/plugin.json`
- `/plugins/openclaw-officecli/.claude-plugin/plugin.json`

如果你想先在本地验证插件目录，也可以用 Claude Code 直接加载：

```bash
claude --plugin-dir ./plugins/officecli --plugin-dir ./plugins/openclaw-officecli
```

## 30 秒上手

如果你想先跑通一次，最短路径就是下面 4 步：

### 1）构建命令行工具

```bash
go build -o officecli ./cmd/officecli
```

### 2）准备配置文件

最简单的办法是依次完成这些配置命令：

```bash
./officecli config set-generation
./officecli config set-license
./officecli config set-publish   # 如需在线预览
./officecli config set-defaults  # 可选
```

你也可以随时查看当前配置状态：

```bash
./officecli config status
```

如果你更想手动复制，也可以：

```bash
mkdir -p ~/.config/officecli
cp ./examples/config.example.json ~/.config/officecli/config.json
```

然后把里面与你环境对应的服务地址、访问凭证、额度配置和发布配置改成你自己的。

### 3）执行一条命令

```bash
./officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景"
```

### 4）查看结果

- 本地文件默认在 `./output`
- 如果发布端已配置且启用，会同时返回在线访问地址和密码

如果你只想先验证本地生成是否正常，可以这样跑：

```bash
./officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景" --no-publish
```

如果你更习惯用一键命令，也可以直接：

```bash
make build
officecli config status
make run-help
```

---

## 常用命令速查

如果你只想快速抄命令，可以先看这一节。

### 快速生成一份 PPT

```bash
./officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景"
```

默认会尝试为合适页面自动配图，并把图片直接嵌入生成的 `pptx` 文件。

### 只生成本地文件，不发布预览

```bash
./officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景" --no-publish
```

### 用长提示词文件生成

```bash
./officecli new pptx "企业协作平台介绍" --prompt-file ./examples/prompt.txt
```

### 进入更高质量的补问模式

```bash
./officecli new pptx "企业协作平台介绍" --mode best
```

### 第一次使用时配置服务

```bash
./officecli config set-generation
./officecli config set-license
```

如需在线预览，再执行：

```bash
./officecli config set-publish
```

### 生成 Word 文档

```bash
./officecli new docx "季度复盘" "为管理层写一份季度项目复盘，强调结果、问题和下一步计划"
```

### 生成 Excel 表格

```bash
./officecli new xlsx "销售分析表" "生成一个包含地区、销售额、同比、负责人字段的季度销售分析表"
```

### 指定输出目录

```bash
./officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景" --out ./dist
```

### 输出 JSON，方便脚本调用

```bash
./officecli new pptx "企业协作平台介绍" --prompt-file ./examples/prompt.txt --json
```

### 评估一份本地 PPT 的质量

```bash
./officecli review pptx ./output/企业协作平台介绍.pptx
```

默认会先做结构检查；如果本机安装了 LibreOffice（`soffice`），还会自动补充一轮基于 PDF 的视觉评审。

如需只跑结构规则、不触发视觉评审：

```bash
./officecli review pptx ./output/企业协作平台介绍.pptx --no-visual
```

### 启动面向 agent 的 JSON-RPC bridge

```bash
./officecli agent-bridge
```

这个子命令会通过 `stdio` 暴露 `JSON-RPC 2.0` 接口，适合给 `codex-cli`、`claudecode` 这类 TUI agent 作为本地 bridge 使用。

### 查看当前授权状态

```bash
./officecli auth status
```

### 写入付费额度密钥

```bash
./officecli auth set-key <your-paid-key>
```

### Agent Bridge 方法

- `initialize`
- `capabilities/get`
- `session/open`
- `session/close`
- `task/invoke`
- `task/respond`
- `task/status`
- `task/cancel`

### Agent Bridge 协议用法

`agent-bridge` 走的是 `JSON-RPC 2.0 over stdio`，消息使用 `Content-Length` 分帧，适合本地 TUI agent 作为子进程连接。

最小初始化请求示例：

```text
Content-Length: 58

{"jsonrpc":"2.0","id":1,"method":"initialize"}
```

返回结果会包含：

- `server_name`
- `server_version`
- `protocol_version`
- `capabilities`
- `tools`

发起文档生成时，推荐调用 `task/invoke`，工具名固定为 `office.generate`。最小请求示例：

```text
Content-Length: 248

{"jsonrpc":"2.0","id":2,"method":"task/invoke","params":{"tool":"office.generate","interactive":false,"output_format":"json","args":{"document_type":"docx","topic":"企业协作平台介绍","prompt":"介绍这款企业协作平台","mode":"fast"}}}
```

bridge 会先返回 `task_id`，然后通过 `event` notification 持续推送结构化中间态。常见事件类型包括：

- `task.started`
- `task.progress`
- `task.question`
- `task.output`
- `task.completed`
- `task.failed`
- `task.cancelled`

如果要评估本地 PPT 质量，可调用 `office.review`，最小请求示例：

```text
Content-Length: 210

{"jsonrpc":"2.0","id":5,"method":"task/invoke","params":{"tool":"office.review","interactive":false,"output_format":"json","args":{"document_type":"pptx","file_path":"./output/demo.pptx","enable_visual":true}}}
```

其中 `task.progress` 的 `payload` 当前会包含这些兼容字段：

- `step`
- `status`
- `content`
- `active_content`
- `elapsed_ms`
- `duration_ms`
- `image_slide_count`
- `error`

如果是 `best` 模式且 `interactive=true`，bridge 会发出 `task.question`，这时应该调用 `task/respond` 回答，而不是像人类终端那样解析 stdout 文本提示。示例：

```text
Content-Length: 132

{"jsonrpc":"2.0","id":3,"method":"task/respond","params":{"task_id":"task-000001","question_id":"question-000003","option_id":"2"}}
```

取消执行时调用 `task/cancel`：

```text
Content-Length: 95

{"jsonrpc":"2.0","id":4,"method":"task/cancel","params":{"task_id":"task-000001"}}
```

建议：

- 对 agent client 优先使用 `agent-bridge`
- 不要把人类 CLI 的 spinner / 行文本当协议
- 用 `task/status` 做状态拉取兜底，用 `event` 作为主事件流
- 后续对接 `openclaw channel` 时，优先复用同一套方法名和事件 envelope，只替换 transport

### 面向 codex-cli / claudecode 的完整对接样例

下面这段更接近真实的 TUI agent 集成顺序，适合 `codex-cli`、`claudecode` 这类“本地起子进程 + 持续收发 stdio 消息”的场景。

1. 启动 bridge 进程

```bash
./officecli agent-bridge
```

2. 发送 `initialize`

```text
Content-Length: 58

{"jsonrpc":"2.0","id":1,"method":"initialize"}
```

3. 可选地打开一个 session

```text
Content-Length: 59

{"jsonrpc":"2.0","id":2,"method":"session/open"}
```

返回示例：

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "id": "session-000001",
    "created_at": "2026-04-03T15:00:00Z"
  }
}
```

4. 发起生成任务

```text
Content-Length: 312

{"jsonrpc":"2.0","id":3,"method":"task/invoke","params":{"session_id":"session-000001","tool":"office.generate","interactive":true,"output_format":"json","args":{"document_type":"pptx","topic":"企业协作平台介绍","prompt":"介绍这款企业协作平台的产品能力、客户价值与应用场景","mode":"best","lang":"zh-CN"}}}
```

bridge 会先返回任务确认：

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "task_id": "task-000002",
    "session_id": "session-000001",
    "status": "running"
  }
}
```

然后会持续收到 `event` notification。例如：

```json
{
  "jsonrpc": "2.0",
  "method": "event",
  "params": {
    "event_id": "event-000003",
    "session_id": "session-000001",
    "request_id": "3",
    "task_id": "task-000002",
    "type": "task.started",
    "ts": "2026-04-03T15:00:01Z",
    "payload": {
      "tool": "office.generate",
      "document_type": "pptx",
      "mode": "best",
            "interactive": true
    }
  }
}
```

后续的进度事件通常长这样：

```json
{
  "jsonrpc": "2.0",
  "method": "event",
  "params": {
    "event_id": "event-000004",
    "session_id": "session-000001",
    "request_id": "3",
    "task_id": "task-000002",
    "type": "task.progress",
    "ts": "2026-04-03T15:00:02Z",
    "payload": {
      "step": "license",
      "status": "completed",
      "content": "授权校验完成"
    }
  }
}
```

如果任务进入补问，bridge 会发 `task.question`：

```json
{
  "jsonrpc": "2.0",
  "method": "event",
  "params": {
    "event_id": "event-000005",
    "session_id": "session-000001",
    "request_id": "3",
    "task_id": "task-000002",
    "type": "task.question",
    "ts": "2026-04-03T15:00:03Z",
    "payload": {
      "id": "question-000006",
      "question": "这份演示稿更偏销售介绍还是内部汇报？",
      "options": [
        {"id": "1", "label": "销售介绍"},
        {"id": "2", "label": "内部汇报"}
      ],
      "allow_freeform": true
    }
  }
}
```

这时 TUI client 应该暂停当前任务 UI，收集回答，再发送 `task/respond`：

```text
Content-Length: 143

{"jsonrpc":"2.0","id":4,"method":"task/respond","params":{"task_id":"task-000002","question_id":"question-000006","option_id":"1"}}
```

如果你需要轮询兜底，可以调用：

```text
Content-Length: 92

{"jsonrpc":"2.0","id":5,"method":"task/status","params":{"task_id":"task-000002"}}
```

任务完成后，一般会先收到 `task.output`，再收到 `task.completed`。例如：

```json
{
  "jsonrpc": "2.0",
  "method": "event",
  "params": {
    "event_id": "event-000007",
    "session_id": "session-000001",
    "request_id": "3",
    "task_id": "task-000002",
    "type": "task.output",
    "ts": "2026-04-03T15:00:10Z",
    "payload": {
      "format": "json",
      "result": {
        "status": "success",
        "file_path": "/abs/path/output/企业协作平台介绍.pptx",
        "document_type": "pptx",
        "document_name": "企业协作平台介绍.pptx"
      }
    }
  }
}
```

### TUI client 实现建议

- 把 `event` 当主数据流，把请求响应当控制面，不要反过来设计
- `task.started` 后就可以在 UI 上创建任务卡片，不必等 `task.completed`
- `task.progress` 适合渲染成阶段状态，而不是百分比进度条
- 收到 `task.question` 时，把任务状态切成 `waiting_input`
- `task/output` 到来时就可以准备展示文件路径、下载入口或后续动作
- `task/completed` 才算最终完成，`task/output` 不等于生命周期结束
- 如果用户中断当前任务，优先发 `task/cancel`，不要直接杀桥接进程
- 如果 bridge 重启或通知丢失，再用 `task/status` 做恢复

### OpenClaw 用户安装与使用

如果你的目标是让 OpenClaw 用户在 Telegram / Discord / Slack 等 channel 中直接使用 `officecli`，本仓库现在提供了一个本地同机运行的 OpenClaw skill 包。

安装步骤：

1. 安装并配置 `officecli`

```bash
bash ./scripts/install-openclaw-skill.sh
bash ~/.openclaw/skills/openclaw-officecli/check-officecli-env.sh
bash ~/.openclaw/skills/openclaw-officecli/fix-officecli-env.sh
```

如果本机还没有 `officecli`，或者只缺生成/额度配置，`fix-officecli-env.sh` 会优先尝试自动安装并补齐缺失项；只有在线预览发布配置仍然保持可选。

2. 安装 OpenClaw skill

```bash
bash ./scripts/install-openclaw-skill.sh
```

或者直接从公共 skills 仓安装：

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-openclaw-skill.sh | bash
```

如只想安装 OpenClaw skill、不自动安装 `officecli` 二进制：

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-openclaw-skill.sh | AUTO_INSTALL_BINARY=0 bash
```

默认会安装到：

```bash
~/.openclaw/skills/openclaw-officecli
```

3. 把 skill 挂到你的 OpenClaw agent：

```yaml
agents:
  office-bot:
    model: openai/gpt-4o
    channels: [telegram]
    skills: [openclaw-officecli]
    tools: [shell, file_read]
```

4. 重启 OpenClaw

5. 在聊天里直接发送自然语言请求，例如：

- `生成一个五页的 PPT，介绍企业协作平台`
- `帮我写一份给客户的 docx，介绍我们的协作平台`
- `做一个项目预算 excel 表`

运行时约定：

- OpenClaw skill 通过 `officecli agent-bridge` 与本地 `officecli` 通信
- skill 会先执行环境检查，必要时自动修复 `officecli` 安装与基础配置
- skill 会把 `task.question` 转成聊天补问
- 生成完成后，skill 应当把 `result.file_path` 对应文件作为附件回传

快速开始文档见：[docs/openclaw-user-quickstart.md](/Users/luyang/workspace/shimo/void-oversea/officecli/docs/openclaw-user-quickstart.md)

### 查看“使用限制”测试资产

- 测试用例清单：`docs/usage-limits-test-cases.md`
- 测试报告：`docs/usage-limits-test-report.md`
- 联调 / E2E 指南：`docs/usage-limits-e2e.md`
- 商业化 4-lane 现状 / blocker 盘点：`docs/commercialization-rollout-status.md`
- Discord / growth / analytics 当前真实能力边界：`docs/platform-go-live-blockers.md`

---

你可以把它理解成：

- 用一句话生成 Office 文档
- 既能本地落盘，也能自动发布预览
- 默认带免费额度校验，额度耗尽后可切换到付费额度密钥
- 简单需求一条命令直接完成
- 复杂需求可以进入补问模式，把内容补完整再生成

---

## 适合谁用

如果你经常遇到下面这些场景，这个工具会很顺手：

- 需要快速做一版 PPT 初稿
- 需要根据一句需求生成 Word 文档或 Excel 表格
- 想把生成流程接进脚本、CI 或内部自动化系统
- 既想保留本地文件，也想拿到在线访问地址发给别人看

---

## 安装

在仓库根目录执行：

```bash
go build -o officecli ./cmd/officecli
```

生成的可执行文件就是 `officecli`。

如果你希望全局使用，可以把它放到你的 `PATH` 里，例如：

```bash
mv ./officecli /usr/local/bin/officecli
```

然后就可以直接运行：

```bash
officecli
```

如果你在本地反复调试，也可以直接用仓库自带的 `Makefile`：

```bash
make build
officecli config status
make test
make run-help
make install
```

如果你想给构建产物打版本号，可以这样：

```bash
make build VERSION=0.1.0
./officecli --version
```

---

## 快速开始

### 1）生成一个 PPT

```bash
officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景"
```

典型输出：

```text
生成完成！已保存至 /workspace/output/企业协作平台介绍.pptx
在线访问地址：https://claudeoffice.com/preview/t/8908f910-6e2d-485a-a490-86642425659c；访问密码：123456
```

如果你没有配置发布端，也没关系，工具会只保存本地文件，并提示已跳过在线预览。

### 2）生成一个 Word 文档

```bash
officecli new docx "季度复盘" "为管理层写一份季度项目复盘，强调结果、问题和下一步计划"
```

### 3）生成一个 Excel 表格

```bash
officecli new xlsx "销售分析表" "生成一个包含地区、销售额、同比、负责人字段的季度销售分析表"
```

---

## 命令格式

```bash
officecli new <pptx|docx|xlsx> <topic> [brief]
```

参数含义：

- `<pptx|docx|xlsx>`：文档类型
- `<topic>`：文档主题或标题
- `[brief]`：可选的补充说明

最常见的理解方式是：

- `topic` 负责“这份文档是什么”
- `brief` 负责“你希望它重点讲什么”

比如：

```bash
officecli new pptx "AI 出海方案" "面向管理层，重点讲市场机会、竞争格局和落地路径"
```

---

## 常用选项

### `--prompt`

直接指定完整提示词。

```bash
officecli new pptx "企业协作平台介绍" --prompt "做一份 6 页以内的公司介绍 PPT，受众是潜在企业客户，风格专业克制"
```

### `--prompt-file`

从文件读取提示词，适合长内容。

```bash
officecli new docx "项目方案" --prompt-file ./prompt.txt
```

### `--mode fast|best`

控制生成策略：

- `fast`：直接生成，适合信息已经比较完整的情况
- `best`：必要时进入补问，适合你只有一个大致方向、希望工具帮你把内容补全

例如：

```bash
officecli new pptx "企业协作平台介绍" --mode best
```

### `--lang`

指定语言。

```bash
officecli new pptx "Collaboration Platform Overview" --lang en --prompt "Create a company overview deck for enterprise buyers"
```

### `--style`

指定风格，例如：`正式`、`专业`、`简洁`、`销售导向`。

```bash
officecli new pptx "产品发布会" --style "简洁有说服力"
```

### `--audience`

指定受众，例如：`管理层`、`客户`、`投资人`、`内部团队`。

```bash
officecli new docx "年度总结" --audience "管理层"
```

### `--out`

指定输出目录。

```bash
officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景" --out ./dist
```

### `--publish` / `--no-publish`

控制是否发布在线预览。

- `--publish`：即使默认关闭，也强制发布
- `--no-publish`：即使默认开启，也只保留本地文件

例如：

```bash
officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景" --no-publish
```

### `--no-images`

关闭 PPT 自动配图。

- 默认情况下，`pptx` 会为合适的内容页自动生成图片并嵌入文件
- 加上 `--no-images` 后，会退回纯文本/图表版 PPT
- `docx/xlsx` 会接受这个参数，但不会启用图片链路

例如：

```bash
officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景" --no-images
```

### `--json`

输出机器可读 JSON，适合脚本或 CI。

```bash
officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景" --json
```

示例输出：

```json
{
  "status": "success",
  "file_path": "/workspace/output/企业协作平台介绍.pptx",
  "document_type": "pptx",
  "document_name": "企业协作平台介绍.pptx",
  "published": true,
  "access_url": "https://claudeoffice.com/preview/t/xxx",
  "password": "123456",
  "warnings": []
}
```

---

## 输入优先级

如果你同时提供了多种输入方式，工具会按下面的优先级选择内容：

1. `--prompt`
2. `--prompt-file`
3. stdin
4. 位置参数里的 `[brief]`
5. 最后兜底为 `<topic>`

也就是说，下面这条命令会优先使用 `--prompt` 的内容：

```bash
echo "来自 stdin 的内容" | officecli new pptx "主题" "位置参数内容" --prompt-file ./prompt.txt --prompt "最终采用这段提示词"
```

---

## 补问模式怎么工作

当你使用 `--mode best` 时，工具会尽量先理解你的需求。

如果信息不够，它会在终端里继续问你一些问题，例如：

- 这份文档的目标是什么
- 面向谁
- 更偏汇报、提案还是介绍
- 内容应该偏保守还是偏说服

适合这种情况：

- 你只知道“想做一份什么”，但还没把信息整理完整
- 你希望最终结果更贴近真实使用场景
- 你愿意多花几十秒换更好的首稿质量

如果你在非交互环境里跑 `--mode best`，工具不会卡住等待输入，而是会明确报错，提醒你改用 `--mode fast` 或在终端中执行。

---

## 用文件或 stdin 输入长提示词

### 用文件输入

`prompt.txt`

```text
请生成一份企业协作平台产品介绍 PPT：
- 面向中大型企业客户
- 重点讲协作能力、权限管理、知识沉淀和安全性
- 风格专业、克制、结论先行
- 控制在 6 页以内
```

仓库里也提供了现成示例：

```text
./examples/prompt.txt
```

如果你想看不同文档类型的例子，也可以直接用这些：

```text
./examples/prompt.txt
./examples/docx-prompt.txt
./examples/xlsx-prompt.txt
```

命令：

```bash
officecli new pptx "企业协作平台介绍" --prompt-file ./examples/prompt.txt
```

### 用 stdin 输入

```bash
cat ./prompt.txt | officecli new pptx "企业协作平台介绍"
```

---

## 配置

第一次使用时，推荐先依次完成：

```bash
officecli config set-generation
officecli config set-license
```

如果需要在线预览，再继续执行：

```bash
officecli config set-publish
```

如果你不希望交互，也可以直接写入配置文件，或者在后续版本中通过更细粒度的 `config` 子命令完成配置。

工具支持三层配置来源，优先级从低到高大致可以理解为：

1. 配置文件
2. 环境变量
3. 命令行 flags

默认配置文件路径会因操作系统不同而不同：

```text
macOS: ~/Library/Application Support/officecli/config.json
Linux: ~/.config/officecli/config.json
Windows: %AppData%\officecli\config.json
```

你也可以通过环境变量指定配置文件：

```bash
export OFFICE_CLI_CONFIG=/path/to/config.json
```

### 配置文件示例

仓库里也提供了一份可直接复制的示例文件：

```text
./examples/config.example.json
```

这份示例文件仍然保留了当前版本的底层字段结构，适合高级用户或内部联调用途。普通用户只需要通过 `officecli config ...` 系列命令完成配置，不需要理解底层字段命名。

如果你运行 `officecli new ...` 时还没有完成生成服务配置，工具也会直接提示你先执行：

```bash
officecli config set-generation
```

### 常用配置层级

对普通用户来说，主要只需要关注三类配置：

- 生成服务接入
- 额度 / 授权服务接入
- 在线预览发布接入

发布相关：

- `OFFICE_CLI_PUBLISH_PROVIDER`
- `OFFICE_CLI_PUBLISH_BASE_URL`
- `OFFICE_CLI_PUBLISH_API_KEY`
- `OFFICE_CLI_PUBLISH_ENABLED`
- `OFFICE_CLI_PUBLISH_TIMEOUT_SEC`

默认行为相关：

- `OFFICE_CLI_OUTPUT_DIR`
- `OFFICE_CLI_MODE`
- `OFFICE_CLI_DEFAULT_PUBLISH`

---

## 推荐使用方式

### 场景一：我只想快速出一版

直接用：

```bash
officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景"
```

### 场景二：我想控制生成质量

建议加上：

```bash
officecli new pptx "企业协作平台介绍" --mode best --audience "企业客户" --style "专业"
```

### 场景三：我要接脚本或 CI

建议用：

```bash
officecli new pptx "企业协作平台介绍" --prompt-file ./examples/prompt.txt --json
```

### 场景四：我只要本地文件，不要在线预览

```bash
officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景" --no-publish
```

---

## 常见问题

### 为什么没有返回在线访问地址？

通常有两种情况：

- 你没有配置发布端
- 你显式传了 `--no-publish`

这时工具仍然会生成并保存本地文件。

### 为什么 `best` 模式报错？

如果你是在非交互环境里执行，比如脚本、CI 或管道里，`best` 模式无法继续补问。

解决方法：

- 改用 `--mode fast`
- 或者在终端里直接运行，让工具能够和你交互

### 文件会保存到哪里？

默认保存到：

```text
./output
```

你也可以用 `--out` 或配置文件里的 `defaults.output_dir` 覆盖。

### `auth set-key` 一定要把付费额度密钥写在命令参数里吗？

不一定。

你可以直接这样执行：

```bash
officecli auth set-key
```

工具会在终端里提示你输入付费额度密钥，这样比把密钥直接写进命令历史更友好。

---

## examples 目录说明

仓库里的 `examples/` 目录是给第一次上手的人准备的，可以直接复制或修改：

- `examples/config.example.json`：最小可用配置模板
- `examples/prompt.txt`：PPT 示例提示词
- `examples/docx-prompt.txt`：Word 示例提示词
- `examples/xlsx-prompt.txt`：Excel 示例提示词

你可以直接这样试：

```bash
./officecli new pptx "企业协作平台介绍" --prompt-file ./examples/prompt.txt
./officecli new docx "季度复盘" --prompt-file ./examples/docx-prompt.txt
./officecli new xlsx "销售分析表" --prompt-file ./examples/xlsx-prompt.txt
```

---

## Makefile 快捷命令

仓库根目录提供了一份 `Makefile`，适合开发、联调和演示时直接使用。

常用命令：

```bash
make help
make build
officecli config status
make test
make install
make uninstall
make run-help
make demo
make release VERSION=0.1.0
make demo-ppt
make demo-docx
make demo-xlsx
```

它们分别用于：

- `make build`：构建 `officecli`
- `officecli config status`：查看当前配置状态
- `make test`：运行全部测试
- `make install`：安装到系统二进制目录，默认是 `/usr/local/bin`
- `make uninstall`：移除已安装的二进制
- `make run-help`：显示 CLI 帮助
- `make demo`：运行一套本地演示流程
- `make release VERSION=0.1.0`：构建本地验证用的发布产物到 `dist/`
- `make demo-ppt`：用 PPT 示例提示词跑一遍
- `make demo-docx`：用 Word 示例提示词跑一遍
- `make demo-xlsx`：用 Excel 示例提示词跑一遍

如果你想安装到自定义目录，也可以覆盖变量：

```bash
make install PREFIX=$HOME/.local
```

或者：

```bash
make install BIN_DIR=$HOME/bin
```

如果你想构建带版本号的二进制，也可以覆盖 `VERSION`：

```bash
make build VERSION=0.1.0
```

这样运行：

```bash
./officecli --version
```

会看到类似：

```text
officecli version 0.1.0
```

---

## 本地演示

如果你的配置已经准备好，想快速跑一遍完整演示，可以直接执行：

```bash
make demo
```

它会依次执行：

- `--version`
- `--help`
- PPT 示例
- DOCX 示例
- XLSX 示例

默认都使用 `--no-publish`，更适合先验证本地生成链路。

---

## 发布版本

如果你想在本地先验证多平台构建产物，可以直接执行：

```bash
make release VERSION=0.1.0
```

默认会在 `dist/` 目录下生成这些二进制：

- `officecli_0.1.0_darwin_amd64`
- `officecli_0.1.0_darwin_arm64`
- `officecli_0.1.0_linux_amd64`
- `officecli_0.1.0_linux_arm64`

这些本地构建产物会同时注入：

- 版本号
- git commit 短 SHA
- 构建时间（UTC）

你可以用下面的命令验证本机构建版本：

```bash
./officecli --version
```

正式对外分发不直接依赖当前仓库的源码公开发布，而是采用：

- 私有源码仓构建
- 公共分发仓承载 release 资产
- 公共 Homebrew tap 仓承载 formula
- 公共 skills 仓同步 `skills/`

当前对应的公共仓分别是：

- `officecli/officecli-dist`
- `officecli/homebrew-officecli`
- `officecli/officecli-skills`

维护细节见：

```text
./docs/distribution-architecture.md
```

仓库里也提供了首个变更记录文件：

```text
./CHANGELOG.md
```

---

## 面向开发者的说明

如果你是要继续扩展这个项目，当前代码大致分为几层：

- `cmd/officecli`：命令入口
- `internal/cli`：参数解析、配置、交互、结果输出
- `internal/runtime`：把生成结果组装成实际 Office 文件
- `internal/providers`：对接外部服务的 provider 层
- `engine`：工作流与核心接口
- `pkg/officegen`：OOXML 生成
- `pkg/ooxmledit`：OOXML 修改

如果你只是想使用工具，看到这里就够了。
