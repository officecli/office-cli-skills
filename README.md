# OfficeCLI Skills

This repository contains the public Codex skill and OpenClaw skill package for the closed-source `officecli` product.

## Install

### One-line install

Use `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | bash -s -- officecli
```

Or use `curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | bash -s -- officecli
```

The installer will:

- install the `officecli` skill into `~/.codex/skills`
- try to auto-install the `officecli` binary when it is missing
- use Homebrew on macOS when available, and fall back to direct binary install when brew fails
- use the public Linux installer and install into `~/.local/bin` by default
- install the default `latest` CLI from the rolling latest public dist build

If `officecli` is still reported as not found after installation, first try the current-shell fix:

```bash
export PATH="$HOME/.local/bin:$PATH"
officecli --version
```

Then add `~/.local/bin` to the startup config for your shell if needed.

Re-running the same installer command refreshes the local skill to the latest version from GitHub.

If you only want the skill and do not want to auto-install the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | AUTO_INSTALL_BINARY=0 bash -s -- officecli
```

### Update

To update an existing local skill from GitHub, run the same install command again:

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | bash -s -- officecli
```

Or with `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | bash -s -- officecli
```

### Manual install

Copy the skill directory into your local Codex skills directory:

```bash
mkdir -p ~/.codex/skills

tmpdir="$(mktemp -d)"
git clone https://github.com/officecli/officecli-skills.git "$tmpdir/repo"
cp -R "$tmpdir/repo/skills/officecli" ~/.codex/skills/
```

After copying, restart Codex.

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

## Scope

- Public `SKILL.md` content and examples
- No closed-source `officecli` implementation code
- No private repository metadata or internal deployment details

## Layout

- `skills/officecli/`: public skill definition
- `skills/openclaw-officecli/`: public OpenClaw skill definition
- `scripts/install-skill.sh`: shell installer for direct `wget` / `curl` usage
- `scripts/install-openclaw-skill.sh`: shell installer for OpenClaw users
