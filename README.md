# OfficeCLI

OfficeCLI is a command-line tool that turns natural-language prompts into `PPTX`, `DOCX`, `XLSX`, workbook-backed `Report`, and standalone image outputs. When publishing is configured, document outputs can also return an online preview URL.

For `PPTX`, OfficeCLI generates and embeds images by default when the slide plan benefits from visuals. Use `--no-images` if you want a text-only deck.

Standalone `new img` generation always goes through the OfficeCLI server and uses the generation quota from `config set-license` / license checks. It does not use local `config set-generation` image provider settings. Standalone images publish online by default when publishing is configured; use `--no-publish` for local-only output.

## Claude Marketplace Install

This repository also contains the publishable wrapper for Claude Code marketplace plugins:

- `officecli`: the general Office document skill for Claude Code and other agents
- `openclaw-officecli`: the Office document skill variant for OpenClaw integrations

Recommended public marketplace repository name:

- `officecli/officecli-skills`

When that repository is publicly reachable, Claude Code users can install the plugin like this:

```text
/plugin marketplace add officecli/officecli-skills
/plugin install officecli@officecli-skills
```

To install the OpenClaw variant:

```text
/plugin install openclaw-officecli@officecli-skills
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

For trusted publishing without exposing this private source repository on npmjs, the long-term target is a separate public repository, for example `officecli/officecli-npm`, that receives synced wrapper files and runs the actual npm publish workflow.

## Quick Start

Build the binary:

```bash
go build -o officecli ./cmd/officecli
```

Initialize configuration:

```bash
./officecli config set-generation
./officecli config set-license
./officecli config set-publish   # optional for documents, required for standalone image preview links
./officecli config set-defaults  # optional
```

`config set-license` is required for standalone image generation. A successful standalone image consumes one generation count. Free usage has a separate image bucket of 3 images per user per day, while document generation keeps the 10 documents per day free bucket:

```bash
./officecli new img "Launch Visual" --prompt "A polished product launch hero image" --ratio landscape --reference-image ./reference.png
```

`config set-generation` also configures PPT image assets. It does not configure standalone `new img` generation:

- It first tries to reuse the text-generation provider for image creation.
- If the text model does not expose image endpoints, switch to a dedicated image provider.
- A dedicated image provider requires its own `url`, `ak`, and model name.

Inspect the current config at any time:

```bash
./officecli config status
```

You can also bootstrap the config manually:

```bash
mkdir -p ~/.config/officecli
cp ./examples/config.example.json ~/.config/officecli/config.json
```

Then update the service URLs, credentials, quota settings, and publish settings for your environment.

Generate a PPTX:

```bash
./officecli new pptx "Enterprise Collaboration Platform" "Introduce the product capabilities, customer value, and usage scenarios of this enterprise collaboration platform."
```

By default, generated files are written to `./output`.

If publish is configured and enabled, OfficeCLI also returns an online access URL and password. Standalone `new img` publishes by default; pass `--no-publish` to keep it local-only.

To validate local generation only:

```bash
./officecli new pptx "Enterprise Collaboration Platform" "Introduce the product capabilities, customer value, and usage scenarios of this enterprise collaboration platform." --no-publish
```

You can also use the convenience commands:

```bash
make build
officecli config status
make run-help
```

## Command Cheatsheet

Generate a PPTX:

```bash
./officecli new pptx "Enterprise Collaboration Platform" "Introduce the product capabilities, customer value, and usage scenarios of this enterprise collaboration platform."
```

Generate without publishing:

```bash
./officecli new pptx "Enterprise Collaboration Platform" "Introduce the product capabilities, customer value, and usage scenarios of this enterprise collaboration platform." --no-publish
```

Generate from a prompt file:

```bash
./officecli new pptx "Enterprise Collaboration Platform" --prompt-file ./examples/prompt.txt
```

Run in higher-quality interactive mode:

```bash
./officecli new pptx "Enterprise Collaboration Platform" --mode best
```

Generate a DOCX:

```bash
./officecli new docx "Quarterly Review" "Write a quarterly project review for leadership, focusing on outcomes, issues, and the next-step plan."
```

Generate an XLSX:

```bash
./officecli new xlsx "Sales Analysis" "Generate a quarterly sales analysis sheet with region, revenue, year-over-year change, and owner columns."
```

Generate a workbook-backed Report:

```bash
./officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --prompt "Summarize regional revenue shifts and the board-level decision points."
```

Generate a standalone image:

```bash
./officecli new img "Launch Visual" --prompt "Create a polished product launch hero image for an enterprise collaboration platform." --ratio landscape --reference-image ./reference.png
```

`new img` supports `--ratio square|landscape|portrait` and one `--reference-image <path-or-url>`, defaults to `square`, saves one local image, publishes an online image preview by default when publishing is configured, and consumes one image generation count only after a successful image response. Use `--no-publish` for local-only output. Local preview sidecars are not supported for standalone images.

Write output to a custom directory:

```bash
./officecli new pptx "Enterprise Collaboration Platform" "Introduce the product capabilities, customer value, and usage scenarios of this enterprise collaboration platform." --out ./dist
```

Return JSON output:

```bash
./officecli new pptx "Enterprise Collaboration Platform" --prompt-file ./examples/prompt.txt --json
```

Score a local PPTX:

```bash
./officecli score pptx ./output/Enterprise-Collaboration-Platform.pptx
```

The compatibility alias remains available:

```bash
./officecli review pptx ./output/Enterprise-Collaboration-Platform.pptx
```

By default, scoring performs structural checks first. If LibreOffice (`soffice`) is installed, OfficeCLI also adds a PDF-based visual review pass.

Run structural checks only:

```bash
./officecli review pptx ./output/Enterprise-Collaboration-Platform.pptx --no-visual
```

Start the JSON-RPC bridge for agents:

```bash
./officecli agent-bridge
```

Check the current license status:

```bash
./officecli auth status
```

## Development Notes

- The CLI binary is the source of truth for generation, licensing, publishing, and update behavior.
- Agent-facing skills must not be used to work around missing binary capabilities.
- The platform production environment is not deployed through a standard remote-image workflow. The release control plane lives in `officecli/officecli-ci`, while the underlying deploy implementation still follows the non-registry image upload path described in [`docs/platform-production-deploy.md`](/home/ubuntu/workspace/officecli/docs/platform-production-deploy.md).
