# Claude Code 下载功能重构设计

**日期**: 2026-03-19
**状态**: 待实施
**文件范围**: `cmd/openbee/config_claude.go`, `cmd/openbee/config_claude_test.go`

---

## 背景与目标

### 现状

`config_claude.go` 中的 `downloadClaude()` 函数通过 `https://cc-download.openbee.dev/claude/download` 服务端点下载 Claude Code 二进制文件：

- 依赖中间服务做 HTTP 302 重定向
- **无 checksum 校验**，无法检测损坏或被篡改的文件
- 无版本感知，始终通过服务获取最新版

### 目标

参考 `upgrade.go` 的升级逻辑，将 Claude Code 下载功能重构为直接从 GitHub Releases 下载，并加入 SHA256 完整性校验，与 openbee 自身升级模式保持一致。

---

## 现有代码分析

### upgrade.go 的关键模式

| 功能 | 实现 |
|------|------|
| 获取最新版本 | GitHub API `GET /repos/theopenbee/openbee/releases/latest` |
| 下载 checksum | `checksums.txt` from GitHub release |
| 流式 SHA256 | `io.MultiWriter(file, sha256.New())` 边下载边计算 |
| 原子替换 | 先写 temp 文件，成功后 `os.Rename()` |
| 通用下载工具 | `downloadFile(url, dest, extraWriter)` |
| 大小保护 | `io.LimitReader(body, 512MB)` |

### config_claude.go 现状

| 功能 | 实现 |
|------|------|
| 下载 URL | `https://cc-download.openbee.dev/claude/download?os=X&arch=Y` |
| Checksum | ❌ 无 |
| 版本感知 | ❌ 无 |
| 原子替换 | ✅ 已有（temp file + rename） |
| 大小保护 | ✅ 已有（512MB limit） |

### cc-download CI 产出的 GitHub Release 结构

**仓库**: `theopenbee/cc-download`
**Release 资产**（每次发布 7 个文件）：

```
claude-{VERSION}-darwin-arm64
claude-{VERSION}-darwin-x64
claude-{VERSION}-linux-arm64
claude-{VERSION}-linux-x64
claude-{VERSION}-linux-arm64-musl
claude-{VERSION}-linux-x64-musl
checksums-sha256.txt
```

**重要区别**：与 openbee 不同，cc-download 的 release 资产是**纯二进制文件**（不是 tar.gz），无需解压。

**Checksum 文件格式**（标准 sha256sum 格式）：
```
abc123...  claude-1.2.3-darwin-arm64
def456...  claude-1.2.3-darwin-x64
...
```

---

## 重构方案

### 方案选择

| 方案 | 描述 | 推荐 |
|------|------|------|
| **A: 直连 GitHub Releases** | 完全绕过 cc-download.openbee.dev 服务，直接从 GitHub 下载 | ✅ 推荐 |
| B: 保留服务 + 加 checksum | 给 cc-download.openbee.dev 增加 checksum 端点 | ❌ 仍依赖中间服务 |
| C: 服务解析 URL + 直连下载 | 用服务获取 URL，再直连 GitHub | ❌ 复杂度高无收益 |

**推荐方案 A** — 与 upgrade.go 模式完全一致，消除中间服务依赖，加入完整性校验。

---

## 详细设计

### 一、新常量（替换 `claudeDownloadURL`）

```go
const (
    claudeGitHubAPI     = "https://api.github.com/repos/theopenbee/cc-download/releases/latest"
    claudeGitHubRelBase = "https://github.com/theopenbee/cc-download/releases/download"
)
```

### 二、新函数 `fetchLatestClaudeVersion()`

镜像 `upgrade.go` 中的 `fetchLatestVersion()`：

```go
func fetchLatestClaudeVersion() (string, error) {
    client := &http.Client{Timeout: 15 * time.Second}
    resp, err := client.Get(claudeGitHubAPI)
    // ... 同 upgrade.go
    // 返回带 "v" 前缀的 tag，如 "v1.2.3"
}
```

### 三、修改 `buildClaudeDownloadURL()` 签名

加入 `version` 参数，构建 GitHub release 资产 URL：

```go
// 旧签名: buildClaudeDownloadURL(p claudePlatform) string
// 新签名:
func buildClaudeDownloadURL(p claudePlatform, version string) string {
    versionNum := strings.TrimPrefix(version, "v")
    platformStr := p.os + "-" + p.arch
    if p.variant != "" {
        platformStr += "-" + p.variant
    }
    assetName := fmt.Sprintf("claude-%s-%s", versionNum, platformStr)
    return fmt.Sprintf("%s/%s/%s", claudeGitHubRelBase, version, assetName)
}
```

示例输出：`https://github.com/theopenbee/cc-download/releases/download/v1.2.3/claude-1.2.3-darwin-arm64`

### 四、重写 `downloadClaude()`

核心流程，镜像 `doUpgrade()`：

