# Engine Unify Session Prefix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove all config-file generation from all three engine adapters (Claude, Codex, PI) and rename `SetupWorkspace` to `Prepare(workDir string, opts PrepareOptions) error` — a lightweight initialisation hook that Claude uses to clean up legacy files.

**Architecture:** `EngineAdapter` gains a `PrepareOptions` struct and a renamed `Prepare` method. Claude's `Prepare` deletes `.openbee.md` and strips `@.openbee.md` from `CLAUDE.md`; Codex and PI return nil. System-rule injection is already handled via the prompt-prefix mechanism in `Feeder` and `TaskDispatcher` and is left untouched.

**Tech Stack:** Go 1.25, standard library (`os`, `bytes`, `path/filepath`), `go test`

**Spec:** `docs/superpowers/specs/2026-04-10-engine-unify-session-prefix-design.md`

---

## File Map

| File | Action | What changes |
|------|--------|--------------|
| `internal/ai/engine.go` | Modify | Add `PrepareOptions`; rename interface method; remove `WorkspaceOptions`, `LoadInstruction` |
| `internal/ai/workspace.go` | Delete | Entire file (shared AGENTS.md logic) |
| `internal/ai/workspace_test.go` | Delete | Entire file |
| `internal/ai/rules.go` | Modify | Remove `BeePersona`, `BeeRules()`, `WorkerRules()` |
| `internal/ai/rules_test.go` | Keep | No changes needed (tested functions remain) |
| `internal/ai/claude/adapter.go` | Modify | Implement `Prepare` with cleanup; remove `SetupWorkspace`, `writeCLAUDEMD` |
| `internal/ai/claude/claudemd.go` | Modify | Remove `EnsureSystemRules`; add `removeImportLine` helper; delete file if empty |
| `internal/ai/claude/adapter_test.go` | Rewrite | Remove old `SetupWorkspace` tests; add `Prepare` cleanup tests |
| `internal/ai/claude/claudemd_test.go` | Delete | Entire file (`EnsureSystemRules` tests) |
| `internal/ai/codex/adapter.go` | Modify | `Prepare` returns nil; remove AGENTS.md write |
| `internal/ai/codex/adapter_test.go` | Rewrite | Replace `SetupWorkspace` tests with `Prepare` no-op test |
| `internal/ai/pi/adapter.go` | Modify | `Prepare` returns nil; remove AGENTS.md write |
| `internal/ai/pi/adapter_test.go` | Rewrite | Replace `SetupWorkspace` tests with `Prepare` no-op test |
| `internal/domain/worker/manager.go` | Modify | Update 2 `SetupWorkspace` call sites |
| `internal/domain/worker/manager_test.go` | Modify | Update `mockEngine` stub |
| `internal/domain/bee/bee_process.go` | Modify | Rename method |
| `internal/domain/bee/feeder.go` | Modify | Update call site |
| `internal/domain/bee/feeder_test.go` | Modify | Update 2 stubs (`mockBeeRunner`, `callbackBeeRunner`) |
| `internal/mcp/tools_test.go` | Modify | Update `stubEngineAdapter` stub |
| `internal/ai/registry_test.go` | Modify | Update `stubAdapter` stub |

---

## Task 1: Update `EngineAdapter` interface in engine.go

**Files:**
- Modify: `internal/ai/engine.go`

> ⚠️ After this task the repo will NOT compile. That is expected — all implementers are updated in Task 2.

- [ ] **Step 1: Edit engine.go**

Replace the `WorkspaceOptions` struct and `SetupWorkspace` interface method. Also remove the `LoadInstruction` constant (only used by the AGENTS.md generation being removed).

The file should become:

