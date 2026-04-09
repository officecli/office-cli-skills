#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OFFICECLI_SKILL_DIR="${REPO_ROOT}/skills/officecli"
OPENCLAW_SKILL_DIR="${REPO_ROOT}/skills/openclaw-officecli"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "${TMP_ROOT}"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local got="$1"
  local want="$2"
  local msg="$3"
  [[ "${got}" == "${want}" ]] || fail "${msg}: got=${got} want=${want}"
}

assert_contains() {
  local file="$1"
  local needle="$2"
  grep -q --fixed-strings "${needle}" "${file}" || fail "expected ${file} to contain ${needle}"
}

make_fake_officecli() {
  local dest="$1"
  cat > "${dest}" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
STATE_DIR="${OFFICECLI_FAKE_STATE_DIR:?missing OFFICECLI_FAKE_STATE_DIR}"
STATE_FILE="${STATE_DIR}/state.sh"
mkdir -p "${STATE_DIR}"
if [[ ! -f "${STATE_FILE}" ]]; then
  cat > "${STATE_FILE}" <<'STATE'
GENERATION_READY=0
LICENSE_READY=0
PUBLISH_READY=0
BRIDGE_READY=1
STATE
fi
# shellcheck disable=SC1090
source "${STATE_FILE}"
save_state() {
  cat > "${STATE_FILE}" <<STATE
GENERATION_READY=${GENERATION_READY}
LICENSE_READY=${LICENSE_READY}
PUBLISH_READY=${PUBLISH_READY}
BRIDGE_READY=${BRIDGE_READY}
STATE
}
case "${1:-}" in
  --version)
    echo "officecli test-1.0.0"
    ;;
  --help)
    echo "officecli help"
    ;;
  new)
    shift
    case "${1:-}" in
      pptx)
        shift || true
        if [[ "${1:-}" == "--help" ]]; then
          echo "officecli new pptx help"
        else
          exit 1
        fi
        ;;
      *)
        exit 1
        ;;
    esac
    ;;
  config)
    shift
    case "${1:-}" in
      status)
        echo "配置文件路径：${OFFICE_CLI_CONFIG:-${STATE_DIR}/config.json}"
        echo "生成服务已配置：$([[ ${GENERATION_READY} -eq 1 ]] && echo true || echo false)"
        echo "额度校验已启用：$([[ ${LICENSE_READY} -eq 1 ]] && echo true || echo false)"
        echo "付费额度密钥已配置：false"
        echo "在线预览发布已启用：$([[ ${PUBLISH_READY} -eq 1 ]] && echo true || echo false)"
        ;;
      set-generation)
        read -r _base_url
        read -r _api_key
        GENERATION_READY=1
        save_state
        ;;
      set-license)
        read -r enabled
        read -r _api_key
        if [[ "${enabled}" == "yes" ]]; then
          LICENSE_READY=1
        else
          LICENSE_READY=0
        fi
        save_state
        ;;
      set-publish)
        read -r enabled
        read -r _base_url
        read -r _api_key
        if [[ "${enabled}" == "yes" ]]; then
          PUBLISH_READY=1
        else
          PUBLISH_READY=0
        fi
        save_state
        ;;
      *)
        exit 1
        ;;
    esac
    ;;
  agent-bridge)
    if [[ "${2:-}" == "--help" && ${BRIDGE_READY} -eq 1 ]]; then
      echo "agent-bridge help"
    else
      exit 1
    fi
    ;;
  *)
    exit 1
    ;;
esac
SCRIPT
  chmod +x "${dest}"
}

run_check() {
  local script="$1"
  local out="$2"
  shift 2
  set +e
  "$script" "$@" >"${out}" 2>&1
  local code=$?
  set -e
  return "${code}"
}

# case 1: officecli skill now requires publish by default
case1_dir="${TMP_ROOT}/case1"
mkdir -p "${case1_dir}/bin" "${case1_dir}/state"
make_fake_officecli "${case1_dir}/bin/officecli"
cat > "${case1_dir}/state/state.sh" <<'STATE'
GENERATION_READY=1
LICENSE_READY=1
PUBLISH_READY=0
BRIDGE_READY=1
STATE
PATH="${case1_dir}/bin:${PATH}" OFFICECLI_FAKE_STATE_DIR="${case1_dir}/state" run_check "${OFFICECLI_SKILL_DIR}/check-officecli-env.sh" "${case1_dir}/out.json" || code=$?
code=${code:-0}
assert_eq "${code}" "10" "officecli check missing publish exit code"
assert_contains "${case1_dir}/out.json" '"status":"repairable"'
assert_contains "${case1_dir}/out.json" '"publish_ready":false'
assert_contains "${case1_dir}/out.json" 'publish_config'
unset code

# case 2: missing binary is repairable
case2_dir="${TMP_ROOT}/case2"
mkdir -p "${case2_dir}"
PATH="/usr/bin:/bin" run_check "${OFFICECLI_SKILL_DIR}/check-officecli-env.sh" "${case2_dir}/out.json" || code=$?
code=${code:-0}
assert_eq "${code}" "10" "officecli check missing binary exit code"
assert_contains "${case2_dir}/out.json" 'officecli_binary'
unset code

