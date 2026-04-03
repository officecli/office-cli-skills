# OpenClaw 用户快速开始

这份文档面向最终用户，目标是在现有 OpenClaw channel 中直接使用 `officecli` 生成 `pptx`、`docx`、`xlsx` 文件。

## 前置条件

你需要：

1. 已安装 OpenClaw
2. 已配置至少一个可用 channel，例如 Telegram / Discord / Slack
3. 已安装 `officecli`
4. 已执行过：

```bash
officecli config set-generation
officecli config set-license
```

## 第一步：安装 OpenClaw skill

在本仓库根目录运行：

```bash
bash ./scripts/install-openclaw-skill.sh
```

默认会安装到：

```bash
~/.openclaw/skills/openclaw-officecli
```

安装后你会看到：

- `SKILL.md`
- `manifest.yaml`
- `config.yaml`
- `agent-config.example.yaml`

## 第二步：确认本地命令可用

确认 `officecli` 可以直接运行：

```bash
officecli --version
officecli auth status
```

确认本地 bridge 可以启动：

```bash
officecli agent-bridge
```

看到进程进入等待状态即可，按 `Ctrl+C` 退出。

## 第三步：把 skill 挂到你的 OpenClaw agent

编辑 `~/.openclaw/config.yaml`，把 skill 加入目标 agent：

```yaml
agents:
  office-bot:
    model: openai/gpt-4o
    channels: [telegram]
    skills: [openclaw-officecli]
    tools: [shell, file_read]
```

如果你的 agent 已经存在，只需要把 `openclaw-officecli` 加进 `skills` 列表。

## 第四步：重启或重新加载 OpenClaw

根据你的部署方式重启 OpenClaw，使新 skill 生效。

## 第五步：开始使用

在你的 channel 中直接发送：

- `生成一个五页的 PPT，介绍企业协作平台`
- `帮我写一份给客户的 docx，介绍我们的协作平台`
- `做一个项目预算 excel 表`

## 交互行为

默认行为：

- skill 会把请求路由到 `officecli agent-bridge`
- 如果信息足够，直接开始生成
- 如果需要更多信息，会在聊天里继续补问
- 生成完成后，文件应当作为附件回传到当前 channel

## 常见问题

### 1. skill 已安装，但 agent 没反应

检查：

- `~/.openclaw/config.yaml` 里的 agent 是否挂载了 `openclaw-officecli`
- `officecli` 是否在 `PATH` 中
- 当前 agent 是否允许本地命令与文件读取

### 2. 只返回文本，没有附件

检查当前 channel 是否支持附件上传，并确认 OpenClaw agent 对该 channel 拥有发送文件权限。

### 3. bridge 报配置错误

通常说明 `officecli config set-generation` 尚未完成，或者 `config.json` 里缺少生成服务 / 平台服务配置。

## 相关文件

- OpenClaw skill 说明：[skills/openclaw-officecli/README.md](/Users/luyang/workspace/shimo/void-oversea/officecli/skills/openclaw-officecli/README.md)
- OpenClaw skill 定义：[skills/openclaw-officecli/SKILL.md](/Users/luyang/workspace/shimo/void-oversea/officecli/skills/openclaw-officecli/SKILL.md)
- OpenClaw skill 配置示例：[skills/openclaw-officecli/config.example.yaml](/Users/luyang/workspace/shimo/void-oversea/officecli/skills/openclaw-officecli/config.example.yaml)
