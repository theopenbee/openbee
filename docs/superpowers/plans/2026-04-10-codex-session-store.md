# Codex Session Store Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decouple codex session management by having the codex adapter internally maintain an openbee-uuid → codex thread_id mapping on disk, so feeder and manager no longer need to handle `OutputSessionID` events.

**Architecture:** A new `SessionStore` (per-session files under `~/.openbee/.codex/sessions/`) is owned by the codex `Invoker`. On first run the invoker stores the thread_id; on resume it looks up the thread_id. From feeder/manager's perspective, codex behaves identically to claude and pi.

**Tech Stack:** Go standard library (`os`, `path/filepath`, `sync` not needed — per-file approach avoids locking)

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/ai/codex/session_store.go` | Create | SessionStore: Get/Set per-session files |
| `internal/ai/codex/session_store_test.go` | Create | Unit tests for SessionStore |
| `internal/ai/codex/invoker.go` | Modify | Accept store, use Get/Set, remove OutputSessionID emit |
| `internal/ai/codex/invoker_test.go` | Modify | Update tests for new invoker signature |
| `internal/ai/codex/adapter.go` | Modify | Create SessionStore in NewAdapter, pass to Invoker |
| `internal/domain/bee/feeder.go` | Modify | Simplify waitBeeOutput, remove engineSessionID handling |
| `internal/domain/worker/manager.go` | Modify | Remove OutputSessionID case from monitorExecution |
| `internal/ai/engine.go` | Modify | Remove OutputSessionID constant |
| `internal/ai/pi/invoker_test.go` | Modify | Remove OutputSessionID guard assertion |
| `internal/infra/store/execution_store.go` | Modify | Remove dead UpdateSessionID method |

---

### Task 1: SessionStore — tests

**Files:**
- Create: `internal/ai/codex/session_store_test.go`

- [ ] **Step 1: Write failing tests**

```go
package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionStore_GetMissing(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}
	_, ok := store.Get("nonexistent-uuid")
	if ok {
		t.Fatal("expected ok=false for missing session")
	}
}

func TestSessionStore_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	if err := store.Set("openbee-uuid-1", "codex-thread-abc"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	threadID, ok := store.Get("openbee-uuid-1")
	if !ok {
		t.Fatal("expected ok=true after Set")
	}
	if threadID != "codex-thread-abc" {
		t.Errorf("got %q, want %q", threadID, "codex-thread-abc")
	}
}

func TestSessionStore_SetOverwrite(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	store.Set("uuid-1", "thread-v1")
	store.Set("uuid-1", "thread-v2")

	threadID, ok := store.Get("uuid-1")
	if !ok || threadID != "thread-v2" {
		t.Errorf("got (%q, %v), want (thread-v2, true)", threadID, ok)
	}
}

func TestSessionStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	if err := store.Set("uuid-atomic", "thread-xyz"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// No temp files should be left behind
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestNewSessionStore(t *testing.T) {
	// Use a non-existent subdir to verify auto-creation
	parent := t.TempDir()
	dir := filepath.Join(parent, "deep", "sessions")
	store, err := newSessionStoreAt(dir)
	if err != nil {
		t.Fatalf("newSessionStoreAt: %v", err)
	}
	if _, statErr := os.Stat(store.dir); statErr != nil {
		t.Errorf("sessions dir not created: %v", statErr)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /path/to/openbee
go test ./internal/ai/codex/... -run TestSessionStore -v
```

Expected: compilation error — `SessionStore` not defined yet.

---

### Task 2: SessionStore — implementation

**Files:**
- Create: `internal/ai/codex/session_store.go`

- [ ] **Step 1: Implement SessionStore**

```go
package codex

import (
	"fmt"
	"os"
	"path/filepath"
)

// SessionStore maps openbee session UUIDs to codex thread IDs on disk.
// Each session is stored as a single file named by the openbee UUID.
// Per-session files eliminate concurrent write conflicts between sessions.
type SessionStore struct {
	dir string // ~/.openbee/.codex/sessions/
}

// NewSessionStore creates a SessionStore rooted at the default location.
func NewSessionStore() (*SessionStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".codex", "sessions")
	return newSessionStoreAt(dir)
}

func newSessionStoreAt(dir string) (*SessionStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &SessionStore{dir: dir}, nil
}

// Get returns the codex thread_id for the given openbee UUID, or ("", false) if not found.
func (s *SessionStore) Get(openbeeUUID string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(s.dir, openbeeUUID))
	if err != nil {
		return "", false
	}
	threadID := string(data)
	if threadID == "" {
		return "", false
	}
	return threadID, true
}

// Set atomically writes the codex thread_id for the given openbee UUID.
func (s *SessionStore) Set(openbeeUUID, threadID string) error {
	dest := filepath.Join(s.dir, openbeeUUID)
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, []byte(threadID), 0o644); err != nil {
		return fmt.Errorf("write temp session file: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename session file: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
go test ./internal/ai/codex/... -run TestSessionStore -v
```

Expected: all 5 `TestSessionStore_*` and `TestNewSessionStore` tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/ai/codex/session_store.go internal/ai/codex/session_store_test.go
git commit -m "feat(codex): add SessionStore for openbee-uuid to thread_id mapping"
```

---

### Task 3: Update Invoker to use SessionStore

**Files:**
- Modify: `internal/ai/codex/invoker.go`

The invoker currently extracts the thread_id from the `thread.started` event and emits `OutputSessionID`. After this change it stores the mapping internally instead and never emits `OutputSessionID`.

- [ ] **Step 1: Write a test that verifies no OutputSessionID is emitted (add to invoker_test.go)**

Add to `internal/ai/codex/invoker_test.go`:

```go
func TestBuildArgs_ResumeUsesThreadID(t *testing.T) {
	// buildArgs receives the resolved thread_id, not the openbee UUID
	args := buildArgs("thread-xyz-from-store", true, "follow up")
	want := []string{"exec", "resume", "thread-xyz-from-store", "--json", "--dangerously-bypass-approvals-and-sandbox", "follow up"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}
```

- [ ] **Step 2: Run the new test to verify it passes** (buildArgs signature unchanged)

```bash
go test ./internal/ai/codex/... -run TestBuildArgs -v
```

Expected: all `TestBuildArgs_*` tests PASS.

- [ ] **Step 3: Replace invoker.go with the updated version**

Replace the full content of `internal/ai/codex/invoker.go`:

```go
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	"go.uber.org/zap"
)

var log = zap.L().Named("codex")

// Invoker spawns Codex CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
	store   *SessionStore
}

// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
func NewInvoker(binary, openbeeURL string, store *SessionStore) *Invoker {
	return &Invoker{binary: binary, baseEnv: ai.BuildBaseEnv(openbeeURL), store: store}
}

type codexEvent struct {
	Type     string     `json:"type"`
	ThreadID string     `json:"thread_id,omitempty"`
	Item     *codexItem `json:"item,omitempty"`
}

type codexItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func buildArgs(threadID string, resume bool, prompt string) []string {
	if resume && threadID != "" {
		args := []string{"exec", "resume", threadID, "--json", "--dangerously-bypass-approvals-and-sandbox"}
		if prompt != "" {
			args = append(args, prompt)
		}
		return args
	}
	return []string{"exec", "-", "--json", "--dangerously-bypass-approvals-and-sandbox"}
}

// extractSessionID reads a Codex JSON stream and returns the thread_id from
// the first "thread.started" event, or "" if not found.
func extractSessionID(r io.Reader) string {
	var threadID string
	ai.ScanJSONLines(r, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == "thread.started" && event.ThreadID != "" {
			threadID = event.ThreadID
			return false
		}
		return true
	})
	return threadID
}

// ExtractResultFromLog scans a Codex JSON log file and returns the text of the
// last agent_message item, or "" if none found.
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	ai.ScanJSONLines(f, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == "item.completed" && event.Item != nil &&
			event.Item.Type == "agent_message" && event.Item.Text != "" {
			lastText = event.Item.Text
		}
		return true
	})
	return lastText
}

// Run starts a Codex CLI process, redirecting output to logPath.
// For new sessions (Resume=false), prompt is passed via stdin.
// For resume sessions, the codex thread_id is resolved from the SessionStore
// using opts.SessionID (the openbee UUID) before building the command.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	threadID, resume := inv.resolveThread(opts.SessionID, opts.Resume)
	args := buildArgs(threadID, resume, prompt)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	pr, pw := io.Pipe()
	writer := io.MultiWriter(logFile, pw)

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdout = writer
	cmd.Stderr = logFile
	cmd.Env = append(inv.baseEnv, "OPENBEE_API_KEY="+opts.APIKey)

	if !resume {
		cmd.Stdin = strings.NewReader(prompt)
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		pr.Close()
		pw.Close()
		return nil, nil, fmt.Errorf("start codex: %w", err)
	}

	proc := ai.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 2)

	go func() {
		defer close(ch)
		defer logFile.Close()

		doneCh := make(chan error, 1)
		go func() {
			doneCh <- cmd.Wait()
			pw.Close()
		}()

		if !resume {
			if newThreadID := extractSessionID(pr); newThreadID != "" {
				if err := inv.store.Set(opts.SessionID, newThreadID); err != nil {
					log.Error("store codex session", zap.String("uuid", opts.SessionID), zap.Error(err))
				}
			}
		}
		io.Copy(io.Discard, pr)

		if err := <-doneCh; err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
		} else {
			ch <- ai.Output{Type: ai.OutputDone}
		}
	}()

	return proc, ch, nil
}

// resolveThread maps an openbee UUID to a codex thread_id for resume.
// If the mapping is not found, it falls back to a new session and logs a warning.
func (inv *Invoker) resolveThread(openbeeUUID string, resume bool) (threadID string, resolvedResume bool) {
	if !resume {
		return "", false
	}
	threadID, ok := inv.store.Get(openbeeUUID)
	if !ok {
		log.Warn("codex session mapping not found, starting new session", zap.String("uuid", openbeeUUID))
		return "", false
	}
	return threadID, true
}
```

- [ ] **Step 4: Run all codex tests**

```bash
go test ./internal/ai/codex/... -v
```

Expected: all existing tests PASS. Note: `TestExtractSessionID` still passes because `extractSessionID` function is unchanged.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/codex/invoker.go internal/ai/codex/invoker_test.go
git commit -m "feat(codex): invoker uses SessionStore, removes OutputSessionID emission"
```

---

### Task 4: Update Adapter to wire SessionStore

**Files:**
- Modify: `internal/ai/codex/adapter.go`

- [ ] **Step 1: Update adapter.go**

Replace the full content of `internal/ai/codex/adapter.go`:

```go
package codex

import (
	"context"
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineCodex, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = ai.EngineCodex
		}
		return NewAdapter(path, cfg.OpenbeeURL)
	})
}

type codexAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string) (ai.EngineAdapter, error) {
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init codex session store: %w", err)
	}
	return &codexAdapter{invoker: NewInvoker(binaryPath, openbeeURL, store)}, nil
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

Note: `NewAdapter` now returns `(ai.EngineAdapter, error)` — the `init()` registration pattern in the registry already accepts factory errors, so this is consistent with how pi adapter works.

- [ ] **Step 2: Update adapter_test.go for new NewAdapter signature**

`adapter_test.go` calls `codex.NewAdapter(...)` without error handling. Update both call sites:

```go
// internal/ai/codex/adapter_test.go

func TestAdapter_Prepare_NoOp(t *testing.T) {
	dir := t.TempDir()
	a, err := codex.NewAdapter("echo", "http://localhost:9999")
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}

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
	a, err := codex.NewAdapter("echo", "http://localhost:9999")
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		if err := a.Prepare(dir, ai.PrepareOptions{Role: role}); err != nil {
			t.Errorf("Prepare(%s): %v", role, err)
		}
	}
}
```

- [ ] **Step 4: Build to check for compilation errors**

```bash
go build ./internal/ai/codex/...
```

Expected: no errors.

- [ ] **Step 5: Run full codex tests**

```bash
go test ./internal/ai/codex/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/codex/adapter.go internal/ai/codex/adapter_test.go
git commit -m "feat(codex): wire SessionStore into adapter"
```

---

### Task 5: Simplify feeder.go — remove OutputSessionID handling

**Files:**
- Modify: `internal/domain/bee/feeder.go`

Currently `waitBeeOutput` returns `(string, error)` — the string is `engineSessionID` used only when codex returns a different thread_id. After this change it returns just `error`.

- [ ] **Step 1: Update `waitBeeOutput` signature and body**

Find `waitBeeOutput` at around line 293. Replace it:

```go
// waitBeeOutput consumes the output channel and waits for a lifecycle signal.
// Returns nil on OutputDone, or an error on OutputError or channel close without signal.
func (f *Feeder) waitBeeOutput(ch <-chan ai.Output) error {
	for out := range ch {
		switch out.Type {
		case ai.OutputDone:
			return nil
		case ai.OutputError:
			return errors.New(out.Content)
		}
	}
	return fmt.Errorf("output channel closed without completion signal")
}
```

- [ ] **Step 2: Update the call site in `processBeeGroup`**

