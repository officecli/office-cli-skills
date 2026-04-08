#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=skills/openclaw-officecli/env-common.sh
source "${SCRIPT_DIR}/env-common.sh"

config_file="${OPENCLAW_SKILL_CONFIG:-${SCRIPT_DIR}/config.yaml}"
officecli_path_override=""
agent_bridge_command=""
if [[ -f "${config_file}" ]]; then
  officecli_path_override="$(sed -n 's/^office_cli_path:[[:space:]]*"\(.*\)"/\1/p' "${config_file}" | head -n1)"
  agent_bridge_command="$(sed -n 's/^agent_bridge_command:[[:space:]]*"\(.*\)"/\1/p' "${config_file}" | head -n1)"
fi

missing_items=()
config_path="${OFFICE_CLI_CONFIG:-${HOME}/.config/officecli/config.json}"
officecli_found=false
officecli_path=""
generation_ready=false
license_ready=false
publish_ready=false
cli_surface_ready=false
bridge_ready=false
fixable=true
status="repairable"

if [[ -n "${officecli_path_override}" && -x "${officecli_path_override}" ]]; then
  officecli_path="${officecli_path_override}"
  officecli_found=true
elif officecli_path="$(resolve_officecli_path 2>/dev/null)"; then
  officecli_found=true
else
  missing_items+=("officecli_binary")
fi

if [[ "${officecli_found}" == true ]]; then
  check_cli_surface_ready "${officecli_path}" && cli_surface_ready=true
  status_output="$(run_config_status "${officecli_path}")"
  check_generation_ready "${status_output}" && generation_ready=true
  check_license_ready "${status_output}" && license_ready=true
  check_publish_ready "${status_output}" && publish_ready=true
  if [[ -n "${agent_bridge_command}" ]]; then
    check_bridge_ready "${officecli_path}" "${agent_bridge_command}" && bridge_ready=true
  else
    check_bridge_ready "${officecli_path}" && bridge_ready=true
  fi
fi

if [[ "${officecli_found}" != true ]]; then
  status="repairable"
else
  [[ "${cli_surface_ready}" == true ]] || missing_items+=("cli_surface")
  [[ "${generation_ready}" == true ]] || missing_items+=("generation_config")
  [[ "${license_ready}" == true ]] || missing_items+=("license_config")
  [[ "${bridge_ready}" == true ]] || missing_items+=("agent_bridge")
  if should_configure_publish && [[ "${publish_ready}" != true ]]; then
    missing_items+=("publish_config")
  fi
  if [[ ${#missing_items[@]} -eq 0 ]]; then
    status="ready"
  else
    status="repairable"
  fi
fi

if [[ "${officecli_found}" != true ]] && [[ -z "${OFFICECLI_INSTALL_COMMAND:-}" ]] && ! command -v curl >/dev/null 2>&1; then
  fixable=false
  status="blocked"
fi

print_check_json "${status}" "${officecli_found}" "${officecli_path}" "${config_path}" "${generation_ready}" "${license_ready}" "${publish_ready}" "${bridge_ready}" "${fixable}" "${missing_items[@]}"

case "${status}" in
  ready) exit 0 ;;
  repairable) exit 10 ;;
  *) exit 20 ;;
esac
