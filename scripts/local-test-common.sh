#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

timestamp() {
  date +"%H:%M:%S"
}

info() {
  printf '[%s] [INFO] %s\n' "$(timestamp)" "$*"
}

pass() {
  printf '[%s] [PASS] %s\n' "$(timestamp)" "$*"
}

warn() {
  printf '[%s] [WARN] %s\n' "$(timestamp)" "$*" >&2
}

fail() {
  printf '[%s] [FAIL] %s\n' "$(timestamp)" "$*" >&2
  exit 1
}

phase() {
  printf '\n[%s] ===== %s =====\n' "$(timestamp)" "$*"
}

run_cmd() {
  info "执行命令: $*"
  "$@"
}
