#!/usr/bin/env bash

set -euo pipefail

PUBLIC_SKILLS_REPO="${PUBLIC_SKILLS_REPO:-}"
PUBLIC_SKILLS_DEFAULT_BRANCH="${PUBLIC_SKILLS_DEFAULT_BRANCH:-main}"
SOURCE_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PUBLIC_SKILL_NAME="officecli"
OPENCLAW_SKILL_NAME="openclaw-officecli"
CLAUDE_MARKETPLACE_DIR=".claude-plugin"
CLAUDE_PLUGIN_ROOT="plugins"
PUBLIC_DEMOS_SOURCE_DIR="public/skills-demos"
DIST_REPO="${DIST_REPO:-officecli/officecli-dist}"
HOMEBREW_TAP_REPO="${HOMEBREW_TAP_REPO:-officecli/homebrew-officecli}"
HOMEBREW_TAP_NAME="${HOMEBREW_TAP_NAME:-officecli/officecli}"
HOMEBREW_FORMULA="${HOMEBREW_FORMULA:-officecli}"

PUBLIC_SKILLS_DRY_RUN_DIR="${PUBLIC_SKILLS_DRY_RUN_DIR:-}"

if [[ -z "${PUBLIC_SKILLS_DRY_RUN_DIR}" && ( -z "${GH_TOKEN:-}" || -z "${PUBLIC_SKILLS_REPO}" ) ]]; then
  echo "GH_TOKEN and PUBLIC_SKILLS_REPO are required" >&2
  exit 1
fi
if [[ -n "${PUBLIC_SKILLS_DRY_RUN_DIR}" && -z "${PUBLIC_SKILLS_REPO}" ]]; then
  PUBLIC_SKILLS_REPO="officecli/officecli"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

if [[ -n "${PUBLIC_SKILLS_DRY_RUN_DIR}" ]]; then
  rm -rf "${PUBLIC_SKILLS_DRY_RUN_DIR}"
  mkdir -p "${PUBLIC_SKILLS_DRY_RUN_DIR}"
  cd "${PUBLIC_SKILLS_DRY_RUN_DIR}"
  git init -q
else
  git clone "https://x-access-token:${GH_TOKEN}@github.com/${PUBLIC_SKILLS_REPO}.git" "${tmpdir}/skills-repo"
  cd "${tmpdir}/skills-repo"
  git checkout "${PUBLIC_SKILLS_DEFAULT_BRANCH}"
fi

find . -mindepth 1 -maxdepth 1 ! -name '.git' -exec rm -rf {} +
mkdir -p "skills/${PUBLIC_SKILL_NAME}" "skills/${OPENCLAW_SKILL_NAME}"
cp -R "${SOURCE_REPO_ROOT}/skills/${PUBLIC_SKILL_NAME}/." "./skills/${PUBLIC_SKILL_NAME}/"
cp -R "${SOURCE_REPO_ROOT}/skills/${OPENCLAW_SKILL_NAME}/." "./skills/${OPENCLAW_SKILL_NAME}/"
if [[ -d "${SOURCE_REPO_ROOT}/${CLAUDE_MARKETPLACE_DIR}" ]]; then
  mkdir -p "${CLAUDE_MARKETPLACE_DIR}"
  cp -R "${SOURCE_REPO_ROOT}/${CLAUDE_MARKETPLACE_DIR}/." "./${CLAUDE_MARKETPLACE_DIR}/"
fi
if [[ -d "${SOURCE_REPO_ROOT}/${CLAUDE_PLUGIN_ROOT}" ]]; then
  mkdir -p "${CLAUDE_PLUGIN_ROOT}"
  cp -R "${SOURCE_REPO_ROOT}/${CLAUDE_PLUGIN_ROOT}/." "./${CLAUDE_PLUGIN_ROOT}/"
fi
if [[ -d "${SOURCE_REPO_ROOT}/${PUBLIC_DEMOS_SOURCE_DIR}" ]]; then
  mkdir -p demos
  cp -R "${SOURCE_REPO_ROOT}/${PUBLIC_DEMOS_SOURCE_DIR}/." "./demos/"
fi
mkdir -p scripts

cat > scripts/install-skill.sh <<'INSTALLER'
#!/usr/bin/env bash

set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/officecli/officecli.git}"
SKILL_NAME="${1:-officecli}"
DEST_ROOT="${DEST_ROOT:-${HOME}/.codex/skills}"
AUTO_INSTALL_BINARY="${AUTO_INSTALL_BINARY:-1}"
DIST_REPO="${DIST_REPO:-officecli/officecli-dist}"
HOMEBREW_TAP_REPO="${HOMEBREW_TAP_REPO:-officecli/homebrew-officecli}"
HOMEBREW_TAP_NAME="${HOMEBREW_TAP_NAME:-officecli/officecli}"
HOMEBREW_FORMULA="${HOMEBREW_FORMULA:-officecli}"
LINUX_PREFIX="${LINUX_PREFIX:-${HOME}/.local}"
LINUX_BIN_DIR="${LINUX_BIN_DIR:-${LINUX_PREFIX}/bin}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

