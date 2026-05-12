# Claude Adapter Prepare 高内聚改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 claude 引擎的 "清理遗留 `.openbee.md` 配置" 逻辑从 `EngineAdapter.Prepare` 接口方法收回为 claude 包内部细节，由 `claudeAdapter.Run` 自动触发。

**Architecture:** 删除 `ai.EngineAdapter` 接口里的 `Prepare`，让 claude 包内新增私有 `cleanupLegacyRules(workDir)` 函数，并在 `claudeAdapter` 自己的 `Run` 里前置调用。外部调用方（worker / execution / feeder / bee_process）只调 `Run`。`PrepareOptions` 类型整体删除（`ai.Role` 仍保留供其他模块使用）。

**Tech Stack:** Go 1.x，标准库 `os` / `path/filepath` / `bytes` / `errors`。

参考 spec: `docs/superpowers/specs/2026-05-12-claude-prepare-cohesion-design.md`

---

## File Map

**Modify:**
- `internal/ai/engine/claude/adapter.go` — 把 `Prepare` 方法重构为私有 `cleanupLegacyRules` 函数，新增 `Run` 覆写；常量降级为 unexported。
- `internal/ai/engine/claude/adapter_test.go` — `package claude_test` → `package claude`，直接测私有函数。
- `internal/ai/ai.go` — `EngineAdapter` 接口删除 `Prepare`；`DynamicAdapter.Prepare` 删除；`PrepareOptions` 类型删除。
- `internal/ai/core/adapter.go` — 删除 `BaseAdapter.Prepare`。
- `internal/ai/core/adapter_test.go` — 删除 `TestBaseAdapter_PrepareIsNoop`。
- `internal/ai/ai_test.go` — `stubEngine` / `stubAdapter` 删除 `Prepare`；删除 `TestDynamicAdapter_PrepareCallsAll`。
- `internal/ai/engine/codex/adapter_test.go` — 删除 `TestAdapter_Prepare_NoOp` 和 `TestAdapter_Prepare_BothRoles`。
- `internal/ai/engine/kimi/adapter_test.go` — 删除 `Prepare` 相关测试。
- `internal/ai/engine/pi/adapter_test.go` — 删除 `Prepare` 相关测试。
- `internal/domain/worker/worker.go` — 删除 `engine.Prepare(...)` 调用（line 187 附近）。
- `internal/domain/worker/execution.go` — 删除 `engine.Prepare(...)` 调用（line 35 附近）。
- `internal/domain/worker/manager_test.go` — `mockEngine` / `silentMockEngine` 删除 `Prepare`。
- `internal/domain/bee/feeder.go` — 删除 `f.runner.Prepare(...)` 调用（line 121 附近）。
- `internal/domain/bee/feeder_test.go` — `mockBeeRunner` / `callbackBeeRunner` 删除 `Prepare`。
- `internal/domain/bee/bee_process.go` — 删除 `BeeProcess.Prepare` 包装方法。
- `internal/rpc/tools_test.go` — `stubEngineAdapter` 删除 `Prepare`。
- `internal/tokenstat/syncer_test.go` — `fakeAdapter` 删除 `Prepare`。

策略：TDD + 安全演进顺序。先在 claude 包内新增私有函数与 Run 覆写（保留对外 Prepare 以维持兼容），再在调用方删除 `Prepare` 调用，最后一次性切断接口并清理所有 stub/测试。

---

### Task 1: 在 claude 包内引入 cleanupLegacyRules 并覆写 Run

为后续移除 `Prepare` 接口做铺垫——在 claude 包内拷出私有清理函数与 `Run` 包装。同时为了能直接测私有函数，把测试文件切到同包。该步骤保留原 `Prepare` 方法和导出常量，构建/全部已有测试仍然通过。

**Files:**
- Modify: `internal/ai/engine/claude/adapter.go`
- Modify: `internal/ai/engine/claude/adapter_test.go`

- [ ] **Step 1: 修改 `internal/ai/engine/claude/adapter_test.go`，切到 `package claude` 同包，并把所有 `Prepare(...)` 调用替换为 `cleanupLegacyRules(dir)`**

完整替换文件内容为：

