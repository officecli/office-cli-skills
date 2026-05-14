# Install OfficeCLI

Use this guide when the search intent is specifically about how to install the `officecli` binary.

Main page:

- `https://officecli.io/officecli/install`

Recommended install path:

```bash
npm install -g officecli
```

Verify after install:

```bash
officecli --version
officecli auth status
```

Alternative binary install paths:

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-dist/main/scripts/install-officecli.sh | bash
```

```bash
brew tap officecli/officecli
brew install officecli
```

Optional AI-agent skill install guides:

- [Claude Code](../claude-code/README.md)
- [Codex](../codex/README.md)
- [OpenClaw](../openclaw/README.md)

The public repository documents the CLI install path first. Skill wrappers are optional integrations
that route agent tasks into the same local `officecli` binary.

Quick verification:

```bash
officecli --version
officecli config status
officecli agent-bridge
```
