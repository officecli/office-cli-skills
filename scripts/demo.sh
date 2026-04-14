#!/usr/bin/env bash

set -euo pipefail

BINARY_PATH="${1:-./officecli}"

echo "== officecli demo =="
echo
echo "1) Show version"
"${BINARY_PATH}" --version
echo
echo "2) Show help"
"${BINARY_PATH}" --help
echo
echo "3) Run the PPT example (save locally only)"
"${BINARY_PATH}" new pptx "Enterprise Collaboration Platform Overview" --prompt-file ./examples/prompt.txt --no-publish
echo
echo "4) Run the DOCX example (save locally only)"
"${BINARY_PATH}" new docx "Quarterly Retrospective" --prompt-file ./examples/docx-prompt.txt --no-publish
echo
echo "5) Run the XLSX example (save locally only)"
"${BINARY_PATH}" new xlsx "Sales Analysis Workbook" --prompt-file ./examples/xlsx-prompt.txt --no-publish
echo
echo "Demo completed."