install_skill() {
  local tmpdir src dest

  need_cmd git
  need_cmd cp
  need_cmd mkdir

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' RETURN

  git clone --depth 1 "${REPO_URL}" "${tmpdir}/repo" >/dev/null 2>&1

  src="${tmpdir}/repo/skills/${SKILL_NAME}"
  dest="${DEST_ROOT}/${SKILL_NAME}"

  if [[ ! -d "${src}" ]]; then
    echo "skill not found: ${SKILL_NAME}" >&2
    exit 1
  fi

  mkdir -p "${DEST_ROOT}"
  rm -rf "${dest}"
  cp -R "${src}" "${dest}"

  echo "installed skill to ${dest}"
}

install_binary_linux() {
  install_binary_via_dist
}

install_binary_via_dist() {
  if ! command -v curl >/dev/null 2>&1; then
    echo "warning: curl not found, skipped officecli binary auto-install" >&2
    return 1
  fi

  if curl -fsSL "https://raw.githubusercontent.com/${DIST_REPO}/main/scripts/install-officecli.sh"     | PREFIX="${LINUX_PREFIX}" BIN_DIR="${LINUX_BIN_DIR}" INSTALL_DIR="${LINUX_BIN_DIR}" DIST_REPO="${DIST_REPO}" bash; then
    if [[ ":$PATH:" != *":${LINUX_BIN_DIR}:"* ]]; then
      echo "note: add ${LINUX_BIN_DIR} to PATH to use officecli directly"
    fi
    return 0
  fi

  echo "warning: failed to auto-install officecli binary from public dist" >&2
  return 1
}

install_binary_macos() {
  if command -v brew >/dev/null 2>&1; then
    brew untap "${HOMEBREW_TAP_NAME}" >/dev/null 2>&1 || true
    brew tap "${HOMEBREW_TAP_NAME}" >/dev/null

    local tap_repo
    tap_repo="$(brew --repository "${HOMEBREW_TAP_NAME}" 2>/dev/null || true)"
    if [[ -n "${tap_repo}" ]]; then
      rm -f "${tap_repo}/Formula/officecli.rb"
    fi

    if brew install "${HOMEBREW_FORMULA}" >/dev/null 2>&1 || brew upgrade "${HOMEBREW_FORMULA}" >/dev/null 2>&1; then
      command -v officecli >/dev/null 2>&1 && return 0
    fi

    echo "warning: brew install failed, falling back to direct binary install" >&2
  else
    echo "warning: Homebrew not found, falling back to direct binary install" >&2
  fi

  install_binary_via_dist
}

auto_install_binary_if_missing() {
  local os_name

  if [[ "${AUTO_INSTALL_BINARY}" != "1" ]]; then
    echo "skipped officecli binary auto-install (AUTO_INSTALL_BINARY=${AUTO_INSTALL_BINARY})"
    return 0
  fi

  if command -v officecli >/dev/null 2>&1; then
    echo "officecli binary already available: $(command -v officecli)"
    return 0
  fi

  os_name="$(uname -s)"
  case "${os_name}" in
    Linux)
      install_binary_linux || return 0
      ;;
    Darwin)
      install_binary_macos || return 0
      ;;
    *)
      echo "warning: unsupported OS for automatic officecli install: ${os_name}" >&2
      return 0
      ;;
  esac

  if command -v officecli >/dev/null 2>&1; then
    echo "installed officecli binary: $(command -v officecli)"
  else
    echo "warning: officecli binary auto-install completed without a usable PATH entry" >&2
  fi
}

install_skill
auto_install_binary_if_missing

echo "restart your client to pick up the refreshed skill"
INSTALLER

chmod +x scripts/install-skill.sh

cat > scripts/install-openclaw-skill.sh <<'INSTALLER'
#!/usr/bin/env bash

set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/officecli/officecli.git}"
SKILL_NAME="${SKILL_NAME:-openclaw-officecli}"
OPENCLAW_HOME="${OPENCLAW_HOME:-${HOME}/.openclaw}"
DEST_ROOT="${OPENCLAW_HOME}/skills"
AUTO_INSTALL_BINARY="${AUTO_INSTALL_BINARY:-1}"
DIST_REPO="${DIST_REPO:-officecli/officecli-dist}"
HOMEBREW_TAP_REPO="${HOMEBREW_TAP_REPO:-officecli/homebrew-officecli}"
HOMEBREW_TAP_NAME="${HOMEBREW_TAP_NAME:-officecli/officecli}"
HOMEBREW_FORMULA="${HOMEBREW_FORMULA:-officecli}"
LINUX_PREFIX="${LINUX_PREFIX:-${HOME}/.local}"
LINUX_BIN_DIR="${LINUX_BIN_DIR:-${LINUX_PREFIX}/bin}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

