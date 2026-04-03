#!/usr/bin/env bash

set -euo pipefail

DIST_REPO="${DIST_REPO:-}"
DIST_DEFAULT_BRANCH="${DIST_DEFAULT_BRANCH:-main}"
HOMEBREW_TAP_REPO="${HOMEBREW_TAP_REPO:-}"
SOURCE_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -z "${GH_TOKEN:-}" || -z "${DIST_REPO}" ]]; then
  echo "GH_TOKEN and DIST_REPO are required" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

git clone "https://x-access-token:${GH_TOKEN}@github.com/${DIST_REPO}.git" "${tmpdir}/dist"
cd "${tmpdir}/dist"
git checkout "${DIST_DEFAULT_BRANCH}"

mkdir -p scripts
find ./scripts -maxdepth 1 -type f ! -name 'install-officecli.sh' -delete
cp "${SOURCE_REPO_ROOT}/scripts/install-officecli.sh" ./scripts/install-officecli.sh
chmod +x ./scripts/install-officecli.sh

cat > README.md <<EOF
# OfficeCLI Distribution

This repository publishes public release assets for the closed-source \`officecli\` binary.

## Install

### macOS (Homebrew)

\`\`\`bash
brew tap officecli/officecli
brew install officecli/officecli/officecli
\`\`\`

To update later:

\`\`\`bash
brew upgrade officecli/officecli/officecli
\`\`\`

### Linux

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/${DIST_REPO}/${DIST_DEFAULT_BRANCH}/scripts/install-officecli.sh | DIST_REPO=${DIST_REPO} bash
\`\`\`

Re-running the same installer command refreshes the local binary to the latest published version.

If your shell still reports \`officecli: command not found\`, first try:

\`\`\`bash
export PATH="\$HOME/.local/bin:\$PATH"
officecli --version
\`\`\`

If that works, add \`~/.local/bin\` to your shell startup file so future shells can find the command.

To install a specific version, set \`VERSION\` before invoking the script:

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/${DIST_REPO}/${DIST_DEFAULT_BRANCH}/scripts/install-officecli.sh | VERSION=v0.1.1 DIST_REPO=${DIST_REPO} bash
\`\`\`

## Manual Download

Download archives and \`checksums.txt\` from the Releases page of this repository.

## Notes

- This repository contains binaries, checksums, and install helpers only.
- It does not contain \`officecli\` source code.
EOF

if [[ -z "$(git status --short -- README.md scripts/)" ]]; then
  echo "public dist repository already up to date"
  exit 0
fi

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add README.md scripts/
git commit -m "chore: sync public distribution docs"
git push origin "${DIST_DEFAULT_BRANCH}"
