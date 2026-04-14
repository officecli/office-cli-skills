#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  cat <<'EOF'
Usage:
  bash ./scripts/run-production-inspection.sh <readonly|isolated>

Modes:
  readonly   Read-only production inspection: homepage, pricing, routes, app/admin, healthz
  isolated   Isolated write inspection: license flow with a dedicated fingerprint and test keys

Environment:
  REPORT_DIR=...                     Output directory for inspection reports
  SITE_BASE_URL=...                  Base URL for read-only site checks, default https://officecli.io
  PLATFORM_BASE_URL=...              Platform base URL, default https://platform.officecli.io
  INSPECTION_FINGERPRINT_HASH=...    Dedicated fingerprint for isolated checks; must start with inspection- or cron-
  INSPECTION_API_KEY=...             Dedicated usable key for isolated checks
  INSPECTION_BLOCKED_API_KEY=...     Dedicated blocked key for isolated checks
  INSPECTION_USER_ID=...             Optional dedicated developer user_id
  INSPECTION_REQUEST_PREFIX=...      Default: inspection
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
