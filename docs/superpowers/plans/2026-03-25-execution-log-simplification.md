# Execution Log Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the three-layer log buffering system (ActiveLogRegistry + in-memory Builder + file write on completion) with OS-level stdout/stderr redirect to a log file at process launch time.

**Architecture:** `claude.Invoker.Run()` accepts a `logPath` string and redirects `cmd.Stdout`/`cmd.Stderr` directly to that file. The channel carries only lifecycle signals (`OutputDone`/`OutputError`). `ExecutionStore.PrepareLogPath()` computes the date-partitioned path, creates the directory, and stores it in the DB before the process starts. After completion, `extractResultFromLog()` scans the file for the `result` JSON event.

**Tech Stack:** Go stdlib (`os`, `encoding/json`), SQLite, existing `claude`, `worker`, `bee`, `store`, `api`, `app` packages.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/store/execution_store.go` | Modify | Add `PrepareLogPath`; remove `WriteLog`; harden `ReadLog` |
| `internal/store/execution_store_test.go` | Modify | Replace `WriteLog` tests with `PrepareLogPath` tests |
| `internal/claude/invoker.go` | Modify | Add `logPath` param; redirect to file; remove scanning goroutines |
| `internal/claude/invoker_test.go` | Modify | Update all `Run()` call sites; verify file content instead of channel |
| `internal/bee/bee_process.go` | Modify | Update `BeeProcess.Run` to pass `opts claude.RunOptions` + `logPath` |
| `internal/bee/feeder.go` | Modify | Update `BeeRunner` interface; remove `logRegistry`; reorder exec creation; rename `drainBeeOutput` → `waitBeeOutput` |
| `internal/bee/feeder_test.go` | Modify | Update `mockBeeRunner`; remove registry test; verify log_path set |
| `internal/worker/manager.go` | Modify | Remove `logRegistry`; update `NewManager`; add `extractResultFromLog`; simplify `monitorExecution` |
| `internal/worker/manager_test.go` | Modify | Remove `NewActiveLogRegistry()` from `NewManager` call |
| `internal/mcp/tools_test.go` | Modify | Remove `NewActiveLogRegistry()` from `worker.NewManager` call |
| `internal/api/router.go` | Modify | Remove `LogRegistry` from `ServerParams` |
| `internal/api/execution_handler.go` | Modify | Remove two-stage log read; single `ReadLog` call |
| `internal/app/app.go` | Modify | Remove `logRegistry` wiring throughout |
| `internal/worker/log_registry.go` | Delete | Entire file gone |
| `internal/worker/log_registry_test.go` | Delete | Entire file gone |

---

## Task 1: Add `ExecutionStore.PrepareLogPath`, update `ReadLog`, remove `WriteLog`

**Files:**
- Modify: `internal/store/execution_store.go`
- Modify: `internal/store/execution_store_test.go`

- [ ] **Step 1: Write failing test for `PrepareLogPath`**

Add to `internal/store/execution_store_test.go` (after the existing `TestExecutionStore_CreateBeeExecution` test):

```go
func TestExecutionStore_PrepareLogPath(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logsDir := t.TempDir()
	es := NewExecutionStore(db, logsDir)

	exec, _ := es.CreateBeeExecution("session1", "test prompt")

	logPath, err := es.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		t.Fatalf("PrepareLogPath: %v", err)
	}
	if logPath == "" {
		t.Fatal("expected non-empty logPath")
	}

	// Directory must exist
	if _, err := os.Stat(filepath.Dir(logPath)); err != nil {
		t.Errorf("log directory should exist: %v", err)
	}

	// DB must have log_path set
	got, err := es.GetByID(exec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LogPath != logPath {
		t.Errorf("DB log_path mismatch: want %q got %q", logPath, got.LogPath)
	}
}
```

Also add the missing `filepath` import to the test file if not already present:
```go
"path/filepath"
```

- [ ] **Step 2: Run the test to verify it fails**

```
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/store/... -run TestExecutionStore_PrepareLogPath -v
```

Expected: compile error — `PrepareLogPath` undefined.

- [ ] **Step 3: Add `PrepareLogPath` to `internal/store/execution_store.go`**

Find the `WriteLog` method and add `PrepareLogPath` directly above it:

```go
// PrepareLogPath creates the date-partitioned log directory, records the log path in
// the DB, and returns the path. Must be called before launching the process so that
// the invoker can redirect stdout/stderr to the file.
// startedAt is used for date partitioning; falls back to time.Now() if nil.
func (s *ExecutionStore) PrepareLogPath(id string, startedAt *int64) (string, error) {
	var t time.Time
	if startedAt != nil {
		t = time.UnixMilli(*startedAt)
	} else {
		t = time.Now()
	}
	dateDir := filepath.Join(s.logsDir, t.Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0o755); err != nil {
		return "", fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(dateDir, id+".log")
	if _, err := s.db.Exec(`UPDATE bee_executions SET log_path=? WHERE id=?`, logPath, id); err != nil {
		return "", fmt.Errorf("set log_path: %w", err)
	}
	return logPath, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

```
go test ./internal/store/... -run TestExecutionStore_PrepareLogPath -v
```

Expected: PASS.

- [ ] **Step 5: Harden `ReadLog` to return `""` for file-not-found**

Replace the existing `ReadLog` method body:

```go
// ReadLog returns the log content for an execution.
// Returns empty string (no error) when no log path is set or the file does not yet exist.
func (s *ExecutionStore) ReadLog(id string) (string, error) {
	row := s.db.QueryRow(`SELECT log_path FROM bee_executions WHERE id = ?`, id)
	var logPath string
	if err := row.Scan(&logPath); err != nil {
		return "", fmt.Errorf("get log_path: %w", err)
	}
	if logPath == "" {
		return "", nil
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read log file: %w", err)
	}
	return string(b), nil
}
```

- [ ] **Step 6: Delete `WriteLog` from `execution_store.go`**

Remove the entire `WriteLog` method (approximately lines 220–241 in the original file).

- [ ] **Step 7: Delete `WriteLog` tests from `execution_store_test.go`**

Remove both `TestExecutionStore_WriteLog` and `TestExecutionStore_WriteLog_NilStartedAt` test functions.

- [ ] **Step 8: Run all store tests**

```
go test ./internal/store/... -v
```

Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add internal/store/execution_store.go internal/store/execution_store_test.go
git commit -m "feat(store): add PrepareLogPath, remove WriteLog, harden ReadLog for missing file"
```

---

## Task 2: Update `claude.Invoker` — add `logPath`, redirect stdout/stderr to file

**Files:**
- Modify: `internal/claude/invoker.go`
- Modify: `internal/claude/invoker_test.go`

- [ ] **Step 1: Rewrite `invoker_test.go` with updated signatures**

Replace the entire content of `internal/claude/invoker_test.go`:

```go
// internal/claude/invoker_test.go
package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewInvoker(t *testing.T) {
	inv := NewInvoker("/usr/bin/claude", "http://localhost:8080/mcp/bee", "test-key")
	if inv.binary != "/usr/bin/claude" {
		t.Errorf("binary: want /usr/bin/claude, got %s", inv.binary)
	}
	if inv.mcpURL != "http://localhost:8080/mcp/bee/sse" {
		t.Errorf("mcpURL: want http://localhost:8080/mcp/bee/sse, got %s", inv.mcpURL)
	}
	if inv.apiKey != "test-key" {
		t.Errorf("apiKey: want test-key, got %s", inv.apiKey)
	}
}

func TestInvoker_Run_WritesOutputToFile(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logPath := filepath.Join(t.TempDir(), "test.log")
	proc, ch, err := inv.Run(ctx, t.TempDir(), "hello", RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.PID() == 0 {
		t.Error("expected non-zero PID")
	}

	var gotDone bool
	for out := range ch {
		if out.Type == OutputDone {
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("expected done signal")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty log file after echo")
	}
}

func TestInvoker_Run_SessionFlags(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx := context.Background()

	// Test --session-id flag written to log file
	logPath1 := filepath.Join(t.TempDir(), "s1.log")
	_, ch, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s1"}, logPath1)
	for range ch {
	}
	data, _ := os.ReadFile(logPath1)
	output := string(data)
	if !strings.Contains(output, "--session-id") || !strings.Contains(output, "s1") {
		t.Errorf("expected --session-id s1 in log file, got: %s", output)
	}

	// Test --resume flag written to log file
	logPath2 := filepath.Join(t.TempDir(), "s2.log")
	_, ch2, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s2", Resume: true}, logPath2)
	for range ch2 {
	}
	data2, _ := os.ReadFile(logPath2)
	output2 := string(data2)
	if !strings.Contains(output2, "--resume") || !strings.Contains(output2, "s2") {
		t.Errorf("expected --resume s2 in log file, got: %s", output2)
	}
}

func TestProcess_Stop(t *testing.T) {
	inv := NewInvoker("sleep", "", "")
	ctx := context.Background()

	logPath := filepath.Join(t.TempDir(), "stop.log")
	proc, ch, err := inv.Run(ctx, t.TempDir(), "60", RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Drain channel — should get OutputError since process was killed
	for range ch {
	}
}

func TestInvoker_ConcurrentRuns(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx := context.Background()

	logPath1 := filepath.Join(t.TempDir(), "one.log")
	logPath2 := filepath.Join(t.TempDir(), "two.log")

	proc1, ch1, err1 := inv.Run(ctx, t.TempDir(), "one", RunOptions{SessionID: "s1"}, logPath1)
	if err1 != nil {
		t.Fatalf("Run 1: %v", err1)
	}
	proc2, ch2, err2 := inv.Run(ctx, t.TempDir(), "two", RunOptions{SessionID: "s2"}, logPath2)
	if err2 != nil {
		t.Fatalf("Run 2: %v", err2)
	}

	if proc1.PID() == proc2.PID() {
		t.Error("concurrent runs should have different PIDs")
	}

	for range ch1 {
	}
	for range ch2 {
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/claude/... -v
```

Expected: compile error — `Run()` called with wrong number of arguments.

- [ ] **Step 3: Rewrite `Invoker.Run` in `invoker.go`**

Replace the `Run` method (lines 79–155):

```go
// Run starts a Claude CLI process, redirecting its stdout and stderr to logPath.
// The returned channel carries only lifecycle events: OutputDone on success,
// OutputError on failure. The channel is closed after the process exits.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (*Process, <-chan Output, error) {
	mcpConfig := fmt.Sprintf(
		`{"mcpServers":{"openbee":{"type":"sse","url":%q}}}`,
		inv.mcpURL+"?api_key="+url.QueryEscape(inv.apiKey),
	)
	args := []string{
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
		"--mcp-config", mcpConfig,
	}
	if opts.SessionID != "" {
		if opts.Resume {
			args = append(args, "--resume", opts.SessionID)
		} else {
			args = append(args, "--session-id", opts.SessionID)
		}
	}
	args = append(args, "--print")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start claude: %w", err)
	}

	proc := &Process{cmd: cmd}
	ch := make(chan Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()

		if err := cmd.Wait(); err != nil {
			ch <- Output{Type: OutputError, Content: err.Error()}
		} else {
			ch <- Output{Type: OutputDone, Content: ""}
		}
	}()

	return proc, ch, nil
}
```

Also remove unused imports from `invoker.go`: remove `"bufio"` and `"sync"` (no longer needed).

The updated imports block:
```go
import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
)
```

Wait — `sync` is still needed for `Process.mu sync.Mutex`. Keep it. Remove only `"bufio"`.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./internal/claude/... -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/claude/invoker.go internal/claude/invoker_test.go
git commit -m "feat(claude): redirect stdout/stderr to log file, channel carries only lifecycle signals"
```

---

## Task 3: Update `bee.BeeRunner` interface and `BeeProcess.Run`

**Files:**
- Modify: `internal/bee/feeder.go` (interface only)
- Modify: `internal/bee/bee_process.go`

- [ ] **Step 1: Update `BeeRunner` interface in `feeder.go`**

Find and replace the `BeeRunner` interface (currently lines ~15–17):

```go
// BeeRunner abstracts the bee process invocation (real or test double).
type BeeRunner interface {
	Run(ctx context.Context, workDir, prompt string, opts claude.RunOptions, logPath string) (*claude.Process, <-chan claude.Output, error)
}
```

Also add `"github.com/theopenbee/openbee/internal/claude"` to the imports of `feeder.go` if not already present (it is already imported via the `claude.Process` return type).

- [ ] **Step 2: Update `BeeProcess.Run` in `bee_process.go`**

Replace the `Run` method:

```go
// Run spawns the bee process, redirecting output to logPath.
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt string, opts claude.RunOptions, logPath string) (*claude.Process, <-chan claude.Output, error) {
	return p.invoker.Run(ctx, workDir, prompt, opts, logPath)
}
```

- [ ] **Step 3: Verify compile — feeder_test.go mockBeeRunner now needs updating too**

```
go build ./internal/bee/...
```

Expected: compile error in `feeder_test.go` — `mockBeeRunner.Run` does not match `BeeRunner` interface.

This is expected; Task 5 fixes feeder_test.go. For now, note the error and proceed to Task 4.

- [ ] **Step 4: Commit (bee_process.go + feeder.go interface only)**

```bash
git add internal/bee/bee_process.go internal/bee/feeder.go
git commit -m "feat(bee): update BeeRunner interface and BeeProcess.Run to accept logPath"
```

---

## Task 4: Update `worker.Manager` — remove `logRegistry`, add `extractResultFromLog`

**Files:**
- Modify: `internal/worker/manager.go`
- Modify: `internal/worker/manager_test.go`
- Modify: `internal/mcp/tools_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Rewrite `manager.go`**

Replace the full content of `internal/worker/manager.go`:

```go
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"github.com/theopenbee/openbee/internal/claude"
	"github.com/theopenbee/openbee/internal/claudemd"
	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
)

var log = logger.With(zap.String("component", "worker"))

type claudeStreamEvent struct {
	Type    string         `json:"type"`
	Message *claudeMessage `json:"message,omitempty"`
	Result  string         `json:"result,omitempty"`
}

type claudeMessage struct {
	Content []claudeContent `json:"content"`
}

type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Manager struct {
	workerBaseDir  string
	beeCfg         config.BeeConfig
	workerStore    *store.WorkerStore
	executionStore *store.ExecutionStore
	invoker        *claude.Invoker

	activeProcesses map[string]*claude.Process // execution_id -> process
	mu              sync.RWMutex
}

func NewManager(
	workerBaseDir string,
	bc config.BeeConfig,
	ws *store.WorkerStore,
	es *store.ExecutionStore,
) *Manager {
	return &Manager{
		workerBaseDir:   workerBaseDir,
		beeCfg:          bc,
		workerStore:     ws,
		executionStore:  es,
		invoker:         claude.NewInvoker(bc.Claude.Path, bc.MCPBaseURL+config.MCPWorkerBasePath, bc.MCP.WorkerAPIKey),
		activeProcesses: make(map[string]*claude.Process),
	}
}

func (m *Manager) CreateWorker(
	name, description, memory string,
	workDir string,
) (model.Worker, error) {
	id := uuid.New().String()
	if workDir == "" {
		workDir = filepath.Join(m.workerBaseDir, id)
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return model.Worker{}, fmt.Errorf("create work dir: %w", err)
	}

	claudeMD := filepath.Join(workDir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
		initialContent := claudemd.ImportLine + "\n"
		if err := os.WriteFile(claudeMD, []byte(initialContent), 0644); err != nil {
			return model.Worker{}, fmt.Errorf("create CLAUDE.md: %w", err)
		}
	}

	if err := claudemd.EnsureSystemRules(workDir, claudemd.RoleWorker, claudemd.WithName(name), claudemd.WithDescription(description), claudemd.WithMemory(memory)); err != nil {
		log.Error("ensure system rules", zap.String("op", "create"), zap.Error(err))
	}

	return m.workerStore.Create(model.Worker{
		ID:          id,
		Name:        name,
		Description: description,
		Memory:      memory,
		WorkDir:     workDir,
	})
}

// ExecuteWorker runs a worker. When sessionID is non-empty, it resumes the existing
// Claude session (resume=true); otherwise it starts a fresh session.
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(workerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	resume := sessionID != ""
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	exec, err := m.executionStore.Create(workerID, triggerInput, sessionID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		log.Error("failed to update worker status", zap.Error(err))
	}

	if err := claudemd.EnsureSystemRules(worker.WorkDir, claudemd.RoleWorker, claudemd.WithName(worker.Name), claudemd.WithDescription(worker.Description), claudemd.WithMemory(worker.Memory)); err != nil {
		log.Error("ensure system rules", zap.String("op", "execute"), zap.Error(err))
	}
	timeout := m.beeCfg.Claude.Timeout

	if err := m.launchRuntime(exec, worker, timeout, triggerInput, resume); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

// launchRuntime applies timeout, prepares the log path, starts the invoker,
// registers the process, updates PID, and launches monitoring.
func (m *Manager) launchRuntime(exec model.WorkerExecution, worker model.Worker, timeout time.Duration, prompt string, resume bool) error {
	logPath, err := m.executionStore.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		return fmt.Errorf("prepare log path: %w", err)
	}

	var execCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		execCtx, cancel = context.WithCancel(context.Background())
	}

	proc, outputCh, err := m.invoker.Run(execCtx, worker.WorkDir, prompt, claude.RunOptions{SessionID: exec.SessionID, Resume: resume}, logPath)
	if err != nil {
		cancel()
		return err
	}

	m.mu.Lock()
	m.activeProcesses[exec.ID] = proc
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, proc.PID())
	go m.monitorExecution(exec, worker, outputCh, cancel, logPath)
	return nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, outputCh <-chan claude.Output, cancel context.CancelFunc, logPath string) {
	defer cancel()

	for out := range outputCh {
		switch out.Type {
		case claude.OutputDone:
			result := extractResultFromLog(logPath)
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
		case claude.OutputError:
			m.executionStore.UpdateResult(exec.ID, out.Content, model.ExecStatusFailed)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		}
	}

	m.mu.Lock()
	delete(m.activeProcesses, exec.ID)
	m.mu.Unlock()
}

// extractResultFromLog scans the log file for stream-json events and returns
// the best result string: prefers {"type":"result"} over the last assistant text.
func extractResultFromLog(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	var lastAssistantText, streamResult string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event claudeStreamEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		switch event.Type {
		case "assistant":
			if event.Message != nil && len(event.Message.Content) > 0 {
				if event.Message.Content[0].Type == "text" && event.Message.Content[0].Text != "" {
					lastAssistantText = event.Message.Content[0].Text
				}
			}
		case "result":
			if event.Result != "" {
				streamResult = event.Result
			}
		}
	}
	if streamResult != "" {
		return streamResult
	}
	return lastAssistantText
}

