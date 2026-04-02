#!/bin/sh
# OpenBee 一键安装脚本（中国大陆加速版）
# 用法:
#   curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.zh.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.zh.sh | sh -s -- --version v1.0.0
#   curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.zh.sh | sh -s -- --install-dir /custom/path
#   curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.zh.sh | sh -s -- --cdn-url https://my-cdn.example.com

set -e

# ============================================================
# CDN 配置 — 修改此处指向阿里云（或其他国内）CDN 根地址
#
# CDN 上需预先同步以下文件结构（由 CI 在 GoReleaser 发布后自动上传）：
#   ${CDN_BASE_URL}/releases/latest
#       → 纯文本文件，内容为最新版本号，如 v1.0.0
#   ${CDN_BASE_URL}/releases/${VERSION}/openbee-${VERSION_NUM}-${OS}-${ARCH}.tar.gz
#       → GoReleaser 生成的二进制压缩包
#   ${CDN_BASE_URL}/releases/${VERSION}/checksums.txt
#       → GoReleaser 生成的 SHA256 校验文件
# ============================================================
CDN_BASE_URL="https://dl.theopenbee.cn"

# ============================================================
# 默认参数
# ============================================================
BINARY_NAME="openbee"
INSTALL_DIR="/usr/local/bin"
VERSION=""
FORCE=false
NO_VERIFY=false

# ============================================================
# 颜色输出
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
# 清理临时文件
# ============================================================
TMPDIR_INSTALL=""
cleanup() {
    if [ -n "$TMPDIR_INSTALL" ] && [ -d "$TMPDIR_INSTALL" ]; then
        rm -rf "$TMPDIR_INSTALL"
    fi
}
trap cleanup EXIT INT TERM

# ============================================================
# 依赖检查
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
        error "需要 curl 或 wget，请先安装其中之一"
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
# 平台检测
# ============================================================
detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      error "不支持的操作系统: $OS（仅支持 linux 和 macOS）" ;;
    esac

    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64)  ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *)             error "不支持的架构: $ARCH（仅支持 amd64 和 arm64）" ;;
    esac

    info "检测到平台: ${OS}/${ARCH}"
}

# ============================================================
# 版本获取
# ============================================================
fetch_latest_version() {
    info "正在获取最新版本号..."

    VERSION=$(download_text "${CDN_BASE_URL}/releases/latest" 2>/dev/null) || true
    VERSION=$(echo "$VERSION" | tr -d '[:space:]')

    if [ -z "$VERSION" ]; then
        error "无法获取最新版本号，请检查 CDN 连接或手动指定版本: --version v1.0.0"
    fi

    # 确保版本号以 v 开头
    case "$VERSION" in
        v*) ;;
        *)  VERSION="v${VERSION}" ;;
    esac
}

# ============================================================
# 校验
# ============================================================
verify_checksum() {
    archive_file="$1"
    archive_name="$2"
    checksum_file="$3"

    expected=$(grep "${archive_name}" "$checksum_file" | awk '{print $1}')
    if [ -z "$expected" ]; then
        warn "checksums.txt 中未找到 ${archive_name} 的校验值，跳过校验"
        return 0
    fi

    if check_command sha256sum; then
        actual=$(sha256sum "$archive_file" | awk '{print $1}')
    elif check_command shasum; then
        actual=$(shasum -a 256 "$archive_file" | awk '{print $1}')
    else
        warn "未找到 sha256sum 或 shasum，跳过校验"
        return 0
    fi

    if [ "$actual" != "$expected" ]; then
        error "SHA256 校验失败！\n  期望: ${expected}\n  实际: ${actual}\n文件可能已损坏，请重试"
    fi

    ok "SHA256 校验通过"
}