install_skill() {
  local tmpdir src dest

  need_cmd git
  need_cmd cp
  need_cmd mkdir

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' RETURN

  git clone --depth 1 "${REPO_URL}" "${tmpdir}/repo" >/dev/null 2>&1

  src="${tmpdir}/repo/skills/${SKILL_NAME}"
  dest="${DEST_ROOT}/${SKILL_NAME}"

  if [[ ! -d "${src}" ]]; then
    echo "skill not found: ${SKILL_NAME}" >&2
    exit 1
  fi

  mkdir -p "${DEST_ROOT}"
  rm -rf "${dest}"
  cp -R "${src}" "${dest}"

  if [[ ! -f "${dest}/config.yaml" && -f "${dest}/config.example.yaml" ]]; then
    cp "${dest}/config.example.yaml" "${dest}/config.yaml"
  fi

  echo "installed OpenClaw skill to ${dest}"
}

install_binary_linux() {
  install_binary_via_dist
}

install_binary_via_dist() {
  if ! command -v curl >/dev/null 2>&1; then
    echo "warning: curl not found, skipped officecli binary auto-install" >&2
    return 1
  fi

  if curl -fsSL "https://raw.githubusercontent.com/${DIST_REPO}/main/scripts/install-officecli.sh" \
    | PREFIX="${LINUX_PREFIX}" BIN_DIR="${LINUX_BIN_DIR}" INSTALL_DIR="${LINUX_BIN_DIR}" DIST_REPO="${DIST_REPO}" bash; then
    if [[ ":$PATH:" != *":${LINUX_BIN_DIR}:"* ]]; then
      echo "note: add ${LINUX_BIN_DIR} to PATH to use officecli directly"
    fi
    return 0
  fi

  echo "warning: failed to auto-install officecli binary from public dist" >&2
  return 1
}

install_binary_macos() {
  if command -v brew >/dev/null 2>&1; then
    brew untap "${HOMEBREW_TAP_NAME}" >/dev/null 2>&1 || true
    brew tap "${HOMEBREW_TAP_NAME}" >/dev/null

    local tap_repo
    tap_repo="$(brew --repository "${HOMEBREW_TAP_NAME}" 2>/dev/null || true)"
    if [[ -n "${tap_repo}" ]]; then
      rm -f "${tap_repo}/Formula/cli-office.rb"
    fi

    if brew install "${HOMEBREW_FORMULA}" >/dev/null 2>&1 || brew upgrade "${HOMEBREW_FORMULA}" >/dev/null 2>&1; then
      command -v officecli >/dev/null 2>&1 && return 0
    fi

    echo "warning: brew install failed, falling back to direct binary install" >&2
  else
    echo "warning: Homebrew not found, falling back to direct binary install" >&2
  fi

  install_binary_via_dist
}

auto_install_binary_if_missing() {
  local os_name

  if [[ "${AUTO_INSTALL_BINARY}" != "1" ]]; then
    echo "skipped officecli binary auto-install (AUTO_INSTALL_BINARY=${AUTO_INSTALL_BINARY})"
    return 0
  fi

  if command -v officecli >/dev/null 2>&1; then
    echo "officecli binary already available: $(command -v officecli)"
    return 0
  fi

  os_name="$(uname -s)"
  case "${os_name}" in
    Linux)
      install_binary_linux || return 0
      ;;
    Darwin)
      install_binary_macos || return 0
      ;;
    *)
      echo "warning: unsupported OS for automatic officecli install: ${os_name}" >&2
      return 0
      ;;
  esac

  if command -v officecli >/dev/null 2>&1; then
    echo "installed officecli binary: $(command -v officecli)"
  else
    echo "warning: officecli binary auto-install completed without a usable PATH entry" >&2
  fi
}

install_skill
auto_install_binary_if_missing

cat <<EOF

Next steps:
1. Run: officecli config set-generation
2. Run: officecli config set-license
3. Verify: officecli --version
4. Verify bridge: officecli agent-bridge
5. Attach the skill in ${OPENCLAW_HOME}/config.yaml:

agents:
  office-bot:
    model: openai/gpt-4o
    channels: [telegram]
    skills: [${SKILL_NAME}]
    tools: [shell, file_read]

6. Restart OpenClaw

PPT image behavior for all agents:
  - read initialize/capabilities.get first
  - use document_generation.pptx.image_support as the source of truth
  - if users want a text-only deck, send enable_images=false
  - if a PPT returns without images, run: officecli config set-generation

Standalone img behavior for all agents:
  - use top-level image_generation as the source of truth
  - call office.generate with document_type=img and optional ratio
  - img uses the OfficeCLI server provider and requires config set-license
  - img does not use local config set-generation image provider settings
EOF
INSTALLER

chmod +x scripts/install-openclaw-skill.sh

cat > README.md <<'README'
# OfficeCLI

**AI document generation CLI for PPTX, DOCX, XLSX, reports, and images.**

