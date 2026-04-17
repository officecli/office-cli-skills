#!/usr/bin/env bash

set -euo pipefail

DIST_REPO="${DIST_REPO:-}"
KEEP_TAG="${KEEP_TAG:-}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

if [[ -z "${GH_TOKEN:-}" || -z "${DIST_REPO}" || -z "${KEEP_TAG}" ]]; then
  echo "GH_TOKEN, DIST_REPO, and KEEP_TAG are required" >&2
  exit 1
fi

need_cmd gh
need_cmd git

mapfile -t release_tags < <(gh release list -R "${DIST_REPO}" --limit 1000 --json tagName --jq '.[].tagName')
for tag in "${release_tags[@]}"; do
  if [[ -z "${tag}" || "${tag}" == "${KEEP_TAG}" ]]; then
    continue
  fi
  gh release delete "${tag}" -R "${DIST_REPO}" --yes --cleanup-tag || true
done

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

git clone "https://x-access-token:${GH_TOKEN}@github.com/${DIST_REPO}.git" "${tmpdir}/dist"
while IFS= read -r tag; do
  if [[ -z "${tag}" || "${tag}" == "${KEEP_TAG}" ]]; then
    continue
  fi
  git -C "${tmpdir}/dist" push origin ":refs/tags/${tag}" || true
done < <(git -C "${tmpdir}/dist" tag --list)