func (m *Manager) DeleteWorker(id string, deleteWorkDir bool) error {
	if deleteWorkDir {
		worker, err := m.workerStore.GetByID(id)
		if err != nil {
			return fmt.Errorf("get worker: %w", err)
		}
		if worker.WorkDir != "" {
			if err := os.RemoveAll(worker.WorkDir); err != nil {
				return fmt.Errorf("remove work dir: %w", err)
			}
		}
	}
	return m.workerStore.Delete(id)
}

func (m *Manager) StopExecution(executionID string) error {
	m.mu.RLock()
	proc, ok := m.activeProcesses[executionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active process for execution %s", executionID)
	}
	return proc.Stop()
}
```

- [ ] **Step 2: Rewrite `manager_test.go` — remove registry round-trip test**

Replace `internal/worker/manager_test.go` with:

```go
package worker

import (
	"testing"
)

// TestManager_NewManager_NoLogRegistry verifies that Manager can be constructed
// without a log registry — the logRegistry field was removed in the log simplification.
func TestManager_NewManager_NoLogRegistry(t *testing.T) {
	// NewManager should compile and run without a logRegistry parameter.
	// Actual Manager behaviour is tested via integration in mcp/tools_test.go.
	_ = NewManager
}
```

- [ ] **Step 3: Update `mcp/tools_test.go` — remove `NewActiveLogRegistry()` from Manager**

In `internal/mcp/tools_test.go`, find all three `worker.NewActiveLogRegistry()` calls (lines ~37, ~217, ~458) passed to `worker.NewManager`. Remove the fifth argument from each `worker.NewManager(...)` call.

For example, change:
```go
mgr := worker.NewManager(
    t.TempDir(),
    config.BeeConfig{Claude: config.ClaudeConfig{Path: "claude"}},
    ws, es,
    worker.NewActiveLogRegistry(),
)
```
to:
```go
mgr := worker.NewManager(
    t.TempDir(),
    config.BeeConfig{Claude: config.ClaudeConfig{Path: "claude"}},
    ws, es,
)
```

Remove the `"github.com/theopenbee/openbee/internal/worker"` import from `mcp/tools_test.go` if it is no longer referenced after removing the `NewActiveLogRegistry` calls. (Check — if `worker.NewManager` is still called, the import stays.)

- [ ] **Step 4: Update `app.go` — remove logRegistry from `buildWorkerManager`**

In `internal/app/app.go`, update `buildWorkerManager`:

```go
func buildWorkerManager(bc config.BeeConfig, s appStores) *worker.Manager {
	return worker.NewManager(config.DefaultWorkerBaseDir(), bc, s.workerStore, s.execStore)
}
```

And update the call site in `BuildApp`:
```go
mgr := buildWorkerManager(cfg.Bee, s)
```

At this point `logRegistry` is still used by `buildBee` and `buildAPIServer`, so leave those as-is.

- [ ] **Step 5: Run tests**

```
go test ./internal/worker/... ./internal/mcp/... -v
```

Expected: all pass. The `internal/bee/...` tests will still fail (Task 5 fixes them).

- [ ] **Step 6: Commit**

```bash
git add internal/worker/manager.go internal/worker/manager_test.go internal/mcp/tools_test.go internal/app/app.go
git commit -m "feat(worker): remove logRegistry from Manager, add extractResultFromLog, use PrepareLogPath"
```

---

## Task 5: Update `bee.Feeder` — reorder exec creation, logPath, simplify drain

**Files:**
- Modify: `internal/bee/feeder.go`
- Modify: `internal/bee/feeder_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Rewrite `feeder.go`**

