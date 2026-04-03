#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PLATFORM_DIR="${ROOT_DIR}/platform"

SERVER_HOST="${SERVER_HOST:-18.141.191.80}"
SERVER_USER="${SERVER_USER:-ubuntu}"
SERVER="${SERVER_USER}@${SERVER_HOST}"
SSH_PORT="${SSH_PORT:-22}"
REMOTE_WORKDIR="${REMOTE_WORKDIR:-/opt/officecli-platform}"
REMOTE_SITE_DIR="${REMOTE_SITE_DIR:-/var/www/officecli.io/dist}"
REMOTE_APP_DIR="${REMOTE_APP_DIR:-/var/www/platform.officecli.io/app}"
REMOTE_ADMIN_DIR="${REMOTE_ADMIN_DIR:-/var/www/platform.officecli.io/admin}"
KUBE_NAMESPACE="${KUBE_NAMESPACE:-officecli}"
DEPLOYMENT_NAME="${DEPLOYMENT_NAME:-officecli-platform}"
CONTAINER_NAME="${CONTAINER_NAME:-platform}"
IMAGE_REPO="${IMAGE_REPO:-docker.io/library/officecli-platform}"
SSH_OPTS_STRING="${SSH_OPTS:-}"
SSH_OPTS=()
if [[ -n "${SSH_OPTS_STRING}" ]]; then
  read -r -a SSH_OPTS <<<"${SSH_OPTS_STRING}"
fi
SSH_BASE_OPTS=(-p "${SSH_PORT}" "${SSH_OPTS[@]}")
SCP_BASE_OPTS=(-P "${SSH_PORT}" "${SSH_OPTS[@]}")
RSYNC_SSH_CMD=(ssh -p "${SSH_PORT}" "${SSH_OPTS[@]}")

usage() {
  cat <<'EOF'
用法：
  scripts/deploy-platform-prod.sh [tag]

说明：
  - 不传 tag 时，会自动生成类似 v0.1.0-prod-YYYYMMDD-N 的 tag。
  - 会在本地执行 go test、构建 web/site + web/app + web/admin。
  - 会构建 linux/amd64 镜像，打包后上传到生产服务器并导入 k3s containerd。
  - 会同步官网、app、admin 静态资源到 Nginx 目录。
  - 会把 Deployment 强制设为 imagePullPolicy=Never + strategy=Recreate。

可覆盖环境变量：
  SERVER_HOST SERVER_USER SSH_PORT SSH_OPTS REMOTE_WORKDIR REMOTE_SITE_DIR REMOTE_APP_DIR REMOTE_ADMIN_DIR
  KUBE_NAMESPACE DEPLOYMENT_NAME CONTAINER_NAME IMAGE_REPO
EOF
}

log() {
  printf '\n[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

die() {
  echo "错误: $*" >&2
  exit 1
}

ssh_cmd() {
  ssh "${SSH_BASE_OPTS[@]}" "$@"
}

scp_cmd() {
  scp "${SCP_BASE_OPTS[@]}" "$@"
}

rsync_cmd() {
  rsync -e "$(printf '%q ' "${RSYNC_SSH_CMD[@]}")" "$@"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"
}

detect_base_version() {
  local package_json="${PLATFORM_DIR}/web/app/package.json"
  if command -v python3 >/dev/null 2>&1; then
    python3 - <<'PY' "$package_json"
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
data = json.loads(path.read_text())
version = str(data.get("version") or "0.1.0").strip()
print(f"v{version.lstrip('v')}")
PY
    return
  fi

  echo "v0.1.0"
}

detect_next_tag() {
  local base_version="$1"
  local today suffix last remote_value

  today="$(date '+%Y%m%d')"
  suffix=1

  if remote_value="$(
    ssh_cmd "$SERVER" \
      "ls -1 '${REMOTE_WORKDIR}' 2>/dev/null | grep -E '^${base_version}-prod-${today}-[0-9]+\\.tar\\.gz$' | sed 's/\\.tar\\.gz$//' | sed 's/.*-//' | sort -n | tail -1" \
      2>/dev/null
  )"; then
    if [[ -n "${remote_value}" ]]; then
      last="${remote_value}"
      suffix=$((last + 1))
    fi
  fi

  printf '%s-prod-%s-%s' "$base_version" "$today" "$suffix"
}

