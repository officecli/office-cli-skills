#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./local-test-common.sh
source "${SCRIPT_DIR}/local-test-common.sh"

PLATFORM_BASE_URL="${PLATFORM_BASE_URL:-http://127.0.0.1:8080}"

phase "本地 smoke 检查"
info "platform base url: ${PLATFORM_BASE_URL}"

phase "preflight: healthz"
if ! curl -fsS "${PLATFORM_BASE_URL}/healthz" >/dev/null; then
  fail "无法访问 ${PLATFORM_BASE_URL}/healthz。请先启动本地 platform 及其依赖后再执行 make test-smoke"
fi
pass "platform healthz 可访问"

phase "usage limits smoke"
run_cmd bash "${REPO_ROOT}/scripts/usage-limits-smoke.sh"
pass "usage limits smoke 完成"

warn "当前 smoke 仅覆盖 license 闭环；CLI / app / admin 三端一致性后续可在此脚本继续扩展。"
