#!/usr/bin/env bash

set -euo pipefail

OFFICECLI_ENV_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_REPO_DEFAULT="${DIST_REPO:-officecli/officecli-dist}"
DEFAULT_LICENSE_BASE_URL="${OFFICECLI_SETUP_LICENSE_BASE_URL:-https://platform.officecli.io}"
DEFAULT_PUBLISH_BASE_URL="${OFFICECLI_SETUP_DEFAULT_PUBLISH_BASE_URL:-https://claudeoffice.com}"
PUBLIC_SKILLS_REPO_DEFAULT="${PUBLIC_SKILLS_REPO:-officecli/officecli-skills}"
PUBLIC_SKILLS_BRANCH_DEFAULT="${PUBLIC_SKILLS_BRANCH:-main}"

json_escape() {
  local value="${1:-}"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '%s' "$value"
}

join_json_array() {
  local first=1
  local item
  printf '['
  for item in "$@"; do
    if [[ ${first} -eq 0 ]]; then
      printf ','
    fi
    first=0
    printf '"%s"' "$(json_escape "$item")"
  done
  printf ']'
}

truthy() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

default_publish_base_url() {
  local publish_base_url="${OFFICECLI_SETUP_PUBLISH_BASE_URL:-}"
  if [[ -n "${publish_base_url}" ]]; then
    printf '%s\n' "${publish_base_url}"
    return 0
  fi

  printf '%s\n' "${DEFAULT_PUBLISH_BASE_URL}"
}

resolve_officecli_path() {
  if [[ -n "${OFFICECLI_BIN:-}" && -x "${OFFICECLI_BIN}" ]]; then
    printf '%s\n' "${OFFICECLI_BIN}"
    return 0
  fi
  if command -v officecli >/dev/null 2>&1; then
    command -v officecli
    return 0
  fi
  return 1
}

run_officecli_no_preflight() {
  local officecli_bin="$1"
  shift
  OFFICECLI_SKIP_SKILL_PREFLIGHT=1 "${officecli_bin}" "$@"
}

run_config_status() {
  local officecli_bin="$1"
  run_officecli_no_preflight "${officecli_bin}" config status 2>/dev/null || true
}

check_generation_ready() {
  [[ "$1" == *"生成服务已配置：true"* ]]
}

check_license_ready() {
  [[ "$1" == *"额度校验已启用：true"* ]]
}

check_publish_ready() {
  [[ "$1" == *"在线预览发布已启用：true"* ]]
}

