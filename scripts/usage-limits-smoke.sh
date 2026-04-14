#!/usr/bin/env bash

set -euo pipefail

PLATFORM_BASE_URL="${PLATFORM_BASE_URL:-http://127.0.0.1:8080}"
FINGERPRINT_HASH="${FINGERPRINT_HASH:-usage-limits-smoke-machine}"
FREE_LIMIT="${FREE_LIMIT:-2}"

info() {
  printf '[INFO] %s\n' "$*"
}

pass() {
  printf '[PASS] %s\n' "$*"
}

fail() {
  printf '[FAIL] %s\n' "$*" >&2
  exit 1
}

extract_json() {
  local json="$1"
  local expr="$2"
  printf '%s' "$json" | jq -er "$expr"
}

request() {
  local method="$1"
  local path="$2"
  local body="$3"
  local tmp
  tmp="$(mktemp)"
  local status
  status="$(curl -sS -o "$tmp" -w '%{http_code}' -X "$method" "${PLATFORM_BASE_URL}${path}" -H 'Content-Type: application/json' -d "$body")"
  cat "$tmp"
  rm -f "$tmp"
  printf '\n__STATUS__=%s\n' "$status"
}

extract_status() {
  printf '%s' "$1" | awk -F= '/__STATUS__=/{print $2}' | tail -n1
}

extract_body() {
  printf '%s' "$1" | sed '/^__STATUS__=/d'
}

assert_status() {
  local got="$1"
  local want="$2"
  local message="$3"
  [[ "$got" == "$want" ]] || fail "$message: status=$got want=$want"
  pass "$message"
}

assert_body_contains() {
  local body="$1"
  local needle="$2"
  local message="$3"
  printf '%s' "$body" | rg -q "$needle" || fail "$message: body=$body"
  pass "$message"
}

info "platform=${PLATFORM_BASE_URL}"
info "fingerprint=${FINGERPRINT_HASH}"
info "Recommendation: set the free_limit for this fingerprint to ${FREE_LIMIT} in the admin backend or database first"

info "1) First free check"
resp="$(request POST /api/license/check "$(cat <<JSON
{
  "fingerprint_hash":"${FINGERPRINT_HASH}",
  "request_nonce":"${FINGERPRINT_HASH}-nonce-1",
  "action":"generate"
}
JSON
)")"
status="$(extract_status "$resp")"
body="$(extract_body "$resp")"
assert_status "$status" "200" "First free check returns 200"
assert_body_contains "$body" '"access_mode":"free"' "First free check returns free mode"
assert_body_contains "$body" '"allowed":true' "First free check is allowed"
commit_token_1="$(extract_json "$body" '.data.commit_token')"
request_id_1="$(extract_json "$body" '.data.commit_token.request_id')"

info "2) First free consume"
resp="$(request POST /api/license/consume "$(cat <<JSON
{
  "fingerprint_hash":"${FINGERPRINT_HASH}",
  "request_id":"${request_id_1}",
  "usage_type":"generate",
  "access_mode":"free",
  "commit_token":${commit_token_1}
}
JSON
)")"
status="$(extract_status "$resp")"
body="$(extract_body "$resp")"
assert_status "$status" "200" "First free consume returns 200"
assert_body_contains "$body" '"access_mode":"free"' "First free consume returns free mode"

info "3) Second free check"
resp="$(request POST /api/license/check "$(cat <<JSON
{
  "fingerprint_hash":"${FINGERPRINT_HASH}",
  "request_nonce":"${FINGERPRINT_HASH}-nonce-2",
  "action":"generate"
}
JSON
)")"
status="$(extract_status "$resp")"
body="$(extract_body "$resp")"
assert_status "$status" "200" "Second free check returns 200"
assert_body_contains "$body" '"access_mode":"free"' "Second free check returns free mode"
assert_body_contains "$body" '"allowed":true' "Second free check is allowed"
commit_token_2="$(extract_json "$body" '.data.commit_token')"
request_id_2="$(extract_json "$body" '.data.commit_token.request_id')"

info "4) Second free consume"
resp="$(request POST /api/license/consume "$(cat <<JSON
{
  "fingerprint_hash":"${FINGERPRINT_HASH}",
  "request_id":"${request_id_2}",
  "usage_type":"generate",
  "access_mode":"free",
  "commit_token":${commit_token_2}
}
JSON
)")"
status="$(extract_status "$resp")"
body="$(extract_body "$resp")"
if [[ "$FREE_LIMIT" -ge 2 ]]; then
  assert_status "$status" "200" "Second free consume returns 200"
else
  assert_status "$status" "409" "Second free consume returns 409"
fi

info "5) Check again and verify blocking after free quota exhaustion"
resp="$(request POST /api/license/check "$(cat <<JSON
{
  "fingerprint_hash":"${FINGERPRINT_HASH}",
  "request_nonce":"${FINGERPRINT_HASH}-nonce-overflow",
  "action":"generate"
}
JSON
)")"
status="$(extract_status "$resp")"
body="$(extract_body "$resp")"
assert_status "$status" "200" "Quota-exhausted check still returns 200"
assert_body_contains "$body" '"allowed":false' "Quota-exhausted check returns blocked"
assert_body_contains "$body" '"reason_code":"free_quota_exhausted"' "Quota-exhausted reason code is correct"

info "6) Invalid JSON returns 400"
tmp="$(mktemp)"
status="$(curl -sS -o "$tmp" -w '%{http_code}' -X POST "${PLATFORM_BASE_URL}/api/license/check" -H 'Content-Type: application/json' -d '{invalid')"
body="$(cat "$tmp")"
rm -f "$tmp"
assert_status "$status" "400" "Invalid JSON returns 400"

pass "usage limits smoke completed"