[![GitHub Release](https://img.shields.io/github/v/release/officecli/officecli-dist?label=release)](https://github.com/officecli/officecli-dist/releases)
[![npm](https://img.shields.io/npm/v/officecli?label=npm)](https://www.npmjs.com/package/officecli)
[![License](https://img.shields.io/github/license/officecli/officecli)](./LICENSE)
[![Website](https://img.shields.io/badge/website-officecli.io-0f766e)](https://officecli.io/officecli)
[![Discord](https://img.shields.io/badge/community-Discord-5865F2)](https://discord.gg/ezAHMkdG)

OfficeCLI turns natural-language prompts into editable Office files and standalone images from a terminal,
script, CI job, or local automation flow. Use one `officecli` binary to generate `PPTX`, `DOCX`, `XLSX`,
workbook-backed `Report`, and `img` outputs, with hosted trial access for first runs and External Mode
when you want to bring your own LLM endpoint.

Chinese documentation: [README.zh-CN.md](./README.zh-CN.md)

- Website: [officecli.io/officecli](https://officecli.io/officecli)
- Demo gallery: [demos/README.md](./demos/README.md)
- Optional agent skills: [Claude Code](./claude-code/README.md), [Codex](./codex/README.md), [OpenClaw](./openclaw/README.md)
- Community: [Discord](https://discord.gg/ezAHMkdG)

## Install in one command

npm is the recommended install path for most users:

```bash
npm install -g officecli
```

Verify the installed binary:

```bash
officecli --version
officecli auth status
```

Alternative install paths are available when npm is not a good fit.

Install the latest release binary directly:

```bash
curl -fsSL https://raw.githubusercontent.com/officecli/officecli-dist/main/scripts/install-officecli.sh | bash
```

Or install with Homebrew:

```bash
brew tap officecli/officecli
brew install officecli
```

## Generate files from the CLI

Hosted anonymous trial access is available by default, so the first run does not require a local model
endpoint or API key.

Generate a PPTX deck:

```bash
officecli new pptx "Q3 Business Review" --prompt "Create a six-slide executive deck for a SaaS quarterly business review. Cover growth, retention, risks, and next-quarter actions."
```

Generate a DOCX:

```bash
officecli new docx "Product Launch Brief" --prompt "Write a concise launch brief with audience, positioning, timeline, risks, and next steps."
```

Generate an XLSX workbook:

```bash
officecli new xlsx "Sales Pipeline" --prompt "Create a sales pipeline workbook with stages, owners, deal values, probability, and next action columns."
```

Generate a workbook-backed report:

```bash
officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --prompt "Summarize regional revenue shifts and the board-level decision points."
```

Generate a standalone image:

```bash
officecli new img "Launch Visual" --prompt "Create a polished product launch hero image for an enterprise collaboration platform." --ratio landscape --reference-image ./reference.png
```

Generated files are written to `./output` by default. Use `--out <dir>` to choose another directory,
`--json` for machine-readable output, and `--no-publish` when you want local-only output.

For `PPTX`, OfficeCLI generates and embeds suitable images by default. Use `--no-images` for a
text-only deck.

## What OfficeCLI is best for

| Search intent | OfficeCLI capability |
| --- | --- |
| AI PPTX generator | Create editable PowerPoint decks from prompts, with images by default and `--no-images` for text-only decks. |
| DOCX generator CLI | Draft editable Word documents for briefs, memos, proposals, and customer-facing documents. |
| XLSX automation | Generate workbooks, trackers, dashboards, and analysis sheets from structured prompts. |
| Report generation | Use an existing workbook as evidence and generate a readable workbook-backed report. |
| Image generation CLI | Generate standalone `img` outputs with `--ratio square\|landscape\|portrait` and optional reference images. |
| Local-first document automation | Run from a terminal, script, CI job, or agent workflow with one installable binary. |

## Runtime and access modes

Check the current access mode:

```bash
officecli auth status
```

When the hosted trial is used up, create or purchase a hosted key from https://officecli.io/pricing,
then save it:

```bash
officecli auth set-key <api-key>
```

To use your own model endpoint instead of Hosted Mode, switch to External Mode and initialize
generation:

```bash
officecli config set-runtime external
officecli config set-generation
```

Configure optional online publishing:

```bash
officecli config set-publish
```

Inspect the current config:

```bash
officecli config status
```

## What the binary supports

The `officecli` binary supports:

- AI PPTX generation for decks, proposals, and executive briefings
- AI DOCX drafting for retrospectives, memos, and customer-facing documents
- AI XLSX generation for workbooks, trackers, and analysis sheets
- workbook-backed reports
- standalone image generation with `--ratio square|landscape|portrait`
- optional reference images for standalone `img` output
- local-only generation with `--no-publish`
- structural PPTX scoring with `officecli score pptx <file>`

The compatibility alias remains available:

```bash
officecli review pptx ./output/Enterprise-Collaboration-Platform.pptx
```

## Generation demos

These checked-in demos show the visible result and the reproducible inputs together. Each demo includes
a preview image, the generated artifact, `prompt.md`, `metadata.json`, and the command used to reproduce
the flow with a configured OfficeCLI runtime.

| AI PPTX, DOCX, XLSX, report, and image demos | Preview | Files |
| --- | --- | --- |
| AI PPTX generator with images | ![AI PPTX generator preview created by OfficeCLI](./demos/pptx-image-rich/preview.png) | [PPTX](./demos/pptx-image-rich/image-rich-strategy-deck.pptx) · [Prompt](./demos/pptx-image-rich/prompt.md) · [Metadata](./demos/pptx-image-rich/metadata.json) |
| Text-only AI PPTX generator | ![Text-only AI PPTX generator preview created by OfficeCLI](./demos/pptx-text-only/preview.png) | [PPTX](./demos/pptx-text-only/text-only-executive-briefing.pptx) · [Prompt](./demos/pptx-text-only/prompt.md) · [Metadata](./demos/pptx-text-only/metadata.json) |
| DOCX generator CLI | ![DOCX generator CLI preview created by OfficeCLI](./demos/docx-brief/preview.png) | [DOCX](./demos/docx-brief/officecli-customer-brief.docx) · [Prompt](./demos/docx-brief/prompt.md) · [Metadata](./demos/docx-brief/metadata.json) |
| XLSX automation dashboard | ![XLSX automation preview created by OfficeCLI](./demos/xlsx-dashboard/preview.png) | [XLSX](./demos/xlsx-dashboard/demo-adoption-dashboard.xlsx) · [Prompt](./demos/xlsx-dashboard/prompt.md) · [Metadata](./demos/xlsx-dashboard/metadata.json) |
| Workbook-backed report generation | ![Report generation preview created by OfficeCLI](./demos/report-workbook/preview.png) | [HTML report](./demos/report-workbook/demo-program-readiness-report.html) · [Source XLSX](./demos/report-workbook/demo-program-source-workbook.xlsx) · [Prompt](./demos/report-workbook/prompt.md) |
| Image generation CLI | ![Image generation CLI preview created by OfficeCLI](./demos/standalone-img/preview.png) | [PNG](./demos/standalone-img/officecli-hero-image.png) · [Prompt](./demos/standalone-img/prompt.md) · [Metadata](./demos/standalone-img/metadata.json) |

See [demos/README.md](./demos/README.md) for the complete reproducibility table and verification notes.

## Optional AI agent integrations

AI agents can use OfficeCLI through public skills when you want the agent to route Office tasks into the
local binary instead of calling the CLI manually. This is optional: the primary interface is still the
`officecli` command.

### Claude Code

OfficeCLI can be installed directly by Claude Code from the marketplace source.

OfficeCLI marketplace source:

```text
/plugin marketplace add __PUBLIC_SKILLS_REPO__
```

Install the primary plugin:

```text
/plugin install officecli@officecli
```

### Codex and other local agents

Use the direct installer when you want the public skill files without marketplace installation.

Use `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-skill.sh | bash -s -- officecli
```

Or use `curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-skill.sh | bash -s -- officecli
```

If you only want the skill and do not want to auto-install the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-skill.sh | AUTO_INSTALL_BINARY=0 bash -s -- officecli
```

### OpenClaw

If you want OpenClaw users to generate local Office files directly from Telegram, Discord, Slack, or
other channels, install the OpenClaw-facing skill package.

Use `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-openclaw-skill.sh | bash
```

Or use `curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-openclaw-skill.sh | bash
```

The OpenClaw installer will:

- install `openclaw-officecli` into `~/.openclaw/skills`
- create `config.yaml` from `config.example.yaml` when needed
- try to auto-install the `officecli` binary when it is missing

If you only want the OpenClaw skill and do not want to auto-install the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-openclaw-skill.sh | AUTO_INSTALL_BINARY=0 bash
```

## Requirements

- a local `officecli` binary
- OfficeCLI access through hosted trial, hosted key, or External Mode configuration
- optional publishing configuration when you want online preview URLs
- for agent integrations, permission for the agent client to invoke local commands on the same machine

Quick verification after installation:

```bash
officecli --version
officecli config status
```

For OpenClaw, also verify:

```bash
officecli agent-bridge
```

## OfficeCLI and officecli fit together

- `officecli` is the command-line binary you use to generate Office files and images
- this repository is the public install, docs, demos, and integration surface for that binary
- `officecli` is also the general optional skill for Claude Code, Codex, and other local agents
- `openclaw-officecli` is the OpenClaw-oriented package

## FAQ

### Is this repository a hosted SaaS plugin backend?

No. This repository distributes binary install docs, demos, scripts, and optional local skill wrappers.

### Does the CLI require an AI agent?

No. The main workflow is direct CLI usage through commands like `officecli new pptx`, `officecli new
docx`, `officecli new xlsx`, `officecli new report`, and `officecli new img`.

### Can AI agents create PPTX, DOCX, XLSX, report, or img outputs with this repository?

Yes, when the local `officecli` runtime is installed and configured. The repository tells the agent how
to route supported Office and image tasks into the local OfficeCLI binary.

### Why does this repository mention Codex as well as Claude Code?

Because marketplace install is only one entrypoint. This repository also distributes direct skill files
for Codex-style local agents and other agent runtimes.

### Does this repository contain the full OfficeCLI implementation?

No. It contains public install docs, demos, scripts, skill definitions, and plugin wrappers.

## Layout

- `install/README.md`: install entrypoint for search and onboarding
- `claude-code/README.md`: Claude Code marketplace install guide
- `codex/README.md`: direct local install guide for Codex-style agents
- `openclaw/README.md`: OpenClaw installer and bridge guide
- `faq/README.md`: FAQ entrypoint
- `.claude-plugin/marketplace.json`: Claude Code marketplace definition
- `demos/`: reproducible demo gallery with preview images, prompts, metadata, and generated files
- `plugins/officecli/`: Claude Code plugin wrapper for the `officecli` skill
- `plugins/openclaw-officecli/`: Claude Code plugin wrapper for the `openclaw-officecli` skill
- `skills/officecli/`: public OfficeCLI skill definition
- `skills/openclaw-officecli/`: public OpenClaw skill definition
- `scripts/install-skill.sh`: shell installer for direct `wget` / `curl` usage
- `scripts/install-openclaw-skill.sh`: shell installer for OpenClaw users
README

PUBLIC_SKILLS_REPO="${PUBLIC_SKILLS_REPO}" PUBLIC_SKILLS_DEFAULT_BRANCH="${PUBLIC_SKILLS_DEFAULT_BRANCH}" python3 - <<'PY'
from pathlib import Path
import os

path = Path("README.md")
content = path.read_text()
content = content.replace("__PUBLIC_SKILLS_REPO__", os.environ["PUBLIC_SKILLS_REPO"])
content = content.replace("__PUBLIC_SKILLS_DEFAULT_BRANCH__", os.environ["PUBLIC_SKILLS_DEFAULT_BRANCH"])
path.write_text(content)
PY

cat > README.zh-CN.md <<'README'
# OfficeCLI

**面向 PPTX、DOCX、XLSX、报告和图片的 AI 文档生成 CLI。**

[![GitHub Release](https://img.shields.io/github/v/release/officecli/officecli-dist?label=release)](https://github.com/officecli/officecli-dist/releases)
[![npm](https://img.shields.io/npm/v/officecli?label=npm)](https://www.npmjs.com/package/officecli)
[![License](https://img.shields.io/github/license/officecli/officecli)](./LICENSE)
[![Website](https://img.shields.io/badge/website-officecli.io-0f766e)](https://officecli.io/officecli)
[![Discord](https://img.shields.io/badge/community-Discord-5865F2)](https://discord.gg/ezAHMkdG)

OfficeCLI 可以把自然语言 prompt 生成可编辑的 Office 文件和独立图片。你可以在终端、脚本、CI 或本地自动化流程中直接使用一个 `officecli` 二进制生成 `PPTX`、`DOCX`、`XLSX`、基于工作簿的 `Report` 和 `img` 输出。首次运行默认可用 hosted trial；如果要接入自己的模型 endpoint，可以切换到 External Mode。

- 官网：[officecli.io/officecli](https://officecli.io/officecli)
- 英文 README：[README.md](./README.md)
- 生成示例：[demos/README.md](./demos/README.md)
- 可选 Agent 集成：[Claude Code](./claude-code/README.md)、[Codex](./codex/README.md)、[OpenClaw](./openclaw/README.md)
- 社区：[Discord](https://discord.gg/ezAHMkdG)

## 一行安装

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

## 适合哪些搜索和场景

| 搜索意图 | OfficeCLI 能力 |
| --- | --- |
| AI PPTX generator | 从 prompt 生成可编辑 PowerPoint，默认支持图片，也支持 `--no-images` 纯文本 deck。 |
| DOCX generator CLI | 生成可编辑 Word 文档，适合 brief、memo、proposal 和客户文档。 |
| XLSX automation | 生成 workbook、tracker、dashboard 和分析表格。 |
| Report generation | 以现有 workbook 为数据来源，生成可阅读的工作簿报告。 |
| Image generation CLI | 使用 `--ratio square\|landscape\|portrait` 和可选 reference image 生成独立图片。 |
| 本地优先文档自动化 | 一个二进制即可在终端、脚本、CI 或 Agent workflow 中运行。 |

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

这些示例把可视预览、生成文件、`prompt.md`、`metadata.json` 和可复现命令放在一起，方便确认 OfficeCLI 的实际输出效果。

| AI PPTX、DOCX、XLSX、report 和 image 示例 | 预览 | 文件 |
| --- | --- | --- |
| 带图 AI PPTX generator | ![AI PPTX generator preview created by OfficeCLI](./demos/pptx-image-rich/preview.png) | [PPTX](./demos/pptx-image-rich/image-rich-strategy-deck.pptx) · [Prompt](./demos/pptx-image-rich/prompt.md) · [Metadata](./demos/pptx-image-rich/metadata.json) |
| 纯文本 AI PPTX generator | ![Text-only AI PPTX generator preview created by OfficeCLI](./demos/pptx-text-only/preview.png) | [PPTX](./demos/pptx-text-only/text-only-executive-briefing.pptx) · [Prompt](./demos/pptx-text-only/prompt.md) · [Metadata](./demos/pptx-text-only/metadata.json) |
| DOCX generator CLI | ![DOCX generator CLI preview created by OfficeCLI](./demos/docx-brief/preview.png) | [DOCX](./demos/docx-brief/officecli-customer-brief.docx) · [Prompt](./demos/docx-brief/prompt.md) · [Metadata](./demos/docx-brief/metadata.json) |
| XLSX automation dashboard | ![XLSX automation preview created by OfficeCLI](./demos/xlsx-dashboard/preview.png) | [XLSX](./demos/xlsx-dashboard/demo-adoption-dashboard.xlsx) · [Prompt](./demos/xlsx-dashboard/prompt.md) · [Metadata](./demos/xlsx-dashboard/metadata.json) |
| workbook-backed report generation | ![Report generation preview created by OfficeCLI](./demos/report-workbook/preview.png) | [HTML report](./demos/report-workbook/demo-program-readiness-report.html) · [Source XLSX](./demos/report-workbook/demo-program-source-workbook.xlsx) · [Prompt](./demos/report-workbook/prompt.md) |
| image generation CLI | ![Image generation CLI preview created by OfficeCLI](./demos/standalone-img/preview.png) | [PNG](./demos/standalone-img/officecli-hero-image.png) · [Prompt](./demos/standalone-img/prompt.md) · [Metadata](./demos/standalone-img/metadata.json) |

完整可复现命令见 [demos/README.md](./demos/README.md)。

## 可选 AI Agent 集成

当你希望 AI Agent 自动把 Office 任务路由到本地 OfficeCLI 二进制时，可以安装 public skills。这是可选能力；主使用方式仍然是直接执行 `officecli` 命令。

## Claude Code 安装

```text
/plugin marketplace add __PUBLIC_SKILLS_REPO__
/plugin install officecli@officecli
```

如需安装 OpenClaw 变体：

```text
/plugin install openclaw-officecli@officecli
```

## Codex 和其他本地 Agent 安装

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-skill.sh | bash -s -- officecli
```

如果只安装 skill，不自动安装 `officecli` 二进制：

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-skill.sh | AUTO_INSTALL_BINARY=0 bash -s -- officecli
```

## OpenClaw 安装

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-openclaw-skill.sh | bash
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
README

PUBLIC_SKILLS_REPO="${PUBLIC_SKILLS_REPO}" PUBLIC_SKILLS_DEFAULT_BRANCH="${PUBLIC_SKILLS_DEFAULT_BRANCH}" python3 - <<'PY'
from pathlib import Path
import os

path = Path("README.zh-CN.md")
content = path.read_text()
content = content.replace("__PUBLIC_SKILLS_REPO__", os.environ["PUBLIC_SKILLS_REPO"])
content = content.replace("__PUBLIC_SKILLS_DEFAULT_BRANCH__", os.environ["PUBLIC_SKILLS_DEFAULT_BRANCH"])
path.write_text(content)
PY

mkdir -p install claude-code codex openclaw faq

cat > install/README.md <<'README'
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
README

cat > claude-code/README.md <<'README'
# officecli for Claude Code

Use this guide when you want the Claude Code marketplace installation path for `officecli`.

Main page:

- `https://officecli.io/officecli/claude-code`

Marketplace install:

```text
/plugin marketplace add __PUBLIC_SKILLS_REPO__
/plugin install officecli@officecli
```

This path is for local Office workflows through Claude Code. It is not a hosted plugin backend.
README

cat > codex/README.md <<'README'
# officecli for Codex

Use this guide when you want the direct local installer for Codex-style agents.

Main page:

- `https://officecli.io/officecli/codex`

Direct install:

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-skill.sh | bash -s -- officecli
```

This path installs the public skill bundle directly on the local machine without a marketplace layer.
README

cat > openclaw/README.md <<'README'
# officecli for OpenClaw

Use this guide when you want the OpenClaw-oriented package and `officecli agent-bridge`.

Main page:

- `https://officecli.io/officecli/openclaw`

Install:

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-openclaw-skill.sh | bash
```

After installation, attach `openclaw-officecli` to the target OpenClaw agent and verify:

```bash
officecli agent-bridge
```
README

cat > faq/README.md <<'README'
# officecli FAQ

Use this guide when the question is not installation itself, but whether `officecli` is the right entrypoint.

Main page:

- `https://officecli.io/officecli/faq`

Key answers:

- `officecli` is the public repository for wrappers, skills, and installers
- `officecli` is the local runtime that produces the final Office file
- marketplace install is only one path; direct local install is another
- this repository is not a hosted SaaS plugin backend
README

PUBLIC_SKILLS_REPO="${PUBLIC_SKILLS_REPO}" PUBLIC_SKILLS_DEFAULT_BRANCH="${PUBLIC_SKILLS_DEFAULT_BRANCH}" python3 - <<'PY'
from pathlib import Path
import os

paths = [
    Path("claude-code/README.md"),
    Path("codex/README.md"),
    Path("openclaw/README.md"),
]
for path in paths:
    content = path.read_text()
    content = content.replace("__PUBLIC_SKILLS_REPO__", os.environ["PUBLIC_SKILLS_REPO"])
    content = content.replace("__PUBLIC_SKILLS_DEFAULT_BRANCH__", os.environ["PUBLIC_SKILLS_DEFAULT_BRANCH"])
    path.write_text(content)
PY

python3 - <<'PY'
from pathlib import Path
import json

demo_root = Path("demos")
rows = []
if demo_root.is_dir():
    for meta_path in sorted(demo_root.glob("*/metadata.json")):
        meta = json.loads(meta_path.read_text())
        slug = meta_path.parent.name
        rows.append((slug, meta))

if rows:
    keyword_map = {
        "pptx-image-rich": "AI PPTX generator, PowerPoint automation, image-rich deck",
        "pptx-text-only": "text-only AI PPTX generator, executive briefing",
        "docx-brief": "DOCX generator CLI, AI Word document generator",
        "xlsx-dashboard": "XLSX automation, AI spreadsheet generator, dashboard workbook",
        "report-workbook": "report generation, workbook-backed report, XLSX to report",
        "standalone-img": "image generation CLI, AI image generator, standalone image",
    }
    alt_map = {
        "pptx-image-rich": "AI PPTX generator preview created by OfficeCLI",
        "pptx-text-only": "Text-only AI PPTX generator preview created by OfficeCLI",
        "docx-brief": "DOCX generator CLI preview created by OfficeCLI",
        "xlsx-dashboard": "XLSX automation preview created by OfficeCLI",
        "report-workbook": "Report generation preview created by OfficeCLI",
        "standalone-img": "Image generation CLI preview created by OfficeCLI",
    }
    lines = [
        "# OfficeCLI generation demos",
        "",
        "This gallery gives GitHub visitors and search engines concrete proof that OfficeCLI is an AI document generation CLI, not only an install script. Every demo includes a preview image, generated artifact, prompt, metadata, and a reproducible OfficeCLI command.",
        "",
        "## Reproducible demo matrix",
        "",
        "| Demo | Output type | Search keywords | Preview | Artifact | Reproduce |",
        "| --- | --- | --- | --- | --- | --- |",
    ]
    for slug, meta in rows:
        title = meta["title"]
        typ = meta["type"]
        artifact = meta["artifact"]
        prompt = meta["prompt_file"]
        preview = meta["preview"]
        command = meta["command"].replace("|", "\\|")
        keywords = keyword_map.get(slug, f"OfficeCLI {typ} generation")
        alt = alt_map.get(slug, f"OfficeCLI {typ} generation preview")
        lines.append(
            f"| {title} | `{typ}` | {keywords} | ![{alt}](./{slug}/{preview}) | "
            f"[{artifact}](./{slug}/{artifact}) | `{command}` with [prompt](./{slug}/{prompt}) |"
        )
    lines.extend([
        "",
        "## How to read a demo",
        "",
        "- `preview.png` is the GitHub-friendly visual proof shown in the README gallery.",
        "- The artifact is the generated `PPTX`, `DOCX`, `XLSX`, report, or `PNG` output.",
        "- `prompt.md` is the reusable input for the command.",
        "- `metadata.json` records the command, output type, verification timestamp, and checksums.",
        "",
        "## Verification",
        "",
        "The source repository validates this directory with:",
        "",
        "```bash",
        "scripts/validate-skills-demos.py public/skills-demos",
        "```",
        "",
        "Every artifact is kept under 3MB so the public skills repository stays lightweight.",
        "",
    ])
    demo_root.joinpath("README.md").write_text("\n".join(lines))

    ref_lines = [
        "# OfficeCLI demo gallery reference",
        "",
        "Use this reference when a user asks what OfficeCLI-generated outputs look like or wants a reproducible example.",
        "",
        "| Demo | Type | Public files |",
        "| --- | --- | --- |",
    ]
    for slug, meta in rows:
        ref_lines.append(
            f"| {meta['title']} | `{meta['type']}` | "
            f"`demos/{slug}/{meta['preview']}`, `demos/{slug}/{meta['artifact']}`, `demos/{slug}/{meta['prompt_file']}` |"
        )
    ref_lines.extend([
        "",
        "Public repository: https://github.com/officecli/officecli",
        "",
    ])
    for target in [
        Path("skills/officecli/references/demos.md"),
        Path("plugins/officecli/skills/officecli/references/demos.md"),
    ]:
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text("\n".join(ref_lines))
PY

cat > LICENSE <<'LICENSE'
MIT License

Copyright (c) 2026 officecli

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
LICENSE

if [[ -n "${PUBLIC_SKILLS_DRY_RUN_DIR}" ]]; then
  echo "public skills dry run ready: ${PUBLIC_SKILLS_DRY_RUN_DIR}"
  exit 0
fi

if git diff --quiet; then
  echo "public skills repository already up to date"
  exit 0
fi

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add .
git commit -m "chore: sync public skills"
git push origin "${PUBLIC_SKILLS_DEFAULT_BRANCH}"