Find the block starting around line 211:

```go
engineSessionID, drainErr := f.waitBeeOutput(outputCh)
if engineSessionID != "" {
    sessionID = engineSessionID
    if err := f.execStore.UpdateSessionID(exec.ID, engineSessionID); err != nil {
        log.Error("update bee execution session id", zap.Error(err))
    }
}
```

Replace it with:

```go
drainErr := f.waitBeeOutput(outputCh)
```

- [ ] **Step 3: Build to check for compilation errors**

```bash
go build ./internal/domain/bee/...
```

Expected: no errors. If `UpdateSessionID` is referenced via the interface, it may still compile — that's fine, we clean up the interface in Task 7.

- [ ] **Step 4: Run feeder tests**

```bash
go test ./internal/domain/bee/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/bee/feeder.go
git commit -m "refactor(bee): remove OutputSessionID handling from feeder"
```

---

### Task 6: Simplify manager.go — remove OutputSessionID handling

**Files:**
- Modify: `internal/domain/worker/manager.go`

- [ ] **Step 1: Remove the OutputSessionID case from monitorExecution**

Find `monitorExecution` around line 157. The current switch:

```go
for out := range outputCh {
    switch out.Type {
    case ai.OutputSessionID:
        if err := m.executionStore.UpdateSessionID(exec.ID, out.Content); err != nil {
            log.Error("update execution session id", zap.String("execID", exec.ID), zap.Error(err))
        }
    case ai.OutputDone:
        result := m.engine.ExtractResult(logPath)
        m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
        m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
    case ai.OutputError:
        result := m.engine.ExtractResult(logPath)
        if result == "" {
            result = out.Content
        }
        m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusFailed)
        m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
    }
}
```

Replace with:

```go
for out := range outputCh {
    switch out.Type {
    case ai.OutputDone:
        result := m.engine.ExtractResult(logPath)
        m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
        m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
    case ai.OutputError:
        result := m.engine.ExtractResult(logPath)
        if result == "" {
            result = out.Content
        }
        m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusFailed)
        m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
    }
}
```

- [ ] **Step 2: Build and test**

```bash
go build ./internal/domain/worker/...
go test ./internal/domain/worker/... -v
```

Expected: all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/worker/manager.go
git commit -m "refactor(worker): remove OutputSessionID handling from manager"
```

---

### Task 7: Remove OutputSessionID constant and dead UpdateSessionID

**Files:**
- Modify: `internal/ai/engine.go`
- Modify: `internal/ai/pi/invoker_test.go`
- Modify: `internal/infra/store/execution_store.go`

- [ ] **Step 1: Remove `OutputSessionID` from engine.go**

In `internal/ai/engine.go`, find and remove the `OutputSessionID` line:

```go
// Remove this line:
OutputSessionID OutputType = "session_id"
```

The const block becomes:

```go
const (
	OutputDone  OutputType = "done"
	OutputError OutputType = "error"
)
```

- [ ] **Step 2: Remove OutputSessionID guard from pi/invoker_test.go**

In `internal/ai/pi/invoker_test.go`, around line 104–108, remove:

```go
for out := range ch {
    if out.Type == ai.OutputSessionID {
        t.Errorf("unexpected OutputSessionID event with content %q", out.Content)
    }
}
```

Replace with:

```go
for range ch {
}
```

- [ ] **Step 3: Remove dead UpdateSessionID from execution_store.go**

In `internal/infra/store/execution_store.go`, find and remove the `UpdateSessionID` method (around line 192–200). Also remove it from the `ExecutionStore` interface if it is defined there.

Run a grep first to confirm the interface location:

```bash
grep -n "UpdateSessionID" internal/infra/store/execution_store.go internal/domain/bee/feeder.go internal/domain/worker/manager.go
```

Remove `UpdateSessionID` from the interface definition and the concrete implementation.

- [ ] **Step 4: Build the entire project**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Run all tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/engine.go internal/ai/pi/invoker_test.go internal/infra/store/execution_store.go
git commit -m "refactor: remove OutputSessionID output type and dead UpdateSessionID store method"
```

---

## Migration Note

Existing deployed instances that used codex will have codex `thread_id` values stored in the `bee_session_contexts` database table. After this change, those values are treated as openbee UUIDs by the adapter. Since the `sessions/` directory will be empty, `store.Get` will return `false`, and the adapter will fall back to starting a new codex session. Existing sessions will not resume — they will start fresh. This is acceptable for a development branch.