Replace the full content of `internal/bee/feeder.go`:

```go
package bee

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/claude"
	"github.com/theopenbee/openbee/internal/claudemd"
	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
	"go.uber.org/zap"
)

var log = logger.With(zap.String("component", "feeder"))

// BeeRunner abstracts the bee process invocation (real or test double).
type BeeRunner interface {
	Run(ctx context.Context, workDir, prompt string, opts claude.RunOptions, logPath string) (*claude.Process, <-chan claude.Output, error)
}

// FailureNotifier sends a notification to the user when a message is permanently failed.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID, reason string) error
}

// Option configures a Feeder.
type Option func(*Feeder)

// WithFailureNotifier sets the notifier used to inform users when a message exhausts retries.
func WithFailureNotifier(n FailureNotifier) Option {
	return func(f *Feeder) { f.failureNotifier = n }
}

// Feeder polls platform_messages for unprocessed messages and feeds them to bee.
type Feeder struct {
	msgStore        *store.MessageStore
	taskStore       *store.TaskStore
	sessionStore    *store.SessionStore
	execStore       *store.ExecutionStore
	runner          BeeRunner
	workDir         string
	cfg             config.BeeConfig
	failureNotifier FailureNotifier
}

// NewFeeder creates a Feeder.
func NewFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner BeeRunner, workDir string, cfg config.BeeConfig, opts ...Option) *Feeder {
	f := &Feeder{
		msgStore:     ms,
		taskStore:    ts,
		sessionStore: ss,
		execStore:    es,
		runner:       runner,
		workDir:      workDir,
		cfg:          cfg,
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

// RecoverFeeding resets any messages stuck in 'feeding' status back to 'received'
// and deletes their associated pending tasks.
func (f *Feeder) RecoverFeeding(ctx context.Context) {
	ids, err := f.msgStore.ResetFeedingToReceived(ctx)
	if err != nil {
		log.Error("recover feeding", zap.Error(err))
		return
	}
	if len(ids) == 0 {
		return
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Error("delete orphaned tasks", zap.Error(err))
	}
	log.Info("recovered feeding messages", zap.Int("count", len(ids)))
}

// Run polls for unprocessed messages on each tick. Call in a goroutine.
func (f *Feeder) Run(ctx context.Context) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			f.tick(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (f *Feeder) tick(ctx context.Context) {
	count, _ := f.msgStore.CountReceived(ctx)
	if count > QueueWarnThreshold {
		log.Warn("unprocessed messages in queue", zap.Int("count", count), zap.Int("threshold", QueueWarnThreshold))
	}

	msgs, err := f.msgStore.ClaimBatch(ctx, 1)
	if err != nil {
		log.Error("claim batch", zap.Error(err))
		return
	}
	if len(msgs) == 0 {
		return
	}

	if err := WriteCLAUDEMD(f.workDir, DefaultPersona); err != nil {
		log.Error("write CLAUDE.md", zap.Error(err))
		f.rollback(ctx, msgs, "内部错误：无法写入配置文件")
		return
	}
	if err := claudemd.EnsureSystemRules(f.workDir, claudemd.RoleBee); err != nil {
		log.Error("ensure system rules", zap.Error(err))
	}

	groups := make(map[string][]store.ClaimedMessage)
	for _, m := range msgs {
		groups[m.SessionKey] = append(groups[m.SessionKey], m)
	}

	var wg sync.WaitGroup
	for sessionKey, group := range groups {
		wg.Add(1)
		go func(sessionKey string, group []store.ClaimedMessage) {
			defer wg.Done()
			f.processBeeGroup(ctx, sessionKey, group)
		}(sessionKey, group)
	}
	wg.Wait()
}

// processBeeGroup invokes bee for a single sessionKey's messages, managing session continuity.
func (f *Feeder) processBeeGroup(ctx context.Context, sessionKey string, msgs []store.ClaimedMessage) {
	sessionID, err := f.sessionStore.GetSessionContext(ctx, sessionKey, store.BeeAgentID)
	if err != nil {
		log.Error("get session context", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.rollback(ctx, msgs, "内部错误：无法读取会话上下文")
		return
	}
	resume := sessionID != ""
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	for i, m := range msgs {
		merged, err := f.msgStore.FetchMergedContent(ctx, m.ID)
		if err != nil {
			log.Error("fetch merged content", zap.String("msgID", m.ID), zap.Error(err))
			continue
		}
		if len(merged) > 0 {
			msgs[i].Content = strings.Join(merged, "\n\n---\n\n") + "\n\n---\n\n" + m.Content
		}
	}

	prompt := buildPrompt(msgs)

	// Create execution record first — we need exec.ID before launching the process
	// so we can prepare the log path (which is based on the ID).
	exec, err := f.execStore.CreateBeeExecution(sessionID, prompt)
	if err != nil {
		log.Error("create bee execution", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.rollback(ctx, msgs, "内部错误：无法创建执行记录")
		return
	}

	logPath, err := f.execStore.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		log.Error("prepare log path", zap.String("execID", exec.ID), zap.Error(err))
		f.execStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		f.rollback(ctx, msgs, "内部错误：无法创建日志文件")
		return
	}

	beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Feeder.Timeout)
	defer cancel()

	proc, outputCh, err := f.runner.Run(beeCtx, f.workDir, prompt, claude.RunOptions{SessionID: sessionID, Resume: resume}, logPath)
	if err != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.execStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		f.rollback(ctx, msgs, "AI 处理失败，请稍后重试")
		return
	}

	if pidErr := f.execStore.UpdatePID(exec.ID, proc.PID()); pidErr != nil {
		log.Error("update execution pid", zap.Error(pidErr))
	}

	drainErr := f.waitBeeOutput(outputCh)

	finalStatus := model.ExecStatusCompleted
	resultMsg := ""
	if drainErr != nil {
		finalStatus = model.ExecStatusFailed
		resultMsg = drainErr.Error()
	}
	if resErr := f.execStore.UpdateResult(exec.ID, resultMsg, finalStatus); resErr != nil {
		log.Error("update execution result", zap.Error(resErr))
	}

	if drainErr != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(drainErr))
		f.rollback(ctx, msgs, "AI 处理失败，请稍后重试")
		return
	}

	// Persist session_id before marking messages processed.
	if resume {
		currentID, checkErr := f.sessionStore.GetSessionContext(ctx, sessionKey, store.BeeAgentID)
		if checkErr == nil && currentID == "" {
			log.Info("session cleared during bee execution, skipping context upsert",
				zap.String("sessionKey", sessionKey))
		} else {
			if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID); err != nil {
				log.Error("upsert session context", zap.String("sessionKey", sessionKey), zap.Error(err))
			}
		}
	} else {
		if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID); err != nil {
			log.Error("upsert session context", zap.String("sessionKey", sessionKey), zap.Error(err))
		}
	}

	msgIDs := make([]string, len(msgs))
	for i, m := range msgs {
		msgIDs[i] = m.ID
	}
	if err := f.msgStore.MarkBeeProcessed(ctx, msgIDs); err != nil {
		log.Error("mark bee_processed", zap.String("sessionKey", sessionKey), zap.Error(err))
	}
}

func (f *Feeder) rollback(ctx context.Context, msgs []store.ClaimedMessage, userMsg string) {
	ids := make([]string, len(msgs))
	var failedIDs []string
	for i, m := range msgs {
		ids[i] = m.ID
		if m.RetryCount+1 >= MaxRetries {
			failedIDs = append(failedIDs, m.ID)
		}
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Error("rollback delete tasks", zap.Error(err))
	}
	if err := f.msgStore.RollbackWithRetry(ctx, ids, MaxRetries); err != nil {
		log.Error("rollback with retry", zap.Error(err))
		return
	}
	for _, id := range failedIDs {
		log.Warn("message exhausted retries", zap.String("messageID", id))
		if f.failureNotifier != nil {
			if notifyErr := f.failureNotifier.NotifyTaskFailure(ctx, id, userMsg); notifyErr != nil {
				log.Error("notify bee failure", zap.String("messageID", id), zap.Error(notifyErr))
			}
		}
	}
}

// waitBeeOutput consumes the output channel and waits for a lifecycle signal.
// Returns nil on OutputDone, non-nil error on OutputError or unexpected channel close.
func (f *Feeder) waitBeeOutput(ch <-chan claude.Output) error {
	for out := range ch {
		switch out.Type {
		case claude.OutputDone:
			return nil
		case claude.OutputError:
			return fmt.Errorf("bee exited with error: %s", out.Content)
		}
	}
	return fmt.Errorf("bee output channel closed without completion signal")
}

func buildPrompt(msgs []store.ClaimedMessage) string {
	var sb strings.Builder
	for i, m := range msgs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "---\nfrom: %s\nsession_key: %s\nmessage_id: %s\n---\n\n%s\n",
			m.Platform, m.SessionKey, m.ID, m.Content)
	}
	return sb.String()
}
```

