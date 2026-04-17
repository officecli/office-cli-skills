#!/usr/bin/env bash

set -euo pipefail

SOURCE_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_SCRIPT="${SOURCE_REPO_ROOT}/scripts/install-officecli.sh"

expect_policy_failure() {
  local version="$1"
  local snippet="$2"
  local output=""
  local status=0

  set +e
  output="$(VERSION="${version}" DIST_REPO="officecli/officecli-dist" bash "${INSTALL_SCRIPT}" 2>&1)"
  status=$?
  set -e

  if [[ "${status}" -eq 0 ]]; then
    echo "expected install script to reject VERSION=${version}" >&2
    exit 1
  fi

  if ! printf '%s' "${output}" | rg -Fq "${snippet}"; then
    echo "expected error output for VERSION=${version} to include: ${snippet}" >&2
    echo "${output}" >&2
    exit 1
  fi
}

expect_policy_failure "latest" "Public distribution now keeps only the current stable release."
expect_policy_failure "v0.1.1" "Public distribution now keeps only the current stable release."
