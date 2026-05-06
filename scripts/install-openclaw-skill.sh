#!/usr/bin/env bash

set -euo pipefail

SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OPENCLAW_HOME="${OPENCLAW_HOME:-${HOME}/.openclaw}"
SKILL_NAME="${SKILL_NAME:-openclaw-officecli}"
SOURCE_SKILL_DIR="${SOURCE_ROOT}/skills/${SKILL_NAME}"
DEST_SKILL_DIR="${OPENCLAW_HOME}/skills/${SKILL_NAME}"

if [[ ! -d "${SOURCE_SKILL_DIR}" ]]; then
  echo "skill source directory not found: ${SOURCE_SKILL_DIR}" >&2
  exit 1
fi

mkdir -p "${OPENCLAW_HOME}/skills"
rm -rf "${DEST_SKILL_DIR}"
cp -R "${SOURCE_SKILL_DIR}" "${DEST_SKILL_DIR}"

if [[ ! -f "${DEST_SKILL_DIR}/config.yaml" && -f "${DEST_SKILL_DIR}/config.example.yaml" ]]; then
  cp "${DEST_SKILL_DIR}/config.example.yaml" "${DEST_SKILL_DIR}/config.yaml"
fi

echo "installed OpenClaw skill to ${DEST_SKILL_DIR}"

if command -v officecli >/dev/null 2>&1; then
  echo "detected officecli: $(command -v officecli)"
else
  echo "warning: officecli is not in PATH; install it before using this skill" >&2
fi

cat <<EOF

Next steps:
1. Check env: bash ${DEST_SKILL_DIR}/check-officecli-env.sh
2. Repair env if needed: bash ${DEST_SKILL_DIR}/fix-officecli-env.sh
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

User quickstart:
  ${SOURCE_ROOT}/docs/openclaw-user-quickstart.md
EOF