```go
package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupLegacyRules_Stub(t *testing.T) {
	dir := t.TempDir()
	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}
}

func TestCleanupLegacyRules_DeletesOpenbeeFile(t *testing.T) {
	dir := t.TempDir()
	openbeeFile := filepath.Join(dir, systemRulesFile)
	if err := os.WriteFile(openbeeFile, []byte("old rules"), 0o644); err != nil {
		t.Fatalf("write .openbee.md: %v", err)
	}

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}

	if _, err := os.Stat(openbeeFile); !os.IsNotExist(err) {
		t.Error(".openbee.md should have been deleted")
	}
}

func TestCleanupLegacyRules_RemovesImportLine(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	content := "# My Bot\n" + importLine + "\nOther content\n"
	if err := os.WriteFile(claudeFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	got := string(data)
	if strings.Contains(got, importLine) {
		t.Errorf("CLAUDE.md should not contain import line, got:\n%s", got)
	}
	if !strings.Contains(got, "# My Bot") {
		t.Error("CLAUDE.md should preserve other content")
	}
	if !strings.Contains(got, "Other content") {
		t.Error("CLAUDE.md should preserve other content")
	}
}

func TestCleanupLegacyRules_PreservesOtherCLAUDEMDContent(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	original := "# Custom instructions\nDo something special.\n"
	if err := os.WriteFile(claudeFile, []byte(original), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	if string(data) != original {
		t.Errorf("CLAUDE.md should be unchanged when import line is absent.\nGot: %q\nWant: %q", string(data), original)
	}
}

func TestCleanupLegacyRules_NoopWhenFilesAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules should not error when no files exist: %v", err)
	}
}

func TestCleanupLegacyRules_BothLegacyFilesPresent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, systemRulesFile), []byte("rules"), 0o644)
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(importLine+"\n"), 0o644)

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, systemRulesFile)); !os.IsNotExist(err) {
		t.Errorf(".openbee.md should be deleted")
	}
}
```

注意：
- `TestClaudeAdapter_ExtraEnvInBaseEnv` 被去掉了（它只断言 `NewAdapter` 不 panic 且满足接口，价值不大，被新的同包测试隐式覆盖）。
- 旧的 `TestClaudeAdapter_Prepare_BothRoles` 不再按 Role 区分（清理逻辑里本来就不读 Role）。

- [ ] **Step 2: 运行测试，确认会编译失败（因为 `cleanupLegacyRules` / `systemRulesFile` / `importLine` 尚未定义）**

Run: `go test ./internal/ai/engine/claude/...`
Expected: 编译失败，提示 `undefined: cleanupLegacyRules` 等。

- [ ] **Step 3: 修改 `internal/ai/engine/claude/adapter.go`，把 `Prepare` 重构为私有 `cleanupLegacyRules`，新增 `Run` 覆写，导出常量降级**

完整替换文件内容为：

```go
package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
)

const (
	// systemRulesFile is the legacy rules file Claude's Run cleanup removes.
	systemRulesFile = ".openbee.md"
	// importLine is the legacy reference line removed from CLAUDE.md.
	importLine = "@" + systemRulesFile
)

func init() {
	ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.ExtraEnv()), nil
	})
}

// claudeAdapter embeds core.BaseAdapter and wraps Run to clean up the legacy
// openbee rules file and matching import line in CLAUDE.md before each run.
type claudeAdapter struct {
	*core.BaseAdapter
}

// NewAdapter constructs a Claude engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{
		BaseAdapter: &core.BaseAdapter{
			Invoker:   NewInvoker(binaryPath, extraEnv),
			Collector: NewCollector(),
			Extract:   ExtractResultFromLog,
		},
	}
}

// Run cleans up legacy openbee rules before delegating to the embedded BaseAdapter.
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	if err := cleanupLegacyRules(workDir); err != nil {
		return ai.RunResult{}, err
	}
	return a.BaseAdapter.Run(ctx, workDir, prompt, opts, logPath)
}

func cleanupLegacyRules(workDir string) error {
	rulesPath := filepath.Join(workDir, systemRulesFile)
	if err := os.Remove(rulesPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", systemRulesFile, err)
	}
	return removeImportLine(workDir)
}

func removeImportLine(workDir string) error {
	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	target := []byte(importLine)
	lines := bytes.Split(data, []byte("\n"))
	out := lines[:0]
	for _, line := range lines {
		if !bytes.Equal(bytes.TrimRight(line, "\r"), target) {
			out = append(out, line)
		}
	}
	cleaned := bytes.Join(out, []byte("\n"))
	if bytes.Equal(cleaned, data) {
		return nil
	}
	return os.WriteFile(claudePath, cleaned, 0o644)
}

// Prepare is retained as a transitional no-op so existing callers keep
// compiling; it will be removed once callers and the interface are cleaned up.
func (a *claudeAdapter) Prepare(string, ai.PrepareOptions) error { return nil }
```

