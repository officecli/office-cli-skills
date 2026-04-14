#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./local-test-common.sh
source "${SCRIPT_DIR}/local-test-common.sh"

PLATFORM_BASE_URL="${PLATFORM_BASE_URL:-http://127.0.0.1:8080}"

phase "Local smoke checks"
info "platform base url: ${PLATFORM_BASE_URL}"

phase "preflight: healthz"
if ! curl -fsS "${PLATFORM_BASE_URL}/healthz" >/dev/null; then
  fail "Cannot reach ${PLATFORM_BASE_URL}/healthz. Start the local platform stack and its dependencies before running make test-smoke"
fi
pass "platform healthz is reachable"

phase "usage limits smoke"
run_cmd bash "${REPO_ROOT}/scripts/usage-limits-smoke.sh"
pass "usage limits smoke completed"

warn "The current smoke suite only covers the license loop; CLI / app / admin consistency checks can be extended here later."
