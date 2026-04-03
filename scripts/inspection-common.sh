#!/usr/bin/env bash

set -euo pipefail

INSPECTION_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSPECTION_REPO_ROOT="$(cd "${INSPECTION_SCRIPT_DIR}/.." && pwd)"

timestamp() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
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

sanitize_field() {
  printf '%s' "$1" | tr '\t\r\n' ' ' | tr -s ' '
}

init_report() {
  MODE_NAME="$1"
  REPORT_DIR="${REPORT_DIR:-${INSPECTION_REPO_ROOT}/.inspection-reports/${MODE_NAME}}"
  mkdir -p "${REPORT_DIR}"
  RESULTS_TSV="${REPORT_DIR}/checks.tsv"
  SUMMARY_MD="${REPORT_DIR}/summary.md"
  META_TXT="${REPORT_DIR}/meta.env"
  OVERALL_STATUS="passed"
  printf 'phase\tname\tstatus\tmethod\turl\thttp_status\trequest_id\tdetail\n' >"${RESULTS_TSV}"
}

record_check() {
  local phase_name="$1"
  local check_name="$2"
  local status="$3"
  local method="$4"
  local url="$5"
  local http_status="$6"
  local request_id="$7"
  local detail="$8"

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$(sanitize_field "${phase_name}")" \
    "$(sanitize_field "${check_name}")" \
    "$(sanitize_field "${status}")" \
    "$(sanitize_field "${method}")" \
    "$(sanitize_field "${url}")" \
    "$(sanitize_field "${http_status}")" \
    "$(sanitize_field "${request_id}")" \
    "$(sanitize_field "${detail}")" >>"${RESULTS_TSV}"

  if [[ "${status}" != "passed" ]]; then
    OVERALL_STATUS="failed"
  fi
}

http_request() {
  local method="$1"
  local url="$2"
  local body="${3:-}"

  HTTP_HEADERS_FILE="$(mktemp)"
  HTTP_BODY_FILE="$(mktemp)"
  HTTP_CURL_EXIT=0

  local -a curl_args
  curl_args=(
    -sS
    --connect-timeout "${CURL_CONNECT_TIMEOUT:-10}"
    --max-time "${CURL_MAX_TIME:-30}"
    -A "${INSPECTION_USER_AGENT:-officecli-production-inspection/1.0}"
    -D "${HTTP_HEADERS_FILE}"
    -o "${HTTP_BODY_FILE}"
    -w '%{http_code}'
    -X "${method}"
    "${url}"
  )

  if [[ -n "${body}" ]]; then
    curl_args+=(-H 'Content-Type: application/json' --data "${body}")
  fi

  HTTP_STATUS="$(curl "${curl_args[@]}")" || HTTP_CURL_EXIT=$?
}

cleanup_http_files() {
  rm -f "${HTTP_HEADERS_FILE:-}" "${HTTP_BODY_FILE:-}"
}

header_get() {
  local file="$1"
  local key="$2"
  awk -F': ' -v search_key="${key}" 'tolower($1) == tolower(search_key) {print $2}' "${file}" | tr -d '\r' | tail -n1
}

json_get() {
  local file="$1"
  local path="$2"
  python3 - "$file" "$path" <<'PY'
import json
import sys

file_path, path = sys.argv[1], sys.argv[2]
with open(file_path, "r", encoding="utf-8") as fh:
    data = json.load(fh)

cur = data
for part in path.split("."):
    if isinstance(cur, dict) and part in cur:
        cur = cur[part]
    else:
        sys.exit(1)

if cur is None:
    sys.exit(1)
if isinstance(cur, bool):
    print("true" if cur else "false")
elif isinstance(cur, (dict, list)):
    print(json.dumps(cur, ensure_ascii=False))
else:
    print(cur)
PY
}

json_check_pricing_payload() {
  local file="$1"
  python3 - "$file" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    payload = json.load(fh)

data = payload.get("data", payload)
if not isinstance(data, list) or not data:
    sys.exit(1)

first = data[0]
required = ("code", "name")
for key in required:
    if key not in first or first[key] in ("", None):
        sys.exit(1)
PY
}

body_matches() {
  local pattern="$1"
  local file="$2"

  if command -v rg >/dev/null 2>&1; then
    rg -qi -- "${pattern}" "${file}"
    return
  fi

  grep -Eqi -- "${pattern}" "${file}"
}

finalize_report() {
  local mode_name="$1"
  printf 'MODE=%s\nSTATUS=%s\n' "${mode_name}" "${OVERALL_STATUS}" >"${META_TXT}"

  python3 - "${RESULTS_TSV}" "${SUMMARY_MD}" "${mode_name}" "${OVERALL_STATUS}" <<'PY'
import csv
import sys

tsv_path, summary_path, mode_name, overall_status = sys.argv[1:5]

with open(tsv_path, "r", encoding="utf-8") as fh:
    rows = list(csv.DictReader(fh, delimiter="\t"))

passed = sum(1 for row in rows if row["status"] == "passed")
failed = [row for row in rows if row["status"] != "passed"]

lines = [
    f"# {mode_name} inspection report",
    "",
    f"- overall: `{overall_status}`",
    f"- total checks: `{len(rows)}`",
    f"- passed: `{passed}`",
    f"- failed: `{len(failed)}`",
    "",
]

if failed:
    lines.extend([
        "## Failed checks",
        "",
        "| phase | name | method | url | http_status | request_id | detail |",
        "| --- | --- | --- | --- | --- | --- | --- |",
    ])
    for row in failed:
        lines.append(
            f"| {row['phase']} | {row['name']} | {row['method']} | {row['url']} | "
            f"{row['http_status']} | {row['request_id']} | {row['detail']} |"
        )
    lines.append("")

lines.extend([
    "## All checks",
    "",
    "| phase | name | status | method | url | http_status | request_id |",
    "| --- | --- | --- | --- | --- | --- | --- |",
])

for row in rows:
    lines.append(
        f"| {row['phase']} | {row['name']} | {row['status']} | {row['method']} | "
        f"{row['url']} | {row['http_status']} | {row['request_id']} |"
    )

with open(summary_path, "w", encoding="utf-8") as fh:
    fh.write("\n".join(lines) + "\n")
PY
}
