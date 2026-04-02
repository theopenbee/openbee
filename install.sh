#!/bin/sh
# OpenBee installer
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.sh | sh -s -- --version v1.0.0
#   curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.sh | sh -s -- --install-dir /custom/path

set -e

# ============================================================
# Configuration
# ============================================================
GITHUB_REPO="theopenbee/openbee"
GITHUB_BASE_URL="https://github.com/${GITHUB_REPO}"
GITHUB_API_URL="https://api.github.com/repos/${GITHUB_REPO}"

# ============================================================
# Defaults
# ============================================================
BINARY_NAME="openbee"
INSTALL_DIR="/usr/local/bin"
VERSION=""
FORCE=false
NO_VERIFY=false

# ============================================================
# Colors
# ============================================================
setup_colors() {
    if [ -t 1 ] && [ -n "$(tput colors 2>/dev/null)" ]; then
        RED="\033[0;31m"
        GREEN="\033[0;32m"
        YELLOW="\033[0;33m"
        BLUE="\033[0;34m"
        RESET="\033[0m"
    else
        RED=""
        GREEN=""
        YELLOW=""
        BLUE=""
        RESET=""
    fi
}

info()  { printf "${BLUE}[info]${RESET}  %s\n" "$1"; }
ok()    { printf "${GREEN}[ok]${RESET}    %s\n" "$1"; }
warn()  { printf "${YELLOW}[warn]${RESET}  %s\n" "$1"; }
error() { printf "${RED}[error]${RESET} %s\n" "$1" >&2; exit 1; }

# ============================================================
# Cleanup
# ============================================================
TMPDIR_INSTALL=""
cleanup() {
    if [ -n "$TMPDIR_INSTALL" ] && [ -d "$TMPDIR_INSTALL" ]; then
        rm -rf "$TMPDIR_INSTALL"
    fi
}
trap cleanup EXIT INT TERM

# ============================================================
# Dependencies
# ============================================================
check_command() {
    command -v "$1" >/dev/null 2>&1
}

detect_downloader() {
    if check_command curl; then
        DOWNLOADER="curl"
    elif check_command wget; then
        DOWNLOADER="wget"
    else
        error "curl or wget is required. Please install one of them first."
    fi
}

download() {
    url="$1"
    output="$2"
    if [ "$DOWNLOADER" = "curl" ]; then
        curl -fsSL -o "$output" "$url"
    else
        wget -qO "$output" "$url"
    fi
}

download_text() {
    url="$1"
    if [ "$DOWNLOADER" = "curl" ]; then
        curl -fsSL "$url"
    else
        wget -qO- "$url"
    fi
}

# ============================================================
# Platform detection
# ============================================================
detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      error "Unsupported OS: $OS (only linux and macOS are supported)" ;;
    esac

    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64)  ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *)             error "Unsupported architecture: $ARCH (only amd64 and arm64 are supported)" ;;
    esac

    info "Detected platform: ${OS}/${ARCH}"
}

# ============================================================
# Version detection
# ============================================================
fetch_latest_version() {
    info "Fetching latest version..."
    response=$(download_text "${GITHUB_API_URL}/releases/latest" 2>/dev/null) || \
        error "Failed to fetch latest version. Check your network connection or specify a version manually: --version v1.0.0"

    if check_command jq; then
        VERSION=$(printf '%s' "$response" | jq -r '.tag_name // empty')
    else
        VERSION=$(printf '%s' "$response" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    fi

    VERSION=$(echo "$VERSION" | tr -d '[:space:]')

    if [ -z "$VERSION" ]; then
        error "Retrieved version is empty. Please specify a version manually: --version v1.0.0"
    fi

    # Ensure version has the v prefix
    case "$VERSION" in
        v*) ;;
        *)  VERSION="v${VERSION}" ;;
    esac
}

# ============================================================
# Checksum verification
# ============================================================
verify_checksum() {
    archive_file="$1"
    archive_name="$2"
    checksum_file="$3"

    expected=$(grep "${archive_name}" "$checksum_file" | awk '{print $1}')
    if [ -z "$expected" ]; then
        warn "No checksum found for ${archive_name} in checksums.txt, skipping"
        return 0
    fi

    if check_command sha256sum; then
        actual=$(sha256sum "$archive_file" | awk '{print $1}')
    elif check_command shasum; then
        actual=$(shasum -a 256 "$archive_file" | awk '{print $1}')
    else
        warn "sha256sum/shasum not found, skipping checksum verification"
        return 0
    fi

    if [ "$actual" != "$expected" ]; then
        error "SHA256 checksum mismatch!\n  Expected: ${expected}\n  Actual:   ${actual}\nFile may be corrupted, please retry."
    fi

    ok "SHA256 checksum verified"
}

