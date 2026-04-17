#!/usr/bin/env bash

set -euo pipefail

CONTROL_REPO="${CONTROL_REPO:-officecli/officecli-ci}"
WORKFLOW_REF="${WORKFLOW_REF:-main}"
DRY_RUN=0

usage() {
  cat <<'EOF'
Usage:
  scripts/dispatch-platform-prod-workflow.sh deploy <release-tag> [--repo <owner/repo>] [--ref <branch>] [--dry-run]
  scripts/dispatch-platform-prod-workflow.sh sync-secret [--restart-deployment|--no-restart-deployment] [--repo <owner/repo>] [--ref <branch>] [--dry-run]

Examples:
  scripts/dispatch-platform-prod-workflow.sh deploy v0.2.0-prod-20260417-1
  scripts/dispatch-platform-prod-workflow.sh deploy 0.2.0-prod-20260417-1 --dry-run
  scripts/dispatch-platform-prod-workflow.sh sync-secret --restart-deployment
  scripts/dispatch-platform-prod-workflow.sh sync-secret --no-restart-deployment --dry-run
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Error: missing command: $1" >&2
    exit 1
  }
}

die() {
  echo "Error: $*" >&2
  exit 1
}

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

run_cmd() {
  if [[ "${DRY_RUN}" == "1" ]]; then
    printf 'DRY-RUN:'
    printf ' %q' "$@"
    printf '\n'
    return 0
  fi
  "$@"
}

normalize_release_tag() {
  local value="$1"
  value="${value#refs/tags/}"
  if [[ "${value}" != v* ]]; then
    value="v${value}"
  fi
  printf '%s\n' "${value}"
}

dispatch_deploy() {
  local release_tag=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repo)
        [[ $# -ge 2 ]] || die "--repo requires a value"
        CONTROL_REPO="$2"
        shift 2
        ;;
      --ref)
        [[ $# -ge 2 ]] || die "--ref requires a value"
        WORKFLOW_REF="$2"
        shift 2
        ;;
      --dry-run)
        DRY_RUN=1
        shift
        ;;
      -h|--help|help)
        usage
        exit 0
        ;;
      --*)
        die "unsupported argument for deploy: $1"
        ;;
      *)
        if [[ -n "${release_tag}" ]]; then
          die "deploy accepts exactly one release tag"
        fi
        release_tag="$1"
        shift
        ;;
    esac
  done

  [[ -n "${release_tag}" ]] || die "deploy requires a release tag"
  release_tag="$(normalize_release_tag "${release_tag}")"

  run_cmd gh workflow run platform-deploy.yml \
    --repo "${CONTROL_REPO}" \
    --ref "${WORKFLOW_REF}" \
    -f "release_tag=${release_tag}"

  if [[ "${DRY_RUN}" == "1" ]]; then
    log "Would dispatch Platform Deploy to ${CONTROL_REPO} for ${release_tag}"
  else
    log "Dispatched Platform Deploy to ${CONTROL_REPO} for ${release_tag}"
  fi
  echo "Inspect recent runs with:"
  echo "  gh run list --repo ${CONTROL_REPO} --workflow platform-deploy.yml --limit 5"
  echo "Open workflow page:"
  echo "  https://github.com/${CONTROL_REPO}/actions/workflows/platform-deploy.yml"
}

dispatch_sync_secret() {
  local restart_deployment="true"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repo)
        [[ $# -ge 2 ]] || die "--repo requires a value"
        CONTROL_REPO="$2"
        shift 2
        ;;
      --ref)
        [[ $# -ge 2 ]] || die "--ref requires a value"
        WORKFLOW_REF="$2"
        shift 2
        ;;
      --dry-run)
        DRY_RUN=1
        shift
        ;;
      --restart-deployment)
        restart_deployment="true"
        shift
        ;;
      --no-restart-deployment)
        restart_deployment="false"
        shift
        ;;
      -h|--help|help)
        usage
        exit 0
        ;;
      *)
        die "unsupported argument for sync-secret: $1"
        ;;
    esac
  done

  run_cmd gh workflow run platform-sync-prod-secret.yml \
    --repo "${CONTROL_REPO}" \
    --ref "${WORKFLOW_REF}" \
    -f "restart_deployment=${restart_deployment}"

  if [[ "${DRY_RUN}" == "1" ]]; then
    log "Would dispatch Platform Sync Prod Secret to ${CONTROL_REPO} (restart_deployment=${restart_deployment})"
  else
    log "Dispatched Platform Sync Prod Secret to ${CONTROL_REPO} (restart_deployment=${restart_deployment})"
  fi
  echo "Inspect recent runs with:"
  echo "  gh run list --repo ${CONTROL_REPO} --workflow platform-sync-prod-secret.yml --limit 5"
  echo "Open workflow page:"
  echo "  https://github.com/${CONTROL_REPO}/actions/workflows/platform-sync-prod-secret.yml"
}

main() {
  require_cmd gh

  [[ $# -gt 0 ]] || {
    usage
    exit 1
  }

  local command="$1"
  shift

  case "${command}" in
    deploy)
      dispatch_deploy "$@"
      ;;
    sync-secret)
      dispatch_sync_secret "$@"
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      die "unsupported command: ${command}"
      ;;
  esac
}

main "$@"
