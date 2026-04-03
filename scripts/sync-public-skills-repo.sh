#!/usr/bin/env bash

set -euo pipefail

PUBLIC_SKILLS_REPO="${PUBLIC_SKILLS_REPO:-}"
PUBLIC_SKILLS_DEFAULT_BRANCH="${PUBLIC_SKILLS_DEFAULT_BRANCH:-main}"
SOURCE_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PUBLIC_SKILL_NAME="officecli"
DIST_REPO="${DIST_REPO:-officecli/officecli-dist}"
HOMEBREW_TAP_REPO="${HOMEBREW_TAP_REPO:-officecli/homebrew-officecli}"
HOMEBREW_TAP_NAME="${HOMEBREW_TAP_NAME:-officecli/officecli}"
HOMEBREW_FORMULA="${HOMEBREW_FORMULA:-officecli/officecli/officecli}"

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
mkdir -p "skills/${PUBLIC_SKILL_NAME}"
cp -R "${SOURCE_REPO_ROOT}/skills/${PUBLIC_SKILL_NAME}/." "./skills/${PUBLIC_SKILL_NAME}/"
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
HOMEBREW_FORMULA="${HOMEBREW_FORMULA:-officecli/officecli/officecli}"
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

echo "restart Codex to pick up the new skill"
INSTALLER

chmod +x scripts/install-skill.sh

cat > README.md <<README
# OfficeCLI Skills

This repository contains the public Codex skill for the closed-source \
\`officecli\` product.

## Install

### One-line install

Use \`wget\`:

\`\`\`bash
wget -qO- https://raw.githubusercontent.com/${PUBLIC_SKILLS_REPO}/${PUBLIC_SKILLS_DEFAULT_BRANCH}/scripts/install-skill.sh | bash -s -- officecli
\`\`\`

Or use \`curl\`:

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/${PUBLIC_SKILLS_REPO}/${PUBLIC_SKILLS_DEFAULT_BRANCH}/scripts/install-skill.sh | bash -s -- officecli
\`\`\`

The installer will:

- install the \`officecli\` skill into \`~/.codex/skills\`
- try to auto-install the \`officecli\` binary when it is missing
- use Homebrew on macOS when available, and fall back to direct binary install when brew fails
- use the public Linux installer and install into \`~/.local/bin\` by default

If \`officecli\` is still reported as not found after installation, first try the current-shell fix:

\`\`\`bash
export PATH="\$HOME/.local/bin:\$PATH"
officecli --version
\`\`\`

Then add \`~/.local/bin\` to the startup config for your shell if needed.

Re-running the same installer command refreshes the local skill to the latest version from GitHub.

If you only want the skill and do not want to auto-install the binary:

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/${PUBLIC_SKILLS_REPO}/${PUBLIC_SKILLS_DEFAULT_BRANCH}/scripts/install-skill.sh | AUTO_INSTALL_BINARY=0 bash -s -- officecli
\`\`\`

### Update

To update an existing local skill from GitHub, run the same install command again:

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/${PUBLIC_SKILLS_REPO}/${PUBLIC_SKILLS_DEFAULT_BRANCH}/scripts/install-skill.sh | bash -s -- officecli
\`\`\`

Or with \`wget\`:

\`\`\`bash
wget -qO- https://raw.githubusercontent.com/${PUBLIC_SKILLS_REPO}/${PUBLIC_SKILLS_DEFAULT_BRANCH}/scripts/install-skill.sh | bash -s -- officecli
\`\`\`

### Manual install

Copy the skill directory into your local Codex skills directory:

\`\`\`bash
mkdir -p ~/.codex/skills

tmpdir="\$(mktemp -d)"
git clone https://github.com/${PUBLIC_SKILLS_REPO}.git "\$tmpdir/repo"
cp -R "\$tmpdir/repo/skills/officecli" ~/.codex/skills/
\`\`\`

After copying, restart Codex.

## Scope

- Public \`SKILL.md\` content and examples
- No closed-source \`officecli\` implementation code
- No private repository metadata or internal deployment details

## Layout

- \`skills/officecli/\`: public skill definition
- \`scripts/install-skill.sh\`: shell installer for direct \`wget\` / \`curl\` usage
README

if git diff --quiet; then
  echo "public skills repository already up to date"
  exit 0
fi

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add .
git commit -m "chore: sync public skills"
git push origin "${PUBLIC_SKILLS_DEFAULT_BRANCH}"
