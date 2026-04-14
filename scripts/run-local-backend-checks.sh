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

phase "Backend and CLI checks (${MODE})"

phase "CLI Go tests"
run_cmd bash -lc "cd '${REPO_ROOT}' && go test ./..."
pass "CLI Go tests completed"

phase "Platform Go tests"
run_cmd bash -lc "cd '${REPO_ROOT}/platform' && go test ./..."
pass "Platform Go tests completed"

if [[ "${MODE}" == "full" ]]; then
  phase "CLI build"
  run_cmd bash -lc "cd '${REPO_ROOT}' && make build"
  pass "CLI build completed"

  phase "Platform build"
  run_cmd bash -lc "cd '${REPO_ROOT}/platform' && make build"
  pass "Platform build completed"
fi

phase "Backend and CLI checks completed (${MODE})"
