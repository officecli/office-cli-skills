#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./inspection-common.sh
source "${SCRIPT_DIR}/inspection-common.sh"

PLATFORM_BASE_URL="${PLATFORM_BASE_URL:-https://platform.officecli.io}"
INSPECTION_FINGERPRINT_HASH="${INSPECTION_FINGERPRINT_HASH:-}"
INSPECTION_API_KEY="${INSPECTION_API_KEY:-}"
INSPECTION_BLOCKED_API_KEY="${INSPECTION_BLOCKED_API_KEY:-}"
INSPECTION_USER_ID="${INSPECTION_USER_ID:-}"
INSPECTION_REQUEST_PREFIX="${INSPECTION_REQUEST_PREFIX:-inspection}"

init_report "isolated-write"

skip_if_missing_env() {
  local missing=()

  [[ -n "${INSPECTION_FINGERPRINT_HASH}" ]] || missing+=("INSPECTION_FINGERPRINT_HASH")
  [[ -n "${INSPECTION_API_KEY}" ]] || missing+=("INSPECTION_API_KEY")
  [[ -n "${INSPECTION_BLOCKED_API_KEY}" ]] || missing+=("INSPECTION_BLOCKED_API_KEY")

  if [[ "${#missing[@]}" -eq 0 ]]; then
    return 0
  fi

  record_check "setup" "isolated_write_prerequisites" "skipped" "ENV" "isolated-write" "N/A" "" "missing: ${missing[*]}"
  OVERALL_STATUS="skipped"
  finalize_report "isolated-write"
  pass "isolated-write skipped: missing dedicated inspection secrets (${missing[*]})"
  exit 0
}

validate_isolation_contract() {
  local status="passed"
  local detail="ok"

  if [[ ! "${INSPECTION_FINGERPRINT_HASH}" =~ ^(inspection|cron)- ]]; then
    status="failed"
    detail="fingerprint must start with inspection- or cron-"
  elif [[ ! "${INSPECTION_REQUEST_PREFIX}" =~ ^(inspection|cron)$ ]]; then
    status="failed"
    detail="request prefix must be inspection or cron"
  fi

  record_check "setup" "isolation_contract" "${status}" "ENV" "inspection-contract" "N/A" "" "${detail}"
  if [[ "${status}" != "passed" ]]; then
    finalize_report "isolated-write"
    fail "isolation contract validation failed"
  fi
}

skip_if_missing_env
validate_isolation_contract

build_request_id() {
  printf '%s-%s-%s' "${INSPECTION_REQUEST_PREFIX}" "${GITHUB_RUN_ID:-manual}" "${1}"
}

build_check_json() {
  local api_key="$1"
  local request_nonce="$2"
  python3 - "$INSPECTION_FINGERPRINT_HASH" "$INSPECTION_USER_ID" "$api_key" "$request_nonce" <<'PY'
import json
import sys

fingerprint, user_id, api_key, request_nonce = sys.argv[1:]
check_body = {
    "fingerprint_hash": fingerprint,
    "api_key": api_key,
    "action": "generate",
    "document_type": "pptx",
    "request_nonce": request_nonce,
}
if user_id:
    check_body["user_id"] = int(user_id)
print(json.dumps(check_body))
PY
}

build_consume_json() {
  local request_id="$1"
  local access_mode="$2"
  local api_key="$3"
  local commit_token_json="$4"
  python3 - "$INSPECTION_FINGERPRINT_HASH" "$INSPECTION_USER_ID" "$request_id" "$access_mode" "$api_key" "$commit_token_json" <<'PY'
import json
import sys

fingerprint, user_id, request_id, access_mode, api_key, commit_token_json = sys.argv[1:]
consume_body = {
    "fingerprint_hash": fingerprint,
    "request_id": request_id,
    "usage_type": "generate",
    "access_mode": access_mode,
    "api_key": api_key,
    "commit_token": json.loads(commit_token_json),
}
if user_id:
    consume_body["user_id"] = int(user_id)
print(json.dumps(consume_body))
PY
}

run_api_check() {
  local phase_name="$1"
  local check_name="$2"
  local method="$3"
  local url="$4"
  local body="$5"
  local status="passed"
  local detail="ok"
  local request_id=""

  phase "${phase_name}: ${check_name}"
  http_request "${method}" "${url}" "${body}"

  if [[ "${HTTP_CURL_EXIT}" != "0" ]]; then
    status="failed"
    detail="curl_exit=${HTTP_CURL_EXIT}"
  fi

  if [[ "${status}" == "passed" ]]; then
    request_id="$(json_get "${HTTP_BODY_FILE}" "request_id" 2>/dev/null || true)"
    [[ "${HTTP_STATUS}" == "200" ]] || { status="failed"; detail="expected 200"; }
  fi

  record_check "${phase_name}" "${check_name}" "${status}" "${method}" "${url}" "${HTTP_STATUS:-000}" "${request_id}" "${detail}"

  if [[ "${status}" == "passed" ]]; then
    pass "${check_name}"
  else
    warn "${check_name} failed: ${detail}"
  fi
}

request_nonce_one="$(build_request_id one)"
active_check_body="$(build_check_json "${INSPECTION_API_KEY}" "${request_nonce_one}")"
active_consume_body=""