# ============================================================
# 安装
# ============================================================
install_binary() {
    TMPDIR_INSTALL="$(mktemp -d)"

    VERSION_NUM="${VERSION#v}"
    ARCHIVE_NAME="${BINARY_NAME}-${VERSION_NUM}-${OS}-${ARCH}.tar.gz"
    ARCHIVE_URL="${CDN_BASE_URL}/releases/${VERSION}/${ARCHIVE_NAME}"
    CHECKSUM_URL="${CDN_BASE_URL}/releases/${VERSION}/checksums.txt"

    info "正在下载 ${ARCHIVE_NAME}..."
    download "$ARCHIVE_URL" "${TMPDIR_INSTALL}/${ARCHIVE_NAME}" || \
        error "下载失败: ${ARCHIVE_URL}\n请确认版本 ${VERSION} 存在且 CDN 地址正确（修改脚本顶部 CDN_BASE_URL 变量）"

    if [ "$NO_VERIFY" = false ]; then
        info "正在校验文件完整性..."
        download "${CHECKSUM_URL}" "${TMPDIR_INSTALL}/checksums.txt" || \
            warn "无法下载 checksums.txt，跳过校验"
        if [ -f "${TMPDIR_INSTALL}/checksums.txt" ]; then
            verify_checksum "${TMPDIR_INSTALL}/${ARCHIVE_NAME}" "${ARCHIVE_NAME}" "${TMPDIR_INSTALL}/checksums.txt"
        fi
    else
        warn "跳过校验（--no-verify）"
    fi

    info "正在解压..."
    tar -xzf "${TMPDIR_INSTALL}/${ARCHIVE_NAME}" -C "${TMPDIR_INSTALL}"

    if [ ! -f "${TMPDIR_INSTALL}/${BINARY_NAME}" ]; then
        error "解压后未找到 ${BINARY_NAME} 二进制文件"
    fi

    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMPDIR_INSTALL}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    elif check_command sudo; then
        info "需要 sudo 权限安装到 ${INSTALL_DIR}"
        sudo mv "${TMPDIR_INSTALL}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        warn "无 ${INSTALL_DIR} 写权限且 sudo 不可用，将安装到 ~/.local/bin"
        INSTALL_DIR="${HOME}/.local/bin"
        mkdir -p "$INSTALL_DIR"
        mv "${TMPDIR_INSTALL}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    ok "${BINARY_NAME} ${VERSION} 已安装到 ${INSTALL_DIR}/${BINARY_NAME}"
}

# ============================================================
# 安装后检查
# ============================================================
post_install_check() {
    if ! check_command "${BINARY_NAME}"; then
        warn "${INSTALL_DIR} 不在 PATH 中，请手动添加:"
        echo ""
        echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        echo ""
        echo "  将上面这行添加到 ~/.bashrc 或 ~/.zshrc 中以永久生效"
        echo ""
    else
        installed_version=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | awk '{print $2}' || echo "unknown")
        ok "验证安装: ${installed_version}"
    fi
}

# ============================================================
# 已安装检测
# ============================================================
check_existing() {
    if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ] && [ "$FORCE" = false ]; then
        existing_version=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | awk '{print $2}' || echo "")
        if [ -n "$existing_version" ]; then
            info "已安装 openbee: ${existing_version}"
            info "如需重新安装，请使用 --force"
            exit 0
        fi
    fi
}

# ============================================================
# 参数解析
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
            --cdn-url)
                CDN_BASE_URL="$2"
                shift 2
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            *)
                error "未知参数: $1（使用 --help 查看帮助）"
                ;;
        esac
    done
}

usage() {
    cat <<EOF
OpenBee 安装脚本（中国大陆加速版）

用法:
  curl -fsSL <url>/install.zh.sh | sh
  curl -fsSL <url>/install.zh.sh | sh -s -- [选项]

选项:
  --version, -v <版本>        指定版本（如 v1.0.0），默认安装最新版
  --install-dir, -d <路径>    安装目录，默认 /usr/local/bin
  --force, -f                 强制重新安装
  --no-verify                 跳过 SHA256 校验
  --cdn-url <地址>             指定 CDN 根地址，覆盖默认值（${CDN_BASE_URL}）
  --help, -h                  显示帮助

说明:
  本脚本通过国内 CDN 下载，适用于中国大陆网络环境。
  如 CDN 地址有变更，可通过 --cdn-url 参数指定，或修改脚本顶部的 CDN_BASE_URL 变量。
  国际网络环境请使用 install.sh。

示例:
  # 安装最新版
  curl -fsSL <url>/install.zh.sh | sh

  # 安装指定版本
  curl -fsSL <url>/install.zh.sh | sh -s -- --version v1.0.0

  # 安装到自定义目录
  curl -fsSL <url>/install.zh.sh | sh -s -- --install-dir ~/.local/bin

  # 使用自定义 CDN 地址
  curl -fsSL <url>/install.zh.sh | sh -s -- --cdn-url https://my-cdn.example.com
EOF
}

# ============================================================
# 主流程
# ============================================================
main() {
    setup_colors
    parse_args "$@"

    echo ""
    info "OpenBee 安装程序（中国大陆加速版）"
    echo ""

    detect_downloader
    detect_platform

    if [ -z "$VERSION" ]; then
        fetch_latest_version
    fi
    info "目标版本: ${VERSION}"

    check_existing
    install_binary
    post_install_check

    echo ""
    ok "安装完成！运行 'openbee --help' 开始使用。"
    echo ""
}

main "$@"