# case 3: fix installs and configures officecli skill
case3_dir="${TMP_ROOT}/case3"
mkdir -p "${case3_dir}/home/.local/bin" "${case3_dir}/state" "${case3_dir}/install"
make_fake_officecli "${case3_dir}/install/officecli"
INSTALL_CMD="mkdir -p '${case3_dir}/home/.local/bin' && cp '${case3_dir}/install/officecli' '${case3_dir}/home/.local/bin/officecli' && chmod +x '${case3_dir}/home/.local/bin/officecli'"
set +e
HOME="${case3_dir}/home" \
PATH="/usr/bin:/bin" \
OFFICECLI_INSTALL_COMMAND="${INSTALL_CMD}" \
OFFICECLI_FAKE_STATE_DIR="${case3_dir}/state" \
OFFICECLI_SETUP_LLM_BASE_URL="https://example.com/v1" \
OFFICECLI_SETUP_LLM_API_KEY="sk-test" \
OFFICECLI_SETUP_LICENSE_API_KEY="" \
OFFICECLI_SETUP_PUBLISH_BASE_URL="https://claudeoffice.com" \
"${OFFICECLI_SKILL_DIR}/fix-officecli-env.sh" >"${case3_dir}/out.json" 2>&1
code=$?
set -e
assert_eq "${code}" "0" "officecli fix exit code"
assert_contains "${case3_dir}/out.json" '"status":"ready"'
# shellcheck disable=SC1090
source "${case3_dir}/state/state.sh"
assert_eq "${GENERATION_READY}" "1" "generation configured"
assert_eq "${LICENSE_READY}" "1" "license configured"
assert_eq "${PUBLISH_READY}" "1" "publish configured"

# case 4: officecli check fails when CLI help surface is broken
case4_dir="${TMP_ROOT}/case4"
mkdir -p "${case4_dir}/bin" "${case4_dir}/state"
cat > "${case4_dir}/bin/officecli" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  config)
    shift
    if [[ "${1:-}" == "status" ]]; then
      echo "生成服务已配置：true"
      echo "额度校验已启用：true"
      echo "在线预览发布已启用：false"
      exit 0
    fi
    ;;
  --help)
    exit 1
    ;;
  new)
    exit 1
    ;;
esac
exit 1
SCRIPT
chmod +x "${case4_dir}/bin/officecli"
PATH="${case4_dir}/bin:${PATH}" run_check "${OFFICECLI_SKILL_DIR}/check-officecli-env.sh" "${case4_dir}/out.json" || code=$?
code=${code:-0}
assert_eq "${code}" "10" "officecli check broken cli surface exit code"
assert_contains "${case4_dir}/out.json" 'cli_surface'
unset code

# case 5: openclaw check requires bridge
case5_dir="${TMP_ROOT}/case5"
mkdir -p "${case5_dir}/bin" "${case5_dir}/state"
make_fake_officecli "${case5_dir}/bin/officecli"
cat > "${case5_dir}/state/state.sh" <<'STATE'
GENERATION_READY=1
LICENSE_READY=1
PUBLISH_READY=0
BRIDGE_READY=0
STATE
PATH="${case5_dir}/bin:${PATH}" OFFICECLI_FAKE_STATE_DIR="${case5_dir}/state" run_check "${OPENCLAW_SKILL_DIR}/check-officecli-env.sh" "${case5_dir}/out.json" || code=$?
code=${code:-0}
assert_eq "${code}" "10" "openclaw check missing bridge exit code"
assert_contains "${case5_dir}/out.json" 'agent_bridge'
unset code

# case 6: openclaw fix writes config.yaml and returns ready
case6_dir="${TMP_ROOT}/case6"
mkdir -p "${case6_dir}/home/.local/bin" "${case6_dir}/state" "${case6_dir}/install" "${case6_dir}/skill"
make_fake_officecli "${case6_dir}/install/officecli"
INSTALL_CMD="mkdir -p '${case6_dir}/home/.local/bin' && cp '${case6_dir}/install/officecli' '${case6_dir}/home/.local/bin/officecli' && chmod +x '${case6_dir}/home/.local/bin/officecli'"
REFRESH_CMD="mkdir -p '${case6_dir}/home/.openai/skills' && true"
cp "${OPENCLAW_SKILL_DIR}/config.example.yaml" "${case6_dir}/skill/config.yaml"
set +e
HOME="${case6_dir}/home" \
PATH="/usr/bin:/bin" \
OPENCLAW_SKILL_CONFIG="${case6_dir}/skill/config.yaml" \
OFFICECLI_INSTALL_COMMAND="${INSTALL_CMD}" \
OFFICECLI_REFRESH_SKILL_COMMAND="${REFRESH_CMD}" \
OFFICECLI_FAKE_STATE_DIR="${case6_dir}/state" \
OFFICECLI_SETUP_LLM_BASE_URL="https://example.com/v1" \
OFFICECLI_SETUP_LLM_API_KEY="sk-test" \
OFFICECLI_SETUP_LICENSE_API_KEY="" \
OFFICECLI_SETUP_PUBLISH_BASE_URL="https://claudeoffice.com" \
"${OPENCLAW_SKILL_DIR}/fix-officecli-env.sh" >"${case6_dir}/out.json" 2>&1
code=$?
set -e
assert_eq "${code}" "0" "openclaw fix exit code"
assert_contains "${case6_dir}/out.json" '"status":"ready"'
assert_contains "${case6_dir}/skill/config.yaml" 'office_cli_path: "'
assert_contains "${case6_dir}/skill/config.yaml" 'agent_bridge_command: "'

echo "skill environment tests passed"