注意：
- `Prepare` 现在变成 no-op：因为新逻辑搬到了 `Run`，旧调用方暂时调到的 `Prepare` 不再有副作用——这避免了双重清理，也保证调用方代码尚未改前的语义等价（调用方原本期望"Prepare + Run = 一次清理 + 一次执行"，现在是"no-op Prepare + Run（含清理） = 一次清理 + 一次执行"，等价）。
- `SystemRulesFile` / `ImportLine` 改成小写。后面会通过 grep 验证没有外部引用。

- [ ] **Step 4: 运行 claude 包测试，验证通过**

Run: `go test ./internal/ai/engine/claude/...`
Expected: PASS，所有 6 个新测试通过。

- [ ] **Step 5: 全量构建+测试，确认没有别处引用 `claude.SystemRulesFile` / `claude.ImportLine`**

Run: `go build ./... && go test ./...`
Expected: PASS。如果 build 失败，搜索 `claude.SystemRulesFile` / `claude.ImportLine` 引用并清理（仅在 claude 包内被用到）。

- [ ] **Step 6: Commit**

```bash
git add internal/ai/engine/claude/adapter.go internal/ai/engine/claude/adapter_test.go
git commit -m "refactor(claude): fold legacy rules cleanup into Run"
```

---

### Task 2: 删除调用方的 engine.Prepare 调用

清理方还在调 `Prepare`（现在已经是 no-op），把这些显式调用全部删掉。此时 `Prepare` 仍然在接口上，因此其他 stub 暂不改动，构建仍然通过。

**Files:**
- Modify: `internal/domain/worker/worker.go`
- Modify: `internal/domain/worker/execution.go`
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/bee/bee_process.go`

- [ ] **Step 1: 修改 `internal/domain/worker/worker.go`，删除 Prepare 调用**

找到（约 line 187）：

```go
if err := engine.Prepare(p.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
    return model.Worker{}, fmt.Errorf("prepare worker workspace: %w", err)
}

