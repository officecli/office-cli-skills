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
  PUBLIC_SKILLS_REPO="officecli/officecli-skills"
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

REPO_URL="${REPO_URL:-https://github.com/officecli/officecli-skills.git}"
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

REPO_URL="${REPO_URL:-https://github.com/officecli/officecli-skills.git}"
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
# OfficeCLI Skills for Claude Code, Codex, and AI Agents

`officecli-skills` is the public GitHub repository for OfficeCLI skills and plugin wrappers that help
Claude Code, Codex, and other AI agents run local Office document workflows. Use this repository when
you need an AI agent skill for `pptx`, `docx`, `xlsx`, workbook-backed `report`, or standalone `img`
tasks. Document generation stays on the same machine through a local `officecli` runtime; standalone
image generation is routed through the OfficeCLI server provider.

This repository is the public distribution surface for:

- Claude Code marketplace metadata
- the `officecli` skill for general Office document workflows
- the `openclaw-officecli` package for OpenClaw-oriented integrations
- direct install scripts for local Codex-style skill installs

## Fast links

Site pages:

- [Overview](https://officecli.io/officecli-skills)
- [Install](https://officecli.io/officecli-skills/install)
- [Claude Code](https://officecli.io/officecli-skills/claude-code)
- [Codex](https://officecli.io/officecli-skills/codex)
- [OpenClaw](https://officecli.io/officecli-skills/openclaw)
- [FAQ](https://officecli.io/officecli-skills/faq)

GitHub guides in this repository:

- [Install](./install/README.md)
- [Generation demos](./demos/README.md)
- [Claude Code](./claude-code/README.md)
- [Codex](./codex/README.md)
- [OpenClaw](./openclaw/README.md)
- [FAQ](./faq/README.md)

Related product page:

- `https://officecli.io/officecli-skills`

## What OfficeCLI Skills supports

The public `officecli` skill is designed for agent workflows such as:

- AI PPTX generation for decks, proposals, and executive briefings
- AI DOCX drafting for retrospectives, memos, and customer-facing documents
- AI XLSX generation for workbooks, trackers, and analysis sheets
- report workflows routed through OfficeCLI when a workbook-backed report artifact is needed
- standalone image generation through `office.generate` with server-controlled provider settings
- capability checks before execution so the agent can decide whether OfficeCLI supports the request

## Generation demos

These checked-in demos show the visible result and the reproducible inputs together. Each demo includes
a preview image, the generated artifact, `prompt.md`, `metadata.json`, and the command used to reproduce
the flow with a configured OfficeCLI runtime.

| Demo | Preview | Files |
| --- | --- | --- |
| Image-rich strategy deck | ![Image-rich strategy deck](./demos/pptx-image-rich/preview.png) | [PPTX](./demos/pptx-image-rich/image-rich-strategy-deck.pptx) · [Prompt](./demos/pptx-image-rich/prompt.md) · [Metadata](./demos/pptx-image-rich/metadata.json) |
| Text-only executive briefing | ![Text-only executive briefing](./demos/pptx-text-only/preview.png) | [PPTX](./demos/pptx-text-only/text-only-executive-briefing.pptx) · [Prompt](./demos/pptx-text-only/prompt.md) · [Metadata](./demos/pptx-text-only/metadata.json) |
| OfficeCLI Skills customer brief | ![OfficeCLI Skills customer brief](./demos/docx-brief/preview.png) | [DOCX](./demos/docx-brief/officecli-skills-customer-brief.docx) · [Prompt](./demos/docx-brief/prompt.md) · [Metadata](./demos/docx-brief/metadata.json) |
| Demo adoption dashboard | ![Demo adoption dashboard](./demos/xlsx-dashboard/preview.png) | [XLSX](./demos/xlsx-dashboard/demo-adoption-dashboard.xlsx) · [Prompt](./demos/xlsx-dashboard/prompt.md) · [Metadata](./demos/xlsx-dashboard/metadata.json) |
| Demo program readiness report | ![Demo program readiness report](./demos/report-workbook/preview.png) | [HTML report](./demos/report-workbook/demo-program-readiness-report.html) · [Source XLSX](./demos/report-workbook/demo-program-source-workbook.xlsx) · [Prompt](./demos/report-workbook/prompt.md) |
| OfficeCLI Skills hero image | ![OfficeCLI Skills hero image](./demos/standalone-img/preview.png) | [PNG](./demos/standalone-img/officecli-skills-hero-image.png) · [Prompt](./demos/standalone-img/prompt.md) · [Metadata](./demos/standalone-img/metadata.json) |

See [demos/README.md](./demos/README.md) for the complete reproducibility table and verification notes.

## Supported agent runtimes

### Claude Code

Use the marketplace source when you want Claude Code to install the OfficeCLI plugin directly.

Add the OfficeCLI marketplace source:

```text
/plugin marketplace add __PUBLIC_SKILLS_REPO__
```

Install the primary plugin:

```text
/plugin install officecli@officecli-skills
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
- local OfficeCLI generation and license configuration
- standalone image generation requires license configuration and does not use local generation image provider settings
- permission for the agent client to invoke local commands on the same machine

Quick verification after installation:

```bash
officecli --version
officecli config status
```

For OpenClaw, also verify:

```bash
officecli agent-bridge
```

## How OfficeCLI and officecli-skills fit together

- `OfficeCLI` is the local Office document engine
- `officecli-skills` is the public GitHub repository for skills, plugin wrappers, and installers
- `officecli` is the general skill for Claude Code, Codex, and other local agents
- `openclaw-officecli` is the OpenClaw-oriented package

## FAQ

### Is this repository a hosted SaaS plugin backend?

No. This repository distributes local skill wrappers, not a hosted plugin backend.

### Can Claude Code create PPTX, DOCX, XLSX, report, or img outputs with this repository?

Yes, when the local `officecli` runtime is installed and configured. The repository tells the agent how
to route supported Office and image tasks into OfficeCLI.

### Why does this repository mention Codex as well as Claude Code?

Because marketplace install is only one entrypoint. This repository also distributes direct skill files
for Codex-style local agents and other agent runtimes.

### Does this repository contain the OfficeCLI implementation?

No. It contains public skill definitions, plugin wrappers, examples, and install scripts only.

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

mkdir -p install claude-code codex openclaw faq

cat > install/README.md <<'README'
# Install officecli-skills

Use this guide when the search intent is specifically about how to install `officecli-skills`.

Main page:

- `https://officecli.io/officecli-skills/install`

Choose the install path that matches the runtime:

- [Claude Code](../claude-code/README.md)
- [Codex](../codex/README.md)
- [OpenClaw](../openclaw/README.md)

The public repository only distributes skill wrappers and installers. Final Office file generation still
depends on a working local `officecli` runtime.

Quick verification:

```bash
officecli --version
officecli config status
officecli agent-bridge
```
README

cat > claude-code/README.md <<'README'
# officecli-skills for Claude Code

Use this guide when you want the Claude Code marketplace installation path for `officecli-skills`.

Main page:

- `https://officecli.io/officecli-skills/claude-code`

Marketplace install:

```text
/plugin marketplace add __PUBLIC_SKILLS_REPO__
/plugin install officecli@officecli-skills
```

This path is for local Office workflows through Claude Code. It is not a hosted plugin backend.
README

cat > codex/README.md <<'README'
# officecli-skills for Codex

Use this guide when you want the direct local installer for Codex-style agents.

Main page:

- `https://officecli.io/officecli-skills/codex`

Direct install:

```bash
curl -fsSL https://raw.githubusercontent.com/__PUBLIC_SKILLS_REPO__/__PUBLIC_SKILLS_DEFAULT_BRANCH__/scripts/install-skill.sh | bash -s -- officecli
```

This path installs the public skill bundle directly on the local machine without a marketplace layer.
README

cat > openclaw/README.md <<'README'
# officecli-skills for OpenClaw

Use this guide when you want the OpenClaw-oriented package and `officecli agent-bridge`.

Main page:

- `https://officecli.io/officecli-skills/openclaw`

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
# officecli-skills FAQ

Use this guide when the question is not installation itself, but whether `officecli-skills` is the right entrypoint.

Main page:

- `https://officecli.io/officecli-skills/faq`

Key answers:

- `officecli-skills` is the public repository for wrappers, skills, and installers
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
    lines = [
        "# officecli-skills generation demos",
        "",
        "Each demo includes a preview image, generated artifact, prompt, metadata, and a reproducible OfficeCLI command.",
        "",
        "| Demo | Type | Preview | Artifact | Reproduce |",
        "| --- | --- | --- | --- | --- |",
    ]
    for slug, meta in rows:
        title = meta["title"]
        typ = meta["type"]
        artifact = meta["artifact"]
        prompt = meta["prompt_file"]
        preview = meta["preview"]
        command = meta["command"].replace("|", "\\|")
        lines.append(
            f"| {title} | `{typ}` | [preview](./{slug}/{preview}) | "
            f"[{artifact}](./{slug}/{artifact}) | `{command}` with [prompt](./{slug}/{prompt}) |"
        )
    lines.extend([
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
        "Public repository: https://github.com/officecli/officecli-skills",
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
