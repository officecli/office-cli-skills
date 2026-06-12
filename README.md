# OfficeCLI

OfficeCLI is a command-line tool that turns natural-language prompts into `PPTX`, `DOCX`, `XLSX`, workbook-backed `Report`, and standalone image outputs. When publishing is configured, document outputs can also return an online preview URL.

Community: [Discord](https://discord.gg/ezAHMkdG)

For `PPTX`, OfficeCLI generates and embeds images by default when the slide plan benefits from visuals. Use `--no-images` if you want a text-only deck.

Standalone `new img` generation follows the runtime selected by `config set-runtime`: `external` is free and unlimited and uses the local image provider from `config set-generation`, while `hosted` goes through the OfficeCLI-managed runtime and consumes hosted credits. Standalone images publish online by default when publishing is configured; use `--no-publish` for local-only output.

## Claude Marketplace Install

This repository also contains the publishable wrapper for Claude Code marketplace plugins:

- `officecli`: the general Office document skill for Claude Code and other agents
- `openclaw-officecli`: the Office document skill variant for OpenClaw integrations

Recommended public marketplace repository name:

- `officecli/officecli`

When that repository is publicly reachable, Claude Code users can install the plugin like this:

```text
/plugin marketplace add officecli/officecli
/plugin install officecli@officecli
```

To install the OpenClaw variant:

```text
/plugin install openclaw-officecli@officecli
```

Key marketplace files in this repository:

- `/.claude-plugin/marketplace.json`
- `/plugins/officecli/.claude-plugin/plugin.json`
- `/plugins/openclaw-officecli/.claude-plugin/plugin.json`

For local validation, load the plugin directories directly:

```bash
claude --plugin-dir ./plugins/officecli --plugin-dir ./plugins/openclaw-officecli
```

## NPM Wrapper Package

An initial npm wrapper package now lives at `packages/npm/officecli`.

Its purpose is to install the published OfficeCLI binary from `officecli/officecli-dist` and expose it as an npm-installed `officecli` command. The wrapper does not replace the Go CLI implementation.

Use one install channel at a time. If this machine already has OfficeCLI from Homebrew, keep using Homebrew for updates and do not install the npm wrapper on top of it. If you intentionally want to switch from Homebrew to npm, remove the Homebrew install first:

```bash
brew uninstall officecli/homebrew-officecli/officecli
# If Homebrew reports the formula under the short name, use:
brew uninstall officecli
npm install -g officecli
```

After `npm install -g officecli`, the binary can generate through hosted anonymous trial access without a local model endpoint or API key. The one-time free quota is tied to the machine fingerprint; after it is used up, run `officecli login`, then buy account hosted credits from https://officecli.io/pricing. Login prints a browser URL and short code, so it also works in headless or remote shells where OfficeCLI cannot open a browser.

For trusted publishing without exposing this private source repository on npmjs, the long-term target is a separate public repository, for example `officecli/officecli-npm`, that receives synced wrapper files and runs the actual npm publish workflow.

## Quick Start

Install the published binary:

```bash
npm install -g officecli
```

If you previously installed with Homebrew, do not run the npm install command above until you have removed the Homebrew version:

```bash
brew uninstall officecli/homebrew-officecli/officecli
# or, for short-form installs:
brew uninstall officecli
```

Then start the persistent TUI. Hosted anonymous trial access is the default, so no local model endpoint or API key is required for the first run:

```bash
officecli --version
officecli
officecli --no-alt-screen
officecli "Create a Q3 business review deck"
```

The default `officecli` command opens a persistent Codex-style TUI: the latest conversation and visible generation progress stay near the bottom input, which remains available for the next document request. Use `--no-alt-screen` when you want the TUI to preserve terminal scrollback instead of using a full-screen alternate buffer.

OfficeCLI and the TUI treat the current execution directory as the workspace. Generated files are written to `<workspace>/output` by default, and PPTX reference scanning also starts from that workspace unless you override it. For `PPTX`, OfficeCLI generates and embeds suitable images by default; add `--no-images` when you want a text-only deck.

For scripts and automation, use `officecli new ...`.

Check the current access mode:

```bash
officecli whoami
```

When the hosted trial is used up, run `officecli login`, then buy account hosted credits from https://officecli.io/pricing. If the browser cannot open automatically, copy the printed login URL into any browser and confirm the short code. For advanced automation, save an account API key:

```bash
officecli set-key <api-key>
```

## Command Cheatsheet

Generate a PPTX:

```bash
officecli new pptx "Q3 Business Review" --prompt "Create a six-slide executive deck for a SaaS quarterly business review. Cover growth, retention, risks, and next-quarter actions."
```

Generate a DOCX:

```bash
officecli new docx "Product Launch Brief" --prompt "Write a concise launch brief with audience, positioning, timeline, risks, and next steps."
```

Generate an XLSX:

```bash
officecli new xlsx "Sales Pipeline" --prompt "Create a sales pipeline workbook with stages, owners, deal values, probability, and next action columns."
```

Generate a workbook-backed Report:

```bash
officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --prompt "Summarize regional revenue shifts and the board-level decision points."
```

Generate without publishing:

```bash
officecli new pptx "Internal Draft" --prompt "Create a short internal strategy update." --no-publish
```

Generate from a prompt file:

```bash
officecli new pptx "Enterprise Collaboration Platform" --prompt-file ./examples/prompt.txt
```

Run in higher-quality interactive mode:

```bash
officecli new pptx "Enterprise Collaboration Platform" --mode best
```

Generate a standalone image:

```bash
officecli new img "Launch Visual" --prompt "Create a polished product launch hero image for an enterprise collaboration platform." --ratio landscape --reference-image ./reference.png
```

`new img` supports `--ratio square|landscape|portrait` and one `--reference-image <path-or-url>`, defaults to `square`, saves one local image, publishes an online image preview by default when publishing is configured, and only charges hosted credits after a successful hosted image response. In `external` runtime mode it is free and unlimited and uses your configured image provider; in `hosted` runtime mode it consumes hosted credits. Use `--no-publish` for local-only output. Local preview sidecars are not supported for standalone images.

Write output to a custom directory:

```bash
officecli new pptx "Enterprise Collaboration Platform" --prompt-file ./examples/prompt.txt --out ./dist
```

Return JSON output:

```bash
officecli new pptx "Enterprise Collaboration Platform" --prompt-file ./examples/prompt.txt --json
```

Score a local PPTX:

```bash
officecli score pptx ./output/Enterprise-Collaboration-Platform.pptx
```

The compatibility alias remains available:

```bash
officecli review pptx ./output/Enterprise-Collaboration-Platform.pptx
```

## Advanced Configuration

To use your own model endpoint instead of Hosted Mode, switch to External Mode and initialize generation:

```bash
officecli config set-runtime external
officecli config set-generation
```

External Mode is free and unlimited for standalone image generation and uses the image provider configured by `config set-generation`.

Configure optional online publishing:

```bash
officecli config set-publish
```

Inspect the current config at any time:

```bash
officecli config status
```

Build the binary from source:

```bash
go build -o officecli ./cmd/officecli
```

You can also bootstrap the config manually:

```bash
mkdir -p ~/.config/officecli
cp ./examples/config.example.json ~/.config/officecli/config.json
```

Then update the service URLs, credentials, quota settings, and publish settings for your environment.

By default, scoring performs structural checks first. If LibreOffice (`soffice`) is installed, OfficeCLI also adds a PDF-based visual review pass.

Run structural checks only:

```bash
officecli review pptx ./output/Enterprise-Collaboration-Platform.pptx --no-visual
```

Start the JSON-RPC bridge for agents:

```bash
officecli agent-bridge
```

## Development Notes

- The CLI binary is the source of truth for generation, licensing, publishing, and update behavior.
- Agent-facing skills must not be used to work around missing binary capabilities.
- The platform production environment is not deployed through a standard remote-image workflow. The release control plane lives in `officecli/officecli-ci`, while the underlying deploy implementation still follows the non-registry image upload path described in [`docs/platform-production-deploy.md`](/home/ubuntu/workspace/officecli/docs/platform-production-deploy.md).
