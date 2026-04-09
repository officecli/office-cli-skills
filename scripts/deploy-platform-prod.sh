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
SECRET_NAME="${SECRET_NAME:-officecli-platform-env}"
POSTGRES_SECRET_NAME="${POSTGRES_SECRET_NAME:-officecli-platform-postgres}"
POSTGRES_STS_NAME="${POSTGRES_STS_NAME:-officecli-platform-postgres}"
POSTGRES_SERVICE_NAME="${POSTGRES_SERVICE_NAME:-officecli-platform-postgres}"
POSTGRES_HEADLESS_SERVICE_NAME="${POSTGRES_HEADLESS_SERVICE_NAME:-officecli-platform-postgres-headless}"
POSTGRES_DB="${POSTGRES_DB:-officecli_platform}"
POSTGRES_USER="${POSTGRES_USER:-officecli}"
POSTGRES_BACKUP_CRONJOB_NAME="${POSTGRES_BACKUP_CRONJOB_NAME:-officecli-platform-postgres-backup}"
POSTGRES_MANIFEST_DIR="${PLATFORM_DIR}/deploy/k8s/postgres"
PLATFORM_ENV_FILE="${PLATFORM_ENV_FILE:-}"
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
  - 若目标 namespace / service / deployment 不存在，会自动执行首次 bootstrap。
  - 若首次 bootstrap 且缺少 Secret，可传 PLATFORM_ENV_FILE=<本地 env 文件路径> 自动创建 Secret。

可覆盖环境变量：
  SERVER_HOST SERVER_USER SSH_PORT SSH_OPTS REMOTE_WORKDIR REMOTE_SITE_DIR REMOTE_APP_DIR REMOTE_ADMIN_DIR
  KUBE_NAMESPACE DEPLOYMENT_NAME CONTAINER_NAME IMAGE_REPO SECRET_NAME PLATFORM_ENV_FILE
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

