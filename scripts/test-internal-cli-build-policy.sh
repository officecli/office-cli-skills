#!/usr/bin/env bash

set -euo pipefail

WORKFLOW_PATH="${1:-.github/workflows/internal-cli-build.yml}"
README_PATH=".github/workflows/README.md"
DOC_PATH="docs/internal-test-build.md"

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

require_file() {
  local path="$1"
  [[ -f "${path}" ]] || fail "missing required file: ${path}"
}

require_contains() {
  local path="$1"
  local pattern="$2"
  rg -n --fixed-strings -- "${pattern}" "${path}" >/dev/null || fail "${path} does not contain: ${pattern}"
}

reject_contains() {
  local path="$1"
  local pattern="$2"
  if rg -n --fixed-strings -- "${pattern}" "${path}" >/dev/null; then
    fail "${path} must not contain: ${pattern}"
  fi
}

require_file "${WORKFLOW_PATH}"
require_file "${README_PATH}"
require_file "${DOC_PATH}"

require_contains "${WORKFLOW_PATH}" "workflow_dispatch:"
reject_contains "${WORKFLOW_PATH}" "push:"
reject_contains "${WORKFLOW_PATH}" "pull_request:"
reject_contains "${WORKFLOW_PATH}" "release:"

require_contains "${WORKFLOW_PATH}" "refs/tags/"
require_contains "${WORKFLOW_PATH}" "internal"
require_contains "${WORKFLOW_PATH}" "internal_version may only contain"
require_contains "${WORKFLOW_PATH}" ".tmp/internal-builds/"
require_contains "${WORKFLOW_PATH}" "actions/upload-artifact"
require_contains "${WORKFLOW_PATH}" "retention-days: 7"
require_contains "${WORKFLOW_PATH}" "checksums.txt"
require_contains "${WORKFLOW_PATH}" "make release"

reject_contains "${WORKFLOW_PATH}" "gh release"
reject_contains "${WORKFLOW_PATH}" "npm publish"
reject_contains "${WORKFLOW_PATH}" "sync-public-"
reject_contains "${WORKFLOW_PATH}" "sync-homebrew"
reject_contains "${WORKFLOW_PATH}" "git tag"
reject_contains "${WORKFLOW_PATH}" "git push"
reject_contains "${WORKFLOW_PATH}" "DIST_REPO"
reject_contains "${WORKFLOW_PATH}" "NPM_TOKEN"
reject_contains "${WORKFLOW_PATH}" "GH_TOKEN"

require_contains "${README_PATH}" "internal-cli-build.yml"
require_contains "${README_PATH}" "only exception"
require_contains "${README_PATH}" "must not be used as a release control plane"

require_contains "${DOC_PATH}" "0.2.28-internal"
require_contains "${DOC_PATH}" "must not be uploaded to any public repository"
require_contains "${DOC_PATH}" 'must be rebuilt through the public `CLI Release` / `NPM Publish` flow'

printf 'internal CLI build policy checks passed\n'
