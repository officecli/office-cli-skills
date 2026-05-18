#!/usr/bin/env bash

set -euo pipefail

TEST_DOMAIN="${TEST_DOMAIN:-officecli.shimodev.com}"
TEST_SCHEME="${TEST_SCHEME:-https}"
TEST_RESOLVE_IP="${TEST_RESOLVE_IP:-}"
BASE_URL="${TEST_BASE_URL:-${TEST_SCHEME}://${TEST_DOMAIN}}"

curl_args=(curl -fsSL -D - -o /tmp/officecli-test-inspection-body)
if [[ -n "${TEST_RESOLVE_IP}" ]]; then
  curl_args+=(--noproxy "*" --resolve "${TEST_DOMAIN}:80:${TEST_RESOLVE_IP}" --resolve "${TEST_DOMAIN}:443:${TEST_RESOLVE_IP}")
fi

check() {
  local name="$1"
  local path="$2"
  local expected="$3"
  local url="${BASE_URL}${path}"

  echo "--- ${name}: ${url}"
  rm -f /tmp/officecli-test-inspection-body
  "${curl_args[@]}" "${url}" >/tmp/officecli-test-inspection-headers
  case "${expected}" in
    html)
      grep -Eiq '<html|officecli' /tmp/officecli-test-inspection-body
      ;;
    json)
      grep -Eq '"data"|"pricing"|"status"' /tmp/officecli-test-inspection-body
      ;;
    healthz)
      grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' /tmp/officecli-test-inspection-body
      ;;
    *)
      echo "unsupported expected kind: ${expected}" >&2
      return 1
      ;;
  esac
}

check site_home "/" html
check pricing_api "/api/pricing" json
check app_shell "/app/" html
check admin_shell "/admin/" html
check healthz "/healthz" healthz

echo "officecli testing environment inspection passed: ${BASE_URL}"