require_env_keys_in_file() {
  local env_file="$1"
  shift
  [[ -f "${env_file}" ]] || die "未找到 env 文件: ${env_file}"
  local missing=()
  local key
  for key in "$@"; do
    if ! grep -Eq "^${key}=" "${env_file}"; then
      missing+=("${key}")
    fi
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    die "env 文件缺少必填项: ${missing[*]} (${env_file})"
  fi
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
  local remote_env_file=""
  local remote_postgres_manifest_dir="${REMOTE_WORKDIR}/postgres-manifests"

  log "确保远端工作目录存在 -> ${REMOTE_WORKDIR}"
  ssh_cmd "$SERVER" "sudo mkdir -p '${REMOTE_WORKDIR}' && sudo chown '${SERVER_USER}:${SERVER_USER}' '${REMOTE_WORKDIR}'"

  log "上传镜像归档 -> ${REMOTE_WORKDIR}/$(basename "$archive_path")"
  scp_cmd "${archive_path}" "${SERVER}:${REMOTE_WORKDIR}/"

  if [[ -n "${PLATFORM_ENV_FILE}" ]]; then
    [[ -f "${PLATFORM_ENV_FILE}" ]] || die "未找到 PLATFORM_ENV_FILE: ${PLATFORM_ENV_FILE}"
    remote_env_file="${REMOTE_WORKDIR}/$(basename "${PLATFORM_ENV_FILE}")"
    log "上传 bootstrap env 文件 -> ${remote_env_file}"
    scp_cmd "${PLATFORM_ENV_FILE}" "${SERVER}:${remote_env_file}"
  fi

  if [[ -d "${POSTGRES_MANIFEST_DIR}" ]]; then
    log "上传 PostgreSQL k8s 清单 -> ${remote_postgres_manifest_dir}"
    ssh_cmd "$SERVER" "mkdir -p '${remote_postgres_manifest_dir}'"
    scp_cmd "${POSTGRES_MANIFEST_DIR}/service-headless.yaml" "${SERVER}:${remote_postgres_manifest_dir}/"
    scp_cmd "${POSTGRES_MANIFEST_DIR}/service.yaml" "${SERVER}:${remote_postgres_manifest_dir}/"
    scp_cmd "${POSTGRES_MANIFEST_DIR}/statefulset.yaml" "${SERVER}:${remote_postgres_manifest_dir}/"
    scp_cmd "${POSTGRES_MANIFEST_DIR}/backup-cronjob.yaml" "${SERVER}:${remote_postgres_manifest_dir}/"
  fi

  log "在服务器导入镜像并更新 Deployment"
  ssh_cmd "$SERVER" \
    TAG="$tag" \
    IMAGE_REF="$image_ref" \
    ARCHIVE_NAME="$(basename "$archive_path")" \
    REMOTE_WORKDIR="$REMOTE_WORKDIR" \
    KUBE_NAMESPACE="$KUBE_NAMESPACE" \
    DEPLOYMENT_NAME="$DEPLOYMENT_NAME" \
    CONTAINER_NAME="$CONTAINER_NAME" \
    SECRET_NAME="$SECRET_NAME" \
    POSTGRES_SECRET_NAME="$POSTGRES_SECRET_NAME" \
    POSTGRES_STS_NAME="$POSTGRES_STS_NAME" \
    POSTGRES_SERVICE_NAME="$POSTGRES_SERVICE_NAME" \
    POSTGRES_HEADLESS_SERVICE_NAME="$POSTGRES_HEADLESS_SERVICE_NAME" \
    POSTGRES_DB="$POSTGRES_DB" \
    POSTGRES_USER="$POSTGRES_USER" \
    POSTGRES_BACKUP_CRONJOB_NAME="$POSTGRES_BACKUP_CRONJOB_NAME" \
    REMOTE_POSTGRES_MANIFEST_DIR="$remote_postgres_manifest_dir" \
    REMOTE_ENV_FILE="$remote_env_file" \
    'bash -se' <<'EOF'
set -euo pipefail

die() {
  echo "错误: $*" >&2
  exit 1
}

ensure_namespace() {
  if ! kubectl get namespace "$KUBE_NAMESPACE" >/dev/null 2>&1; then
    kubectl create namespace "$KUBE_NAMESPACE" >/dev/null
  fi
}

ensure_secret() {
  if kubectl -n "$KUBE_NAMESPACE" get secret "$SECRET_NAME" >/dev/null 2>&1; then
    return
  fi
  [[ -n "${REMOTE_ENV_FILE:-}" ]] || die "缺少 Secret: ${KUBE_NAMESPACE}/${SECRET_NAME}，且未提供 PLATFORM_ENV_FILE"
  [[ -f "$REMOTE_ENV_FILE" ]] || die "未找到 bootstrap env 文件: $REMOTE_ENV_FILE"
  kubectl -n "$KUBE_NAMESPACE" create secret generic "$SECRET_NAME" --from-env-file="$REMOTE_ENV_FILE"
}

sync_secret_from_env_file() {
  [[ -n "${REMOTE_ENV_FILE:-}" ]] || return
  [[ -f "$REMOTE_ENV_FILE" ]] || die "未找到 bootstrap env 文件: $REMOTE_ENV_FILE"

  payload="$(
    python3 - <<'PY' "$REMOTE_ENV_FILE"
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
data = {}
for raw in path.read_text(encoding="utf-8").splitlines():
    line = raw.strip()
    if not line or line.startswith("#"):
        continue
    if "=" not in line:
        continue
    key, value = line.split("=", 1)
    key = key.strip()
    if not key:
        continue
    data[key] = value
print(json.dumps({"stringData": data}, ensure_ascii=False))
PY
  )"
  [[ -n "${payload}" ]] || die "无法从 env 文件生成 Secret patch: ${REMOTE_ENV_FILE}"
  kubectl -n "$KUBE_NAMESPACE" patch secret "$SECRET_NAME" --type merge -p "${payload}" >/dev/null
}

