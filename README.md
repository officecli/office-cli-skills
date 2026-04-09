# OfficeCLI Skills

This repository contains the public Claude Code and OpenClaw skill packages for the closed-source
`officecli` product.

The primary plugin currently intended for Anthropic marketplace review is:

- `officecli`

This repository also includes:

- a Claude Code marketplace definition
- a Claude Code plugin wrapper for `officecli`
- a Claude Code plugin wrapper for `openclaw-officecli`
- public skill definitions and install scripts

## Claude Code

### Marketplace source

Add the OfficeCLI marketplace source:

```text
/plugin marketplace add officecli/officecli-skills
```

Install the primary plugin:

```text
/plugin install officecli@officecli-skills
```

### What the plugin does

The `officecli` plugin helps Claude Code handle local Office document workflows for:

- `pptx`
- `docx`
- `xlsx`
- generate or convert supported Office files through a local `officecli` installation
- check whether a requested workflow is supported before execution
- keep Office file generation on the local machine instead of using a hosted plugin backend

### Requirements

- Claude Code with plugin support
- a local `officecli` binary
- local OfficeCLI generation and license configuration
- permission for Claude Code to invoke local commands on the same machine

### Quick verification

After installation, verify the local dependency chain:

```bash
officecli --version
officecli config status
```

Then use Claude Code for a supported Office document request such as:

```text
Create a 6-slide PPTX introducing our enterprise collaboration platform.
```

## Direct install scripts

If you want the public skill files without marketplace installation, use the direct installer.

### Codex-style local skill install

Use `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | bash -s -- officecli
```

Or use `curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | bash -s -- officecli
```

If you only want the skill and do not want to auto-install the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | AUTO_INSTALL_BINARY=0 bash -s -- officecli
```

## OpenClaw Install

If you want OpenClaw users to generate local Office files directly from Telegram, Discord, Slack, or other channels, install the OpenClaw-facing skill package:

Use `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-openclaw-skill.sh | bash
```

Or use `curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-openclaw-skill.sh | bash
```

The OpenClaw installer will:

- install `openclaw-officecli` into `~/.openclaw/skills`
- create `config.yaml` from `config.example.yaml` when needed
- try to auto-install the `officecli` binary when it is missing

If you only want the OpenClaw skill and do not want to auto-install the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-openclaw-skill.sh | AUTO_INSTALL_BINARY=0 bash
```

After installation, 3 more steps are still required before the skill can be used:

1. Configure `officecli` itself:

```bash
officecli config set-generation
officecli config set-license
```

2. Attach `openclaw-officecli` to your OpenClaw agent in `~/.openclaw/config.yaml` and ensure the agent can use `shell` and `file_read`.

3. Restart OpenClaw, then verify both `officecli --version` and `officecli agent-bridge` work on the same host where OpenClaw runs.

## Safety and scope

- this repository distributes local skill wrappers, not a hosted SaaS integration
- Office file generation is executed through the user's local `officecli` installation
- this repository does not contain the closed-source OfficeCLI implementation
- the primary marketplace submission target is `officecli`, not `openclaw-officecli`

## Scope

- Public `SKILL.md` content and examples
- Claude Code marketplace metadata and plugin wrappers
- No closed-source `officecli` implementation code
- No private repository metadata or internal deployment details

## Layout

- `.claude-plugin/marketplace.json`: Claude Code marketplace definition
- `plugins/officecli/`: Claude Code plugin wrapper for the `officecli` skill
- `plugins/openclaw-officecli/`: Claude Code plugin wrapper for the `openclaw-officecli` skill
- `skills/officecli/`: public skill definition
- `skills/openclaw-officecli/`: public OpenClaw skill definition
- `scripts/install-skill.sh`: shell installer for direct `wget` / `curl` usage
- `scripts/install-openclaw-skill.sh`: shell installer for OpenClaw users
