#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./local-test-common.sh
source "${SCRIPT_DIR}/local-test-common.sh"

MODE="${1:-local}"

usage() {
  cat <<'EOF'
Usage:
  bash ./scripts/run-local-test-flow.sh <fast|full|smoke|local>

Modes:
  fast   运行快速本地回归：CLI Go tests、platform Go tests、app/admin 前端 tests
  full   运行完整本地回归：fast + CLI/platform build + app/admin/site tests/builds
  smoke  运行本地联调 smoke：要求 platform 服务已启动
  local  顺序执行 fast -> full -> smoke

Environment:
  LOCAL_TEST_SKIP_SITE=1   跳过 platform-site tests/build
  LOCAL_TEST_SKIP_BUILD=1  在 full 模式下跳过前端 builds
  PLATFORM_BASE_URL=...    指定 smoke 使用的 platform 地址
  FINGERPRINT_HASH=...     透传给 usage-limits-smoke.sh
  FREE_LIMIT=...           透传给 usage-limits-smoke.sh
EOF
}

case "${MODE}" in
  fast)
    phase "本地自动化测试流程: fast"
    run_cmd bash "${SCRIPT_DIR}/run-local-backend-checks.sh" fast
    run_cmd bash "${SCRIPT_DIR}/run-local-web-checks.sh" fast
    pass "fast 流程完成"
    ;;
  full)
    phase "本地自动化测试流程: full"
    run_cmd bash "${SCRIPT_DIR}/run-local-backend-checks.sh" full
    run_cmd bash "${SCRIPT_DIR}/run-local-web-checks.sh" full
    pass "full 流程完成"
    ;;
  smoke)
    phase "本地自动化测试流程: smoke"
    run_cmd bash "${SCRIPT_DIR}/run-local-smoke.sh"
    pass "smoke 流程完成"
    ;;
  local)
    phase "本地自动化测试流程: local"
    run_cmd bash "${SCRIPT_DIR}/run-local-test-flow.sh" fast
    run_cmd bash "${SCRIPT_DIR}/run-local-test-flow.sh" full
    run_cmd bash "${SCRIPT_DIR}/run-local-test-flow.sh" smoke
    pass "local 流程完成"
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage
    fail "unsupported mode: ${MODE}"
    ;;
esac
