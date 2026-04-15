#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=skills/openclaw-officecli/env-common.sh
source "${SCRIPT_DIR}/env-common.sh"

uninstall_officecli_binary

printf '{"status":"ok","message":"officecli binary removed"}\n'
