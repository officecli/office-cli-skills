#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-${ROOT_DIR}/dist/dev}"
VERSION_LABEL="${VERSION_LABEL:-dev}"
COMMIT="${COMMIT:-$(git -C "${ROOT_DIR}" rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
DEV_PLATFORM_BASE_URL="${OFFICECLI_DEV_PLATFORM_BASE_URL:-https://officecli.shimodev.com}"

mkdir -p "${DIST_DIR}"

GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
  -buildvcs=false \
  -ldflags "-s -w -X github.com/officecli/officecli-internal/internal/cli.Version=${VERSION_LABEL} -X github.com/officecli/officecli-internal/internal/cli.Commit=${COMMIT} -X github.com/officecli/officecli-internal/internal/cli.BuildDate=${BUILD_DATE}" \
  -o "${DIST_DIR}/officecli" \
  "${ROOT_DIR}/cmd/officecli"

cat >"${DIST_DIR}/officecli-dev" <<EOF
#!/usr/bin/env bash
set -euo pipefail
export OFFICE_CLI_PROFILE="\${OFFICE_CLI_PROFILE:-dev}"
export OFFICECLI_DEV_PLATFORM_BASE_URL="\${OFFICECLI_DEV_PLATFORM_BASE_URL:-${DEV_PLATFORM_BASE_URL}}"
append_no_proxy() {
  case ",\${NO_PROXY:-},\${no_proxy:-}," in
    *,officecli.shimodev.com,*) ;;
    *)
      if [[ -n "\${NO_PROXY:-}" ]]; then
        export NO_PROXY="\${NO_PROXY},officecli.shimodev.com"
      else
        export NO_PROXY="officecli.shimodev.com"
      fi
      ;;
  esac
  export no_proxy="\${NO_PROXY}"
}
append_no_proxy
exec "\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)/officecli" "\$@"
EOF
chmod +x "${DIST_DIR}/officecli-dev"

printf 'Built %s\n' "${DIST_DIR}/officecli-dev"