```go
package ai

import "context"

const (
	// SystemRulesFile is the legacy rules file that Claude's Prepare hook cleans up.
	SystemRulesFile = ".openbee.md"
	// ImportLine is the legacy reference line that Claude's Prepare hook removes from CLAUDE.md.
	ImportLine = "@" + SystemRulesFile
)

// Role identifies the openbee agent role.
type Role string

const (
	RoleBee    Role = "bee"
	RoleWorker Role = "worker"
)

// PrepareOptions carries parameters for the engine-specific Prepare hook.
// Add future fields here without changing the Prepare method signature.
type PrepareOptions struct {
	Role Role
}

// RunOptions controls session behaviour for an engine invocation.
type RunOptions struct {
	SessionID string
	Resume    bool
	APIKey    string
}

// OutputType classifies a lifecycle event from a running process.
type OutputType string

const (
	OutputDone      OutputType = "done"
	OutputError     OutputType = "error"
	OutputSessionID OutputType = "session_id"
)

// Output is a single lifecycle event.
type Output struct {
	Type    OutputType `json:"type"`
	Content string     `json:"content"`
}

// Process is the handle for a running engine process.
type Process interface {
	PID() int
	Stop() error
}

// EngineAdapter is the complete plugin contract for an AI engine.
// Implementations must be safe for concurrent use.
type EngineAdapter interface {
	// Prepare is an engine-specific initialisation hook called before each Run.
	// It must be idempotent. Claude uses it to clean up legacy config files;
	// other engines return nil.
	Prepare(workDir string, opts PrepareOptions) error

	// Run executes a task and returns a process handle and an event channel.
	// The channel is closed after the process exits.
	Run(ctx context.Context, workDir, prompt string,
		opts RunOptions, logPath string) (Process, <-chan Output, error)

	// ExtractResult parses the engine-specific log file and returns the final
	// result string, or "" if none found.
	ExtractResult(logPath string) string
}
```

- [ ] **Step 2: Verify the file saved correctly**

```bash
head -5 internal/ai/engine.go
```
Expected: `package ai`

---

## Task 2: Fix compilation — update all implementers, callers, and stubs

**Files:**
- Modify: `internal/ai/claude/adapter.go`
- Modify: `internal/ai/codex/adapter.go`
- Modify: `internal/ai/pi/adapter.go`
- Modify: `internal/domain/bee/bee_process.go`
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/worker/manager.go`
- Modify: `internal/domain/bee/feeder_test.go`
- Modify: `internal/domain/worker/manager_test.go`
- Modify: `internal/mcp/tools_test.go`
- Modify: `internal/ai/registry_test.go`
- Modify: `internal/ai/codex/adapter_test.go`
- Modify: `internal/ai/pi/adapter_test.go`
- Delete: `internal/ai/claude/adapter_test.go` old content (rewrite)
- Delete: `internal/ai/claude/claudemd_test.go`

> In this task `Prepare` is a no-op stub everywhere — correct behaviour is added in Task 3/4.

- [ ] **Step 1: Update claude/adapter.go**

Replace the file content. The `Prepare` stub returns nil for now; cleanup logic is added in Task 4. Remove `writeCLAUDEMD` (it wrote CLAUDE.md — being deleted). Keep `Run` and `ExtractResult` unchanged.

```go
package claude

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register("claude", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = "claude"
		}
		return NewAdapter(path, cfg.OpenbeeURL), nil
	})
}

type claudeAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string) ai.EngineAdapter {
	return &claudeAdapter{invoker: NewInvoker(binaryPath, openbeeURL)}
}

func (a *claudeAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *claudeAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}
```

- [ ] **Step 2: Update codex/adapter.go**

```go
package codex

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register("codex", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = "codex"
		}
		return NewAdapter(path, cfg.OpenbeeURL), nil
	})
}

type codexAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string) ai.EngineAdapter {
	return &codexAdapter{invoker: NewInvoker(binaryPath, openbeeURL)}
}

func (a *codexAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *codexAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *codexAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}

var _ ai.EngineAdapter = (*codexAdapter)(nil)
```

- [ ] **Step 3: Update pi/adapter.go**

```go
package pi

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register("pi", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = "pi"
		}
		extraEnv, _ := cfg.Raw["env"].(map[string]string)
		return NewAdapter(path, cfg.OpenbeeURL, extraEnv), nil
	})
}

type piAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &piAdapter{invoker: NewInvoker(binaryPath, openbeeURL, extraEnv)}
}

func (a *piAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *piAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *piAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}

