#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./local-test-common.sh
source "${SCRIPT_DIR}/local-test-common.sh"

MODE="${1:-fast}"
SKIP_SITE="${LOCAL_TEST_SKIP_SITE:-0}"
SKIP_BUILD="${LOCAL_TEST_SKIP_BUILD:-0}"

case "${MODE}" in
  fast|full) ;;
  *)
    fail "unsupported mode: ${MODE} (expected: fast|full)"
    ;;
esac

phase "前端检查 (${MODE})"

phase "platform-app tests"
run_cmd bash -lc "cd '${REPO_ROOT}/platform/web/app' && npm test -- --run"
pass "platform-app tests 完成"

phase "platform-admin tests"
run_cmd bash -lc "cd '${REPO_ROOT}/platform/web/admin' && npm test -- --run"
pass "platform-admin tests 完成"

if [[ "${MODE}" == "full" ]]; then
  if [[ "${SKIP_SITE}" != "1" ]]; then
    phase "platform-site tests"
    run_cmd bash -lc "cd '${REPO_ROOT}/platform/web/site' && npm test -- --run"
    pass "platform-site tests 完成"
  else
    warn "LOCAL_TEST_SKIP_SITE=1，跳过 platform-site tests"
  fi

  if [[ "${SKIP_BUILD}" != "1" ]]; then
    phase "platform web builds"
    run_cmd bash -lc "cd '${REPO_ROOT}/platform/web/app' && npm run build"
    run_cmd bash -lc "cd '${REPO_ROOT}/platform/web/admin' && npm run build"
    if [[ "${SKIP_SITE}" != "1" ]]; then
      run_cmd bash -lc "cd '${REPO_ROOT}/platform/web/site' && npm run build"
    fi
    pass "platform web builds 完成"
  else
    warn "LOCAL_TEST_SKIP_BUILD=1，跳过前端 build"
  fi
fi

phase "前端检查完成 (${MODE})"
