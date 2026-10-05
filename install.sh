#!/usr/bin/env bash
set -euo pipefail

REPO="rorkai/App-Store-Connect-CLI"
BIN_NAME="asc"
DEFAULT_INSTALL_DIR="/usr/local/bin"
if [ -n "${HOME:-}" ]; then
  DEFAULT_INSTALL_DIR="${HOME}/.local/bin"
fi
INSTALL_DIR="${INSTALL_DIR:-${DEFAULT_INSTALL_DIR}}"
DOWNLOAD_MAX_ATTEMPTS=3
DOWNLOAD_RETRY_DELAY_SECONDS=1

curl_with_retry() {
  local attempt=1
  while true; do
    if curl -fsSL "$@"; then
      return 0
    fi
    if [ "${attempt}" -ge "${DOWNLOAD_MAX_ATTEMPTS}" ]; then
      return 1
    fi
    attempt=$((attempt + 1))
    echo "Download failed; retrying (${attempt}/${DOWNLOAD_MAX_ATTEMPTS})..." >&2
    sleep "${DOWNLOAD_RETRY_DELAY_SECONDS}"
  done
}

OS="$(uname -s)"
ARCH="$(uname -m)"

case "${OS}" in
  Darwin) OS="macOS" ;;
  Linux) OS="linux" ;;
  *)
    echo "Unsupported OS: ${OS}"
    exit 1
    ;;
esac

case "${ARCH}" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: ${ARCH}"
    exit 1
    ;;
esac

LATEST_URL="$(curl_with_retry -o /dev/null -w "%{url_effective}" "https://github.com/${REPO}/releases/latest")"
VERSION="${LATEST_URL##*/}"
if [ -z "${VERSION}" ] || [ "${VERSION}" = "latest" ]; then
  echo "Could not determine latest version."
  exit 1
fi

ASSET="${BIN_NAME}_${VERSION}_${OS}_${ARCH}"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
BIN_URL="${BASE_URL}/${ASSET}"
CHECKSUMS_ASSET="${BIN_NAME}_${VERSION}_checksums.txt"
CHECKSUMS_URL="${BASE_URL}/${CHECKSUMS_ASSET}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

# Checksum verification is mandatory. When it cannot run, abort unless the
# caller explicitly opts out with ASC_INSTALL_INSECURE=1.
verification_unavailable() {
  local reason="$1"
  if [ "${ASC_INSTALL_INSECURE:-}" = "1" ]; then
    echo "!!! WARNING: ${reason}" >&2
    echo "!!! ASC_INSTALL_INSECURE=1 is set; installing WITHOUT checksum verification." >&2
    echo "!!! The downloaded binary has NOT been verified against the release checksums." >&2
    return 0
  fi
  echo "Error: ${reason}" >&2
  echo "Refusing to install without SHA-256 checksum verification." >&2
  echo "If you understand the risk and must install anyway, re-run with ASC_INSTALL_INSECURE=1." >&2
  exit 1
}

echo "Downloading ${ASSET}..."
curl_with_retry "${BIN_URL}" -o "${TMP_DIR}/${ASSET}"

if ! curl_with_retry "${CHECKSUMS_URL}" -o "${TMP_DIR}/checksums.txt"; then
  verification_unavailable "Could not download ${CHECKSUMS_ASSET} from ${CHECKSUMS_URL}."
elif ! command -v shasum >/dev/null 2>&1 && ! command -v sha256sum >/dev/null 2>&1; then
  verification_unavailable "No checksum tool (shasum/sha256sum) available."
else
  EXPECTED="$(awk -v asset="${ASSET}" '$2 == asset || $2 == "*" asset { print $1 }' "${TMP_DIR}/checksums.txt")"
  if [ -z "${EXPECTED}" ]; then
    verification_unavailable "Asset ${ASSET} not found in ${CHECKSUMS_ASSET}."
  else
    if command -v shasum >/dev/null 2>&1; then
      ACTUAL="$(shasum -a 256 "${TMP_DIR}/${ASSET}" | awk '{print $1}')"
    else
      ACTUAL="$(sha256sum "${TMP_DIR}/${ASSET}" | awk '{print $1}')"
    fi
    if [ "${EXPECTED}" != "${ACTUAL}" ]; then
      echo "Error: Checksum verification failed for ${ASSET}." >&2
      echo "Expected: ${EXPECTED}" >&2
      echo "Actual:   ${ACTUAL}" >&2
      exit 1
    fi
    echo "Checksum verified."
  fi
fi

if ! mkdir -p "${INSTALL_DIR}" 2>/dev/null; then
  if command -v sudo >/dev/null 2>&1; then
    sudo mkdir -p "${INSTALL_DIR}"
  else
    echo "Cannot create ${INSTALL_DIR}; try running with sudo or set INSTALL_DIR."
    exit 1
  fi
fi

if [ -w "${INSTALL_DIR}" ]; then
  install -m 755 "${TMP_DIR}/${ASSET}" "${INSTALL_DIR}/${BIN_NAME}"
else
  if command -v sudo >/dev/null 2>&1; then
    sudo install -m 755 "${TMP_DIR}/${ASSET}" "${INSTALL_DIR}/${BIN_NAME}"
  else
    echo "Cannot write to ${INSTALL_DIR}; try running with sudo or set INSTALL_DIR."
    exit 1
  fi
fi

echo "Installed ${BIN_NAME} to ${INSTALL_DIR}/${BIN_NAME}"
echo "Run: ${BIN_NAME} --help"
if [[ ":${PATH}:" != *":${INSTALL_DIR}:"* ]]; then
  echo "Note: ${INSTALL_DIR} is not in your PATH."
  echo "Add it to your shell profile, e.g.:"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
fi
