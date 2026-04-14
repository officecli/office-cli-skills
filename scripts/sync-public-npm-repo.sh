#!/usr/bin/env bash

set -euo pipefail

PUBLIC_NPM_REPO="${PUBLIC_NPM_REPO:-}"
PUBLIC_NPM_DEFAULT_BRANCH="${PUBLIC_NPM_DEFAULT_BRANCH:-main}"
SOURCE_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_ROOT="${SOURCE_REPO_ROOT}/packages/npm/officecli"
TEMPLATE_ROOT="${SOURCE_REPO_ROOT}/templates/public-npm"

if [[ -z "${GH_TOKEN:-}" || -z "${PUBLIC_NPM_REPO}" ]]; then
  echo "GH_TOKEN and PUBLIC_NPM_REPO are required" >&2
  exit 1
fi

if [[ ! -d "${PACKAGE_ROOT}" ]]; then
  echo "missing package source directory: ${PACKAGE_ROOT}" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

git clone "https://x-access-token:${GH_TOKEN}@github.com/${PUBLIC_NPM_REPO}.git" "${tmpdir}/npm-repo"
cd "${tmpdir}/npm-repo"
if git rev-parse --verify "origin/${PUBLIC_NPM_DEFAULT_BRANCH}" >/dev/null 2>&1; then
  git checkout "${PUBLIC_NPM_DEFAULT_BRANCH}"
else
  git checkout --orphan "${PUBLIC_NPM_DEFAULT_BRANCH}"
  git rm -rf . >/dev/null 2>&1 || true
fi

find . -mindepth 1 -maxdepth 1 ! -name '.git' -exec rm -rf {} +

mkdir -p .github/workflows bin lib scripts
cp "${PACKAGE_ROOT}/package.json" ./package.json
cp "${PACKAGE_ROOT}/README.md" ./README.md
cp -R "${PACKAGE_ROOT}/bin/." ./bin/
cp -R "${PACKAGE_ROOT}/lib/." ./lib/
cp -R "${PACKAGE_ROOT}/scripts/." ./scripts/
cp "${TEMPLATE_ROOT}/.gitignore" ./.gitignore
cp "${TEMPLATE_ROOT}/.github/workflows/npm-publish.yml" ./.github/workflows/npm-publish.yml

PUBLIC_NPM_REPO="${PUBLIC_NPM_REPO}" python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path("package.json")
data = json.loads(path.read_text(encoding="utf-8"))
repo = os.environ["PUBLIC_NPM_REPO"].strip()
data["repository"] = {
    "type": "git",
    "url": f"git+https://github.com/{repo}.git",
}
data["bugs"] = {
    "url": f"https://github.com/{repo}/issues",
}
path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
PY

if [[ -z "$(git status --short -- .)" ]]; then
  echo "public npm repository already up to date"
  exit 0
fi

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add .
git commit -m "chore: sync public npm wrapper"
git push origin "${PUBLIC_NPM_DEFAULT_BRANCH}"