```
1. 平台检测（不变）
2. 创建 ~/.openbee/bin/ 目录（不变）
3. fetchLatestClaudeVersion() → 获取版本
4. 构建 checksums URL: {relBase}/{version}/checksums-sha256.txt
5. 构建 binary URL:    {relBase}/{version}/claude-{ver}-{platform}
6. tmpDir = os.MkdirTemp("", "openbee-claude-*")，defer os.RemoveAll(tmpDir)
7. 下载 checksums-sha256.txt 到 tmpDir（小文件，先下）
8. 下载 binary 到 destPath+".tmp"，同时流式计算 SHA256（downloadFile + io.MultiWriter）
9. 验证 SHA256：
   - 校验文件下载失败 → warning + 继续
   - 校验值不匹配 → 删除 .tmp，返回 error（fatal）
10. chmod 0755 + os.Rename 到 ~/.openbee/bin/claude（原子替换）
```

**关键差异（相比 upgrade.go）**：

- `downloadFile()` 已在同一 package（`package main`）中定义，直接调用，无需重复
- cc-download release 是**纯二进制**，不需要 `extractBinary()`，直接使用 `downloadFile` 写入目标路径
- temp 文件写在 `binDir` 内（同 upgrade.go 的 temp-in-same-dir 原子 rename 策略）

### 五、Checksum 验证逻辑

复用 `doUpgrade()` 中已有的逻辑（两者格式一致，均为 `sha256sum` 标准输出），无需修改。差异仅在 checksum 文件名：
- upgrade.go: `checksums.txt`
- cc-download: `checksums-sha256.txt`

---

## 影响范围

### 需修改的文件

| 文件 | 修改内容 |
|------|---------|
| `cmd/openbee/config_claude.go` | 替换常量、新增 `fetchLatestClaudeVersion()`、修改 `buildClaudeDownloadURL()` 签名、重写 `downloadClaude()` |
| `cmd/openbee/config_claude_test.go` | 更新 `TestBuildClaudeDownloadURL()` 测试（新增 version 参数）；新增 `fetchLatestClaudeVersion` 相关测试 |

### 不需修改的文件

- `upgrade.go` — `downloadFile()` 已可直接被 `config_claude.go` 调用（同 package）
- 其余 config_*.go 文件
- 平台检测相关函数（`detectPlatform`, `mapArch`, `isMusl` 等）

### 新增 imports

`config_claude.go` 需新增：
```go
"crypto/sha256"
"encoding/hex"
"strings"
```

---

## 数据流对比

### 重构前

```
downloadClaude()
  └─ GET https://cc-download.openbee.dev/claude/download?os=X&arch=Y
       └─ HTTP 302 → binary download
            └─ write to ~/.openbee/bin/claude.tmp
                 └─ rename to claude
                      ❌ no checksum
```

### 重构后

```
downloadClaude()
  ├─ fetchLatestClaudeVersion()
  │    └─ GET https://api.github.com/repos/theopenbee/cc-download/releases/latest
  │         └─ returns "v1.2.3"
  ├─ GET checksums-sha256.txt → save to tmpDir
  ├─ GET claude-1.2.3-darwin-arm64 → write to ~/.openbee/bin/claude.tmp
  │    └─ io.MultiWriter(file, sha256.New())  ← 边下载边计算
  ├─ 验证 SHA256 ✅
  ├─ chmod 0755
  └─ rename .tmp → claude (原子替换)
```

---

## 风险与注意事项

1. **GitHub API 速率限制**：未认证请求限 60 次/小时；对 config 流程影响极小（一次性操作），可接受
2. **网络可达性**：中国大陆直连 GitHub 可能受限；cc-download CI 已上传至阿里云 OSS，可考虑将 OSS 作为备用镜像（当前设计不含此优化，可后续迭代）
3. **版本格式**：cc-download tag 遵循 semver（`v{major}.{minor}.{patch}`），与 openbee 相同，`fetchLatestClaudeVersion` 逻辑可直接复用
4. **checksum 处理**：区分两种情况——
   - checksum 文件**下载失败**：打 warning 并继续（与 upgrade.go 一致，不强制失败）
   - checksum 文件**下载成功但校验不匹配**：**直接返回错误，不继续安装**（与 upgrade.go 一致，mismatch 是 hard error）

---

## 测试计划

- `TestBuildClaudeDownloadURL` — 更新为带 version 参数的测试用例
- `TestFetchLatestClaudeVersion` — mock HTTP server，验证 tag 解析（含/不含 v 前缀）
- `TestDownloadClaude` — 集成测试验证完整下载流程（可用 httptest 模拟）
- `TestBuildClaudeDownloadURL` 预期值示例：
  ```
  version="v1.2.3", platform=darwin/arm64 →
  "https://github.com/theopenbee/cc-download/releases/download/v1.2.3/claude-1.2.3-darwin-arm64"

  version="v1.2.3", platform=linux/x64/musl →
  "https://github.com/theopenbee/cc-download/releases/download/v1.2.3/claude-1.2.3-linux-x64-musl"
  ```
- 现有平台检测测试保持不变（`TestMapArch`, `TestIsMuslWith`, `TestIsSupportedPlatform`, `TestDetectPlatform`）
