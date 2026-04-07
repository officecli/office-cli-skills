#!/usr/bin/env bash

set -euo pipefail

SERVER_HOST="${SERVER_HOST:-18.141.191.80}"
SERVER_USER="${SERVER_USER:-ubuntu}"
SERVER="${SERVER_USER}@${SERVER_HOST}"
SSH_PORT="${SSH_PORT:-22}"
KUBE_NAMESPACE="${KUBE_NAMESPACE:-officecli}"
DEPLOYMENT_NAME="${DEPLOYMENT_NAME:-officecli-platform}"
MYSQL_BACKUP_DIR="${MYSQL_BACKUP_DIR:-/opt/officecli-platform/mysql-backups}"
PLATFORM_BIN_PATH="${PLATFORM_BIN_PATH:-/app/officecli-platform}"

ssh_cmd() {
  ssh -p "${SSH_PORT}" "$@"
}

timestamp="$(date +%Y%m%d-%H%M%S)"

ssh_cmd "${SERVER}" "sudo mkdir -p '${MYSQL_BACKUP_DIR}'"
ssh_cmd "${SERVER}" "export MYSQL_PWD=\"\${MYSQL_PASSWORD:-}\"; mysqldump --single-transaction --databases \"\${MYSQL_DATABASE:-officecli_platform}\" > '${MYSQL_BACKUP_DIR}/officecli-platform-${timestamp}.sql'"
ssh_cmd "${SERVER}" "kubectl -n '${KUBE_NAMESPACE}' scale deployment '${DEPLOYMENT_NAME}' --replicas=0"
ssh_cmd "${SERVER}" "kubectl -n '${KUBE_NAMESPACE}' get deployment '${DEPLOYMENT_NAME}'"
ssh_cmd "${SERVER}" "kubectl -n '${KUBE_NAMESPACE}' run officecli-platform-db-migrate --rm --restart=Never --image=docker.io/library/officecli-platform:latest --env-from=secret/officecli-platform-postgres --command -- ${PLATFORM_BIN_PATH} db migrate"
ssh_cmd "${SERVER}" "kubectl -n '${KUBE_NAMESPACE}' run officecli-platform-db-copy --rm --restart=Never --image=docker.io/library/officecli-platform:latest --env=MYSQL_DSN=\"\${MYSQL_DSN:?set MYSQL_DSN in ssh env}\" --env-from=secret/officecli-platform-postgres --command -- ${PLATFORM_BIN_PATH} db copy"
ssh_cmd "${SERVER}" "kubectl -n '${KUBE_NAMESPACE}' set env deployment/'${DEPLOYMENT_NAME}' --from=secret/officecli-platform-postgres"
ssh_cmd "${SERVER}" "kubectl -n '${KUBE_NAMESPACE}' scale deployment '${DEPLOYMENT_NAME}' --replicas=1"
ssh_cmd "${SERVER}" "kubectl -n '${KUBE_NAMESPACE}' rollout status deployment/'${DEPLOYMENT_NAME}' --timeout=180s"