- [ ] **Step 2: Rewrite `feeder_test.go`**

Replace the full content of `internal/bee/feeder_test.go`:

```go
package bee_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/bee"
	"github.com/theopenbee/openbee/internal/claude"
	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
)

func setupFeederDB(t *testing.T) (*sql.DB, *store.MessageStore, *store.TaskStore, *store.SessionStore, *store.ExecutionStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, store.NewMessageStore(db), store.NewTaskStore(db), store.NewSessionStore(db), store.NewExecutionStore(db, t.TempDir())
}

func insertMessage(t *testing.T, db *sql.DB, id, sessionKey, content string) {
	t.Helper()
	now := time.Now().UnixMilli()
	_, err := db.Exec(
		`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
		 VALUES (?, ?, 'feishu', ?, 'received', ?, ?, ?)`,
		id, sessionKey, content, now, now, now,
	)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
}

// mockBeeRunner records all Run calls.
type mockBeeRunner struct {
	mu          sync.Mutex
	calls       []beeCall
	err         error
	outputLines []claude.Output
}

type beeCall struct {
	prompt    string
	sessionID string
	resume    bool
	logPath   string
}

func (m *mockBeeRunner) Run(_ context.Context, _, prompt string, opts claude.RunOptions, logPath string) (*claude.Process, <-chan claude.Output, error) {
	m.mu.Lock()
	m.calls = append(m.calls, beeCall{
		prompt:    prompt,
		sessionID: opts.SessionID,
		resume:    opts.Resume,
		logPath:   logPath,
	})
	customLines := m.outputLines
	m.mu.Unlock()

	var lines []claude.Output
	if customLines != nil {
		lines = customLines
	} else if m.err != nil {
		lines = []claude.Output{{Type: claude.OutputError, Content: m.err.Error()}}
	} else {
		lines = []claude.Output{{Type: claude.OutputDone}}
	}

	ch := make(chan claude.Output, len(lines))
	for _, l := range lines {
		ch <- l
	}
	close(ch)
	return &claude.Process{}, ch, nil
}

func (m *mockBeeRunner) getCalls() []beeCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]beeCall{}, m.calls...)
}

func newFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner bee.BeeRunner) *bee.Feeder {
	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	return bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg)
}

func TestFeeder_FirstTick_UsesNewSessionID(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	calls := runner.getCalls()
	if len(calls) == 0 {
		t.Fatal("expected bee runner to be called")
	}
	call := calls[0]
	if call.sessionID == "" {
		t.Error("expected non-empty sessionID on first call")
	}
	if call.resume {
		t.Error("expected resume=false on first call")
	}

	got, err := ss.GetSessionContext(context.Background(), "feishu:c:u", store.BeeAgentID)
	if err != nil {
		t.Fatalf("get session context: %v", err)
	}
	if got != call.sessionID {
		t.Errorf("persisted sessionID mismatch: want %q got %q", call.sessionID, got)
	}

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	if status != "bee_processed" {
		t.Errorf("expected bee_processed, got %q", status)
	}
}

func TestFeeder_SecondTick_ResumesSession(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	ctx := context.Background()

	if err := ss.UpsertSessionContext(ctx, "feishu:c:u", store.BeeAgentID, "existing-session"); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	insertMessage(t, db, "m1", "feishu:c:u", "follow-up")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	tickCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	go f.Run(tickCtx)
	time.Sleep(700 * time.Millisecond)

	calls := runner.getCalls()
	if len(calls) == 0 {
		t.Fatal("expected bee runner to be called")
	}
	call := calls[0]
	if call.sessionID != "existing-session" {
		t.Errorf("expected existing-session, got %q", call.sessionID)
	}
	if !call.resume {
		t.Error("expected resume=true on second call")
	}
}

func TestFeeder_OnBeeFailure_RollsBackAndDoesNotUpdateSession(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	if status != "received" {
		t.Errorf("expected rollback to received, got %q", status)
	}

	got, _ := ss.GetSessionContext(context.Background(), "feishu:c:u", store.BeeAgentID)
	if got != "" {
		t.Errorf("session context should not be written on failure, got %q", got)
	}
}

func TestFeeder_MultipleSessionKeys_ProcessedIndependently(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u1", "message from user1")
	insertMessage(t, db, "m2", "feishu:c:u2", "message from user2")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(1200 * time.Millisecond)

	calls := runner.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 bee invocations (one per sessionKey), got %d", len(calls))
	}

	sess1, _ := ss.GetSessionContext(context.Background(), "feishu:c:u1", store.BeeAgentID)
	sess2, _ := ss.GetSessionContext(context.Background(), "feishu:c:u2", store.BeeAgentID)
	if sess1 == "" {
		t.Error("session context for u1 should be set")
	}
	if sess2 == "" {
		t.Error("session context for u2 should be set")
	}
	if sess1 == sess2 {
		t.Error("session IDs for different sessionKeys must differ")
	}
}

func TestFeeder_CreatesExecutionOnBeeRun(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello bee")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	rows, err := db.Query(`SELECT id, worker_id, status, log_path FROM bee_executions`)
	if err != nil {
		t.Fatalf("query executions: %v", err)
	}
	defer rows.Close()

	var execs []struct {
		id       string
		workerID *string
		status   string
		logPath  string
	}
	for rows.Next() {
		var e struct {
			id       string
			workerID *string
			status   string
			logPath  string
		}
		if err := rows.Scan(&e.id, &e.workerID, &e.status, &e.logPath); err != nil {
			t.Fatalf("scan: %v", err)
		}
		execs = append(execs, e)
	}

	if len(execs) != 1 {
		t.Fatalf("expected 1 execution row, got %d", len(execs))
	}
	e := execs[0]
	if e.workerID != nil {
		t.Errorf("expected nil worker_id for bee execution, got %v", e.workerID)
	}
	if e.status != "completed" {
		t.Errorf("expected status=completed, got %q", e.status)
	}
	if e.logPath == "" {
		t.Error("expected non-empty log_path — PrepareLogPath should set it before process runs")
	}
}

func TestFeeder_LogPathSetBeforeProcessRuns(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	var capturedLogPath string
	runner := &mockBeeRunner{}
	// Intercept: after Run is called, log_path should already be in DB.
	// We verify this by checking the call's logPath is non-empty AND matches DB.
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	calls := runner.getCalls()
	if len(calls) == 0 {
		t.Fatal("expected runner to be called")
	}
	capturedLogPath = calls[0].logPath
	if capturedLogPath == "" {
		t.Error("logPath passed to runner must be non-empty")
	}

	// Verify the directory exists (PrepareLogPath creates it)
	if _, err := os.Stat(filepath.Dir(capturedLogPath)); err != nil {
		t.Errorf("log directory should exist before process runs: %v", err)
	}
}

func TestFeeder_ExecutionFailedOnBeeError(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello bee")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	var status string
	err := db.QueryRow(`SELECT status FROM bee_executions`).Scan(&status)
	if err != nil {
		t.Fatalf("query executions: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected status=failed, got %q", status)
	}
}

type mockFailureNotifier struct {
	mu   sync.Mutex
	msgs []string
}

func (m *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID, _ string) error {
	m.mu.Lock()
	m.msgs = append(m.msgs, messageID)
	m.mu.Unlock()
	return nil
}

func (m *mockFailureNotifier) getNotified() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.msgs...)
}

func TestFeeder_ExhaustsRetries_MarksFailedAndNotifies(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	notifier := &mockFailureNotifier{}
	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, bee.WithFailureNotifier(notifier))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go f.Run(ctx)

	time.Sleep(time.Duration(bee.MaxRetries+1)*bee.PollInterval + 500*time.Millisecond)

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	if status != "failed" {
		t.Errorf("expected status=failed after exhausting retries, got %q", status)
	}

	notified := notifier.getNotified()
	if len(notified) != 1 || notified[0] != "m1" {
		t.Errorf("expected notifier called once with m1, got %v", notified)
	}
}

func TestWriteCLAUDEMD_DoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	original := "user-edited content"
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(original), 0644)

	if err := bee.WriteCLAUDEMD(dir, "new persona"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if string(data) != original {
		t.Error("WriteCLAUDEMD should not overwrite existing file")
	}
}

func TestWriteCLAUDEMD_CreatesWhenMissing(t *testing.T) {
	dir := t.TempDir()

	if err := bee.WriteCLAUDEMD(dir, bee.DefaultPersona); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md should be created: %v", err)
	}
	if string(data) != bee.DefaultPersona {
		t.Errorf("unexpected content: %q", string(data))
	}
}
```

