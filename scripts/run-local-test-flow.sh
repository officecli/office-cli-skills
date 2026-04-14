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
  fast   Run a quick local regression: CLI Go tests, platform Go tests, app/admin frontend tests
  full   Run the full local regression: fast + CLI/platform build + app/admin/site tests and builds
  smoke  Run the local integration smoke flow; requires the platform service to be up
  local  Run fast -> full -> smoke in sequence

Environment:
  LOCAL_TEST_SKIP_SITE=1   Skip platform-site tests/build
  LOCAL_TEST_SKIP_BUILD=1  Skip frontend builds in full mode
  PLATFORM_BASE_URL=...    Set the platform URL used by smoke
  FINGERPRINT_HASH=...     Pass through to usage-limits-smoke.sh
  FREE_LIMIT=...           Pass through to usage-limits-smoke.sh
EOF
}

case "${MODE}" in
  fast)
    phase "Local automation flow: fast"
    run_cmd bash "${SCRIPT_DIR}/run-local-backend-checks.sh" fast
    run_cmd bash "${SCRIPT_DIR}/run-local-web-checks.sh" fast
    pass "fast flow completed"
    ;;
  full)
    phase "Local automation flow: full"
    run_cmd bash "${SCRIPT_DIR}/run-local-backend-checks.sh" full
    run_cmd bash "${SCRIPT_DIR}/run-local-web-checks.sh" full
    pass "full flow completed"
    ;;
  smoke)
    phase "Local automation flow: smoke"
    run_cmd bash "${SCRIPT_DIR}/run-local-smoke.sh"
    pass "smoke flow completed"
    ;;
  local)
    phase "Local automation flow: local"
    run_cmd bash "${SCRIPT_DIR}/run-local-test-flow.sh" fast
    run_cmd bash "${SCRIPT_DIR}/run-local-test-flow.sh" full
    run_cmd bash "${SCRIPT_DIR}/run-local-test-flow.sh" smoke
    pass "local flow completed"
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage
    fail "unsupported mode: ${MODE}"
    ;;
esac