run_local_builds() {
  log "运行 Go 测试"
  (cd "$PLATFORM_DIR" && go test ./...)

  log "构建 web/app"
  (cd "${PLATFORM_DIR}/web/app" && npm run build)

  log "构建 web/admin"
  (cd "${PLATFORM_DIR}/web/admin" && npm run build)

  log "构建 web/site"
  (cd "${PLATFORM_DIR}/web/site" && npm run build)
}

build_image_archive() {
  local image_ref="$1"
  local archive_path="$2"
  local dockerfile

  dockerfile="$(mktemp "${TMPDIR:-/tmp}/officecli-platform.Dockerfile.XXXXXX")"
  trap 'rm -f "${dockerfile:-}"' RETURN

  cat >"${dockerfile}" <<'EOF'
FROM --platform=linux/amd64 golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/officecli-platform ./cmd/platform

FROM --platform=linux/amd64 alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/officecli-platform /app/officecli-platform
COPY migrations /app/migrations
COPY web /app/web
EXPOSE 8080
ENTRYPOINT ["/app/officecli-platform"]
EOF

  log "构建镜像 ${image_ref}"
  (
    cd "$PLATFORM_DIR"
    docker buildx build \
      --platform linux/amd64 \
      -f "${dockerfile}" \
      -t "${image_ref}" \
      --load \
      .
  )

  log "导出镜像归档 ${archive_path}"
  docker save "${image_ref}" | gzip >"${archive_path}"
}

sync_static_assets() {
  log "同步官网静态资源 -> ${REMOTE_SITE_DIR}"
  rsync_cmd -az --delete "${PLATFORM_DIR}/web/site/dist/" "${SERVER}:${REMOTE_SITE_DIR}/"

  log "同步 app 静态资源 -> ${REMOTE_APP_DIR}"
  rsync_cmd -az --delete "${PLATFORM_DIR}/web/app/dist/" "${SERVER}:${REMOTE_APP_DIR}/"

  log "同步 admin 静态资源 -> ${REMOTE_ADMIN_DIR}"
  rsync_cmd -az --delete "${PLATFORM_DIR}/web/admin/dist/" "${SERVER}:${REMOTE_ADMIN_DIR}/"
}

