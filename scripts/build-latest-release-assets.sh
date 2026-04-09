#!/usr/bin/env bash

set -euo pipefail

DIST_DIR="${DIST_DIR:-dist/latest}"
VERSION_LABEL="${VERSION_LABEL:-latest}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
APP_NAME="officecli"
CLI_EMBEDDED_PUBLISH_BASE_URL="${CLI_EMBEDDED_PUBLISH_BASE_URL:-}"
CLI_EMBEDDED_PUBLISH_AUTH_KEY_ID="${CLI_EMBEDDED_PUBLISH_AUTH_KEY_ID:-}"
CLI_EMBEDDED_PUBLISH_AUTH_KEY="${CLI_EMBEDDED_PUBLISH_AUTH_KEY:-}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

build_one() {
  local os="$1"
  local arch="$2"
  local workdir="$3"
  local out_name="${APP_NAME}_latest_${os}_${arch}.tar.gz"

  GOOS="${os}" GOARCH="${arch}" CGO_ENABLED=0 go build \
    -buildvcs=false \
    -ldflags "-s -w -X github.com/officecli/officecli/internal/cli.Version=${VERSION_LABEL} -X github.com/officecli/officecli/internal/cli.Commit=${COMMIT} -X github.com/officecli/officecli/internal/cli.BuildDate=${BUILD_DATE} -X github.com/officecli/officecli/internal/providers/publish.EmbeddedPublishBaseURL=${CLI_EMBEDDED_PUBLISH_BASE_URL} -X github.com/officecli/officecli/internal/providers/publish.EmbeddedPublishAuthKeyID=${CLI_EMBEDDED_PUBLISH_AUTH_KEY_ID} -X github.com/officecli/officecli/internal/providers/publish.EmbeddedPublishAuthKey=${CLI_EMBEDDED_PUBLISH_AUTH_KEY}" \
    -o "${workdir}/officecli" \
    ./cmd/officecli

  tar -czf "${DIST_DIR}/${out_name}" -C "${workdir}" officecli
}

need_cmd go
need_cmd tar
need_cmd sha256sum

rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

for target in "darwin amd64" "darwin arm64" "linux amd64" "linux arm64"; do
  os="${target% *}"
  arch="${target#* }"
  workdir="${tmpdir}/${os}-${arch}"
  mkdir -p "${workdir}"
  build_one "${os}" "${arch}" "${workdir}"
done

(
  cd "${DIST_DIR}"
  sha256sum officecli_latest_*.tar.gz > checksums.txt
)