run_api_check "isolated-write" "active_key_check" "POST" "${PLATFORM_BASE_URL}/api/license/check" "${active_check_body}"
if [[ "${OVERALL_STATUS}" == "passed" ]]; then
  allowed="$(json_get "${HTTP_BODY_FILE}" "data.allowed" 2>/dev/null || true)"
  access_mode="$(json_get "${HTTP_BODY_FILE}" "data.access_mode" 2>/dev/null || true)"
  active_request_id="$(json_get "${HTTP_BODY_FILE}" "data.commit_token.request_id" 2>/dev/null || true)"
  active_commit_token="$(json_get "${HTTP_BODY_FILE}" "data.commit_token" 2>/dev/null || true)"
  if [[ "${allowed}" != "true" || "${access_mode}" != "paid" ]]; then
    record_check "isolated-write" "active_key_check_contract" "failed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/check" "${HTTP_STATUS}" "$(json_get "${HTTP_BODY_FILE}" "request_id" 2>/dev/null || true)" "expected allowed=true and access_mode=paid"
    OVERALL_STATUS="failed"
  elif [[ -z "${active_request_id}" || -z "${active_commit_token}" ]]; then
    record_check "isolated-write" "active_key_check_contract" "failed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/check" "${HTTP_STATUS}" "$(json_get "${HTTP_BODY_FILE}" "request_id" 2>/dev/null || true)" "commit_token is missing from check response"
    OVERALL_STATUS="failed"
  else
    record_check "isolated-write" "active_key_check_contract" "passed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/check" "${HTTP_STATUS}" "$(json_get "${HTTP_BODY_FILE}" "request_id" 2>/dev/null || true)" "allowed paid check verified"
    active_consume_body="$(build_consume_json "${active_request_id}" "${access_mode}" "${INSPECTION_API_KEY}" "${active_commit_token}")"
  fi
fi
cleanup_http_files

run_api_check "isolated-write" "active_key_consume" "POST" "${PLATFORM_BASE_URL}/api/license/consume" "${active_consume_body}"
first_remaining=""
first_request_id=""
if [[ "${OVERALL_STATUS}" == "passed" ]]; then
  access_mode="$(json_get "${HTTP_BODY_FILE}" "data.access_mode" 2>/dev/null || true)"
  first_remaining="$(json_get "${HTTP_BODY_FILE}" "data.remaining" 2>/dev/null || true)"
  first_request_id="$(json_get "${HTTP_BODY_FILE}" "request_id" 2>/dev/null || true)"
  if [[ "${access_mode}" != "paid" || -z "${first_remaining}" ]]; then
    record_check "isolated-write" "active_key_consume_contract" "failed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/consume" "${HTTP_STATUS}" "${first_request_id}" "expected paid consume response with remaining"
    OVERALL_STATUS="failed"
  else
    record_check "isolated-write" "active_key_consume_contract" "passed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/consume" "${HTTP_STATUS}" "${first_request_id}" "paid consume verified"
  fi
fi
cleanup_http_files

run_api_check "isolated-write" "active_key_consume_idempotent_retry" "POST" "${PLATFORM_BASE_URL}/api/license/consume" "${active_consume_body}"
if [[ "${OVERALL_STATUS}" == "passed" ]]; then
  retry_remaining="$(json_get "${HTTP_BODY_FILE}" "data.remaining" 2>/dev/null || true)"
  retry_request_id="$(json_get "${HTTP_BODY_FILE}" "request_id" 2>/dev/null || true)"
  if [[ -z "${retry_remaining}" || "${retry_remaining}" != "${first_remaining}" ]]; then
    record_check "isolated-write" "active_key_consume_idempotent_contract" "failed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/consume" "${HTTP_STATUS}" "${retry_request_id}" "expected same remaining on idempotent retry"
    OVERALL_STATUS="failed"
  else
    record_check "isolated-write" "active_key_consume_idempotent_contract" "passed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/consume" "${HTTP_STATUS}" "${retry_request_id}" "idempotent retry verified"
  fi
fi
cleanup_http_files

blocked_check_body="$(build_check_json "${INSPECTION_BLOCKED_API_KEY}" "$(build_request_id blocked)")"

run_api_check "isolated-write" "blocked_key_check" "POST" "${PLATFORM_BASE_URL}/api/license/check" "${blocked_check_body}"
if [[ "${OVERALL_STATUS}" == "passed" ]]; then
  allowed="$(json_get "${HTTP_BODY_FILE}" "data.allowed" 2>/dev/null || true)"
  access_mode="$(json_get "${HTTP_BODY_FILE}" "data.access_mode" 2>/dev/null || true)"
  reason_code="$(json_get "${HTTP_BODY_FILE}" "data.reason_code" 2>/dev/null || true)"
  blocked_request_id="$(json_get "${HTTP_BODY_FILE}" "request_id" 2>/dev/null || true)"
  if [[ "${allowed}" != "false" || "${access_mode}" != "blocked" ]]; then
    record_check "isolated-write" "blocked_key_check_contract" "failed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/check" "${HTTP_STATUS}" "${blocked_request_id}" "expected blocked response"
    OVERALL_STATUS="failed"
  elif [[ ! "${reason_code}" =~ ^(disabled_api_key|expired_api_key|paid_quota_exhausted)$ ]]; then
    record_check "isolated-write" "blocked_key_check_contract" "failed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/check" "${HTTP_STATUS}" "${blocked_request_id}" "unexpected reason_code=${reason_code}"
    OVERALL_STATUS="failed"
  else
    record_check "isolated-write" "blocked_key_check_contract" "passed" "ASSERT" "${PLATFORM_BASE_URL}/api/license/check" "${HTTP_STATUS}" "${blocked_request_id}" "blocked key verified"
  fi
fi
cleanup_http_files

finalize_report "isolated-write"

if [[ "${OVERALL_STATUS}" != "passed" ]]; then
  fail "isolated-write inspection failed"
fi

pass "isolated-write inspection passed"
