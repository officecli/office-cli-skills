#!/usr/bin/env bash

set -euo pipefail

TAP_REPO="${TAP_REPO:-}"
TAP_DEFAULT_BRANCH="${TAP_DEFAULT_BRANCH:-main}"
DIST_REPO="${DIST_REPO:-}"
VERSION="${VERSION:-}"

if [[ -z "${GH_TOKEN:-}" || -z "${TAP_REPO}" || -z "${DIST_REPO}" || -z "${VERSION}" ]]; then
  echo "GH_TOKEN, TAP_REPO, DIST_REPO, and VERSION are required" >&2
  exit 1
fi

version="${VERSION#v}"
release_base="https://github.com/${DIST_REPO}/releases/download/v${version}"
formula_path="Formula/officecli.rb"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

fetch_checksums() {
  local attempt
  for attempt in $(seq 1 12); do
    if curl -fsSL "${release_base}/checksums.txt"; then
      return 0
    fi
    echo "checksums.txt is not available yet for ${version}; retrying (${attempt}/12)" >&2
    sleep 5
  done
  return 1
}

git clone "https://x-access-token:${GH_TOKEN}@github.com/${TAP_REPO}.git" "${tmpdir}/tap"
cd "${tmpdir}/tap"
git checkout "${TAP_DEFAULT_BRANCH}"

checksums="$(fetch_checksums)"
darwin_amd64_sha="$(awk '/officecli_'"${version}"'_darwin_amd64.tar.gz$/ {print $1}' <<<"${checksums}")"
darwin_arm64_sha="$(awk '/officecli_'"${version}"'_darwin_arm64.tar.gz$/ {print $1}' <<<"${checksums}")"

[[ -n "${darwin_amd64_sha}" && -n "${darwin_arm64_sha}" ]] || {
  echo "failed to resolve darwin archive checksums for ${version}" >&2
  exit 1
}

mkdir -p "$(dirname "${formula_path}")"
find Formula -maxdepth 1 -type f ! -name 'officecli.rb' -delete 2>/dev/null || true
cat > "${formula_path}" <<EOF
class Officecli < Formula
  desc "Closed-source Office document generation CLI"
  homepage "https://github.com/${DIST_REPO}"
  version "${version}"

  on_macos do
    if Hardware::CPU.arm?
      url "${release_base}/officecli_${version}_darwin_arm64.tar.gz"
      sha256 "${darwin_arm64_sha}"
    else
      url "${release_base}/officecli_${version}_darwin_amd64.tar.gz"
      sha256 "${darwin_amd64_sha}"
    end
  end

  def install
    bin.install "officecli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/officecli --version")
  end
end
EOF

if [[ -z "$(git status --short -- "${formula_path}" Formula/)" ]]; then
  echo "homebrew formula already up to date"
  exit 0
fi

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add "${formula_path}" Formula/
git commit -m "chore: update officecli ${version}"
git push origin "${TAP_DEFAULT_BRANCH}"
