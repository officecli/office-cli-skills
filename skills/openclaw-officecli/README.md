# OpenClaw OfficeCLI Skill

`openclaw-officecli` 让 OpenClaw 用户在现有 Telegram、Discord、Slack 等 channel 中，直接通过自然语言生成本地 `pptx`、`docx`、`xlsx` 文件。

这个 skill 的运行方式是：

- OpenClaw 负责理解用户消息、在聊天里完成补问，并把结果回传到 channel
- `officecli agent-bridge` 负责本地文档执行与结构化任务事件
- `officecli` 负责最终文件的生成、组装、落盘与可选发布
- agent 应优先读取 `initialize` / `capabilities/get`，并根据 `document_generation.pptx.image_support` 判断 PPT 图片能力

## 适用场景

- 在 Telegram / Discord / Slack 里直接说“生成一个五页的 PPT”
- 让 OpenClaw 通过多轮补问完善文档需求
- 生成成功后，把文件作为附件回传到聊天中

## 前置条件

1. 本机已安装并配置 OpenClaw
2. 本机已安装 `officecli`，或允许 skill 自动安装 `officecli`
3. 已完成 `officecli config set-generation` 与 `officecli config set-license`，或允许 skill 自动补齐这些配置
4. OpenClaw agent 具备：
   - 运行本地命令的能力
   - 读取本地文件的能力
   - 回传文件附件到当前 channel 的能力

## 安装

使用仓库内置安装脚本：

```bash
bash ./scripts/install-openclaw-skill.sh
```

默认会把 skill 安装到：

```bash
~/.openclaw/skills/openclaw-officecli
```

如需自定义 OpenClaw 根目录：

```bash
OPENCLAW_HOME=/opt/openclaw bash ./scripts/install-openclaw-skill.sh
```

## 配置

安装脚本会在 skill 目录下放置 `config.yaml`。默认字段如下：

- `office_cli_path`
- `agent_bridge_command`
- `default_mode`
- `default_output_format`
- `default_lang`
- `default_publish`

如果 `officecli` 已在 `PATH` 中，默认无需额外修改。

## 环境检查与修复

skill 目录现在内置两个脚本：

- `check-officecli-env.sh`
- `fix-officecli-env.sh`

推荐顺序：

```bash
bash ~/.openclaw/skills/openclaw-officecli/check-officecli-env.sh
bash ~/.openclaw/skills/openclaw-officecli/fix-officecli-env.sh
```

行为说明：

- 如果 `officecli` 不在 PATH，中间会尝试自动安装
- 如果只缺生成或额度配置，脚本会只补齐缺失项
- 如果你需要在线预览，再额外提供 publish 配置
- 修复成功后，会把 `office_cli_path` 和 `agent_bridge_command` 写回 skill 的 `config.yaml`

## 挂载到 agent

在 `~/.openclaw/config.yaml` 中，把 skill 名称加入目标 agent：

```yaml
agents:
  office-bot:
    model: openai/gpt-4o
    channels: [telegram]
    skills: [openclaw-officecli]
    tools: [shell, file_read]
```

如果当前 channel 需要附件上传，也请确保该 channel 已正确配置并具备发送文件权限。

## 用户使用方式

用户可以直接发送自然语言请求，例如：

- `生成一个 5 页的 PPT，介绍企业协作平台`
- `帮我写一个给客户的 docx，介绍我们的协作平台`
- `做一个项目预算 excel 表`

如果信息不完整，skill 应当把 `officecli agent-bridge` 的 `task.question` 转成聊天补问。

生成成功后，skill 应当：

1. 读取 `task.output.result.file_path`
2. 把对应文件上传为聊天附件
3. 在消息中补充文档类型、文件名和 warning
4. 如果 `result_meta.image_support.attention_required=true`，优先提示用户检查 `image_base_url`、`image_api_key`、`image_model`，或改用 `--no-images`

## PPT 图片约定

对所有接入这个 skill 的 agent，推荐统一遵循下面的桥接规则：

- `pptx` 默认允许自动配图，是否默认开启以 `document_generation.pptx.image_support.default_enabled` 为准
- 用户明确说“不要图片 / 纯文本版”时，应传 `enable_images=false`
- 用户问“为什么没图”时，应优先提示运行 `officecli config set-generation`
- 优先读取 `result_meta.image_support` 做程序判断，不要只靠 warning 文本猜测

## 调试

先确认本地 bridge 可以正常启动：

```bash
officecli agent-bridge
```

再确认 `officecli` 本身可用：

```bash
officecli --version
officecli auth status
```

如果要检查安装后的 skill 文件：

```bash
ls -la ~/.openclaw/skills/openclaw-officecli
cat ~/.openclaw/skills/openclaw-officecli/config.yaml
```
