#!/usr/bin/env bash

set -euo pipefail
trap 'exit 20' ERR

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=skills/officecli/env-common.sh
source "${SCRIPT_DIR}/env-common.sh"

refresh_codex_officecli_skill
refresh_officecli_binary
export PATH="${HOME}/.local/bin:${PATH}"
officecli_path="$(resolve_officecli_path)"

status_output="$(run_config_status "${officecli_path}")"
if ! check_generation_ready "${status_output}"; then
  gen_base_url="${OFFICECLI_SETUP_LLM_BASE_URL:-}"
  [[ -n "${gen_base_url}" ]] || gen_base_url="$(prompt_value 'Enter the generation service URL' '' 0)"
  gen_api_key="${OFFICECLI_SETUP_LLM_API_KEY:-}"
  [[ -n "${gen_api_key}" ]] || gen_api_key="$(prompt_value 'Enter the generation service credential' '' 0)"
  run_set_generation "${officecli_path}" "${gen_base_url}" "${gen_api_key}"
  status_output="$(run_config_status "${officecli_path}")"
fi

if ! check_license_ready "${status_output}"; then
  license_api_key="${OFFICECLI_SETUP_LICENSE_API_KEY:-}"
  if [[ -z "${license_api_key}" && -t 0 ]]; then
    license_api_key="$(prompt_value 'Enter the paid quota key (optional)' '' 1)"
  fi
  run_set_license "${officecli_path}" "${license_api_key}"
  status_output="$(run_config_status "${officecli_path}")"
fi

if should_configure_publish && ! check_publish_ready "${status_output}"; then
  publish_base_url="$(default_publish_base_url)"
  publish_base_url="$(prompt_value 'Enter the publishing service URL' "${publish_base_url}" 0)"
  publish_api_key="${OFFICECLI_SETUP_PUBLISH_API_KEY:-}"
  [[ -n "${publish_api_key}" ]] || publish_api_key="$(prompt_value 'Enter the publishing service credential (optional, built-in dynamic auth is used by default)' '' 1)"
  run_set_publish "${officecli_path}" "${publish_base_url}" "${publish_api_key}"
fi

exec "${SCRIPT_DIR}/check-officecli-env.sh"