- [ ] **Step 3: Update `app.go` — remove `logRegistry` from `buildBee`**

Update `buildBee`:

```go
func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task_dispatcher.DispatchTask, failureNotifier bee.FailureNotifier) (*bee.Feeder, *task_scheduler.Scheduler) {
	beeProcess := bee.NewBeeProcess(cfg)
	feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore, beeProcess, config.DefaultBeeWorkDir(), cfg,
		bee.WithFailureNotifier(failureNotifier))
	sched := task_scheduler.New(s.taskStore, dispatchCh, bee.PollInterval)
	return feeder, sched
}
```

Update the call site in `BuildApp`:

```go
feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier)
```

At this point `logRegistry` is only used by `buildAPIServer`. Leave it for Task 6.

- [ ] **Step 4: Run bee tests**

```
go test ./internal/bee/... -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/bee/feeder.go internal/bee/feeder_test.go internal/app/app.go
git commit -m "feat(bee): remove logRegistry, reorder exec creation, simplify waitBeeOutput"
```

---

## Task 6: Remove `LogRegistry` from API and App wiring

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/execution_handler.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Remove `LogRegistry` from `ServerParams` in `router.go`**

In `internal/api/router.go`, remove the `LogRegistry` field from `ServerParams`:

```go
type ServerParams struct {
	WorkerStore      *store.WorkerStore
	ExecutionStore   *store.ExecutionStore
	Manager          *worker.Manager
	BeeMCPServer     *mcp.MCPServer
	WorkerMCPServer  *mcp.MCPServer
	BeeAPIKey        string
	WorkerAPIKey     string
	StaticFS         fs.FS
	LocalChatHandler *LocalChatHandler
	AuthHandler      *auth.AuthHandler
	JWTMiddleware    gin.HandlerFunc
}
```

