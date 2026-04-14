#!/usr/bin/env bash

set -euo pipefail

REPORT_ROOT="${1:-}"
DINGTALK_WEBHOOK="${DINGTALK_WEBHOOK:-}"
RUN_URL="${RUN_URL:-}"

if [[ -z "${REPORT_ROOT}" ]]; then
  echo "usage: DINGTALK_WEBHOOK=... RUN_URL=... bash ./scripts/notify-dingtalk.sh <report-root>" >&2
  exit 1
fi

if [[ -z "${DINGTALK_WEBHOOK}" ]]; then
  echo "missing DINGTALK_WEBHOOK" >&2
  exit 1
fi

message="$(
  python3 - "${REPORT_ROOT}" "${RUN_URL}" <<'PY'
import csv
import os
import sys

report_root, run_url = sys.argv[1], sys.argv[2]
failed = []

for root, _, files in os.walk(report_root):
    if "checks.tsv" not in files:
        continue
    mode = os.path.basename(root)
    path = os.path.join(root, "checks.tsv")
    with open(path, "r", encoding="utf-8") as fh:
        for row in csv.DictReader(fh, delimiter="\t"):
            if row["status"] != "passed":
                failed.append({
                    "mode": mode,
                    "phase": row["phase"],
                    "name": row["name"],
                    "url": row["url"],
                    "http_status": row["http_status"],
                    "request_id": row["request_id"],
                    "detail": row["detail"],
                })

lines = ["officecli production inspection failed"]
if run_url:
    lines.append(f"Run: {run_url}")

if failed:
    for item in failed[:5]:
        lines.append(
            f"- [{item['mode']}] {item['phase']}/{item['name']} "
            f"url={item['url']} status={item['http_status']} "
            f"request_id={item['request_id'] or '-'} detail={item['detail']}"
        )
else:
    lines.append("- No failure details were found. Check the workflow artifact.")

print("\n".join(lines))
PY
)"

payload="$(
  python3 - "${message}" <<'PY'
import json
import sys

print(json.dumps({
    "msgtype": "text",
    "text": {
        "content": sys.argv[1]
    }
}, ensure_ascii=False))
PY
)"

curl -sS -X POST "${DINGTALK_WEBHOOK}" \
  -H 'Content-Type: application/json' \
  -d "${payload}" >/dev/null
