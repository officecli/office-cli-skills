#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./local-test-common.sh
source "${SCRIPT_DIR}/local-test-common.sh"

ALL_TYPES=(pptx docx xlsx report)
SELECTED_TYPES=("$@")
RESULT_LABELS=()
RESULT_STATUSES=()
FAILURES=0

usage() {
  cat <<'EOF'
Usage:
  bash ./scripts/run-generation-type-tests.sh [pptx|docx|xlsx|report ...]

Description:
  Run generation-focused Go tests for all four document types by default
  and print a PASS/FAIL summary at the end.

Examples:
  bash ./scripts/run-generation-type-tests.sh
  bash ./scripts/run-generation-type-tests.sh pptx report
EOF
}

if [[ ${#SELECTED_TYPES[@]} -eq 0 ]]; then
  SELECTED_TYPES=("${ALL_TYPES[@]}")
fi

should_run() {
  local expected="$1"
  local current
  for current in "${SELECTED_TYPES[@]}"; do
    if [[ "${current}" == "${expected}" ]]; then
      return 0
    fi
  done
  return 1
}

run_suite() {
  local label="$1"
  shift

  phase "${label} generation tests"
  info "Running command: $*"
  if (cd "${REPO_ROOT}" && "$@"); then
    RESULT_LABELS+=("${label}")
    RESULT_STATUSES+=("PASS")
    pass "${label} generation tests completed"
  else
    RESULT_LABELS+=("${label}")
    RESULT_STATUSES+=("FAIL")
    FAILURES=$((FAILURES + 1))
    warn "${label} generation tests failed"
  fi
}

for type in "${SELECTED_TYPES[@]}"; do
  case "${type}" in
    pptx|docx|xlsx|report) ;;
    -h|--help|help)
      usage
      exit 0
      ;;
    *)
      usage
      fail "unsupported generation type: ${type}"
      ;;
  esac
done

PPTX_PATTERN='^(TestServiceGeneratePPTX.*|TestBuildPPTXPrompt_.*|TestNormalizePPTXPayload_.*|TestBuildPPTXFromJSON_.*|TestPPTX.*|TestTargetAspectRatioForSlide)$'
DOCX_PATTERN='^(TestServiceGenerateDOCX.*|TestBuildDOCXPrompt_AndBuildDOCXFromJSON|TestNewDOCXGeneratorAvailable)$'
XLSX_PATTERN='^(TestServiceGenerateXLSX.*|TestBuildXLSXPrompt_AndBuildXLSXFromJSON)$'
REPORT_PATTERN='^(TestServiceGenerateReport.*|TestBuildReportPrompt_AndBuildReportFromJSON|TestBuildReport_RendersEChartsAndSectionContent|TestNormalizeReport_NormalizesUnsupportedChartType)$'

phase "Document generation test matrix"

if should_run pptx; then
  run_suite pptx go test ./internal/runtime ./pkg/officegen -count=1 -run "${PPTX_PATTERN}"
fi

if should_run docx; then
  run_suite docx go test ./internal/runtime ./engine/generate ./pkg/officegen -count=1 -run "${DOCX_PATTERN}"
fi

if should_run xlsx; then
  run_suite xlsx go test ./internal/runtime ./engine/generate -count=1 -run "${XLSX_PATTERN}"
fi

if should_run report; then
  run_suite report go test ./internal/runtime ./engine/generate ./pkg/officegen -count=1 -run "${REPORT_PATTERN}"
fi

phase "Generation test summary"
for ((i = 0; i < ${#RESULT_LABELS[@]}; i++)); do
  printf '[%s] [%-4s] %s\n' "$(timestamp)" "${RESULT_STATUSES[i]}" "${RESULT_LABELS[i]}"
done

if [[ ${FAILURES} -eq 0 ]]; then
  pass "All ${#RESULT_LABELS[@]} generation suites passed"
else
  fail "${FAILURES} of ${#RESULT_LABELS[@]} generation suites failed"
fi