backfill_publish_auth_secret() {
  local legacy_auth_key=""
  local shared_secret=""
  local key_id=""

  legacy_auth_key="$(kubectl -n "$KUBE_NAMESPACE" get secret "$SECRET_NAME" -o jsonpath='{.data.CLAUDEOFFICE_AUTH_KEY}' 2>/dev/null | base64 -d || true)"
  shared_secret="$(kubectl -n "$KUBE_NAMESPACE" get secret "$SECRET_NAME" -o jsonpath='{.data.CLAUDEOFFICE_AUTH_SHARED_SECRET}' 2>/dev/null | base64 -d || true)"
  key_id="$(kubectl -n "$KUBE_NAMESPACE" get secret "$SECRET_NAME" -o jsonpath='{.data.CLAUDEOFFICE_AUTH_KEY_ID}' 2>/dev/null | base64 -d || true)"

  if [[ -z "$shared_secret" && -n "$legacy_auth_key" ]]; then
    kubectl -n "$KUBE_NAMESPACE" patch secret "$SECRET_NAME" --type merge -p "{\"stringData\":{\"CLAUDEOFFICE_AUTH_SHARED_SECRET\":\"${legacy_auth_key}\"}}" >/dev/null
  fi
  if [[ -z "$key_id" ]]; then
    kubectl -n "$KUBE_NAMESPACE" patch secret "$SECRET_NAME" --type merge -p '{"stringData":{"CLAUDEOFFICE_AUTH_KEY_ID":"platform-prod"}}' >/dev/null
  fi
}

assert_secret_keys() {
  local missing=()
  local key
  for key in CLAUDEOFFICE_BASE_URL CLAUDEOFFICE_AUTH_SHARED_SECRET; do
    if [[ -z "$(kubectl -n "$KUBE_NAMESPACE" get secret "$SECRET_NAME" -o "jsonpath={.data.${key}}" 2>/dev/null || true)" ]]; then
      missing+=("$key")
    fi
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    die "Secret ${KUBE_NAMESPACE}/${SECRET_NAME} 缺少必填项: ${missing[*]}"
  fi
}

ensure_postgres_secret() {
  if kubectl -n "$KUBE_NAMESPACE" get secret "$POSTGRES_SECRET_NAME" >/dev/null 2>&1; then
    return
  fi
  POSTGRES_PASSWORD="$(python3 - <<'PY'
import secrets
print(secrets.token_urlsafe(24))
PY
)"
  POSTGRES_DSN="host=${POSTGRES_SERVICE_NAME}.${KUBE_NAMESPACE}.svc.cluster.local port=5432 user=${POSTGRES_USER} password=${POSTGRES_PASSWORD} dbname=${POSTGRES_DB} sslmode=disable TimeZone=UTC"
  kubectl -n "$KUBE_NAMESPACE" create secret generic "$POSTGRES_SECRET_NAME" \
    --from-literal=POSTGRES_DB="$POSTGRES_DB" \
    --from-literal=POSTGRES_USER="$POSTGRES_USER" \
    --from-literal=POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
    --from-literal=POSTGRES_DSN="$POSTGRES_DSN"
}

sync_platform_secret_postgres_dsn() {
  POSTGRES_DSN="$(kubectl -n "$KUBE_NAMESPACE" get secret "$POSTGRES_SECRET_NAME" -o jsonpath='{.data.POSTGRES_DSN}' | base64 -d)"
  kubectl -n "$KUBE_NAMESPACE" patch secret "$SECRET_NAME" --type merge -p "{\"stringData\":{\"POSTGRES_DSN\":\"${POSTGRES_DSN}\"}}"
}

apply_postgres_manifests() {
  [[ -d "${REMOTE_POSTGRES_MANIFEST_DIR}" ]] || die "未找到 PostgreSQL 清单目录: ${REMOTE_POSTGRES_MANIFEST_DIR}"
  kubectl apply -f "${REMOTE_POSTGRES_MANIFEST_DIR}/service-headless.yaml"
  kubectl apply -f "${REMOTE_POSTGRES_MANIFEST_DIR}/service.yaml"
  kubectl apply -f "${REMOTE_POSTGRES_MANIFEST_DIR}/statefulset.yaml"
  kubectl apply -f "${REMOTE_POSTGRES_MANIFEST_DIR}/backup-cronjob.yaml"
}

wait_postgres_ready() {
  kubectl -n "$KUBE_NAMESPACE" rollout status "statefulset/${POSTGRES_STS_NAME}" --timeout=240s
}

