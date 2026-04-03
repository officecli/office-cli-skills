#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./inspection-common.sh
source "${SCRIPT_DIR}/inspection-common.sh"

SITE_BASE_URL="${SITE_BASE_URL:-https://officecli.io}"
PLATFORM_BASE_URL="${PLATFORM_BASE_URL:-https://platform.officecli.io}"

init_report "readonly"

run_check() {
  local phase_name="$1"
  local check_name="$2"
  local method="$3"
  local url="$4"
  local expected_kind="$5"
  local detail="ok"
  local request_id=""
  local status="passed"

  phase "${phase_name}: ${check_name}"
  http_request "${method}" "${url}"

  if [[ "${HTTP_CURL_EXIT}" != "0" ]]; then
    detail="curl_exit=${HTTP_CURL_EXIT}"
    status="failed"
  fi

  if [[ "${status}" == "passed" ]]; then
    request_id="$(header_get "${HTTP_HEADERS_FILE}" "X-Request-Id")"
    local location
    location="$(header_get "${HTTP_HEADERS_FILE}" "Location")"
    local content_type
    content_type="$(header_get "${HTTP_HEADERS_FILE}" "Content-Type")"

    case "${expected_kind}" in
      site-home)
        [[ "${HTTP_STATUS}" == "200" ]] || { status="failed"; detail="expected 200"; }
        [[ "${status}" != "failed" && "${content_type}" == *"text/html"* ]] || { status="failed"; detail="expected html content"; }
        [[ "${status}" != "failed" ]] && body_matches '<html|officecli' "${HTTP_BODY_FILE}" || { [[ "${status}" == "failed" ]] || { status="failed"; detail="expected html body"; }; }
        ;;
      pricing)
        [[ "${HTTP_STATUS}" == "200" ]] || { status="failed"; detail="expected 200"; }
        [[ "${status}" != "failed" ]] && json_check_pricing_payload "${HTTP_BODY_FILE}" || { [[ "${status}" == "failed" ]] || { status="failed"; detail="pricing payload invalid"; }; }
        ;;
      office-app-redirect)
        [[ "${HTTP_STATUS}" =~ ^30[1278]$ ]] || { status="failed"; detail="expected redirect status"; }
        [[ "${status}" != "failed" && "${location}" == *"https://platform.officecli.io/app"* ]] || { [[ "${status}" == "failed" ]] || { status="failed"; detail="unexpected redirect location"; }; }
        ;;
      platform-root-redirect)
        [[ "${HTTP_STATUS}" =~ ^30[1278]$ ]] || { status="failed"; detail="expected redirect status"; }
        [[ "${status}" != "failed" && "${location}" == */app* ]] || { [[ "${status}" == "failed" ]] || { status="failed"; detail="unexpected redirect location"; }; }
        ;;
      platform-html)
        [[ "${HTTP_STATUS}" == "200" ]] || { status="failed"; detail="expected 200"; }
        [[ "${status}" != "failed" && "${content_type}" == *"text/html"* ]] || { [[ "${status}" == "failed" ]] || { status="failed"; detail="expected html content"; }; }
        [[ "${status}" != "failed" ]] && body_matches '<html|officecli|platform' "${HTTP_BODY_FILE}" || { [[ "${status}" == "failed" ]] || { status="failed"; detail="expected html body"; }; }
        ;;
      healthz)
        [[ "${HTTP_STATUS}" == "200" ]] || { status="failed"; detail="expected 200"; }
        [[ "${status}" != "failed" ]] && [[ "$(json_get "${HTTP_BODY_FILE}" "data.status" 2>/dev/null || true)" == "ok" ]] || { [[ "${status}" == "failed" ]] || { status="failed"; detail="healthz data.status != ok"; }; }
        if [[ "${status}" != "failed" ]]; then
          request_id="$(json_get "${HTTP_BODY_FILE}" "request_id" 2>/dev/null || true)"
          [[ -n "${request_id}" ]] || { status="failed"; detail="missing request_id"; }
        fi
        ;;
      *)
        status="failed"
        detail="unsupported expected kind: ${expected_kind}"
        ;;
    esac
  fi

  record_check "${phase_name}" "${check_name}" "${status}" "${method}" "${url}" "${HTTP_STATUS:-000}" "${request_id}" "${detail}"
  cleanup_http_files

  if [[ "${status}" == "passed" ]]; then
    pass "${check_name}"
  else
    warn "${check_name} failed: ${detail}"
  fi
}

run_check "readonly" "site_home" "GET" "${SITE_BASE_URL}/" "site-home"
run_check "readonly" "pricing_api" "GET" "${SITE_BASE_URL}/api/pricing" "pricing"
run_check "readonly" "office_app_redirect" "GET" "${SITE_BASE_URL}/app" "office-app-redirect"
run_check "readonly" "platform_root_redirect" "GET" "${PLATFORM_BASE_URL}/" "platform-root-redirect"
run_check "readonly" "platform_app_shell" "GET" "${PLATFORM_BASE_URL}/app/" "platform-html"
run_check "readonly" "platform_admin_shell" "GET" "${PLATFORM_BASE_URL}/admin/" "platform-html"
run_check "readonly" "platform_healthz" "GET" "${PLATFORM_BASE_URL}/healthz" "healthz"

finalize_report "readonly"

if [[ "${OVERALL_STATUS}" != "passed" ]]; then
  fail "readonly inspection failed"
fi

pass "readonly inspection passed"
