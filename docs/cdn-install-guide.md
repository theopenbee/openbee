# OpenBee CDN 安装指南

## 安装脚本使用方式

```bash
# 安装最新版
curl -fsSL https://your-cdn.com/openbee/install.sh | sh

# 安装指定版本
curl -fsSL https://your-cdn.com/openbee/install.sh | sh -s -- --version v1.0.0

# 安装到自定义目录
curl -fsSL https://your-cdn.com/openbee/install.sh | sh -s -- --install-dir ~/.local/bin
```

### 支持平台

| 操作系统 | 架构 |
|---------|------|
| macOS   | amd64 (Intel), arm64 (Apple Silicon) |
| Linux   | amd64, arm64 |

### 可用参数

| 参数 | 说明 |
|------|------|
| `--version, -v <版本>` | 指定版本（如 v1.0.0），默认安装最新版 |
| `--install-dir, -d <路径>` | 安装目录，默认 /usr/local/bin |
| `--force, -f` | 强制重新安装 |
| `--no-verify` | 跳过 SHA256 校验 |
| `--help, -h` | 显示帮助 |

## CDN 文件目录结构

CDN 根目录下需要按以下结构组织文件：

```
openbee/
└── releases/
    ├── latest                  ← 文本文件，内容仅为最新版本号，如：v1.0.0
    ├── v1.0.0/
    │   ├── openbee-1.0.0-darwin-amd64.tar.gz
    │   ├── openbee-1.0.0-darwin-arm64.tar.gz
    │   ├── openbee-1.0.0-linux-amd64.tar.gz
    │   ├── openbee-1.0.0-linux-arm64.tar.gz
    │   └── checksums.txt
    ├── v1.1.0/
    │   ├── openbee-1.1.0-darwin-amd64.tar.gz
    │   ├── openbee-1.1.0-darwin-arm64.tar.gz
    │   ├── openbee-1.1.0-linux-amd64.tar.gz
    │   ├── openbee-1.1.0-linux-arm64.tar.gz
    │   └── checksums.txt
    └── ...
```

### 文件说明

- **`latest`** — 纯文本文件，仅包含最新版本号（如 `v1.0.0`），每次发版后更新
- **版本目录** — 以 `v` 开头（如 `v1.0.0`）
- **tar.gz 文件** — GoReleaser 自动生成，命名格式为 `openbee-{版本号不带v}-{os}-{arch}.tar.gz`
- **`checksums.txt`** — GoReleaser 自动生成的 SHA256 校验文件

## 如何上传文件到 CDN

每次发版后，GoReleaser 构建完成会在 `dist/` 目录下生成所有产物，将其上传到 CDN 对应版本目录。

### AWS S3 示例

```bash
VERSION="v1.0.0"

# 上传所有 tar.gz 和 checksums
aws s3 cp dist/ s3://your-bucket/openbee/releases/${VERSION}/ \
  --recursive \
  --exclude "*" \
  --include "openbee-*-darwin-*.tar.gz" \
  --include "openbee-*-linux-*.tar.gz" \
  --include "checksums.txt"

# 更新 latest 文件
echo "${VERSION}" | aws s3 cp - s3://your-bucket/openbee/releases/latest \
  --content-type "text/plain"
```

### 阿里云 OSS 示例

```bash
VERSION="v1.0.0"

ossutil cp dist/ oss://your-bucket/openbee/releases/${VERSION}/ \
  --recursive \
  --include "openbee-*-darwin-*.tar.gz" \
  --include "openbee-*-linux-*.tar.gz" \
  --include "checksums.txt"

echo "${VERSION}" > /tmp/latest
ossutil cp /tmp/latest oss://your-bucket/openbee/releases/latest
```

## 配置说明

安装脚本中 `CDN_BASE_URL` 变量当前为占位值，上线前需修改为实际 CDN 地址：

```bash
# install.sh 中第 14 行
CDN_BASE_URL="https://your-cdn.com/openbee"
```

同时需要将 `install.sh` 本身也上传到 CDN，以便用户通过 `curl | sh` 方式安装。