run_db_pod() {
  local pod_name="$1"
  local args="$2"
  local mysql_dsn="${3:-}"
  kubectl -n "$KUBE_NAMESPACE" delete pod "$pod_name" --ignore-not-found=true >/dev/null 2>&1 || true
  if [[ -n "$mysql_dsn" ]]; then
    kubectl -n "$KUBE_NAMESPACE" run "$pod_name" \
      --image="$IMAGE_REF" \
      --restart=Never \
      --env="POSTGRES_DSN=$(kubectl -n "$KUBE_NAMESPACE" get secret "$POSTGRES_SECRET_NAME" -o jsonpath='{.data.POSTGRES_DSN}' | base64 -d)" \
      --env="MYSQL_DSN=${mysql_dsn}" \
      --command -- /app/officecli-platform $args
  else
    kubectl -n "$KUBE_NAMESPACE" run "$pod_name" \
      --image="$IMAGE_REF" \
      --restart=Never \
      --env="POSTGRES_DSN=$(kubectl -n "$KUBE_NAMESPACE" get secret "$POSTGRES_SECRET_NAME" -o jsonpath='{.data.POSTGRES_DSN}' | base64 -d)" \
      --command -- /app/officecli-platform $args
  fi
  kubectl -n "$KUBE_NAMESPACE" wait --for=condition=Ready pod/"$pod_name" --timeout=180s >/dev/null 2>&1 || true
  if ! kubectl -n "$KUBE_NAMESPACE" wait --for=jsonpath='{.status.phase}'=Succeeded pod/"$pod_name" --timeout=600s; then
    kubectl -n "$KUBE_NAMESPACE" logs "$pod_name" || true
    kubectl -n "$KUBE_NAMESPACE" delete pod "$pod_name" --ignore-not-found=true >/dev/null 2>&1 || true
    die "任务失败: ${pod_name}"
  fi
  kubectl -n "$KUBE_NAMESPACE" logs "$pod_name"
  kubectl -n "$KUBE_NAMESPACE" delete pod "$pod_name" --ignore-not-found=true >/dev/null 2>&1 || true
}

needs_initial_pg_copy() {
  if kubectl -n "$KUBE_NAMESPACE" get configmap officecli-platform-pg-cutover >/dev/null 2>&1; then
    return 1
  fi
  return 0
}

mark_pg_copy_complete() {
  kubectl -n "$KUBE_NAMESPACE" create configmap officecli-platform-pg-cutover \
    --from-literal=completed_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
    --from-literal=image="$IMAGE_REF" \
    --dry-run=client -o yaml | kubectl apply -f -
}

remove_mysql_dsn_from_secret() {
  if kubectl -n "$KUBE_NAMESPACE" get secret "$SECRET_NAME" -o jsonpath='{.data.MYSQL_DSN}' >/dev/null 2>&1; then
    kubectl -n "$KUBE_NAMESPACE" patch secret "$SECRET_NAME" --type json -p '[{"op":"remove","path":"/data/MYSQL_DSN"}]' >/dev/null 2>&1 || true
  fi
}

ensure_service() {
  if kubectl -n "$KUBE_NAMESPACE" get service "$DEPLOYMENT_NAME" >/dev/null 2>&1; then
    return
  fi
  cat <<SERVICE | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: ${DEPLOYMENT_NAME}
  namespace: ${KUBE_NAMESPACE}
  labels:
    app: ${DEPLOYMENT_NAME}
spec:
  selector:
    app: ${DEPLOYMENT_NAME}
  ports:
    - name: http
      port: 80
      targetPort: http
SERVICE
}

ensure_deployment() {
  if kubectl -n "$KUBE_NAMESPACE" get deployment "$DEPLOYMENT_NAME" >/dev/null 2>&1; then
    return
  fi
  cat <<DEPLOYMENT | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${DEPLOYMENT_NAME}
  namespace: ${KUBE_NAMESPACE}
  labels:
    app: ${DEPLOYMENT_NAME}
spec:
  replicas: 1
  revisionHistoryLimit: 3
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: ${DEPLOYMENT_NAME}
  template:
    metadata:
      labels:
        app: ${DEPLOYMENT_NAME}
    spec:
      containers:
        - name: ${CONTAINER_NAME}
          image: ${IMAGE_REF}
          imagePullPolicy: Never
          env:
            - name: HTTP_ADDR
              value: :8080
            - name: ADMIN_STATIC_DIR
              value: /app/web/admin/dist
          envFrom:
            - secretRef:
                name: ${SECRET_NAME}
          ports:
            - name: http
              containerPort: 8080
              hostPort: 29001
              protocol: TCP
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 15
            periodSeconds: 20
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 512Mi
DEPLOYMENT
}

