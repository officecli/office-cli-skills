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
info "说明：建议先在后台或数据库中将该 fingerprint 的 free_limit 设为 ${FREE_LIMIT}"

info "1) 免费首次 check"
resp="$(request POST /api/license/check "$(cat <<JSON
{
  "fingerprint_hash":"${FINGERPRINT_HASH}",
  "action":"generate"
}
JSON
)")"
status="$(extract_status "$resp")"
body="$(extract_body "$resp")"
assert_status "$status" "200" "免费首次 check 返回 200"
assert_body_contains "$body" '"access_mode":"free"' "免费首次 check 返回 free 模式"

info "2) 免费 consume 第 1 次"
resp="$(request POST /api/license/consume "$(cat <<JSON
{
  "fingerprint_hash":"${FINGERPRINT_HASH}",
  "request_id":"${FINGERPRINT_HASH}-req-1",
  "usage_type":"generate",
  "access_mode":"free"
}
JSON
)")"
status="$(extract_status "$resp")"
body="$(extract_body "$resp")"
assert_status "$status" "200" "免费 consume 第 1 次返回 200"
assert_body_contains "$body" '"access_mode":"free"' "免费 consume 第 1 次返回 free 模式"

info "3) 免费 consume 第 2 次"
resp="$(request POST /api/license/consume "$(cat <<JSON
{
  "fingerprint_hash":"${FINGERPRINT_HASH}",
  "request_id":"${FINGERPRINT_HASH}-req-2",
  "usage_type":"generate",
  "access_mode":"free"
}
JSON
)")"
status="$(extract_status "$resp")"
body="$(extract_body "$resp")"
if [[ "$FREE_LIMIT" -ge 2 ]]; then
  assert_status "$status" "200" "免费 consume 第 2 次返回 200"
else
  assert_status "$status" "409" "免费 consume 第 2 次返回 409"
fi

info "4) 再次 consume，验证额度耗尽返回 409"
resp="$(request POST /api/license/consume "$(cat <<JSON
{
  "fingerprint_hash":"${FINGERPRINT_HASH}",
  "request_id":"${FINGERPRINT_HASH}-req-overflow",
  "usage_type":"generate",
  "access_mode":"free"
}
JSON
)")"
status="$(extract_status "$resp")"
body="$(extract_body "$resp")"
assert_status "$status" "409" "免费额度耗尽时 consume 返回 409"
assert_body_contains "$body" 'quota exhausted|free quota exhausted' "免费额度耗尽错误文案正确"

info "5) 非法 JSON 请求返回 400"
tmp="$(mktemp)"
status="$(curl -sS -o "$tmp" -w '%{http_code}' -X POST "${PLATFORM_BASE_URL}/api/license/check" -H 'Content-Type: application/json' -d '{invalid')"
body="$(cat "$tmp")"
rm -f "$tmp"
assert_status "$status" "400" "非法 JSON 返回 400"

pass "usage limits smoke 完成"
