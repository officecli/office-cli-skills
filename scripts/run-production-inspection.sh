#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  cat <<'EOF'
Usage:
  bash ./scripts/run-production-inspection.sh <readonly|isolated>

Modes:
  readonly   只读生产巡检：首页、pricing、路由、app/admin、healthz
  isolated   隔离写入巡检：专用 fingerprint + 专用测试 key 的 license 闭环

Environment:
  REPORT_DIR=...                     巡检报告输出目录
  SITE_BASE_URL=...                  只读巡检官网基址，默认 https://officecli.io
  PLATFORM_BASE_URL=...              平台基址，默认 https://platform.officecli.io
  INSPECTION_FINGERPRINT_HASH=...    隔离写入巡检专用 fingerprint，必须以 inspection- 或 cron- 开头
  INSPECTION_API_KEY=...             隔离写入巡检专用可用 key
  INSPECTION_BLOCKED_API_KEY=...     隔离写入巡检专用 blocked key
  INSPECTION_USER_ID=...             可选，专用开发者 user_id
  INSPECTION_REQUEST_PREFIX=...      默认 inspection
EOF
}

MODE="${1:-}"

case "${MODE}" in
  readonly)
    bash "${SCRIPT_DIR}/production-readonly-checks.sh"
    ;;
  isolated)
    bash "${SCRIPT_DIR}/production-isolated-write-checks.sh"
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage
    exit 1
    ;;
esac