cd "$REMOTE_WORKDIR"

ensure_namespace
ensure_secret
sync_secret_from_env_file
backfill_publish_auth_secret
assert_secret_keys
ensure_postgres_secret
sync_platform_secret_postgres_dsn
apply_postgres_manifests
wait_postgres_ready
ensure_service

deployment_exists=0
if kubectl -n "$KUBE_NAMESPACE" get deployment "$DEPLOYMENT_NAME" >/dev/null 2>&1; then
  deployment_exists=1
fi

previous_replicas=1
if [[ "$deployment_exists" -eq 1 ]]; then
  previous_replicas="$(kubectl -n "$KUBE_NAMESPACE" get deployment "$DEPLOYMENT_NAME" -o jsonpath='{.spec.replicas}')"
  [[ -n "$previous_replicas" ]] || previous_replicas=1
fi

echo "--- import image ---"
gzip -dc "$ARCHIVE_NAME" | sudo k3s ctr images import -

echo
echo "--- run postgres migration ---"
run_db_pod officecli-platform-db-migrate "db migrate"

mysql_dsn=""
if kubectl -n "$KUBE_NAMESPACE" get secret "$SECRET_NAME" -o jsonpath='{.data.MYSQL_DSN}' >/dev/null 2>&1; then
  mysql_dsn="$(kubectl -n "$KUBE_NAMESPACE" get secret "$SECRET_NAME" -o jsonpath='{.data.MYSQL_DSN}' | base64 -d)"
fi

if [[ "$deployment_exists" -eq 1 ]] && [[ -n "$mysql_dsn" ]] && needs_initial_pg_copy; then
  echo
  echo "--- initial mysql -> postgres copy ---"
  rollback_scaledown() {
    kubectl -n "$KUBE_NAMESPACE" scale deployment "$DEPLOYMENT_NAME" --replicas="$previous_replicas" >/dev/null 2>&1 || true
  }
  trap rollback_scaledown ERR
  kubectl -n "$KUBE_NAMESPACE" scale deployment "$DEPLOYMENT_NAME" --replicas=0
  kubectl -n "$KUBE_NAMESPACE" wait --for=delete pod -l "app=${DEPLOYMENT_NAME}" --timeout=180s >/dev/null 2>&1 || true
  run_db_pod officecli-platform-db-copy "db copy" "$mysql_dsn"
  mark_pg_copy_complete
  remove_mysql_dsn_from_secret
  kubectl -n "$KUBE_NAMESPACE" scale deployment "$DEPLOYMENT_NAME" --replicas="$previous_replicas"
  trap - ERR
fi

if [[ "$deployment_exists" -eq 1 ]]; then
  echo "--- preflight deployment ---"
  kubectl -n "$KUBE_NAMESPACE" get deployment "$DEPLOYMENT_NAME" -o wide

  echo
  echo "--- backup deployment yaml ---"
  backup_path="${REMOTE_WORKDIR}/${DEPLOYMENT_NAME}-${TAG}-before.yaml"
  kubectl -n "$KUBE_NAMESPACE" get deployment "$DEPLOYMENT_NAME" -o yaml > "$backup_path"
  echo "$backup_path"

  kubectl -n "$KUBE_NAMESPACE" set image "deployment/$DEPLOYMENT_NAME" \
    "$CONTAINER_NAME=$IMAGE_REF"

  kubectl -n "$KUBE_NAMESPACE" patch deployment "$DEPLOYMENT_NAME" --type json -p \
    '[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Never"}]'

  kubectl -n "$KUBE_NAMESPACE" patch deployment "$DEPLOYMENT_NAME" --type merge -p \
    '{"spec":{"strategy":{"type":"Recreate","rollingUpdate":null}}}'
else
  echo "--- bootstrap deployment ---"
  ensure_deployment
fi

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
