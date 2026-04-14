#!/usr/bin/env bash

set -euo pipefail

DIST_REPO="${DIST_REPO:-officecli/officecli-dist}"
VERSION="${VERSION:-stable}"
PREFIX="${PREFIX:-/usr/local}"
BIN_DIR="${BIN_DIR:-${PREFIX}/bin}"
INSTALL_DIR="${INSTALL_DIR:-${BIN_DIR}}"
LATEST_TAG="${LATEST_TAG:-latest}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

resolve_version() {
  case "${VERSION}" in
    stable|"")
      echo "stable"
      ;;
    latest)
      echo "latest"
      ;;
    *)
      echo "${VERSION#v}"
      ;;
  esac
}

archive_name_for_version() {
  local version="$1"
  local os="$2"
  local arch="$3"
  if [[ "${version}" == "latest" ]]; then
    echo "officecli_latest_${os}_${arch}.tar.gz"
    return
  fi
  echo "officecli_${version}_${os}_${arch}.tar.gz"
}

download() {
  local url="$1"
  local out="$2"
  curl -fsSL "${url}" -o "${out}"
}

resolve_latest_release_tag() {
  local api_url="https://api.github.com/repos/${DIST_REPO}/releases/latest"
  local response

  response="$(curl -fsSL "${api_url}" | tr -d '\n')"
  printf '%s' "${response}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

verify_checksum() {
  local archive_name="$1"
  local checksum_file="$2"
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$(dirname "${checksum_file}")" && grep " ${archive_name}\$" "$(basename "${checksum_file}")" | sha256sum -c -)
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    local expected actual
    expected="$(grep " ${archive_name}\$" "${checksum_file}" | awk '{print $1}')"
    actual="$(shasum -a 256 "$(dirname "${checksum_file}")/${archive_name}" | awk '{print $1}')"
    [[ -n "${expected}" && "${expected}" == "${actual}" ]] || {
      echo "checksum verification failed for ${archive_name}" >&2
      exit 1
    }
    return
  fi
  echo "missing sha256 verifier: need sha256sum or shasum" >&2
  exit 1
}

install_binary() {
  local extracted_dir="$1"
  local target_dir="$2"
  mkdir -p "${target_dir}"
  install -m 0755 "${extracted_dir}/officecli" "${target_dir}/officecli"
}

shell_profile_hint() {
  case "${SHELL:-}" in
    */zsh)
      printf 'To make it available in future zsh sessions, add this to ~/.zshrc:\n'
      printf '  export PATH="$HOME/.local/bin:$PATH"\n'
      ;;
    */bash)
      printf 'To make it available in future bash sessions, add this to ~/.bashrc:\n'
      printf '  export PATH="$HOME/.local/bin:$PATH"\n'
      ;;
    */fish)
      printf 'To make it available in future fish sessions, run:\n'
      printf '  fish_add_path $HOME/.local/bin\n'
      ;;
    *)
      printf 'Add ~/.local/bin to your shell startup file so officecli is available in new sessions.\n'
      ;;
  esac
}

need_cmd curl
need_cmd tar
need_cmd install

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(detect_arch)"

if [[ "${os}" != "linux" && "${os}" != "darwin" ]]; then
  echo "unsupported operating system: ${os}" >&2
  exit 1
fi

resolved_version="$(resolve_version)"
display_version="${resolved_version}"
if [[ "${resolved_version}" == "latest" ]]; then
  tag="${LATEST_TAG}"
  archive_name="$(archive_name_for_version latest "${os}" "${arch}")"
elif [[ "${resolved_version}" == "stable" ]]; then
  tag="$(resolve_latest_release_tag)"
  if [[ -z "${tag}" ]]; then
    echo "failed to resolve latest stable release tag for ${DIST_REPO}" >&2
    exit 1
  fi
  resolved_version="${tag#v}"
  display_version="${tag}"
  archive_name="$(archive_name_for_version "${resolved_version}" "${os}" "${arch}")"
else
  tag="v${resolved_version}"
  archive_name="$(archive_name_for_version "${resolved_version}" "${os}" "${arch}")"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

base_url="https://github.com/${DIST_REPO}/releases/download/${tag}"

if [[ "${resolved_version}" == "latest" ]]; then
  if ! download "${base_url}/${archive_name}" "${tmpdir}/${archive_name}" 2>/dev/null; then
    fallback_tag="$(resolve_latest_release_tag)"
    if [[ -z "${fallback_tag}" ]]; then
      echo "failed to resolve latest release tag for ${DIST_REPO}" >&2
      exit 1
    fi
    display_version="${fallback_tag}"
    fallback_version="${fallback_tag#v}"
    tag="${fallback_tag}"
    archive_name="$(archive_name_for_version "${fallback_version}" "${os}" "${arch}")"
    base_url="https://github.com/${DIST_REPO}/releases/download/${tag}"
    download "${base_url}/${archive_name}" "${tmpdir}/${archive_name}"
  fi
else
  download "${base_url}/${archive_name}" "${tmpdir}/${archive_name}"
fi

download "${base_url}/checksums.txt" "${tmpdir}/checksums.txt"
verify_checksum "${archive_name}" "${tmpdir}/checksums.txt"

tar -xzf "${tmpdir}/${archive_name}" -C "${tmpdir}"
install_binary "${tmpdir}" "${INSTALL_DIR}"

echo "installed officecli ${display_version} to ${INSTALL_DIR}/officecli"

if command -v officecli >/dev/null 2>&1; then
  echo "officecli is already available on PATH: $(command -v officecli)"
else
  case ":${PATH}:" in
    *":${INSTALL_DIR}:"*)
      echo "officecli was installed to ${INSTALL_DIR}, but this shell does not resolve it yet."
      ;;
    *)
      echo "officecli was installed to ${INSTALL_DIR}, but that directory is not currently on PATH."
      ;;
  esac
  echo "Use it in the current shell with:"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
  shell_profile_hint
fi
