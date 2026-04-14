#!/usr/bin/env bash

set -euo pipefail

PUBLIC_SKILLS_REPO="${PUBLIC_SKILLS_REPO:-}"
PUBLIC_SKILLS_DEFAULT_BRANCH="${PUBLIC_SKILLS_DEFAULT_BRANCH:-main}"
SOURCE_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PUBLIC_SKILL_NAME="officecli"
OPENCLAW_SKILL_NAME="openclaw-officecli"
CLAUDE_MARKETPLACE_DIR=".claude-plugin"
CLAUDE_PLUGIN_ROOT="plugins"
DIST_REPO="${DIST_REPO:-officecli/officecli-dist}"
HOMEBREW_TAP_REPO="${HOMEBREW_TAP_REPO:-officecli/homebrew-officecli}"
HOMEBREW_TAP_NAME="${HOMEBREW_TAP_NAME:-officecli/officecli}"
HOMEBREW_FORMULA="${HOMEBREW_FORMULA:-officecli}"

if [[ -z "${GH_TOKEN:-}" || -z "${PUBLIC_SKILLS_REPO}" ]]; then
  echo "GH_TOKEN and PUBLIC_SKILLS_REPO are required" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

git clone "https://x-access-token:${GH_TOKEN}@github.com/${PUBLIC_SKILLS_REPO}.git" "${tmpdir}/skills-repo"
cd "${tmpdir}/skills-repo"
git checkout "${PUBLIC_SKILLS_DEFAULT_BRANCH}"

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
EOF
INSTALLER

chmod +x scripts/install-openclaw-skill.sh

cat > README.md <<'README'
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
/plugin marketplace add __PUBLIC_SKILLS_REPO__
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

## OpenClaw Install

If you want OpenClaw users to generate local Office files directly from Telegram, Discord, Slack, or other channels, install the OpenClaw-facing skill package:

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

After installation, 3 more steps are still required before the skill can be used:

1. Configure `officecli` itself:

```bash
officecli config set-generation
officecli config set-license
```

2. Attach `openclaw-officecli` to your OpenClaw agent in `~/.openclaw/config.yaml` and ensure the agent can use `shell` and `file_read`.

3. Restart OpenClaw, then verify both `officecli --version` and `officecli agent-bridge` work on the same host where OpenClaw runs.

For PPT generation, agents should first read `initialize` / `capabilities/get` and use
`document_generation.pptx.image_support` as the machine-readable contract for image behavior.
If a user wants a text-only deck, the agent should send `enable_images=false`.
If a generated PPT has no images, rerun `officecli config set-generation` and check
`image_base_url`, `image_api_key`, and `image_model`.

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

if git diff --quiet; then
  echo "public skills repository already up to date"
  exit 0
fi

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add .
git commit -m "chore: sync public skills"
git push origin "${PUBLIC_SKILLS_DEFAULT_BRANCH}"
