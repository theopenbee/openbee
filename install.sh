#!/bin/sh
# RoboBee 一键安装脚本
# 用法:
#   curl -fsSL https://your-cdn.com/robobee/install.sh | sh
#   curl -fsSL https://your-cdn.com/robobee/install.sh | sh -s -- --version v1.0.0
#   curl -fsSL https://your-cdn.com/robobee/install.sh | sh -s -- --install-dir /custom/path

set -e

# ============================================================
# CDN 配置 — 修改此处指向你的 CDN 根地址
# ============================================================
CDN_BASE_URL="https://your-cdn.com/robobee"
# CDN 文件目录结构:
#   ${CDN_BASE_URL}/releases/latest           -> 文本文件，内容为最新版本号，如 v1.0.0
#   ${CDN_BASE_URL}/releases/${VERSION}/robobee-${VERSION}-${OS}-${ARCH}.tar.gz
#   ${CDN_BASE_URL}/releases/${VERSION}/checksums.txt

# ============================================================
# 默认参数
# ============================================================
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
        x86_64|amd64)   ARCH="amd64" ;;
        aarch64|arm64)   ARCH="arm64" ;;
        *)               error "不支持的架构: $ARCH（仅支持 amd64 和 arm64）" ;;
    esac

    info "检测到平台: ${OS}/${ARCH}"
}

# ============================================================
# 版本获取
# ============================================================
fetch_latest_version() {
    info "正在获取最新版本号..."
    VERSION=$(download_text "${CDN_BASE_URL}/releases/latest" 2>/dev/null) || \
        error "无法获取最新版本号，请检查网络连接或手动指定版本: --version v1.0.0"

    # 去除可能的空白字符
    VERSION=$(echo "$VERSION" | tr -d '[:space:]')

    if [ -z "$VERSION" ]; then
        error "获取到的版本号为空，请手动指定版本: --version v1.0.0"
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

    # 从 checksums.txt 中提取对应文件的校验值
    expected=$(grep "${archive_name}" "$checksum_file" | awk '{print $1}')
    if [ -z "$expected" ]; then
        warn "checksums.txt 中未找到 ${archive_name} 的校验值，跳过校验"
        return 0
    fi

    # 计算实际校验值
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

    # 去掉版本号开头的 v 以匹配 GoReleaser 的命名规则
    VERSION_NUM="${VERSION#v}"
    ARCHIVE_NAME="robobee-${VERSION_NUM}-${OS}-${ARCH}.tar.gz"
    ARCHIVE_URL="${CDN_BASE_URL}/releases/${VERSION}/${ARCHIVE_NAME}"
    CHECKSUM_URL="${CDN_BASE_URL}/releases/${VERSION}/checksums.txt"

    # 下载压缩包
    info "正在下载 ${ARCHIVE_NAME}..."
    download "$ARCHIVE_URL" "${TMPDIR_INSTALL}/${ARCHIVE_NAME}" || \
        error "下载失败: ${ARCHIVE_URL}\n请确认版本 ${VERSION} 存在且 CDN 地址正确"

    # 下载并校验
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

    # 解压
    info "正在解压..."
    tar -xzf "${TMPDIR_INSTALL}/${ARCHIVE_NAME}" -C "${TMPDIR_INSTALL}"

    # 确认二进制文件存在
    if [ ! -f "${TMPDIR_INSTALL}/robobee" ]; then
        error "解压后未找到 robobee 二进制文件"
    fi

    # 安装到目标目录
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMPDIR_INSTALL}/robobee" "${INSTALL_DIR}/robobee"
        chmod +x "${INSTALL_DIR}/robobee"
    else
        info "需要 sudo 权限安装到 ${INSTALL_DIR}"
        sudo mv "${TMPDIR_INSTALL}/robobee" "${INSTALL_DIR}/robobee"
        sudo chmod +x "${INSTALL_DIR}/robobee"
    fi

    ok "robobee ${VERSION} 已安装到 ${INSTALL_DIR}/robobee"
}

# ============================================================
# 安装后检查
# ============================================================
post_install_check() {
    # 检查是否在 PATH 中
    if ! check_command robobee; then
        warn "${INSTALL_DIR} 不在 PATH 中，请手动添加:"
        echo ""
        echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        echo ""
        echo "  将上面这行添加到 ~/.bashrc 或 ~/.zshrc 中以永久生效"
        echo ""
    else
        installed_version=$("${INSTALL_DIR}/robobee" version 2>/dev/null || echo "unknown")
        ok "验证安装: ${installed_version}"
    fi
}

# ============================================================
# 已安装检测
# ============================================================
check_existing() {
    if [ -f "${INSTALL_DIR}/robobee" ] && [ "$FORCE" = false ]; then
        existing_version=$("${INSTALL_DIR}/robobee" version 2>/dev/null || echo "")
        if [ -n "$existing_version" ]; then
            info "已安装 robobee: ${existing_version}"
            info "如需重新安装，请使用 --force"
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
RoboBee 安装脚本

用法:
  curl -fsSL <url>/install.sh | sh
  curl -fsSL <url>/install.sh | sh -s -- [选项]

选项:
  --version, -v <版本>        指定版本（如 v1.0.0），默认安装最新版
  --install-dir, -d <路径>    安装目录，默认 /usr/local/bin
  --force, -f                 强制重新安装
  --no-verify                 跳过 SHA256 校验
  --help, -h                  显示帮助

示例:
  # 安装最新版
  curl -fsSL <url>/install.sh | sh

  # 安装指定版本
  curl -fsSL <url>/install.sh | sh -s -- --version v1.0.0

  # 安装到自定义目录
  curl -fsSL <url>/install.sh | sh -s -- --install-dir ~/.local/bin
EOF
}

# ============================================================
# 主流程
# ============================================================
main() {
    setup_colors
    parse_args "$@"

    echo ""
    info "RoboBee 安装程序"
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
    ok "安装完成！运行 'robobee --help' 开始使用。"
    echo ""
}

main "$@"
