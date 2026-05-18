#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PLATFORM_DIR="${ROOT_DIR}/platform"

TEST_SERVER_HOST="${TEST_SERVER_HOST:-172.17.9.196}"
TEST_SERVER_USER="${TEST_SERVER_USER:-root}"
TEST_SERVER="${TEST_SERVER_USER}@${TEST_SERVER_HOST}"
TEST_SSH_PORT="${TEST_SSH_PORT:-22}"
TEST_DOMAIN="${TEST_DOMAIN:-officecli.shimodev.com}"
TEST_NAMESPACE="${TEST_NAMESPACE:-officecli}"
TEST_TLS_SECRET="${TEST_TLS_SECRET:-}"
REMOTE_WORKDIR="${REMOTE_WORKDIR:-/opt/officecli-platform-test}"
IMAGE_REPO="${IMAGE_REPO:-docker.io/library/officecli-platform-test}"
DEPLOYMENT_NAME="${DEPLOYMENT_NAME:-officecli-platform}"
CONTAINER_NAME="${CONTAINER_NAME:-platform}"
SECRET_NAME="${SECRET_NAME:-officecli-platform-env}"
POSTGRES_SECRET_NAME="${POSTGRES_SECRET_NAME:-officecli-platform-postgres}"
MINIO_SECRET_NAME="${MINIO_SECRET_NAME:-officecli-minio}"
PLATFORM_ENV_FILE="${PLATFORM_ENV_FILE:-}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16}"
REDIS_IMAGE="${REDIS_IMAGE:-redis:7.4}"
MINIO_IMAGE="${MINIO_IMAGE:-minio/minio:latest}"

SSH_OPTS=(-p "${TEST_SSH_PORT}" -o StrictHostKeyChecking=no)

usage() {
  cat <<'EOF'
Usage:
  PLATFORM_ENV_FILE=/secure/path/officecli-platform-test.env bash scripts/deploy-platform-test.sh [tag]

Deploys the full platform testing stack to root@172.17.9.196 k3s namespace officecli.
Required integrations are real OAuth2, Hosted LLM, and in-namespace MinIO. Stripe may use test/placeholder values.
EOF
}

