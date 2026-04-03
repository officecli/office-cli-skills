#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./local-test-common.sh
source "${SCRIPT_DIR}/local-test-common.sh"

MODE="${1:-fast}"

case "${MODE}" in
  fast|full) ;;
  *)
    fail "unsupported mode: ${MODE} (expected: fast|full)"
    ;;
esac

phase "后端与 CLI 检查 (${MODE})"

phase "CLI Go tests"
run_cmd bash -lc "cd '${REPO_ROOT}' && go test ./..."
pass "CLI Go tests 完成"

phase "Platform Go tests"
run_cmd bash -lc "cd '${REPO_ROOT}/platform' && go test ./..."
pass "Platform Go tests 完成"

if [[ "${MODE}" == "full" ]]; then
  phase "CLI build"
  run_cmd bash -lc "cd '${REPO_ROOT}' && make build"
  pass "CLI build 完成"

  phase "Platform build"
  run_cmd bash -lc "cd '${REPO_ROOT}/platform' && make build"
  pass "Platform build 完成"
fi

phase "后端与 CLI 检查完成 (${MODE})"
