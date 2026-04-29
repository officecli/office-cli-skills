#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=skills/officecli/env-common.sh
source "${SCRIPT_DIR}/env-common.sh"

missing_items=()
config_path="${OFFICE_CLI_CONFIG:-${HOME}/.config/officecli/config.json}"
officecli_found=false
officecli_path=""
generation_ready=false
license_ready=false
publish_ready=false
cli_surface_ready=false
bridge_ready=true
fixable=true
status="repairable"

if officecli_path="$(resolve_officecli_path 2>/dev/null)"; then
  officecli_found=true
  check_cli_surface_ready "${officecli_path}" && cli_surface_ready=true
  status_output="$(run_config_status "${officecli_path}")"
  check_generation_ready "${status_output}" && generation_ready=true
  check_license_ready "${status_output}" && license_ready=true
  check_publish_ready "${status_output}" && publish_ready=true
else
  missing_items+=("officecli_binary")
fi

if [[ "${officecli_found}" != true ]]; then
  status="repairable"
else
  [[ "${cli_surface_ready}" == true ]] || missing_items+=("cli_surface")
  if should_configure_generation && [[ "${generation_ready}" != true ]]; then
    missing_items+=("generation_config")
  fi
  [[ "${license_ready}" == true ]] || missing_items+=("license_config")
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

print_check_json "${status}" "${officecli_found}" "${officecli_path}" "${config_path}" "${generation_ready}" "${license_ready}" "${publish_ready}" "${bridge_ready}" "${fixable}" "${missing_items[@]-}"

case "${status}" in
  ready) exit 0 ;;
  repairable) exit 10 ;;
  *) exit 20 ;;
esac
