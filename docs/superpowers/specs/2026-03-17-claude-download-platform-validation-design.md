# Claude Code 下载平台校验设计

**日期**: 2026-03-17
**状态**: Draft

## 概述

在 `downloadClaude()` 函数中增加客户端平台白名单校验，仅允许以下 6 个平台下载 Claude Code，不支持的平台给出明确提示。

## 支持平台

| 平台标识 | OS | Arch | Variant |
|---|---|---|---|
| darwin-arm64 | darwin | arm64 | - |
| darwin-x64 | darwin | x64 | - |
| linux-arm64 | linux | arm64 | - |
| linux-x64 | linux | x64 | - |
| linux-arm64-musl | linux | arm64 | musl |
| linux-x64-musl | linux | x64 | musl |

## 设计细节

### 1. 平台标识构建

从 Go runtime 获取 OS 和 Arch，并做以下映射和检测：

**架构映射**（Go 名称 → 下载平台名称）：
- `amd64` → `x64`
- `arm64` → `arm64`（不变）
- 其他架构值保持原样传递，会被白名单拒绝（这是预期行为）

**注意**: 此映射会改变现有 URL 中的 arch 参数值（从 `amd64` 变为 `x64`），下载服务端需要接受 `x64` 作为架构标识。

**musl 检测**（仅 Linux）：
- 使用 `filepath.Glob("/lib/ld-musl-*.so*")` 检查是否存在 musl 动态链接器
- 覆盖 `/lib/ld-musl-aarch64.so.1`、`/lib/ld-musl-x86_64.so.1` 等变体
- glob 调用出错时按非 musl 处理（fail-open），避免权限问题阻塞用户
- 若匹配到则 variant 为 `musl`

### 2. 白名单校验

使用 map 实现 O(1) 查找：

```go
type claudePlatform struct {
    os      string // "darwin" or "linux"
    arch    string // "arm64" or "x64"
    variant string // "" or "musl"
}

var supportedPlatforms = map[claudePlatform]struct{}{
    {"darwin", "arm64", ""}:    {},
    {"darwin", "x64", ""}:      {},
    {"linux", "arm64", ""}:     {},
    {"linux", "x64", ""}:       {},
    {"linux", "arm64", "musl"}: {},
    {"linux", "x64", "musl"}:   {},
}
```

`detectPlatform()` 构建平台结构体（检测只执行一次），后续校验和 URL 构建都复用该结果。

### 3. 下载 URL 构建

保持现有参数格式，新增 `variant` 参数：

- 非 musl: `?os=darwin&arch=arm64`
- musl: `?os=linux&arch=arm64&variant=musl`

仅当 variant 非空时追加 `&variant=musl`。

### 4. 错误提示

不支持的平台返回如下错误信息，包含支持列表帮助用户自查：

```
当前系统 (windows/amd64) 不支持自动下载 Claude Code。
支持的平台: darwin-arm64, darwin-x64, linux-arm64, linux-x64, linux-arm64-musl, linux-x64-musl
请手动安装。
```

### 5. 清理项

- 移除 Windows 相关的 `claude.exe` 逻辑（Windows 不在支持列表中）

## 影响范围

仅修改 `cmd/robobee/config_claude.go` 中的 `downloadClaude()` 函数及相关辅助代码。

### 修改点

1. 新增 `claudePlatform` 结构体和 `supportedPlatforms` map
2. 新增 `detectPlatform()` 函数 — 检测当前 OS/Arch/Variant，结果一次性构建
3. 新增 `isMusl()` 函数 — 通过 glob 检测 musl 环境，支持依赖注入以便测试：
   ```go
   func isMuslWith(globFunc func(string) ([]string, error)) bool
   ```
4. 修改 `downloadClaude()` — 增加校验逻辑、更新 URL 构建、移除 Windows 逻辑

## 测试策略

- 单元测试覆盖架构映射（amd64→x64、arm64→arm64、未知架构→原样传递）
- 单元测试覆盖白名单校验（支持/不支持的平台组合）
- musl 检测通过 `isMuslWith` 注入 mock glob 函数测试（匹配、不匹配、glob 出错三种情况）
