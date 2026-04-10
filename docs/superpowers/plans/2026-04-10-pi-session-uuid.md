# PI Session UUID Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace PI's random-hex session file naming with UUID-based naming, eliminating the "generate-then-overwrite" session ID pattern and all `OutputSessionID` emission from the PI adapter.

**Architecture:** The PI invoker will deterministically construct a session file path from the UUID passed in `RunOptions.SessionID`, removing both `newSessionPath()` and the `OutputSessionID` output event. The feeder and manager layers stop listening for `OutputSessionID` and stop overwriting the session ID after execution. The UUID flows unchanged from generation through persistence.

**Tech Stack:** Go, `os.UserHomeDir`, existing `ai.RunOptions`, existing store interfaces.

---

### Task 1: Replace PI invoker session path logic

**Files:**
- Modify: `internal/ai/pi/invoker.go`

- [ ] **Step 1: Read the current Run function**

Open `internal/ai/pi/invoker.go`. Note that `newSessionPath()` generates a random-hex path, and `Run()` emits `OutputSessionID` immediately after starting the process (line 149).

- [ ] **Step 2: Replace `newSessionPath` with `resolveSessionPath`**

Delete `newSessionPath()` entirely (lines 91–105). Add this function in its place:

```go
func resolveSessionPath(sessionID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".pi", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir session dir: %w", err)
	}
	return filepath.Join(dir, sessionID+".jsonl"), nil
}
```

- [ ] **Step 3: Update `Run()` to use `resolveSessionPath`**

Replace the session path block at the top of `Run()`:

```go
// Before (lines 117–127):
sessionPath := opts.SessionID
if !opts.Resume {
    var err error
    sessionPath, err = newSessionPath()
    if err != nil {
        return nil, nil, fmt.Errorf("pi session path: %w", err)
    }
}

// After:
sessionPath, err := resolveSessionPath(opts.SessionID)
if err != nil {
    return nil, nil, fmt.Errorf("pi session path: %w", err)
}
```

- [ ] **Step 4: Remove OutputSessionID emission from `Run()`**

Delete this line from `Run()` (currently line 149):

```go
ch <- ai.Output{Type: ai.OutputSessionID, Content: sessionPath}
```

Change the channel buffer size from 2 to 1 (since we now only send one event — Done or Error):

```go
ch := make(chan ai.Output, 1)
```

- [ ] **Step 5: Remove unused imports**

Remove `"crypto/rand"` and `"encoding/hex"` from the import block — they were only used by `newSessionPath()`.

- [ ] **Step 6: Update the `Run()` doc comment**

Replace the existing comment on `Run()` with:

```go
// Run starts a pi CLI process, redirecting stdout+stderr to logPath.
// opts.SessionID must be a UUID; the session file path is derived as
// ~/.openbee/.pi/sessions/{sessionID}.jsonl. If the file already exists
// and opts.Resume is true, pi resumes that session; otherwise pi creates it.
```

- [ ] **Step 7: Verify the file compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go build ./internal/ai/pi/...
```

Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add internal/ai/pi/invoker.go
git commit -m "refactor(pi): use UUID-based session file path, drop OutputSessionID emission"
```

---

### Task 2: Update PI invoker tests

**Files:**
- Modify: `internal/ai/pi/invoker_test.go`

- [ ] **Step 1: Write a failing test for `resolveSessionPath`**

Add this test to `invoker_test.go`:

```go
func TestResolveSessionPath_UsesUUID(t *testing.T) {
	sessionID := "4d0ce91b-0856-44e2-b0d7-7765d824bba3"
	got, err := resolveSessionPath(sessionID)
	if err != nil {
		t.Fatalf("resolveSessionPath: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".openbee", ".pi", "sessions", sessionID+".jsonl")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/ai/pi/... -run TestResolveSessionPath_UsesUUID -v
```

Expected: PASS (the function was implemented in Task 1).

- [ ] **Step 3: Replace `TestInvoker_Run_EmitsSessionID` with a no-OutputSessionID test**

Delete `TestInvoker_Run_EmitsSessionID` (lines 77–96) and add:

```go
func TestInvoker_Run_NoSessionIDOutput(t *testing.T) {
	inv := NewInvoker("true", "http://localhost:8080", nil)
	logPath := filepath.Join(t.TempDir(), "pi.log")

	_, ch, err := inv.Run(context.Background(), t.TempDir(), "hello",
		ai.RunOptions{SessionID: "test-session-uuid"}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for out := range ch {
		if out.Type == ai.OutputSessionID {
			t.Errorf("unexpected OutputSessionID event with content %q", out.Content)
		}
	}
}
```

- [ ] **Step 4: Run all PI tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/ai/pi/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/pi/invoker_test.go
git commit -m "test(pi): replace EmitsSessionID test with NoSessionIDOutput + resolveSessionPath test"
```

---

### Task 3: Simplify `feeder.go` — remove engineSessionID overwrite

**Files:**
- Modify: `internal/domain/bee/feeder.go`

- [ ] **Step 1: Change `waitBeeOutput` signature to return only `error`**

Locate `waitBeeOutput` (around line 292). Replace the entire function:

```go
// waitBeeOutput consumes the output channel and waits for a lifecycle signal.
// Returns nil on OutputDone, non-nil on OutputError or channel close without signal.
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

Find this block (around lines 213–219):

```go
engineSessionID, drainErr := f.waitBeeOutput(outputCh)
if engineSessionID != "" {
    sessionID = engineSessionID
    if err := f.execStore.UpdateSessionID(exec.ID, sessionID); err != nil {
        log.Error("update execution session id", zap.String("execID", exec.ID), zap.Error(err))
    }
}
```

Replace with:

```go
drainErr := f.waitBeeOutput(outputCh)
```

- [ ] **Step 3: Verify the file compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go build ./internal/domain/bee/...
```

Expected: no errors.

- [ ] **Step 4: Run existing tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/domain/bee/... -v
```

Expected: all PASS (or same result as before this change).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/bee/feeder.go
git commit -m "refactor(feeder): remove engineSessionID overwrite, simplify waitBeeOutput"
```

---

### Task 4: Remove `OutputSessionID` handling from `manager.go`

**Files:**
- Modify: `internal/domain/worker/manager.go`

- [ ] **Step 1: Remove the `OutputSessionID` case from `monitorExecution`**

Locate `monitorExecution` (around line 157). Delete these lines from the `for out := range outputCh` loop:

```go
case ai.OutputSessionID:
    // Persist the engine-assigned session identifier (e.g. pi session file path)
    // so the dispatcher can store it for future resumes.
    if err := m.executionStore.UpdateSessionID(exec.ID, out.Content); err != nil {
        log.Error("update session id", zap.String("execID", exec.ID), zap.Error(err))
    }
```

The switch statement should now only handle `OutputDone` and `OutputError`.

- [ ] **Step 2: Verify the file compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go build ./internal/domain/worker/...
```

Expected: no errors.

- [ ] **Step 3: Run all tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./... 2>&1 | tail -30
```

Expected: all PASS (or no new failures introduced).

- [ ] **Step 4: Commit**

```bash
git add internal/domain/worker/manager.go
git commit -m "refactor(manager): remove OutputSessionID handling — pi now uses stable UUID paths"
```
