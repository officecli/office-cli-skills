# OpenClaw OfficeCLI Skill

`openclaw-officecli` lets OpenClaw users generate local `pptx`, `docx`, and `xlsx` files with natural language from existing Telegram, Discord, Slack, and similar channels.

This skill works as follows:

- OpenClaw interprets the user message, asks follow-up questions in chat, and sends the result back to the channel
- `officecli agent-bridge` handles local document execution and structured task events
- `officecli` generates, assembles, saves, and optionally publishes the final file
- The agent should read `initialize` / `capabilities/get` first and use `document_generation.pptx.image_support` to determine PPT image support
- The agent should also read `initialize` / `capabilities/get -> update` and use structured fields to determine whether the binary is outdated instead of parsing human-facing CLI prompts

## Use Cases

- Say "generate a five-slide PPT" directly in Telegram / Discord / Slack
- Let OpenClaw refine document requirements through multi-turn follow-up questions
- Return the generated file to the chat as an attachment after success

## Prerequisites

1. OpenClaw is installed and configured on the local machine
2. `officecli` is installed locally, or the skill is allowed to auto-install it
3. `officecli config set-generation` and `officecli config set-license` have been completed, or the skill is allowed to fill them in automatically
4. The OpenClaw agent can:
   - run local commands
   - read local files
   - send file attachments back to the current channel

## Installation

Use the repository install script:

```bash
bash ./scripts/install-openclaw-skill.sh
```

By default the skill is installed to:

```bash
~/.openclaw/skills/openclaw-officecli
```

To customize the OpenClaw home directory:

```bash
OPENCLAW_HOME=/opt/openclaw bash ./scripts/install-openclaw-skill.sh
```

## Configuration

The install script places `config.yaml` in the skill directory. Default fields:

- `office_cli_path`
- `agent_bridge_command`
- `default_mode`
- `default_output_format`
- `default_lang`
- `default_publish`

If `officecli` is already on `PATH`, no extra changes are required by default.

## Environment Checks and Repair

The skill directory now includes two built-in scripts:

- `check-officecli-env.sh`
- `fix-officecli-env.sh`

Recommended order:

```bash
bash ~/.openclaw/skills/openclaw-officecli/check-officecli-env.sh
bash ~/.openclaw/skills/openclaw-officecli/fix-officecli-env.sh
```

Behavior:

- If `officecli` is not on `PATH`, the workflow will try to install it automatically
- If only generation or quota config is missing, the script fills in just the missing parts
- If you need online preview, provide publish configuration as well
- After a successful repair, `office_cli_path` and `agent_bridge_command` are written back to the skill `config.yaml`

## Attach to an Agent

In `~/.openclaw/config.yaml`, add the skill name to the target agent:

```yaml
agents:
  office-bot:
    model: openai/gpt-4o
    channels: [telegram]
    skills: [openclaw-officecli]
    tools: [shell, file_read]
```

If the current channel needs attachment upload, make sure it is configured correctly and has permission to send files.

## User Workflow

Users can send natural-language requests directly, for example:

- `generate a 5-slide PPT about an enterprise collaboration platform`
- `write a customer-facing docx about our collaboration platform`
- `create a project budget excel sheet`

If the request is incomplete, the skill should convert `officecli agent-bridge` `task.question` messages into chat follow-up questions.

After generation succeeds, the skill should:

1. Read `task.output.result.file_path`
2. Upload the corresponding file as a chat attachment
3. Include the document type, file name, and warnings in the message
4. If `result_meta.image_support.attention_required=true`, tell the user to check `image_base_url`, `image_api_key`, and `image_model`, or switch to `--no-images`

## PPT Image Rules

For all agents that use this skill, the following bridge rules are recommended:

- `pptx` allows automatic images by default; whether it is enabled by default should follow `document_generation.pptx.image_support.default_enabled`
- If the user explicitly asks for "no images" or a text-only deck, pass `enable_images=false`
- If the user asks why there are no images, first suggest running `officecli config set-generation`
- Prefer `result_meta.image_support` for programmatic checks instead of guessing from warning text alone

## Debugging

First verify that the local bridge starts correctly:

```bash
officecli agent-bridge
```

Then verify that `officecli` itself is usable:

```bash
officecli --version
officecli auth status
```

To inspect the installed skill files:

```bash
ls -la ~/.openclaw/skills/openclaw-officecli
cat ~/.openclaw/skills/openclaw-officecli/config.yaml
```