log() {
  printf '\n[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

die() {
  echo "Error: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Missing command: $1"
}

require_env_file_keys() {
  local env_file="$1"
  shift
  local missing=()
  local placeholder=()
  local key value
  [[ -f "${env_file}" ]] || die "PLATFORM_ENV_FILE was not found: ${env_file}"
  for key in "$@"; do
    if ! grep -Eq "^${key}=" "${env_file}"; then
      missing+=("${key}")
      continue
    fi
    value="$(grep -E "^${key}=" "${env_file}" | tail -1 | cut -d= -f2-)"
    if [[ -z "${value}" || "${value}" == replace-* || "${value}" == *placeholder* || "${value}" == *example.com* ]]; then
      placeholder+=("${key}")
    fi
  done
  [[ ${#missing[@]} -eq 0 ]] || die "Env file is missing required keys: ${missing[*]}"
  [[ ${#placeholder[@]} -eq 0 ]] || die "Env file still contains empty/example values for required testing integrations: ${placeholder[*]}"
}

detect_tag() {
  if [[ -n "${1:-}" ]]; then
    echo "$1"
    return
  fi
  printf 'test-%s-%s' "$(date '+%Y%m%d%H%M%S')" "$(git -C "${ROOT_DIR}" rev-parse --short HEAD)"
}

run_local_checks_and_builds() {
  log "Running platform Go tests"
  (cd "${PLATFORM_DIR}" && go test ./...)

  log "Building web/app"
  (cd "${PLATFORM_DIR}/web/app" && { [[ -x node_modules/.bin/vite ]] || npm ci; } && npm run build)

  log "Building web/admin"
  (cd "${PLATFORM_DIR}/web/admin" && { [[ -x node_modules/.bin/vite ]] || npm ci; } && npm run build)

  log "Building web/site"
  (cd "${PLATFORM_DIR}/web/site" && { [[ -x node_modules/.bin/vite ]] || npm ci; } && npm run build)
}

build_image_archive() {
  local image_ref="$1"
  local archive_path="$2"
  local dockerfile

  dockerfile="$(mktemp "${TMPDIR:-/tmp}/officecli-platform-test.Dockerfile.XXXXXX")"
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

  log "Building image ${image_ref}"
  if docker buildx version >/dev/null 2>&1; then
    (cd "${PLATFORM_DIR}" && docker buildx build --platform linux/amd64 -f "${dockerfile}" -t "${image_ref}" --load .)
  else
    (cd "${PLATFORM_DIR}" && docker build --platform linux/amd64 -f "${dockerfile}" -t "${image_ref}" .)
  fi

  log "Exporting image archive ${archive_path}"
  docker save "${image_ref}" | gzip >"${archive_path}"
}

build_support_image_archive() {
  local archive_path="$1"
  local image

  for image in "${POSTGRES_IMAGE}" "${REDIS_IMAGE}" "${MINIO_IMAGE}"; do
    log "Pulling support image ${image}"
    docker pull --platform linux/amd64 "${image}"
  done

  log "Exporting support images archive ${archive_path}"
  docker save "${POSTGRES_IMAGE}" "${REDIS_IMAGE}" "${MINIO_IMAGE}" | gzip >"${archive_path}"
}

deploy_remote() {
  local tag="$1"
  local image_ref="$2"
  local archive_path="$3"
  local support_archive_path="$4"
  local archive_name support_archive_name env_name

  archive_name="$(basename "${archive_path}")"
  support_archive_name="$(basename "${support_archive_path}")"
  env_name="$(basename "${PLATFORM_ENV_FILE}")"

  log "Uploading artifacts to ${TEST_SERVER}:${REMOTE_WORKDIR}"
  ssh "${SSH_OPTS[@]}" "${TEST_SERVER}" "mkdir -p '${REMOTE_WORKDIR}'"
  rsync -az --partial --inplace -e "ssh -p ${TEST_SSH_PORT} -o StrictHostKeyChecking=no" "${archive_path}" "${TEST_SERVER}:${REMOTE_WORKDIR}/${archive_name}"
  rsync -az --partial --inplace -e "ssh -p ${TEST_SSH_PORT} -o StrictHostKeyChecking=no" "${support_archive_path}" "${TEST_SERVER}:${REMOTE_WORKDIR}/${support_archive_name}"
  rsync -az --partial --inplace -e "ssh -p ${TEST_SSH_PORT} -o StrictHostKeyChecking=no" "${PLATFORM_ENV_FILE}" "${TEST_SERVER}:${REMOTE_WORKDIR}/${env_name}"

  log "Applying k3s resources in namespace ${TEST_NAMESPACE}"
  ssh "${SSH_OPTS[@]}" "${TEST_SERVER}" \
    TAG="${tag}" \
    IMAGE_REF="${image_ref}" \
    ARCHIVE_NAME="${archive_name}" \
    SUPPORT_ARCHIVE_NAME="${support_archive_name}" \
    REMOTE_WORKDIR="${REMOTE_WORKDIR}" \
    TEST_NAMESPACE="${TEST_NAMESPACE}" \
    TEST_DOMAIN="${TEST_DOMAIN}" \
    TEST_TLS_SECRET="${TEST_TLS_SECRET}" \
    SECRET_NAME="${SECRET_NAME}" \
    POSTGRES_SECRET_NAME="${POSTGRES_SECRET_NAME}" \
    MINIO_SECRET_NAME="${MINIO_SECRET_NAME}" \
    POSTGRES_IMAGE="${POSTGRES_IMAGE}" \
    REDIS_IMAGE="${REDIS_IMAGE}" \
    MINIO_IMAGE="${MINIO_IMAGE}" \
    DEPLOYMENT_NAME="${DEPLOYMENT_NAME}" \
    CONTAINER_NAME="${CONTAINER_NAME}" \
    REMOTE_ENV_FILE="${REMOTE_WORKDIR}/${env_name}" \
    'bash -se' <<'REMOTE'
set -euo pipefail

die() {
  echo "Error: $*" >&2
  exit 1
}

kubectl create namespace "${TEST_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

cd "${REMOTE_WORKDIR}"
gzip -dc "${SUPPORT_ARCHIVE_NAME}" | sudo k3s ctr images import -

if ! kubectl -n "${TEST_NAMESPACE}" get secret "${SECRET_NAME}" >/dev/null 2>&1; then
  kubectl -n "${TEST_NAMESPACE}" create secret generic "${SECRET_NAME}" --from-env-file="${REMOTE_ENV_FILE}"
fi

payload="$(python3 - <<'PY' "${REMOTE_ENV_FILE}"
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
data = {}
for raw in path.read_text(encoding="utf-8").splitlines():
    line = raw.strip()
    if not line or line.startswith("#") or "=" not in line:
        continue
    key, value = line.split("=", 1)
    key = key.strip()
    if key:
        data[key] = value
print(json.dumps({"stringData": data}, ensure_ascii=False))
PY
)"
kubectl -n "${TEST_NAMESPACE}" patch secret "${SECRET_NAME}" --type merge -p "${payload}" >/dev/null

for key in OAUTH2_AUTH_URL OAUTH2_TOKEN_URL OAUTH2_USERINFO_URL OAUTH2_CLIENT_ID OAUTH2_CLIENT_SECRET HOSTED_LLM_BASE_URL HOSTED_LLM_API_KEY HOSTED_LLM_TEXT_MODEL HOSTED_LLM_IMAGE_MODEL; do
  value="$(kubectl -n "${TEST_NAMESPACE}" get secret "${SECRET_NAME}" -o "jsonpath={.data.${key}}" 2>/dev/null | base64 -d || true)"
  [[ -n "${value}" ]] || die "Secret ${SECRET_NAME} is missing required testing key: ${key}"
  [[ "${value}" != replace-* && "${value}" != *placeholder* && "${value}" != *example.com* ]] || die "Secret ${SECRET_NAME} contains placeholder value for ${key}"
done

for key in ADMIN_OAUTH2_CLIENT_ID ADMIN_OAUTH2_CLIENT_SECRET; do
  value="$(kubectl -n "${TEST_NAMESPACE}" get secret "${SECRET_NAME}" -o "jsonpath={.data.${key}}" 2>/dev/null | base64 -d || true)"
  [[ -z "${value}" || ( "${value}" != replace-* && "${value}" != *placeholder* && "${value}" != *example.com* ) ]] || die "Secret ${SECRET_NAME} contains placeholder value for optional ${key}"
done
admin_oauth2_client_id="$(kubectl -n "${TEST_NAMESPACE}" get secret "${SECRET_NAME}" -o "jsonpath={.data.ADMIN_OAUTH2_CLIENT_ID}" 2>/dev/null | base64 -d || true)"
admin_oauth2_client_secret="$(kubectl -n "${TEST_NAMESPACE}" get secret "${SECRET_NAME}" -o "jsonpath={.data.ADMIN_OAUTH2_CLIENT_SECRET}" 2>/dev/null | base64 -d || true)"
if { [[ -n "${admin_oauth2_client_id}" && -z "${admin_oauth2_client_secret}" ]] || [[ -z "${admin_oauth2_client_id}" && -n "${admin_oauth2_client_secret}" ]]; }; then
  die "ADMIN_OAUTH2_CLIENT_ID and ADMIN_OAUTH2_CLIENT_SECRET must be configured together"
fi

kubectl -n "${TEST_NAMESPACE}" patch secret "${SECRET_NAME}" --type merge -p "{\"stringData\":{\"APP_ENV\":\"staging\",\"SITE_BASE_URL\":\"https://${TEST_DOMAIN}\",\"PLATFORM_BASE_URL\":\"https://${TEST_DOMAIN}\",\"OAUTH2_REDIRECT_URL\":\"https://${TEST_DOMAIN}/api/auth/oauth2/callback\",\"ADMIN_OAUTH2_REDIRECT_URL\":\"https://${TEST_DOMAIN}/api/admin/auth/oauth2/callback\",\"GOOGLE_REDIRECT_URL\":\"https://${TEST_DOMAIN}/api/auth/google/callback\",\"ADMIN_GOOGLE_REDIRECT_URL\":\"https://${TEST_DOMAIN}/api/admin/auth/google/callback\",\"REDIS_ADDR\":\"officecli-platform-redis.${TEST_NAMESPACE}.svc.cluster.local:6379\",\"PREVIEW_OBJECT_ENDPOINT\":\"officecli-minio.${TEST_NAMESPACE}.svc.cluster.local:9000\",\"PREVIEW_OBJECT_BUCKET\":\"officecli-preview\",\"PREVIEW_OBJECT_USE_SSL\":\"false\",\"ADMIN_STATIC_DIR\":\"/app/web/admin/dist\",\"APP_STATIC_DIR\":\"/app/web/app/dist\",\"SITE_STATIC_DIR\":\"/app/web/site/dist\"}}" >/dev/null

if ! kubectl -n "${TEST_NAMESPACE}" get secret "${POSTGRES_SECRET_NAME}" >/dev/null 2>&1; then
  POSTGRES_PASSWORD="$(python3 - <<'PY'
import secrets
print(secrets.token_urlsafe(24))
PY
)"
  POSTGRES_DSN="host=officecli-platform-postgres.${TEST_NAMESPACE}.svc.cluster.local port=5432 user=officecli password=${POSTGRES_PASSWORD} dbname=officecli_platform sslmode=disable TimeZone=UTC"
  kubectl -n "${TEST_NAMESPACE}" create secret generic "${POSTGRES_SECRET_NAME}" \
    --from-literal=POSTGRES_DB=officecli_platform \
    --from-literal=POSTGRES_USER=officecli \
    --from-literal=POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" \
    --from-literal=POSTGRES_DSN="${POSTGRES_DSN}"
fi
POSTGRES_DSN="$(kubectl -n "${TEST_NAMESPACE}" get secret "${POSTGRES_SECRET_NAME}" -o jsonpath='{.data.POSTGRES_DSN}' | base64 -d)"
kubectl -n "${TEST_NAMESPACE}" patch secret "${SECRET_NAME}" --type merge -p "{\"stringData\":{\"POSTGRES_DSN\":\"${POSTGRES_DSN}\"}}" >/dev/null

if ! kubectl -n "${TEST_NAMESPACE}" get secret "${MINIO_SECRET_NAME}" >/dev/null 2>&1; then
  MINIO_ROOT_USER="officecli"
  MINIO_ROOT_PASSWORD="$(python3 - <<'PY'
import secrets
print(secrets.token_urlsafe(32))
PY
)"
  kubectl -n "${TEST_NAMESPACE}" create secret generic "${MINIO_SECRET_NAME}" \
    --from-literal=MINIO_ROOT_USER="${MINIO_ROOT_USER}" \
    --from-literal=MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD}"
fi
MINIO_ROOT_USER="$(kubectl -n "${TEST_NAMESPACE}" get secret "${MINIO_SECRET_NAME}" -o jsonpath='{.data.MINIO_ROOT_USER}' | base64 -d)"
MINIO_ROOT_PASSWORD="$(kubectl -n "${TEST_NAMESPACE}" get secret "${MINIO_SECRET_NAME}" -o jsonpath='{.data.MINIO_ROOT_PASSWORD}' | base64 -d)"
kubectl -n "${TEST_NAMESPACE}" patch secret "${SECRET_NAME}" --type merge -p "{\"stringData\":{\"PREVIEW_OBJECT_ACCESS_KEY\":\"${MINIO_ROOT_USER}\",\"PREVIEW_OBJECT_SECRET_KEY\":\"${MINIO_ROOT_PASSWORD}\"}}" >/dev/null

cat <<YAML | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: officecli-platform-postgres
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: officecli-platform-postgres
  ports:
    - name: postgres
      port: 5432
      targetPort: 5432
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: officecli-platform-postgres
  namespace: ${TEST_NAMESPACE}
spec:
  serviceName: officecli-platform-postgres
  replicas: 1
  selector:
    matchLabels:
      app: officecli-platform-postgres
  template:
    metadata:
      labels:
        app: officecli-platform-postgres
    spec:
      containers:
        - name: postgres
          image: ${POSTGRES_IMAGE}
          imagePullPolicy: IfNotPresent
          envFrom:
            - secretRef:
                name: ${POSTGRES_SECRET_NAME}
          ports:
            - name: postgres
              containerPort: 5432
          readinessProbe:
            exec:
              command: ["sh", "-c", "pg_isready -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\""]
            initialDelaySeconds: 10
            periodSeconds: 10
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: local-path
        resources:
          requests:
            storage: 20Gi
---
apiVersion: v1
kind: Service
metadata:
  name: officecli-platform-redis
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: officecli-platform-redis
  ports:
    - name: redis
      port: 6379
      targetPort: 6379
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: officecli-platform-redis
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: officecli-platform-redis
  template:
    metadata:
      labels:
        app: officecli-platform-redis
    spec:
      containers:
        - name: redis
          image: ${REDIS_IMAGE}
          imagePullPolicy: IfNotPresent
          args: ["redis-server", "--appendonly", "yes"]
          ports:
            - name: redis
              containerPort: 6379
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: officecli-platform-redis-data
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: officecli-platform-redis-data
  namespace: ${TEST_NAMESPACE}
spec:
  accessModes: ["ReadWriteOnce"]
  storageClassName: local-path
  resources:
    requests:
      storage: 2Gi
---
apiVersion: v1
kind: Service
metadata:
  name: officecli-minio
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: officecli-minio
  ports:
    - name: api
      port: 9000
      targetPort: 9000
    - name: console
      port: 9001
      targetPort: 9001
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: officecli-minio
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: officecli-minio
  template:
    metadata:
      labels:
        app: officecli-minio
    spec:
      containers:
        - name: minio
          image: ${MINIO_IMAGE}
          imagePullPolicy: IfNotPresent
          args: ["server", "/data", "--console-address", ":9001"]
          envFrom:
            - secretRef:
                name: ${MINIO_SECRET_NAME}
          ports:
            - name: api
              containerPort: 9000
            - name: console
              containerPort: 9001
          readinessProbe:
            httpGet:
              path: /minio/health/ready
              port: api
            initialDelaySeconds: 10
            periodSeconds: 10
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: officecli-minio-data
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: officecli-minio-data
  namespace: ${TEST_NAMESPACE}
spec:
  accessModes: ["ReadWriteOnce"]
  storageClassName: local-path
  resources:
    requests:
      storage: 20Gi
YAML

kubectl -n "${TEST_NAMESPACE}" rollout status statefulset/officecli-platform-postgres --timeout=240s
kubectl -n "${TEST_NAMESPACE}" rollout status deployment/officecli-platform-redis --timeout=180s
kubectl -n "${TEST_NAMESPACE}" rollout status deployment/officecli-minio --timeout=180s

gzip -dc "${ARCHIVE_NAME}" | sudo k3s ctr images import -

kubectl -n "${TEST_NAMESPACE}" delete pod officecli-platform-db-migrate --ignore-not-found=true >/dev/null 2>&1 || true
cat <<YAML | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: officecli-platform-db-migrate
  namespace: ${TEST_NAMESPACE}
spec:
  restartPolicy: Never
  containers:
    - name: migrate
      image: ${IMAGE_REF}
      imagePullPolicy: Never
      envFrom:
        - secretRef:
            name: ${SECRET_NAME}
      command: ["/app/officecli-platform", "db", "migrate"]
YAML
if ! kubectl -n "${TEST_NAMESPACE}" wait --for=jsonpath='{.status.phase}'=Succeeded pod/officecli-platform-db-migrate --timeout=600s; then
  kubectl -n "${TEST_NAMESPACE}" logs officecli-platform-db-migrate || true
  exit 1
fi
kubectl -n "${TEST_NAMESPACE}" logs officecli-platform-db-migrate || true
kubectl -n "${TEST_NAMESPACE}" delete pod officecli-platform-db-migrate --ignore-not-found=true >/dev/null 2>&1 || true

cat <<YAML | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: ${DEPLOYMENT_NAME}
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: ${DEPLOYMENT_NAME}
  ports:
    - name: http
      port: 80
      targetPort: http
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${DEPLOYMENT_NAME}
  namespace: ${TEST_NAMESPACE}
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
        deploy-tag: "${TAG}"
    spec:
      containers:
        - name: ${CONTAINER_NAME}
          image: ${IMAGE_REF}
          imagePullPolicy: Never
          envFrom:
            - secretRef:
                name: ${SECRET_NAME}
          ports:
            - name: http
              containerPort: 8080
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
              cpu: 1
              memory: 1Gi
YAML

if [[ -n "${TEST_TLS_SECRET}" ]]; then
  cat <<YAML | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${DEPLOYMENT_NAME}
  namespace: ${TEST_NAMESPACE}
spec:
  ingressClassName: traefik
  tls:
    - hosts:
        - ${TEST_DOMAIN}
      secretName: ${TEST_TLS_SECRET}
  rules:
    - host: ${TEST_DOMAIN}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: ${DEPLOYMENT_NAME}
                port:
                  number: 80
YAML
else
  cat <<YAML | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${DEPLOYMENT_NAME}
  namespace: ${TEST_NAMESPACE}
spec:
  ingressClassName: traefik
  rules:
    - host: ${TEST_DOMAIN}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: ${DEPLOYMENT_NAME}
                port:
                  number: 80
YAML
fi

kubectl -n "${TEST_NAMESPACE}" rollout status deployment/"${DEPLOYMENT_NAME}" --timeout=240s
kubectl -n "${TEST_NAMESPACE}" get all,ingress,pvc
REMOTE
}

main() {
  [[ $# -le 1 ]] || { usage; exit 1; }
  [[ "${1:-}" != "-h" && "${1:-}" != "--help" ]] || { usage; exit 0; }
  [[ -d "${PLATFORM_DIR}" ]] || die "platform directory was not found"
  [[ -n "${PLATFORM_ENV_FILE}" ]] || die "PLATFORM_ENV_FILE is required"

  require_cmd go
  require_cmd npm
  require_cmd docker
  require_cmd gzip
  require_cmd ssh
  require_cmd rsync

  require_env_file_keys "${PLATFORM_ENV_FILE}" \
    OAUTH2_AUTH_URL OAUTH2_TOKEN_URL OAUTH2_USERINFO_URL OAUTH2_CLIENT_ID OAUTH2_CLIENT_SECRET \
    HOSTED_LLM_BASE_URL HOSTED_LLM_API_KEY HOSTED_LLM_TEXT_MODEL HOSTED_LLM_IMAGE_MODEL \
    SESSION_SECRET APP_SESSION_SECRET API_KEY_HASH_SALT API_KEY_ENCRYPTION_KEY LICENSE_PROOF_SEED

  local tag image_ref archive_path support_archive_path
  tag="$(detect_tag "${1:-}")"
  image_ref="${IMAGE_REPO}:${tag}"
  archive_path="${TMPDIR:-/tmp}/officecli-platform-${tag}.tar.gz"
  support_archive_path="${TMPDIR:-/tmp}/officecli-platform-support-images.tar.gz"

  log "Testing deploy tag: ${tag}"
  log "Image reference: ${image_ref}"

  run_local_checks_and_builds
  build_image_archive "${image_ref}" "${archive_path}"
  build_support_image_archive "${support_archive_path}"
  deploy_remote "${tag}" "${image_ref}" "${archive_path}" "${support_archive_path}"

  log "Testing environment deploy completed"
  echo "tag=${tag}"
  echo "image=${image_ref}"
  echo "domain=${TEST_DOMAIN}"
  echo "namespace=${TEST_NAMESPACE}"
}

main "$@"