# ============================================================
# Installation
# ============================================================
install_binary() {
    TMPDIR_INSTALL="$(mktemp -d)"

    VERSION_NUM="${VERSION#v}"
    ARCHIVE_NAME="${BINARY_NAME}-${VERSION_NUM}-${OS}-${ARCH}.tar.gz"
    ARCHIVE_URL="${GITHUB_BASE_URL}/releases/download/${VERSION}/${ARCHIVE_NAME}"
    CHECKSUM_URL="${GITHUB_BASE_URL}/releases/download/${VERSION}/checksums.txt"

    info "Downloading ${ARCHIVE_NAME}..."
    download "$ARCHIVE_URL" "${TMPDIR_INSTALL}/${ARCHIVE_NAME}" || \
        error "Download failed: ${ARCHIVE_URL}\nPlease verify that version ${VERSION} exists."

    if [ "$NO_VERIFY" = false ]; then
        info "Verifying checksum..."
        download "${CHECKSUM_URL}" "${TMPDIR_INSTALL}/checksums.txt" || \
            warn "Could not download checksums.txt, skipping verification"
        if [ -f "${TMPDIR_INSTALL}/checksums.txt" ]; then
            verify_checksum "${TMPDIR_INSTALL}/${ARCHIVE_NAME}" "${ARCHIVE_NAME}" "${TMPDIR_INSTALL}/checksums.txt"
        fi
    else
        warn "Skipping checksum verification (--no-verify)"
    fi

    info "Extracting..."
    tar -xzf "${TMPDIR_INSTALL}/${ARCHIVE_NAME}" -C "${TMPDIR_INSTALL}"

    if [ ! -f "${TMPDIR_INSTALL}/${BINARY_NAME}" ]; then
        error "${BINARY_NAME} binary not found after extraction"
    fi

    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMPDIR_INSTALL}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    elif check_command sudo; then
        info "Requesting sudo to install to ${INSTALL_DIR}"
        sudo mv "${TMPDIR_INSTALL}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        warn "No write permission to ${INSTALL_DIR} and sudo is not available."
        warn "Falling back to ~/.local/bin"
        INSTALL_DIR="${HOME}/.local/bin"
        mkdir -p "$INSTALL_DIR"
        mv "${TMPDIR_INSTALL}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    ok "${BINARY_NAME} ${VERSION} installed to ${INSTALL_DIR}/${BINARY_NAME}"
}

# ============================================================
# Post-install check
# ============================================================
post_install_check() {
    if ! check_command "${BINARY_NAME}"; then
        warn "${INSTALL_DIR} is not in PATH. Add it manually:"
        echo ""
        echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        echo ""
        echo "  Add the above line to ~/.bashrc or ~/.zshrc to make it permanent."
        echo ""
    else
        installed_version=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | awk '{print $2}' || echo "unknown")
        ok "Verified: ${installed_version}"
    fi
}

# ============================================================
# Existing install check
# ============================================================
check_existing() {
    if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ] && [ "$FORCE" = false ]; then
        existing_version=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | awk '{print $2}' || echo "")
        if [ -n "$existing_version" ]; then
            info "Found existing installation: ${existing_version}"
            info "Use --force to reinstall"
            exit 0
        fi
    fi
}

# ============================================================
# Argument parsing
# ============================================================
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --version|-v)
                VERSION="$2"
                shift 2
                ;;
            --install-dir|-d)
                INSTALL_DIR="$2"
                shift 2
                ;;
            --force|-f)
                FORCE=true
                shift
                ;;
            --no-verify)
                NO_VERIFY=true
                shift
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            *)
                error "Unknown option: $1 (use --help for usage)"
                ;;
        esac
    done
}

usage() {
    cat <<EOF
OpenBee installer

Usage:
  curl -fsSL <url>/install.sh | sh
  curl -fsSL <url>/install.sh | sh -s -- [options]

Options:
  --version, -v <version>     Specify version (e.g. v1.0.0), default: latest
  --install-dir, -d <path>    Installation directory, default: /usr/local/bin
  --force, -f                 Force reinstall
  --no-verify                 Skip SHA256 checksum verification
  --help, -h                  Show this help

Examples:
  # Install latest version
  curl -fsSL <url>/install.sh | sh

  # Install specific version
  curl -fsSL <url>/install.sh | sh -s -- --version v1.0.0

  # Install to custom directory
  curl -fsSL <url>/install.sh | sh -s -- --install-dir ~/.local/bin
EOF
}

# ============================================================
# Main
# ============================================================
main() {
    setup_colors
    parse_args "$@"

    echo ""
    info "OpenBee Installer"
    echo ""

    detect_downloader
    detect_platform

    if [ -z "$VERSION" ]; then
        fetch_latest_version
    fi
    info "Target version: ${VERSION}"

    check_existing
    install_binary
    post_install_check

    echo ""
    ok "Installation complete! Run 'openbee --help' to get started."
    echo ""
}

main "$@"