check_bridge_ready() {
  local officecli_bin="$1"
  shift || true
  if [[ $# -gt 0 && -n "${1:-}" ]]; then
    bash -lc "$1 --help" >/dev/null 2>&1
    return $?
  fi
  run_officecli_no_preflight "${officecli_bin}" agent-bridge --help >/dev/null 2>&1
}

check_cli_surface_ready() {
  local officecli_bin="$1"
  run_officecli_no_preflight "${officecli_bin}" --help >/dev/null 2>&1 || return 1
  run_officecli_no_preflight "${officecli_bin}" new --help >/dev/null 2>&1 || return 1
}

print_check_json() {
  local status="$1"
  local officecli_found="$2"
  local officecli_path="$3"
  local config_path="$4"
  local generation_ready="$5"
  local license_ready="$6"
  local publish_ready="$7"
  local bridge_ready="$8"
  local fixable="$9"
  shift 9
  local missing_items=("$@")

  printf '{'
  printf '"status":"%s",' "$(json_escape "$status")"
  printf '"officecli_found":%s,' "$officecli_found"
  printf '"officecli_path":"%s",' "$(json_escape "$officecli_path")"
  printf '"config_path":"%s",' "$(json_escape "$config_path")"
  printf '"generation_ready":%s,' "$generation_ready"
  printf '"license_ready":%s,' "$license_ready"
  printf '"publish_ready":%s,' "$publish_ready"
  printf '"bridge_ready":%s,' "$bridge_ready"
  printf '"fixable":%s,' "$fixable"
  printf '"missing_items":%s' "$(join_json_array "${missing_items[@]}")"
  printf '}\n'
}

prompt_value() {
  local prompt="$1"
  local default_value="${2:-}"
  local allow_empty="${3:-0}"
  local value=""

  if [[ -t 0 ]]; then
    if [[ -n "${default_value}" ]]; then
      printf '%s [%s]: ' "$prompt" "$default_value" >&2
    else
      printf '%s: ' "$prompt" >&2
    fi
    IFS= read -r value || true
    if [[ -z "${value}" ]]; then
      value="${default_value}"
    fi
    if [[ -z "${value}" && "${allow_empty}" != "1" ]]; then
      echo "missing required value for ${prompt}" >&2
      return 1
    fi
    printf '%s' "$value"
    return 0
  fi

  if [[ -n "${default_value}" || "${allow_empty}" == "1" ]]; then
    printf '%s' "$default_value"
    return 0
  fi

  echo "missing required value for ${prompt}" >&2
  return 1
}

install_officecli_binary() {
  local install_cmd="${OFFICECLI_INSTALL_COMMAND:-}"
  if [[ -n "${install_cmd}" ]]; then
    bash -lc "${install_cmd}"
    return 0
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "missing curl, unable to auto-install officecli" >&2
    return 1
  fi
  curl -fsSL "https://raw.githubusercontent.com/${DIST_REPO_DEFAULT}/main/scripts/install-officecli.sh" \
    | PREFIX="${HOME}/.local" BIN_DIR="${HOME}/.local/bin" INSTALL_DIR="${HOME}/.local/bin" DIST_REPO="${DIST_REPO_DEFAULT}" bash
}

refresh_officecli_binary() {
  install_officecli_binary
}

uninstall_officecli_binary() {
  local officecli_bin=""
  officecli_bin="$(resolve_officecli_path 2>/dev/null || true)"
  if [[ -z "${officecli_bin}" ]]; then
    rm -f "${HOME}/.local/bin/officecli"
    return 0
  fi

  rm -f "${officecli_bin}"
  if [[ "${officecli_bin}" != "${HOME}/.local/bin/officecli" ]]; then
    rm -f "${HOME}/.local/bin/officecli"
  fi
}

refresh_codex_officecli_skill() {
  local refresh_cmd="${OFFICECLI_REFRESH_SKILL_COMMAND:-}"
  if [[ -n "${refresh_cmd}" ]]; then
    bash -lc "${refresh_cmd}"
    return 0
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "missing curl, unable to auto-refresh officecli skill" >&2
    return 1
  fi
  curl -fsSL "https://raw.githubusercontent.com/${PUBLIC_SKILLS_REPO_DEFAULT}/${PUBLIC_SKILLS_BRANCH_DEFAULT}/scripts/install-skill.sh" \
    | AUTO_INSTALL_BINARY=0 bash -s -- officecli
}

run_set_generation() {
  local officecli_bin="$1"
  local base_url="$2"
  local api_key="$3"
  printf '%s\n%s\n' "$base_url" "$api_key" | run_officecli_no_preflight "${officecli_bin}" config set-generation >/dev/null
}

run_set_license() {
  local officecli_bin="$1"
  local api_key="$2"
  printf 'yes\n%s\n' "$api_key" | run_officecli_no_preflight "${officecli_bin}" config set-license >/dev/null
}

run_set_publish() {
  local officecli_bin="$1"
  local base_url="$2"
  local api_key="$3"
  printf 'yes\n%s\n%s\n' "$base_url" "$api_key" | run_officecli_no_preflight "${officecli_bin}" config set-publish >/dev/null
}

should_configure_publish() {
  truthy "${OFFICECLI_SKIP_PUBLISH_SETUP:-0}" && return 1
  return 0
}