return m.workerStore.Create(workerModel)
```

替换为：

```go
return m.workerStore.Create(workerModel)
```

- [ ] **Step 2: 修改 `internal/domain/worker/execution.go`，删除 Prepare 调用**

找到（约 line 35）：

```go
if err := engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
    log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
}
timeout := m.workerTimeout
```

替换为：

```go
timeout := m.workerTimeout
```

- [ ] **Step 3: 修改 `internal/domain/bee/feeder.go`，删除 Prepare 调用**

找到（约 line 121）：

```go
if err := f.runner.Prepare(f.workDir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
    // ... 原错误处理
}
```

删除整个 `if err := f.runner.Prepare(...)` 块。如果删除后该函数没有任何 `ai.` 引用，注意保留导入或移除多余导入（go build 会提示）。

- [ ] **Step 4: 修改 `internal/domain/bee/bee_process.go`，删除 BeeProcess.Prepare 包装方法**

删除 line 45–47 的：

```go
func (p *BeeProcess) Prepare(workDir string, opts ai.PrepareOptions) error {
    return p.engine.Prepare(workDir, opts)
}
```

- [ ] **Step 5: 构建+测试，确认通过**

Run: `go build ./... && go test ./...`
Expected: PASS。如果有 import 未使用错误，运行 `goimports -w <file>` 修复。

- [ ] **Step 6: Commit**

```bash
git add internal/domain/worker/worker.go internal/domain/worker/execution.go internal/domain/bee/feeder.go internal/domain/bee/bee_process.go
git commit -m "refactor(domain): drop explicit engine.Prepare calls"
```

---

### Task 3: 从 EngineAdapter 接口删除 Prepare 并清理所有遗留实现/测试

此时所有真实调用方都已经不再调 `Prepare`，可以安全地从接口、`BaseAdapter`、`DynamicAdapter`、`claudeAdapter` 以及所有 stub 上把 `Prepare` 与 `PrepareOptions` 类型一次性铲除。

**Files:**
- Modify: `internal/ai/ai.go`
- Modify: `internal/ai/core/adapter.go`
- Modify: `internal/ai/core/adapter_test.go`
- Modify: `internal/ai/ai_test.go`
- Modify: `internal/ai/engine/claude/adapter.go`
- Modify: `internal/ai/engine/codex/adapter_test.go`
- Modify: `internal/ai/engine/kimi/adapter_test.go`
- Modify: `internal/ai/engine/pi/adapter_test.go`
- Modify: `internal/domain/worker/manager_test.go`
- Modify: `internal/domain/bee/feeder_test.go`
- Modify: `internal/rpc/tools_test.go`
- Modify: `internal/tokenstat/syncer_test.go`

- [ ] **Step 1: 修改 `internal/ai/ai.go`，从 `EngineAdapter` 接口删除 `Prepare`、删除 `PrepareOptions` 类型、删除 `DynamicAdapter.Prepare`**

找到 `EngineAdapter` 接口（约 line 109–125）：

```go
type EngineAdapter interface {
    // Prepare is an engine-specific initialisation hook called before each Run.
    // It must be idempotent. Claude uses it to clean up legacy config files;
    // other engines return nil.
    Prepare(workDir string, opts PrepareOptions) error

    // Run executes a task and returns a RunResult ...
    Run(ctx context.Context, workDir, prompt string,
        opts RunOptions, logPath string) (RunResult, error)

    // CollectTokenUsage reads per-turn token usage ...
    CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error)
}
```

替换为：

```go
type EngineAdapter interface {
    // Run executes a task and returns a RunResult carrying the process handle,
    // event channel, and an engine-bound result extractor. The event channel
    // is closed after the process exits.
    Run(ctx context.Context, workDir, prompt string,
        opts RunOptions, logPath string) (RunResult, error)

    // CollectTokenUsage reads per-turn token usage for the given session from
    // engine-specific storage. Returns ErrSessionDataNotFound when no data is
    // available for the session.
    CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error)
}
```

找到 `PrepareOptions` 类型（约 line 48–51）：

```go
// PrepareOptions carries parameters for the engine-specific Prepare hook.
type PrepareOptions struct {
    Role Role
}
```

整段删除。

找到 `DynamicAdapter.Prepare` 方法（约 line 225–233）：

```go
// Prepare initialises every engine adapter for the given workDir.
func (d *DynamicAdapter) Prepare(workDir string, opts PrepareOptions) error {
    for name, e := range d.engines {
        if err := e.Prepare(workDir, opts); err != nil {
            return fmt.Errorf("prepare engine %q: %w", name, err)
        }
    }
    return nil
}
```

整段删除。

- [ ] **Step 2: 修改 `internal/ai/core/adapter.go`，删除 `BaseAdapter.Prepare`**

找到（line 42–43）：

```go
// Prepare is a no-op default that engines may override (e.g. claude).
func (b *BaseAdapter) Prepare(string, ai.PrepareOptions) error { return nil }
```

整段删除。

- [ ] **Step 3: 修改 `internal/ai/engine/claude/adapter.go`，删除过渡期的 `Prepare` no-op 方法**

找到文件末尾：

```go
// Prepare is retained as a transitional no-op so existing callers keep
// compiling; it will be removed once callers and the interface are cleaned up.
func (a *claudeAdapter) Prepare(string, ai.PrepareOptions) error { return nil }
```

整段删除。

- [ ] **Step 4: 修改 `internal/ai/core/adapter_test.go`，删除 `TestBaseAdapter_PrepareIsNoop`**

找到 `TestBaseAdapter_PrepareIsNoop` 函数（约 line 66–71）整段删除：

```go
func TestBaseAdapter_PrepareIsNoop(t *testing.T) {
    b := &core.BaseAdapter{}
    if err := b.Prepare("/wd", ai.PrepareOptions{}); err != nil {
        t.Error(err)
    }
}
```

- [ ] **Step 5: 修改 `internal/ai/ai_test.go`，删除 `stubEngine.Prepare` / `stubAdapter.Prepare` 以及 `TestDynamicAdapter_PrepareCallsAll`**

`stubEngine` 类型从：

```go
type stubEngine struct {
    name     string
    prepared []string // workDirs seen
}