deploy_remote() {
  local tag="$1"
  local archive_path="$2"
  local image_ref="$3"

  log "确保远端工作目录存在 -> ${REMOTE_WORKDIR}"
  ssh_cmd "$SERVER" "sudo mkdir -p '${REMOTE_WORKDIR}' && sudo chown '${SERVER_USER}:${SERVER_USER}' '${REMOTE_WORKDIR}'"

  log "上传镜像归档 -> ${REMOTE_WORKDIR}/$(basename "$archive_path")"
  scp_cmd "${archive_path}" "${SERVER}:${REMOTE_WORKDIR}/"

  log "在服务器导入镜像并更新 Deployment"
  ssh_cmd "$SERVER" \
    TAG="$tag" \
    IMAGE_REF="$image_ref" \
    ARCHIVE_NAME="$(basename "$archive_path")" \
    REMOTE_WORKDIR="$REMOTE_WORKDIR" \
    KUBE_NAMESPACE="$KUBE_NAMESPACE" \
    DEPLOYMENT_NAME="$DEPLOYMENT_NAME" \
    CONTAINER_NAME="$CONTAINER_NAME" \
    'bash -se' <<'EOF'
set -euo pipefail

resolve_namespace() {
  local preferred="$1"
  local legacy="cli-office"

  if kubectl get namespace "$preferred" >/dev/null 2>&1; then
    printf '%s\n' "$preferred"
    return
  fi
  if [[ "$preferred" != "$legacy" ]] && kubectl get namespace "$legacy" >/dev/null 2>&1; then
    printf '%s\n' "$legacy"
    return
  fi
  printf '%s\n' "$preferred"
}

resolve_deployment() {
  local namespace="$1"
  local preferred="$2"
  local legacy="cli-office-platform"

  if kubectl -n "$namespace" get deployment "$preferred" >/dev/null 2>&1; then
    printf '%s\n' "$preferred"
    return
  fi
  if [[ "$preferred" != "$legacy" ]] && kubectl -n "$namespace" get deployment "$legacy" >/dev/null 2>&1; then
    printf '%s\n' "$legacy"
    return
  fi
  printf '%s\n' "$preferred"
}

cd "$REMOTE_WORKDIR"
gzip -dc "$ARCHIVE_NAME" | sudo k3s ctr images import -

KUBE_NAMESPACE="$(resolve_namespace "$KUBE_NAMESPACE")"
DEPLOYMENT_NAME="$(resolve_deployment "$KUBE_NAMESPACE" "$DEPLOYMENT_NAME")"

kubectl -n "$KUBE_NAMESPACE" set image "deployment/$DEPLOYMENT_NAME" \
  "$CONTAINER_NAME=$IMAGE_REF"

  kubectl -n "$KUBE_NAMESPACE" patch deployment "$DEPLOYMENT_NAME" --type json -p \
    '[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Never"}]'

kubectl -n "$KUBE_NAMESPACE" patch deployment "$DEPLOYMENT_NAME" --type merge -p \
  '{"spec":{"strategy":{"type":"Recreate","rollingUpdate":null}}}'

kubectl -n "$KUBE_NAMESPACE" delete pod -l "app=${DEPLOYMENT_NAME}" --ignore-not-found=true --wait=false || true
kubectl -n "$KUBE_NAMESPACE" rollout status "deployment/$DEPLOYMENT_NAME" --timeout=180s
kubectl -n "$KUBE_NAMESPACE" get pods -o wide

echo
echo "--- deployment image ---"
kubectl -n "$KUBE_NAMESPACE" get deployment "$DEPLOYMENT_NAME" \
  -o jsonpath='{.spec.template.spec.containers[?(@.name=="'"$CONTAINER_NAME"'")].image}{"\n"}'

echo
echo "--- image pull policy ---"
kubectl -n "$KUBE_NAMESPACE" get deployment "$DEPLOYMENT_NAME" \
  -o jsonpath='{.spec.template.spec.containers[?(@.name=="'"$CONTAINER_NAME"'")].imagePullPolicy}{"\n"}'

echo
echo "--- strategy ---"
kubectl -n "$KUBE_NAMESPACE" get deployment "$DEPLOYMENT_NAME" \
  -o jsonpath='{.spec.strategy.type}{"\n"}'

echo
echo "--- localhost probes ---"
curl -fsS -o /dev/null -D - -H "Host: officecli.io" http://127.0.0.1:29001/
curl -fsS -o /dev/null -D - -H "Host: platform.officecli.io" http://127.0.0.1:29001/app/
curl -fsS http://127.0.0.1:29001/healthz
EOF
}

verify_public_endpoints() {
  log "验证公网入口"
  curl -fsS -o /dev/null -D - https://officecli.io/
  curl -fsS -o /dev/null -D - https://officecli.io/api/pricing
  curl -fsS -o /dev/null -D - https://officecli.io/app
  curl -fsS -o /dev/null -D - https://platform.officecli.io/
  curl -fsS -o /dev/null -D - https://platform.officecli.io/app/
  curl -fsS -o /dev/null -D - https://platform.officecli.io/admin/
  curl -fsS https://platform.officecli.io/healthz
}

main() {
  local tag base_version image_ref archive_path archive_name

  [[ -d "$PLATFORM_DIR" ]] || die "未找到 platform 目录: ${PLATFORM_DIR}"
  [[ $# -le 1 ]] || { usage; exit 1; }
  [[ -f "${ROOT_DIR}/AGENTS.md" ]] || die "请从仓库根目录或其子目录执行此脚本"

  require_cmd go
  require_cmd npm
  require_cmd docker
  require_cmd ssh
  require_cmd scp
  require_cmd rsync
  require_cmd gzip
  require_cmd curl

  if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi

  base_version="$(detect_base_version)"
  tag="${1:-$(detect_next_tag "$base_version")}"
  image_ref="${IMAGE_REPO}:${tag}"
  archive_path="${TMPDIR:-/tmp}/officecli-platform-${tag}.tar.gz"
  archive_name="$(basename "$archive_path")"

  log "发布 tag: ${tag}"
  log "镜像引用: ${image_ref}"

  run_local_builds
  build_image_archive "$image_ref" "$archive_path"
  sync_static_assets
  deploy_remote "$tag" "$archive_path" "$image_ref"
  verify_public_endpoints

  log "发布完成"
  echo "tag=${tag}"
  echo "image=${image_ref}"
  echo "archive=${archive_path}"
}

main "$@"