var _ ai.EngineAdapter = (*piAdapter)(nil)
```

- [ ] **Step 4: Update bee/bee_process.go**

Rename `SetupWorkspace` → `Prepare` and update the signature:

```go
func (p *BeeProcess) Prepare(workDir string, opts ai.PrepareOptions) error {
	return p.engine.Prepare(workDir, opts)
}
```

Remove the old `SetupWorkspace` method.

- [ ] **Step 5: Update feeder.go call site**

In `internal/domain/bee/feeder.go` line ~101, change:
```go
// Old
if err := f.runner.SetupWorkspace(f.workDir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
```
To:
```go
// New
if err := f.runner.Prepare(f.workDir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
```

- [ ] **Step 6: Update worker/manager.go — two call sites**

In `internal/domain/worker/manager.go`:

**CreateWorker (~line 68):** change:
```go
// Old
if err := m.engine.SetupWorkspace(workDir, ai.RoleWorker, ai.WorkspaceOptions{
    Name:        name,
    Description: description,
    Memory:      memory,
}); err != nil {
    return model.Worker{}, fmt.Errorf("setup worker workspace: %w", err)
}
```
To:
```go
// New
if err := m.engine.Prepare(workDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
    return model.Worker{}, fmt.Errorf("prepare worker workspace: %w", err)
}
```

**ExecuteWorker (~line 107):** change:
```go
// Old
if err := m.engine.SetupWorkspace(worker.WorkDir, ai.RoleWorker, ai.WorkspaceOptions{
    Name:        worker.Name,
    Description: worker.Description,
    Memory:      worker.Memory,
}); err != nil {
    log.Error("setup worker workspace", zap.String("op", "execute"), zap.Error(err))
}
```
To:
```go
// New
if err := m.engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
    log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
}
```

- [ ] **Step 7: Update feeder_test.go stubs**

In `internal/domain/bee/feeder_test.go`, find and update both stubs:

`mockBeeRunner` (~line 62):
```go
// Old
func (m *mockBeeRunner) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
// New
func (m *mockBeeRunner) Prepare(_ string, _ ai.PrepareOptions) error {
```

`callbackBeeRunner` (~line 513):
```go
// Old
func (r *callbackBeeRunner) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
// New
func (r *callbackBeeRunner) Prepare(_ string, _ ai.PrepareOptions) error {
```

- [ ] **Step 8: Update manager_test.go stub**

In `internal/domain/worker/manager_test.go` (~line 14):
```go
// Old
func (e *mockEngine) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
// New
func (e *mockEngine) Prepare(_ string, _ ai.PrepareOptions) error {
```

- [ ] **Step 9: Update mcp/tools_test.go stub**

In `internal/mcp/tools_test.go` (~line 24):
```go
// Old
func (s *stubEngineAdapter) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
// New
func (s *stubEngineAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
```

- [ ] **Step 10: Update registry_test.go stub**

In `internal/ai/registry_test.go` (~line 15):
```go
// Old
func (s *stubAdapter) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
// New
func (s *stubAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
```

- [ ] **Step 11: Rewrite codex/adapter_test.go**

Replace the entire file. The old tests checked that `SetupWorkspace` created `AGENTS.md` — that behaviour is gone. The new test just verifies `Prepare` is a no-op.

```go
package codex_test

import (
	"os"
	"path/filepath"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/codex"
)

func TestAdapter_Prepare_NoOp(t *testing.T) {
	dir := t.TempDir()
	a := codex.NewAdapter("echo", "http://localhost:9999")

	if err := a.Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	// Prepare must not create any files
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("Prepare must not create files, found: %v", entries)
	}
	_ = filepath.Join(dir, "AGENTS.md") // Ensure path helpers compile
}

func TestAdapter_Prepare_BothRoles(t *testing.T) {
	a := codex.NewAdapter("echo", "http://localhost:9999")
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		if err := a.Prepare(dir, ai.PrepareOptions{Role: role}); err != nil {
			t.Errorf("Prepare(%s): %v", role, err)
		}
	}
}
```

- [ ] **Step 12: Rewrite pi/adapter_test.go**

Replace the entire file:

```go
package pi_test

import (
	"os"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/pi"
)

func TestAdapter_Prepare_NoOp(t *testing.T) {
	dir := t.TempDir()
	a := pi.NewAdapter("echo", "http://localhost:9999", nil)

	if err := a.Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("Prepare must not create files, found: %v", entries)
	}
}

func TestAdapter_Prepare_BothRoles(t *testing.T) {
	a := pi.NewAdapter("echo", "http://localhost:9999", nil)
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		if err := a.Prepare(dir, ai.PrepareOptions{Role: role}); err != nil {
			t.Errorf("Prepare(%s): %v", role, err)
		}
	}
}
```

- [ ] **Step 13: Delete claude/claudemd_test.go**

```bash
rm internal/ai/claude/claudemd_test.go
```

- [ ] **Step 14: Rewrite claude/adapter_test.go (remove old SetupWorkspace tests)**

Keep only the `newTestAdapter` helper; remove all `TestClaudeAdapter_SetupWorkspace_*` tests. New Prepare tests will be added in Task 3. Replace with:

```go
package claude_test

import (
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/claude"
)

func newTestAdapter(t *testing.T) ai.EngineAdapter {
	t.Helper()
	return claude.NewAdapter("echo", "http://localhost:9999")
}

func TestClaudeAdapter_Prepare_Stub(t *testing.T) {
	// Placeholder — replaced in Task 3 with real cleanup tests.
	dir := t.TempDir()
	adapter := newTestAdapter(t)
	if err := adapter.Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
}
```

- [ ] **Step 15: Verify compilation**

```bash
go build ./...
```
Expected: exits 0 (no errors).

- [ ] **Step 16: Run tests**

```bash
go test ./...
```
Expected: all pass.

- [ ] **Step 17: Commit**

```bash
git add -A
git commit -m "refactor: rename SetupWorkspace to Prepare with PrepareOptions

Renames EngineAdapter.SetupWorkspace to Prepare(workDir string, opts PrepareOptions).
All implementations are stubs (return nil) — cleanup logic added in next commit.
Removes WorkspaceOptions and LoadInstruction from engine.go.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 3: Write failing tests for Claude Prepare cleanup

**Files:**
- Modify: `internal/ai/claude/adapter_test.go`

- [ ] **Step 1: Add cleanup tests to claude/adapter_test.go**

Append the following tests to the file (after `TestClaudeAdapter_Prepare_Stub`):

```go
package claude_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/claude"
)

func newTestAdapter(t *testing.T) ai.EngineAdapter {
	t.Helper()
	return claude.NewAdapter("echo", "http://localhost:9999")
}

func TestClaudeAdapter_Prepare_DeletesOpenbeeFile(t *testing.T) {
	dir := t.TempDir()
	openbeeFile := filepath.Join(dir, ai.SystemRulesFile)
	if err := os.WriteFile(openbeeFile, []byte("old rules"), 0o644); err != nil {
		t.Fatalf("write .openbee.md: %v", err)
	}

	if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if _, err := os.Stat(openbeeFile); !os.IsNotExist(err) {
		t.Error(".openbee.md should have been deleted by Prepare")
	}
}

func TestClaudeAdapter_Prepare_RemovesImportLine(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	content := "# My Bot\n" + ai.ImportLine + "\nOther content\n"
	if err := os.WriteFile(claudeFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	got := string(data)
	if strings.Contains(got, ai.ImportLine) {
		t.Errorf("CLAUDE.md should not contain import line after Prepare, got:\n%s", got)
	}
	if !strings.Contains(got, "# My Bot") {
		t.Error("CLAUDE.md should preserve other content")
	}
	if !strings.Contains(got, "Other content") {
		t.Error("CLAUDE.md should preserve other content")
	}
}

func TestClaudeAdapter_Prepare_PreservesOtherCLAUDEMDContent(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	// CLAUDE.md with no import line — must not be modified
	original := "# Custom instructions\nDo something special.\n"
	if err := os.WriteFile(claudeFile, []byte(original), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	if string(data) != original {
		t.Errorf("CLAUDE.md should be unchanged when import line is absent.\nGot: %q\nWant: %q", string(data), original)
	}
}

func TestClaudeAdapter_Prepare_NoopWhenFilesAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
		t.Fatalf("Prepare should not error when no files exist: %v", err)
	}
}

func TestClaudeAdapter_Prepare_BothRoles(t *testing.T) {
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		// Setup: both legacy files present
		os.WriteFile(filepath.Join(dir, ai.SystemRulesFile), []byte("rules"), 0o644)
		os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(ai.ImportLine+"\n"), 0o644)

		if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: role}); err != nil {
			t.Errorf("Prepare(%s): %v", role, err)
		}
		if _, err := os.Stat(filepath.Join(dir, ai.SystemRulesFile)); !os.IsNotExist(err) {
			t.Errorf("role %s: .openbee.md should be deleted", role)
		}
	}
}
```

> Replace the entire file with the above (which includes `newTestAdapter` and all tests).

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/ai/claude/... -run TestClaudeAdapter_Prepare -v
```
Expected: `TestClaudeAdapter_Prepare_DeletesOpenbeeFile` and `TestClaudeAdapter_Prepare_RemovesImportLine` FAIL (Prepare is still a stub that returns nil without deleting anything).

---

## Task 4: Implement Claude Prepare cleanup

**Files:**
- Modify: `internal/ai/claude/claudemd.go`
- Modify: `internal/ai/claude/adapter.go`

- [ ] **Step 1: Replace claudemd.go with cleanup helper only**

`EnsureSystemRules` and all write functions are removed. The file now only contains `removeImportLine`:

```go
package claude

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// removeImportLine removes the "@.openbee.md" line from CLAUDE.md if present.
// It is a no-op if CLAUDE.md does not exist or does not contain the line.
func removeImportLine(workDir string) error {
	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	target := []byte(ai.ImportLine)
	lines := bytes.Split(data, []byte("\n"))
	out := lines[:0]
	for _, line := range lines {
		if !bytes.Equal(bytes.TrimRight(line, "\r"), target) {
			out = append(out, line)
		}
	}
	cleaned := bytes.Join(out, []byte("\n"))
	if bytes.Equal(cleaned, data) {
		return nil // nothing changed
	}
	return os.WriteFile(claudePath, cleaned, 0o644)
}
```

- [ ] **Step 2: Update claude/adapter.go — implement Prepare**

Replace the stub `Prepare` method with the real implementation:

```go
func (a *claudeAdapter) Prepare(workDir string, _ ai.PrepareOptions) error {
	rulesPath := filepath.Join(workDir, ai.SystemRulesFile)
	if err := os.Remove(rulesPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", ai.SystemRulesFile, err)
	}
	return removeImportLine(workDir)
}
```

Add the required imports to claude/adapter.go:

```go
import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)
```

- [ ] **Step 3: Run the new tests**

```bash
go test ./internal/ai/claude/... -run TestClaudeAdapter_Prepare -v
```
Expected: all `TestClaudeAdapter_Prepare_*` PASS.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/claude/adapter.go internal/ai/claude/claudemd.go internal/ai/claude/adapter_test.go
git commit -m "feat: implement Claude Prepare hook to clean up legacy files

Deletes .openbee.md and removes the @.openbee.md import line from
CLAUDE.md. Both operations are no-ops when the files are absent.
Rule injection is now handled exclusively via prompt-prefix on new sessions.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 5: Remove dead code

**Files:**
- Delete: `internal/ai/workspace.go`
- Delete: `internal/ai/workspace_test.go`
- Modify: `internal/ai/rules.go`
- Delete: `internal/ai/claude/claudemd_test.go` (already deleted in Task 2 Step 13 — skip if done)

- [ ] **Step 1: Delete workspace.go and workspace_test.go**

```bash
rm internal/ai/workspace.go internal/ai/workspace_test.go
```

- [ ] **Step 2: Remove dead functions from rules.go**

Edit `internal/ai/rules.go` to remove `BeePersona`, `BeeRules()`, and `WorkerRules()`. The file after editing:

```go
package ai

import "fmt"

// WorkerPersona returns the persona-only content injected into new worker session prompts.
func WorkerPersona(name, description, memory string) string {
	s := "You are a Worker in an AI team.\n"
	if name != "" {
		s += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		s += fmt.Sprintf("Description: %s\n", description)
	}
	if memory != "" {
		s += fmt.Sprintf("\n## Memory Constraints\n%s\n", memory)
	}
	return s
}

// SkillHintPrefix returns the skill invocation hint prepended to the first
// message of a new session.
func SkillHintPrefix(role Role) string {
	switch role {
	case RoleBee:
		return "use openbee-bee skill."
	case RoleWorker:
		return "use openbee-worker skill."
	default:
		return ""
	}
}
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./...
```
Expected: exits 0.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: remove dead code — workspace.go, BeeRules, WorkerRules, BeePersona

All file-generation logic has been removed. Rules are now injected
exclusively via prompt-prefix on new sessions (Feeder and TaskDispatcher).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Self-Review Checklist

**Spec coverage:**
- ✅ `Prepare(workDir string, opts PrepareOptions) error` interface — Task 1
- ✅ `PrepareOptions{Role}` struct — Task 1
- ✅ `WorkspaceOptions` removed — Task 1
- ✅ Claude Prepare: deletes `.openbee.md` — Task 4
- ✅ Claude Prepare: removes `@.openbee.md` line from `CLAUDE.md` — Task 4
- ✅ Codex `Prepare` no-op — Task 2
- ✅ PI `Prepare` no-op — Task 2
- ✅ Manager adds `os.MkdirAll` before Prepare — ⚠️ Manager already calls `os.MkdirAll` at line 64 in `CreateWorker` before the `Prepare` call; no additional change needed. `ExecuteWorker` calls Prepare on an already-existing dir (created at worker creation time) — this is correct.
- ✅ `workspace.go` deleted — Task 5
- ✅ `BeeRules`, `WorkerRules`, `BeePersona` removed — Task 5
- ✅ `WorkerPersona`, `SkillHintPrefix` kept — Task 5 (they remain in rules.go)
- ✅ Existing `.openbee.md`/`CLAUDE.md` cleanup — Task 4
- ✅ Existing `AGENTS.md` left as-is — no task needed

**Placeholder scan:** None found.

**Type consistency:** `PrepareOptions` defined in Task 1, used consistently throughout Tasks 2–4. `ai.SystemRulesFile` and `ai.ImportLine` defined in Task 1, used in Task 4.