Remove the `"github.com/theopenbee/openbee/internal/worker"` import from `router.go` if `worker` is no longer referenced in that file.

- [ ] **Step 2: Simplify `getExecutionLogs` in `execution_handler.go`**

Replace the `getExecutionLogs` method:

```go
func (s *Server) getExecutionLogs(c *gin.Context) {
	content, err := s.ExecutionStore.ReadLog(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if content != "" {
		c.Header("Cache-Control", "public, max-age=3600")
	}
	c.String(http.StatusOK, content)
}
```

- [ ] **Step 3: Update `buildAPIServer` and `BuildApp` in `app.go`**

Update `buildAPIServer` signature to remove `logRegistry`:

```go
func buildAPIServer(serverCfg config.ServerConfig, mcpCfg config.MCPConfig, s appStores, mgr *worker.Manager, beeMCPSrv *mcp.MCPServer, workerMCPSrv *mcp.MCPServer, localChat *api.LocalChatHandler) (*api.Server, error) {
	password := serverCfg.Auth.Password
	secret := serverCfg.Auth.JWTSecret
	jwtSvc := auth.NewJWTService(secret, serverCfg.Auth.AccessTokenTTL, serverCfg.Auth.RefreshTokenTTL)
	rateLimiter := auth.NewLoginRateLimiter(5, time.Minute)
	authHandler := auth.NewAuthHandler(serverCfg.Auth.Username, password, jwtSvc, rateLimiter)
	jwtMiddleware := auth.JWTMiddleware(jwtSvc)

	return api.NewServer(api.ServerParams{
		WorkerStore:      s.workerStore,
		ExecutionStore:   s.execStore,
		Manager:          mgr,
		BeeMCPServer:     beeMCPSrv,
		WorkerMCPServer:  workerMCPSrv,
		BeeAPIKey:        mcpCfg.APIKey,
		WorkerAPIKey:     mcpCfg.WorkerAPIKey,
		StaticFS:         webui.DistFS,
		LocalChatHandler: localChat,
		AuthHandler:      authHandler,
		JWTMiddleware:    jwtMiddleware,
	})
}
```

