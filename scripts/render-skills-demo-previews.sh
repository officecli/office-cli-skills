#!/usr/bin/env bash

set -euo pipefail

ROOT="${ROOT:-public/skills-demos}"
CHROME="${CHROME:-}"

if [[ -z "${CHROME}" ]]; then
  for candidate in \
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
    "/Applications/Chromium.app/Contents/MacOS/Chromium" \
    "google-chrome" \
    "chromium" \
    "chromium-browser"; do
    if command -v "${candidate}" >/dev/null 2>&1 || [[ -x "${candidate}" ]]; then
      CHROME="${candidate}"
      break
    fi
  done
fi

if [[ -z "${CHROME}" ]]; then
  echo "Chrome or Chromium is required to render demo previews" >&2
  exit 1
fi

if [[ ! -d "${ROOT}" ]]; then
  echo "demo root not found: ${ROOT}" >&2
  exit 1
fi

for dir in "${ROOT}"/*; do
  [[ -d "${dir}" ]] || continue
  if [[ -f "${dir}/preview.html" ]]; then
    html_path="$(cd "${dir}" && pwd -P)/preview.html"
    "${CHROME}" \
      --headless=new \
      --disable-gpu \
      --hide-scrollbars \
      --window-size=1280,900 \
      --screenshot="${dir}/preview.png" \
      "file://${html_path}" >/dev/null 2>&1
  elif [[ -f "${dir}/officecli-skills-hero-image.png" ]]; then
    cp "${dir}/officecli-skills-hero-image.png" "${dir}/preview.png"
  fi
done
