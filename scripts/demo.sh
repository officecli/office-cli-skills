#!/usr/bin/env bash

set -euo pipefail

BINARY_PATH="${1:-./officecli}"

echo "== officecli demo =="
echo
echo "1) 查看版本"
"${BINARY_PATH}" --version
echo
echo "2) 查看帮助"
"${BINARY_PATH}" --help
echo
echo "3) 运行 PPT 示例（仅本地保存）"
"${BINARY_PATH}" new pptx "企业协作平台介绍" --prompt-file ./examples/prompt.txt --no-publish
echo
echo "4) 运行 DOCX 示例（仅本地保存）"
"${BINARY_PATH}" new docx "季度复盘" --prompt-file ./examples/docx-prompt.txt --no-publish
echo
echo "5) 运行 XLSX 示例（仅本地保存）"
"${BINARY_PATH}" new xlsx "销售分析表" --prompt-file ./examples/xlsx-prompt.txt --no-publish
echo
echo "Demo completed."
