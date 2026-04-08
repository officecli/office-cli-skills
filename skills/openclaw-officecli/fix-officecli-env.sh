#!/usr/bin/env bash

set -euo pipefail
trap 'exit 20' ERR

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=skills/openclaw-officecli/env-common.sh
source "${SCRIPT_DIR}/env-common.sh"

refresh_openclaw_officecli_skill
refresh_officecli_binary
export PATH="${HOME}/.local/bin:${PATH}"
officecli_path="$(resolve_officecli_path)"

status_output="$(run_config_status "${officecli_path}")"
if ! check_generation_ready "${status_output}"; then
  gen_base_url="${OFFICECLI_SETUP_LLM_BASE_URL:-}"
  [[ -n "${gen_base_url}" ]] || gen_base_url="$(prompt_value '请输入生成服务地址' '' 0)"
  gen_api_key="${OFFICECLI_SETUP_LLM_API_KEY:-}"
  [[ -n "${gen_api_key}" ]] || gen_api_key="$(prompt_value '请输入生成服务访问凭证' '' 0)"
  run_set_generation "${officecli_path}" "${gen_base_url}" "${gen_api_key}"
  status_output="$(run_config_status "${officecli_path}")"
fi

if ! check_license_ready "${status_output}"; then
  license_api_key="${OFFICECLI_SETUP_LICENSE_API_KEY:-}"
  if [[ -z "${license_api_key}" && -t 0 ]]; then
    license_api_key="$(prompt_value '请输入付费额度密钥（可留空）' '' 1)"
  fi
  run_set_license "${officecli_path}" "${license_api_key}"
  status_output="$(run_config_status "${officecli_path}")"
fi

if should_configure_publish && ! check_publish_ready "${status_output}"; then
  publish_base_url="${OFFICECLI_SETUP_PUBLISH_BASE_URL:-}"
  [[ -n "${publish_base_url}" ]] || publish_base_url="$(prompt_value '请输入发布服务地址' '' 0)"
  publish_api_key="${OFFICECLI_SETUP_PUBLISH_API_KEY:-}"
  [[ -n "${publish_api_key}" ]] || publish_api_key="$(prompt_value '请输入发布服务访问凭证' '' 0)"
  run_set_publish "${officecli_path}" "${publish_base_url}" "${publish_api_key}"
fi

config_file="${OPENCLAW_SKILL_CONFIG:-${SCRIPT_DIR}/config.yaml}"
mkdir -p "$(dirname "${config_file}")"
default_mode="fast"
default_output_format="json"
default_lang="zh-CN"
default_publish="false"
if [[ -f "${SCRIPT_DIR}/config.example.yaml" ]]; then
  default_mode="$(sed -n 's/^default_mode:[[:space:]]*"\(.*\)"/\1/p' "${SCRIPT_DIR}/config.example.yaml" | head -n1)"
  default_output_format="$(sed -n 's/^default_output_format:[[:space:]]*"\(.*\)"/\1/p' "${SCRIPT_DIR}/config.example.yaml" | head -n1)"
  default_lang="$(sed -n 's/^default_lang:[[:space:]]*"\(.*\)"/\1/p' "${SCRIPT_DIR}/config.example.yaml" | head -n1)"
  default_publish="$(sed -n 's/^default_publish:[[:space:]]*\(.*\)$/\1/p' "${SCRIPT_DIR}/config.example.yaml" | head -n1)"
fi
cat > "${config_file}" <<CFG
office_cli_path: "${officecli_path}"
agent_bridge_command: "${officecli_path} agent-bridge"
default_mode: "${default_mode}"
default_output_format: "${default_output_format}"
default_lang: "${default_lang}"
default_publish: ${default_publish}
CFG

exec "${SCRIPT_DIR}/check-officecli-env.sh"