func (s *stubEngine) Prepare(workDir string, _ ai.PrepareOptions) error {
    s.prepared = append(s.prepared, workDir)
    return nil
}
```

改为：

```go
type stubEngine struct {
    name string
}
```

删除 `TestDynamicAdapter_PrepareCallsAll` 函数整段（约 line 60–71）。

删除 `stubAdapter.Prepare` 方法（约 line 191）：

```go
func (s *stubAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
```

（注意：`stubAdapter` 仍需满足 `EngineAdapter`，删除 `Prepare` 后正好满足新接口。）

- [ ] **Step 6: 修改 `internal/ai/engine/codex/adapter_test.go`，删除 Prepare 测试**

删除 `TestAdapter_Prepare_NoOp`（line 12–28）和 `TestAdapter_Prepare_BothRoles`（line 30–41）整段。同时如果 `os`、`path/filepath` 包变成未使用，删除对应 import。

- [ ] **Step 7: 修改 `internal/ai/engine/kimi/adapter_test.go`，删除 Prepare 测试**

参照 codex 处理：删除调用 `a.Prepare(...)` 的测试函数（line 15 起、line 26 起对应的 2 个测试）。清理无用 import。

- [ ] **Step 8: 修改 `internal/ai/engine/pi/adapter_test.go`，删除 Prepare 测试**

参照 codex 处理：删除调用 `a.Prepare(...)` 的测试函数（line 18 起、line 32 起对应的 2 个测试）。清理无用 import。

- [ ] **Step 9: 修改 `internal/domain/worker/manager_test.go`，删除 `mockEngine.Prepare` / `silentMockEngine.Prepare`**

找到（line 20、line 44）：

```go
func (e *mockEngine) Prepare(_ string, _ ai.PrepareOptions) error {
    // ...
}
func (e *silentMockEngine) Prepare(_ string, _ ai.PrepareOptions) error { return nil }
```

整段删除。如果 `mockEngine` 内有 `prepared` / `prepareCalls` 之类字段配套使用，一并删除字段；并审视相关测试断言是否还成立。

- [ ] **Step 10: 修改 `internal/domain/bee/feeder_test.go`，删除 `mockBeeRunner.Prepare` / `callbackBeeRunner.Prepare`**

找到（line 64、line 585）：

```go
func (m *mockBeeRunner) Prepare(_ string, _ ai.PrepareOptions) error {
    // ...
}
func (r *callbackBeeRunner) Prepare(_ string, _ ai.PrepareOptions) error {
    // ...
}
```

整段删除。同样清理对应未使用字段（如果有）。

- [ ] **Step 11: 修改 `internal/rpc/tools_test.go`，删除 `stubEngineAdapter.Prepare`**

找到（line 25）：

```go
func (s *stubEngineAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
    // ...
}
```

整段删除。

- [ ] **Step 12: 修改 `internal/tokenstat/syncer_test.go`，删除 `fakeAdapter.Prepare`**

找到（line 20）：

```go
func (f *fakeAdapter) Prepare(string, ai.PrepareOptions) error { return nil }
```

整段删除。

- [ ] **Step 13: 全量构建+测试**

Run: `go build ./...`
Expected: PASS。如果有未使用 import（最常见 `ai` 在测试文件不再被引用），删除对应 import。

Run: `go test ./...`
Expected: PASS。

- [ ] **Step 14: 验证 `ai.PrepareOptions` 和 `.Prepare(` 已无残留**

Run:
```bash
grep -rn "ai\.PrepareOptions" --include="*.go" internal/
grep -rn "\.Prepare(" --include="*.go" internal/
```

Expected: 第一条无输出；第二条只输出与 AI 引擎适配器无关的命中（例如 `execStore.PrepareLogPath(...)` 之类）。

- [ ] **Step 15: Commit**

```bash
git add -A internal/
git commit -m "refactor(ai): remove Prepare from EngineAdapter interface"
```

---

### Task 4: 最终验证

- [ ] **Step 1: 整体构建+测试，作为最后一道防线**

Run: `go build ./... && go test ./...`
Expected: PASS。

- [ ] **Step 2: 可选：用 vet/staticcheck 之类工具确认没有遗留**

Run: `go vet ./...`
Expected: 无新增警告。

- [ ] **Step 3: 总结提交记录**

Run: `git log --oneline refactor/internal-ai-cleanup ^main`
预期得到 3 个新提交：
1. refactor(claude): fold legacy rules cleanup into Run
2. refactor(domain): drop explicit engine.Prepare calls
3. refactor(ai): remove Prepare from EngineAdapter interface
