#!/usr/bin/env bash
set -euo pipefail

gate="all"

for arg in "$@"; do
  case "$arg" in
    --gate=human|--gate=oracle|--gate=untouched|--gate=fidelity)
      gate="${arg#--gate=}"
      ;;
    --gate=*)
      echo "unknown gate: ${arg#--gate=}" >&2
      echo "usage: $0 [--gate=human|oracle|untouched|fidelity]" >&2
      exit 2
      ;;
    -h|--help)
      echo "usage: $0 [--gate=human|oracle|untouched|fidelity]"
      exit 0
      ;;
    *)
      echo "unknown argument: $arg" >&2
      echo "usage: $0 [--gate=human|oracle|untouched|fidelity]" >&2
      exit 2
      ;;
  esac
done

run_gate() {
  local name="$1"
  shift

  if [[ "$gate" != "all" && "$gate" != "$name" ]]; then
    return 0
  fi

  echo "==> ${name}: $*"
  "$@"
}

run_human_gate() {
  if [[ "$gate" != "all" && "$gate" != "human" ]]; then
    return 0
  fi

  if ! command -v soffice >/dev/null 2>&1; then
    echo "SKIP human: soffice not found; no human-viewable smoke check is run in this environment."
    return 0
  fi

  echo "SKIP human: no WS-9-specific soffice smoke check is defined; oracle/untouched/fidelity gates remain authoritative here."
}

run_human_gate
run_gate oracle go test ./internal/runtime/modify/... -run TestModifyFixtures -count=1 -timeout 180s
run_gate untouched go test ./internal/runtime/modify/... -run TestModifyUntouchedDiff -count=1 -timeout 180s
run_gate fidelity go test ./pkg/ooxmledit/... -run 'Test(Fidelity|Normalize)' -count=1 -timeout 90s
