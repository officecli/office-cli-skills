#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=skills/officecli/env-common.sh
source "${SCRIPT_DIR}/env-common.sh"

config_path="${OFFICE_CLI_CONFIG:-${HOME}/.config/officecli/config.json}"
officecli_found=false
officecli_path=""
generation_ready=false
license_ready=false
publish_ready=false
bridge_ready=true
fixable=true

refresh_status_flags() {
  local status_output="$1"
  generation_ready=false
  license_ready=false
  publish_ready=false
  check_generation_ready "${status_output}" && generation_ready=true
  check_license_ready "${status_output}" && license_ready=true
  check_publish_ready "${status_output}" && publish_ready=true
  return 0
}

fail_fix() {
  local reason="$1"
  shift
  print_fix_failure_json "${officecli_found}" "${officecli_path}" "${config_path}" "${generation_ready}" "${license_ready}" "${publish_ready}" "${bridge_ready}" "${fixable}" "${reason}" "$@"
  exit 20
}

if ! refresh_codex_officecli_skill; then
  if [[ -z "${OFFICECLI_REFRESH_SKILL_COMMAND:-}" ]] && ! command -v curl >/dev/null 2>&1; then
    fixable=false
  fi
  fail_fix "failed to refresh officecli skill bundle"
fi

export PATH="${HOME}/.local/bin:${PATH}"
if ! officecli_path="$(prepare_officecli_binary)"; then
  if [[ -z "${OFFICECLI_INSTALL_COMMAND:-}" ]] && ! command -v curl >/dev/null 2>&1; then
    fixable=false
  fi
  fail_fix "failed to install or refresh officecli binary" "officecli_binary"
fi
officecli_found=true

status_output="$(run_config_status "${officecli_path}")"
refresh_status_flags "${status_output}"

if should_configure_generation && [[ "${generation_ready}" != true ]]; then
  gen_base_url="${OFFICECLI_SETUP_LLM_BASE_URL:-}"
  if [[ -z "${gen_base_url}" ]]; then
    if ! gen_base_url="$(prompt_value 'Enter the generation service URL' '' 0)"; then
      fail_fix "missing required value for Enter the generation service URL" "generation_config"
    fi
  fi
  gen_api_key="${OFFICECLI_SETUP_LLM_API_KEY:-}"
  if [[ -z "${gen_api_key}" ]]; then
    if ! gen_api_key="$(prompt_value 'Enter the generation service credential' '' 0)"; then
      fail_fix "missing required value for Enter the generation service credential" "generation_config"
    fi
  fi
  if ! run_set_generation "${officecli_path}" "${gen_base_url}" "${gen_api_key}"; then
    fail_fix "failed to update generation service config" "generation_config"
  fi
  status_output="$(run_config_status "${officecli_path}")"
  refresh_status_flags "${status_output}"
fi

if [[ "${license_ready}" != true ]]; then
  license_api_key="${OFFICECLI_SETUP_LICENSE_API_KEY:-}"
  if should_require_license_api_key && [[ -z "${license_api_key}" ]]; then
    if ! license_api_key="$(prompt_value 'Enter the platform access API key' '' 0)"; then
      fail_fix "missing required value for Enter the platform access API key" "license_config"
    fi
  elif [[ -z "${license_api_key}" && -t 0 ]]; then
    if ! license_api_key="$(prompt_value 'Enter the paid quota key (optional)' '' 1)"; then
      fail_fix "failed to read the paid quota key" "license_config"
    fi
  fi
  if ! run_set_license "${officecli_path}" "${license_api_key}"; then
    fail_fix "failed to update access config" "license_config"
  fi
  status_output="$(run_config_status "${officecli_path}")"
  refresh_status_flags "${status_output}"
fi

if should_configure_publish && [[ "${publish_ready}" != true ]]; then
  publish_base_url="$(default_publish_base_url)"
  if ! publish_base_url="$(prompt_value 'Enter the publishing service URL' "${publish_base_url}" 0)"; then
    fail_fix "missing required value for Enter the publishing service URL" "publish_config"
  fi
  publish_api_key="${OFFICECLI_SETUP_PUBLISH_API_KEY:-}"
  if [[ -z "${publish_api_key}" ]]; then
    if ! publish_api_key="$(prompt_value 'Enter the publishing service credential (optional, built-in dynamic auth is used by default)' '' 1)"; then
      fail_fix "failed to read the publishing service credential" "publish_config"
    fi
  fi
  if ! run_set_publish "${officecli_path}" "${publish_base_url}" "${publish_api_key}"; then
    fail_fix "failed to update online preview publishing config" "publish_config"
  fi
fi

exec "${SCRIPT_DIR}/check-officecli-env.sh"
