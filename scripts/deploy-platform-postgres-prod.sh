#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
MANIFEST_DIR="${ROOT_DIR}/platform/deploy/k8s/postgres"

SERVER_HOST="${SERVER_HOST:-18.141.191.80}"
SERVER_USER="${SERVER_USER:-ubuntu}"
SERVER="${SERVER_USER}@${SERVER_HOST}"
SSH_PORT="${SSH_PORT:-22}"
KUBE_NAMESPACE="${KUBE_NAMESPACE:-officecli}"
REMOTE_DIR="${REMOTE_DIR:-/tmp/officecli-platform-postgres}"

ssh_cmd() {
  ssh -p "${SSH_PORT}" "$@"
}

scp_cmd() {
  scp -P "${SSH_PORT}" "$@"
}

ssh_cmd "${SERVER}" "mkdir -p '${REMOTE_DIR}'"
scp_cmd "${MANIFEST_DIR}"/*.yaml "${SERVER}:${REMOTE_DIR}/"
ssh_cmd "${SERVER}" "kubectl apply -f '${REMOTE_DIR}/secret.yaml' && kubectl apply -f '${REMOTE_DIR}/service-headless.yaml' && kubectl apply -f '${REMOTE_DIR}/service.yaml' && kubectl apply -f '${REMOTE_DIR}/statefulset.yaml' && kubectl apply -f '${REMOTE_DIR}/backup-cronjob.yaml' && kubectl -n '${KUBE_NAMESPACE}' rollout status statefulset/officecli-platform-postgres --timeout=180s && kubectl -n '${KUBE_NAMESPACE}' get pods -l app=officecli-platform-postgres -o wide"
