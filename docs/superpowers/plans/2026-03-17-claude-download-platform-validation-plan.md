# Claude Code 下载平台校验 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `downloadClaude()` 中增加客户端平台白名单校验，仅允许 6 个支持平台下载，不支持的平台给出明确提示。

**Architecture:** 新增平台检测和校验逻辑（`claudePlatform` 结构体 + `supportedPlatforms` map + `detectPlatform()` + `isMusl()`），在下载前校验平台，更新 URL 构建逻辑（arch 映射 + variant 参数），移除 Windows 死代码。

**Tech Stack:** Go, `runtime`, `filepath.Glob`

**Spec:** `docs/superpowers/specs/2026-03-17-claude-download-platform-validation-design.md`

---

## File Structure

- **Modify:** `cmd/robobee/config_claude.go` — 新增平台检测/校验函数，修改 `downloadClaude()`
- **Modify:** `cmd/robobee/config_claude_test.go` — 新增平台校验相关测试

---

### Task 1: 添加架构映射和 musl 检测的测试

**Files:**
- Modify: `cmd/robobee/config_claude_test.go`

- [ ] **Step 1: 写 mapArch 的测试**

注意：需在测试文件 import 块中添加 `"fmt"`（`TestIsMuslWith` 需要）。

```go
func TestMapArch(t *testing.T) {
	tests := []struct {
		goarch string
		want   string
	}{
		{"amd64", "x64"},
		{"arm64", "arm64"},
		{"386", "386"},
		{"riscv64", "riscv64"},
	}
	for _, tt := range tests {
		if got := mapArch(tt.goarch); got != tt.want {
			t.Errorf("mapArch(%q) = %q, want %q", tt.goarch, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: 写 isMuslWith 的测试**

```go
func TestIsMuslWith(t *testing.T) {
	// 匹配到 musl
	found := isMuslWith(func(pattern string) ([]string, error) {
		return []string{"/lib/ld-musl-x86_64.so.1"}, nil
	})
	if !found {
		t.Error("expected true when musl linker found")
	}

	// 未匹配
	notFound := isMuslWith(func(pattern string) ([]string, error) {
		return nil, nil
	})
	if notFound {
		t.Error("expected false when no musl linker")
	}

	// glob 出错 — fail-open
	errCase := isMuslWith(func(pattern string) ([]string, error) {
		return nil, fmt.Errorf("permission denied")
	})
	if errCase {
		t.Error("expected false (fail-open) when glob errors")
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -run "TestMapArch|TestIsMuslWith" -v`
Expected: FAIL — `mapArch` 和 `isMuslWith` 未定义

---

### Task 2: 实现架构映射和 musl 检测

**Files:**
- Modify: `cmd/robobee/config_claude.go`

- [ ] **Step 1: 在 `downloadClaude` 函数之前添加辅助函数**

在 `config_claude.go` 的 `downloadClaude` 函数之前（约 240 行前），添加：

```go
// mapArch converts Go's runtime.GOARCH to the download platform arch name.
func mapArch(goarch string) string {
	if goarch == "amd64" {
		return "x64"
	}
	return goarch
}

// isMuslWith checks if the system uses musl libc by looking for the musl dynamic linker.
// The globFunc parameter allows dependency injection for testing.
func isMuslWith(globFunc func(string) ([]string, error)) bool {
	matches, err := globFunc("/lib/ld-musl-*.so*")
	if err != nil {
		return false // fail-open: treat errors as non-musl
	}
	return len(matches) > 0
}

// isMusl checks if the current system uses musl libc.
func isMusl() bool {
	return isMuslWith(filepath.Glob)
}
```

- [ ] **Step 2: 运行测试确认通过**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -run "TestMapArch|TestIsMuslWith" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/robobee/config_claude.go cmd/robobee/config_claude_test.go
git commit -m "feat: add mapArch and isMusl helpers for platform detection"
```

---

### Task 3: 添加平台白名单校验的测试

**Files:**
- Modify: `cmd/robobee/config_claude_test.go`

- [ ] **Step 1: 写 claudePlatform 白名单校验的测试**

```go
func TestIsSupportedPlatform(t *testing.T) {
	supported := []claudePlatform{
		{"darwin", "arm64", ""},
		{"darwin", "x64", ""},
		{"linux", "arm64", ""},
		{"linux", "x64", ""},
		{"linux", "arm64", "musl"},
		{"linux", "x64", "musl"},
	}
	for _, p := range supported {
		if !isSupportedPlatform(p) {
			t.Errorf("expected supported: %+v", p)
		}
	}

	unsupported := []claudePlatform{
		{"windows", "x64", ""},
		{"windows", "arm64", ""},
		{"freebsd", "x64", ""},
		{"linux", "386", ""},
		{"darwin", "x64", "musl"},  // darwin + musl 不存在
		{"darwin", "arm64", "musl"},
	}
	for _, p := range unsupported {
		if isSupportedPlatform(p) {
			t.Errorf("expected unsupported: %+v", p)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -run "TestIsSupportedPlatform" -v`
Expected: FAIL — `claudePlatform` 和 `isSupportedPlatform` 未定义

---

### Task 4: 实现平台白名单校验

**Files:**
- Modify: `cmd/robobee/config_claude.go`

- [ ] **Step 1: 在辅助函数区域添加平台类型和校验逻辑**

在 `mapArch` 函数之前添加：

```go
// claudePlatform represents a target platform for Claude Code download.
type claudePlatform struct {
	os      string // "darwin" or "linux"
	arch    string // "arm64" or "x64"
	variant string // "" or "musl"
}

// supportedPlatforms lists all platforms that support Claude Code download.
var supportedPlatforms = map[claudePlatform]struct{}{
	{"darwin", "arm64", ""}:    {},
	{"darwin", "x64", ""}:      {},
	{"linux", "arm64", ""}:     {},
	{"linux", "x64", ""}:       {},
	{"linux", "arm64", "musl"}: {},
	{"linux", "x64", "musl"}:   {},
}

// isSupportedPlatform checks if the given platform is in the supported list.
func isSupportedPlatform(p claudePlatform) bool {
	_, ok := supportedPlatforms[p]
	return ok
}

// detectPlatform builds a claudePlatform from the current runtime environment.
func detectPlatform() claudePlatform {
	p := claudePlatform{
		os:   runtime.GOOS,
		arch: mapArch(runtime.GOARCH),
	}
	if runtime.GOOS == "linux" && isMusl() {
		p.variant = "musl"
	}
	return p
}
```

- [ ] **Step 2: 运行测试确认通过**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -run "TestIsSupportedPlatform" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/robobee/config_claude.go cmd/robobee/config_claude_test.go
git commit -m "feat: add claudePlatform whitelist and isSupportedPlatform"
```

---

### Task 5: 添加 downloadClaude 平台校验的测试

**Files:**
- Modify: `cmd/robobee/config_claude_test.go`

- [ ] **Step 1: 写 buildClaudeDownloadURL 和 detectPlatform 的测试**

```go
func TestBuildClaudeDownloadURL(t *testing.T) {
	tests := []struct {
		platform claudePlatform
		want     string
	}{
		{
			claudePlatform{"darwin", "arm64", ""},
			claudeDownloadURL + "?os=darwin&arch=arm64",
		},
		{
			claudePlatform{"darwin", "x64", ""},
			claudeDownloadURL + "?os=darwin&arch=x64",
		},
		{
			claudePlatform{"linux", "x64", ""},
			claudeDownloadURL + "?os=linux&arch=x64",
		},
		{
			claudePlatform{"linux", "arm64", "musl"},
			claudeDownloadURL + "?os=linux&arch=arm64&variant=musl",
		},
	}
	for _, tt := range tests {
		got := buildClaudeDownloadURL(tt.platform)
		if got != tt.want {
			t.Errorf("buildClaudeDownloadURL(%+v) = %q, want %q", tt.platform, got, tt.want)
		}
	}
}

func TestDetectPlatform(t *testing.T) {
	p := detectPlatform()
	if p.os == "" {
		t.Error("detectPlatform() returned empty os")
	}
	if p.arch == "" {
		t.Error("detectPlatform() returned empty arch")
	}
	// On the current machine, the detected platform should be supported
	// (this test runs on darwin or linux CI)
	if !isSupportedPlatform(p) {
		t.Errorf("current platform %+v should be supported", p)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -run "TestBuildClaudeDownloadURL|TestDetectPlatform" -v`
Expected: FAIL — `buildClaudeDownloadURL` 未定义

---

### Task 6: 修改 downloadClaude 函数

**Files:**
- Modify: `cmd/robobee/config_claude.go`

- [ ] **Step 1: 添加 URL 构建辅助函数**

在 `detectPlatform` 函数之后添加：

```go
// buildClaudeDownloadURL constructs the download URL for the given platform.
func buildClaudeDownloadURL(p claudePlatform) string {
	url := fmt.Sprintf("%s?os=%s&arch=%s", claudeDownloadURL, p.os, p.arch)
	if p.variant != "" {
		url += "&variant=" + p.variant
	}
	return url
}
```

- [ ] **Step 2: 修改 downloadClaude 函数**

将 `downloadClaude` 函数（第 241-303 行）替换为：

```go
func downloadClaude(vals *configValues) error {
	// Platform validation
	platform := detectPlatform()
	if !isSupportedPlatform(platform) {
		return fmt.Errorf(
			"当前系统 (%s/%s) 不支持自动下载 Claude Code。\n"+
				"支持的平台: darwin-arm64, darwin-x64, linux-arm64, linux-x64, linux-arm64-musl, linux-x64-musl\n"+
				"请手动安装。",
			runtime.GOOS, runtime.GOARCH,
		)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}

	binDir := filepath.Join(home, ".robobee", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	destPath := filepath.Join(binDir, "claude")
	url := buildClaudeDownloadURL(platform)

	fmt.Printf("正在下载 Claude (%s/%s)...\n", platform.os, platform.arch)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("请求下载地址失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	// Write to temp file first, rename on success to avoid leaving corrupt binaries.
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("写入文件失败: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("关闭文件失败: %w", err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("设置可执行权限失败: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("移动文件失败: %w", err)
	}

	vals.ClaudePath = destPath
	fmt.Printf("Claude 已下载到: %s\n", destPath)
	return nil
}
```

关键改动：
1. 函数开头增加 `detectPlatform()` + `isSupportedPlatform()` 校验
2. 移除 Windows `claude.exe` 逻辑，binary name 固定为 `claude`
3. URL 构建改用 `buildClaudeDownloadURL(platform)`
4. 打印信息使用 platform 结构体中的映射后的值

- [ ] **Step 3: 运行全部测试确认通过**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/robobee/config_claude.go cmd/robobee/config_claude_test.go
git commit -m "feat: add platform validation to downloadClaude with whitelist check"
```

---

### Task 7: 最终验证

- [ ] **Step 1: 运行完整测试套件**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./... -v`
Expected: ALL PASS

- [ ] **Step 2: 运行 go vet**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go vet ./cmd/robobee/`
Expected: 无输出（无问题）

- [ ] **Step 3: 确认编译通过**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go build ./cmd/robobee/`
Expected: 编译成功