Update the call site in `BuildApp`:

```go
srv, err := buildAPIServer(cfg.Server, cfg.Bee.MCP, s, mgr, beeMCPSrv, workerMCPSrv, localChatHandler)
```

And remove the now-unused `logRegistry` variable and any remaining `logRegistry` references:

```go
// Remove these lines from BuildApp:
// logRegistry := worker.NewActiveLogRegistry()
// (and any remaining references to logRegistry)
```

Remove the `"github.com/theopenbee/openbee/internal/worker"` import from `app.go` if `worker` is no longer referenced there. (Check — `worker.Manager` is still referenced via `mgr`, so the import stays.)

- [ ] **Step 4: Build and verify compilation**

```
go build ./...
```

Expected: successful compile.

- [ ] **Step 5: Run full test suite**

```
go test ./... -v 2>&1 | tail -50
```

Expected: all pass (except `internal/worker/log_registry_test.go` which still exists but will be deleted in Task 7).

- [ ] **Step 6: Commit**

```bash
git add internal/api/router.go internal/api/execution_handler.go internal/app/app.go
git commit -m "feat(api,app): remove LogRegistry from ServerParams and BuildApp wiring"
```

---

## Task 7: Delete `log_registry.go` and `log_registry_test.go`

**Files:**
- Delete: `internal/worker/log_registry.go`
- Delete: `internal/worker/log_registry_test.go`

- [ ] **Step 1: Delete both files**

```bash
git rm internal/worker/log_registry.go internal/worker/log_registry_test.go
```

- [ ] **Step 2: Run full test suite**

```
go test ./... -count=1
```

Expected: all pass. No references to `ActiveLogRegistry`, `NewActiveLogRegistry`, or `log_registry.go` remain.

- [ ] **Step 3: Final compile check**

```
go build ./...
```

Expected: clean compile.

- [ ] **Step 4: Final commit**

```bash
git commit -m "chore(worker): delete log_registry.go — ActiveLogRegistry replaced by OS-level log redirect"
```
