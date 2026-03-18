# Bee Logs → executions Table Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace file-based bee process logging (`~/.openbee/bee-logs/`) with rows in the `executions` table, one row per bee invocation.

**Architecture:** Add migration 16 to make `worker_id` nullable (NULL = bee execution); change `model.WorkerExecution.WorkerID` to `*string`; add `CreateBeeExecution` to `ExecutionStore`; wire `execStore` into `Feeder`; rewrite `drainBeeOutput` to return accumulated logs as a string; update `processBeeGroup` to create/update execution rows.

**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), `database/sql`, `strings.Builder`

---

## File Map

| File | Action | What changes |
|------|--------|--------------|
| `internal/store/db.go` | Modify | Add migration 16 (drop + recreate executions) |
| `internal/model/execution.go` | Modify | `WorkerID string` → `*string` with `omitempty` |
| `internal/store/execution_store.go` | Modify | Fix `Create` (`&workerID`); fix `scanExecution` for `*string`; add `CreateBeeExecution` |
| `internal/store/execution_store_test.go` | Modify | Fix `WorkerID` comparison; add `CreateBeeExecution` test |
| `internal/bee/feeder.go` | Modify | Add `execStore` field; reorder `processBeeGroup`; rewrite `drainBeeOutput` |
| `internal/bee/feeder_test.go` | Modify | Update `newFeeder` helper; add execution lifecycle tests |
| `internal/app/app.go` | Modify | Pass `execStore` to `buildBee` / `bee.NewFeeder` |

---

## Task 1: Add migration 16

**Files:**
- Modify: `internal/store/db.go`

Migration 16 drops and recreates the `executions` table with `worker_id TEXT` (nullable, no FK). This discards existing rows — acceptable for a pre-release project.

- [ ] **Step 1: Add migration 16 to `internal/store/db.go`**

Open `internal/store/db.go`. After the last migration entry (version 15), append:

```go
{
    version: 16,
    name:    "20260318_make_executions_worker_id_nullable",
    sql: `DROP TABLE IF EXISTS executions;
CREATE TABLE executions (
    id             TEXT PRIMARY KEY,
    worker_id      TEXT,
    session_id     TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    ai_process_pid INTEGER NOT NULL DEFAULT 0,
    trigger_input  TEXT NOT NULL DEFAULT '',
    result         TEXT NOT NULL DEFAULT '',
    logs           TEXT NOT NULL DEFAULT '',
    started_at     INTEGER,
    completed_at   INTEGER
);
CREATE INDEX idx_executions_worker_id ON executions(worker_id);
CREATE INDEX idx_executions_session_id ON executions(session_id)`,
},
```

> Note: `applyMigrations` runs each migration in its own transaction via `tx.Exec(m.sql)`. SQLite allows multiple statements in one `Exec` call when using `modernc.org/sqlite`. Confirm this works by running the tests.

- [ ] **Step 2: Verify migration compiles and existing tests still pass**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee
go test ./internal/store/... -v -run TestDB 2>&1 | head -40
go build ./...
```

Expected: no compile errors. The `store` package tests use fresh `t.TempDir()` DBs, so migration 16 runs cleanly.

- [ ] **Step 3: Commit**

```bash
git add internal/store/db.go
git commit -m "feat: add migration 16 to make executions.worker_id nullable"
```

---

## Task 2: Update `model.WorkerExecution.WorkerID` to `*string`

**Files:**
- Modify: `internal/model/execution.go`
- Modify: `internal/store/execution_store.go` (compilation fixes)
- Modify: `internal/store/execution_store_test.go` (compilation fix)

This is a breaking type change. Fix all compilation errors in one commit.

- [ ] **Step 1: Update the model**

In `internal/model/execution.go`, change line 13:

```go
// Before
WorkerID     string          `json:"worker_id" db:"worker_id"`

// After
WorkerID     *string         `json:"worker_id,omitempty" db:"worker_id"`
```

- [ ] **Step 2: Fix `execution_store.go` — `Create` method**

In `internal/store/execution_store.go`, the `Create` method sets `WorkerID: workerID` (string). Change to pointer:

```go
func (s *ExecutionStore) Create(workerID, triggerInput, sessionID string) (model.WorkerExecution, error) {
    millis := time.Now().UnixMilli()
    exec := model.WorkerExecution{
        ID:           uuid.New().String(),
        WorkerID:     &workerID,          // ← changed: was workerID
        SessionID:    sessionID,
        TriggerInput: triggerInput,
        Status:       model.ExecStatusPending,
        StartedAt:    &millis,
    }
    _, err := s.db.Exec(
        `INSERT INTO executions (id, worker_id, session_id, trigger_input, status, result, ai_process_pid, started_at)
         VALUES (?, ?, ?, ?, ?, '', 0, ?)`,
        exec.ID, exec.WorkerID, exec.SessionID, exec.TriggerInput, exec.Status, millis,
    )
    if err != nil {
        return model.WorkerExecution{}, fmt.Errorf("insert execution: %w", err)
    }
    return exec, nil
}
```

- [ ] **Step 3: Fix `execution_store.go` — `scanExecution`**

`scanExecution` scans `worker_id` into `&e.WorkerID`. Since `WorkerID` is now `*string`, `database/sql` will correctly set it to nil for SQL NULL. No logic change needed — the scan target type change is sufficient:

```go
func scanExecution(scanner interface{ Scan(...any) error }) (model.WorkerExecution, error) {
    var e model.WorkerExecution
    err := scanner.Scan(
        &e.ID, &e.WorkerID, &e.SessionID, &e.TriggerInput,
        &e.Status, &e.Result, &e.Logs,
        &e.AIProcessPID, &e.StartedAt, &e.CompletedAt, &e.WorkerName,
    )
    return e, err
}
```

(This is unchanged in code, but the compiler now treats `&e.WorkerID` as `**string`, which `database/sql` handles correctly for NULL.)

- [ ] **Step 4: Fix `execution_store_test.go` — dereference `WorkerID`**

In `TestExecutionStore_CreateAndGet`, line 37:

```go
// Before
if got.WorkerID != w.ID {
    t.Errorf("expected worker_id %s, got %s", w.ID, got.WorkerID)
}

// After
if got.WorkerID == nil || *got.WorkerID != w.ID {
    gotStr := "<nil>"
    if got.WorkerID != nil {
        gotStr = *got.WorkerID
    }
    t.Errorf("expected worker_id %s, got %s", w.ID, gotStr)
}
```

- [ ] **Step 5: Verify all existing tests pass**

```bash
go test ./internal/model/... ./internal/store/... -v 2>&1 | tail -20
```

Expected: all PASS. If there are other files referencing `WorkerID` as a plain string, the compiler will catch them here.

- [ ] **Step 6: Commit**

```bash
git add internal/model/execution.go internal/store/execution_store.go internal/store/execution_store_test.go
git commit -m "refactor: change WorkerID to *string to support nullable bee executions"
```

---

## Task 3: Add `CreateBeeExecution` (TDD)

**Files:**
- Modify: `internal/store/execution_store_test.go`
- Modify: `internal/store/execution_store.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/store/execution_store_test.go`:

```go
func TestExecutionStore_CreateBeeExecution(t *testing.T) {
    db, err := InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    es := NewExecutionStore(db)

    sessionID := uuid.New().String()
    exec, err := es.CreateBeeExecution(sessionID, "test prompt")
    if err != nil {
        t.Fatalf("CreateBeeExecution: %v", err)
    }
    if exec.ID == "" {
        t.Error("expected non-empty ID")
    }
    if exec.WorkerID != nil {
        t.Errorf("expected nil WorkerID for bee execution, got %v", exec.WorkerID)
    }
    if exec.Status != model.ExecStatusPending {
        t.Errorf("expected pending, got %s", exec.Status)
    }

    // GetByID must scan NULL worker_id without error
    got, err := es.GetByID(exec.ID)
    if err != nil {
        t.Fatalf("GetByID: %v", err)
    }
    if got.WorkerID != nil {
        t.Errorf("expected nil WorkerID from DB, got %v", got.WorkerID)
    }
    if got.SessionID != sessionID {
        t.Errorf("expected session_id %s, got %s", sessionID, got.SessionID)
    }
}
```

- [ ] **Step 2: Run to confirm test fails**

```bash
go test ./internal/store/... -run TestExecutionStore_CreateBeeExecution -v
```

Expected: FAIL — `es.CreateBeeExecution undefined`

- [ ] **Step 3: Implement `CreateBeeExecution`**

Add to `internal/store/execution_store.go`:

```go
func (s *ExecutionStore) CreateBeeExecution(sessionID, triggerInput string) (model.WorkerExecution, error) {
    millis := time.Now().UnixMilli()
    exec := model.WorkerExecution{
        ID:           uuid.New().String(),
        WorkerID:     nil, // bee execution — no worker
        SessionID:    sessionID,
        TriggerInput: triggerInput,
        Status:       model.ExecStatusPending,
        StartedAt:    &millis,
    }
    _, err := s.db.Exec(
        `INSERT INTO executions (id, worker_id, session_id, trigger_input, status, result, ai_process_pid, started_at)
         VALUES (?, NULL, ?, ?, ?, '', 0, ?)`,
        exec.ID, exec.SessionID, exec.TriggerInput, exec.Status, millis,
    )
    if err != nil {
        return model.WorkerExecution{}, fmt.Errorf("insert bee execution: %w", err)
    }
    return exec, nil
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./internal/store/... -run TestExecutionStore_CreateBeeExecution -v
```

Expected: PASS

- [ ] **Step 5: Run full store test suite**

```bash
go test ./internal/store/... -v 2>&1 | tail -30
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/execution_store.go internal/store/execution_store_test.go
git commit -m "feat: add CreateBeeExecution to ExecutionStore"
```

---

## Task 4: Wire `execStore` into `Feeder` and `app.go`

**Files:**
- Modify: `internal/bee/feeder.go` (struct + constructor only)
- Modify: `internal/bee/feeder_test.go` (`newFeeder` helper)
- Modify: `internal/app/app.go` (`buildBee`)

This task only updates signatures. No behavior changes yet.

- [ ] **Step 1: Add `execStore` to `Feeder` struct and `NewFeeder`**

In `internal/bee/feeder.go`, update the struct and constructor:

```go
type Feeder struct {
    msgStore     *store.MessageStore
    taskStore    *store.TaskStore
    sessionStore *store.SessionStore
    execStore    *store.ExecutionStore  // ← new
    runner       BeeRunner
    workDir      string
    cfg          config.BeeConfig
}

func NewFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner BeeRunner, workDir string, cfg config.BeeConfig) *Feeder {
    return &Feeder{
        msgStore:     ms,
        taskStore:    ts,
        sessionStore: ss,
        execStore:    es,   // ← new
        runner:       runner,
        workDir:      workDir,
        cfg:          cfg,
    }
}
```

- [ ] **Step 2: Fix `app.go` — update `buildBee`**

In `internal/app/app.go`, update `buildBee`:

```go
func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task_dispatcher.DispatchTask) (*bee.Feeder, *task_scheduler.Scheduler) {
    beeProcess := bee.NewBeeProcess(cfg)
    feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore, beeProcess, config.DefaultBeeWorkDir(), cfg)
    sched := task_scheduler.New(s.taskStore, dispatchCh, bee.PollInterval)
    return feeder, sched
}
```

- [ ] **Step 3: Fix `feeder_test.go` — update `newFeeder` helper**

In `internal/bee/feeder_test.go`:

1. Update `setupFeederDB` to also return `*store.ExecutionStore`:

```go
func setupFeederDB(t *testing.T) (*sql.DB, *store.MessageStore, *store.TaskStore, *store.SessionStore, *store.ExecutionStore) {
    t.Helper()
    db, err := store.InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return db, store.NewMessageStore(db), store.NewTaskStore(db), store.NewSessionStore(db), store.NewExecutionStore(db)
}
```

2. Update `newFeeder` helper:

```go
func newFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner bee.BeeRunner) *bee.Feeder {
    cfg := config.BeeConfig{}
    cfg.Feeder.Timeout = 5 * time.Second
    return bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg)
}
```

3. Update all existing test call sites. Every test that calls `setupFeederDB` and `newFeeder` needs to be updated:

```go
// Every test: change
db, ms, ts, ss := setupFeederDB(t)
// ...
f := newFeeder(ms, ts, ss, runner)

// To:
db, ms, ts, ss, es := setupFeederDB(t)
// ...
f := newFeeder(ms, ts, ss, es, runner)
```

Affected tests: `TestFeeder_FirstTick_UsesNewSessionID`, `TestFeeder_SecondTick_ResumesSession`, `TestFeeder_OnBeeFailure_RollsBackAndDoesNotUpdateSession`, `TestFeeder_MultipleSessionKeys_ProcessedIndependently`.

- [ ] **Step 4: Verify compilation and existing tests pass**

```bash
go build ./...
go test ./internal/bee/... ./internal/app/... -v 2>&1 | tail -30
```

Expected: compiles cleanly, all existing tests PASS (no behavior changes yet).

- [ ] **Step 5: Commit**

```bash
git add internal/bee/feeder.go internal/bee/feeder_test.go internal/app/app.go
git commit -m "refactor: wire execStore into Feeder and update test helpers"
```

---

## Task 5: Rewrite `drainBeeOutput` to return logs (TDD)

**Files:**
- Modify: `internal/bee/feeder.go`

- [ ] **Step 1: Change `drainBeeOutput` signature**

In `internal/bee/feeder.go`, replace the entire `drainBeeOutput` method:

```go
// drainBeeOutput consumes the output channel and accumulates logs in memory.
// Returns accumulated log string (partial even on error) and nil on OutputDone,
// or non-nil error on OutputError or channel closed without completion.
func (f *Feeder) drainBeeOutput(ch <-chan claude.Output) (string, error) {
    var sb strings.Builder
    var done bool
    for out := range ch {
        switch out.Type {
        case claude.OutputStdout:
            fmt.Fprintf(&sb, "[stdout] %s\n", out.Content)
        case claude.OutputStderr:
            fmt.Fprintf(&sb, "[stderr] %s\n", out.Content)
        case claude.OutputError:
            fmt.Fprintf(&sb, "[error] %s\n", out.Content)
            return sb.String(), fmt.Errorf("bee exited with error: %s", out.Content)
        case claude.OutputDone:
            done = true
        }
    }
    if !done {
        return sb.String(), fmt.Errorf("bee output channel closed without completion signal")
    }
    return sb.String(), nil
}
```

Remove these imports (no longer needed): `"os"`, `"path/filepath"`. Keep `"strings"` (already imported) and `"fmt"`.

- [ ] **Step 2: Fix the compile error in `processBeeGroup`**

At this point `processBeeGroup` still calls `drainBeeOutput` with the old signature. Temporarily update the call to compile (will be properly fixed in Task 6):

```go
// Temporary: ignore logs for now, will fix in Task 6
if _, err := f.drainBeeOutput(outputCh); err != nil {
```

Remove the old call: `if err := f.drainBeeOutput(outputCh, sessionID); err != nil {`

- [ ] **Step 3: Verify compilation and all tests pass**

```bash
go build ./...
go test ./internal/bee/... -v 2>&1 | tail -30
```

Expected: compiles, all existing tests PASS. `TestFeeder_OnBeeFailure_RollsBackAndDoesNotUpdateSession` still passes because the failure path rolls back correctly.

- [ ] **Step 4: Commit**

```bash
git add internal/bee/feeder.go
git commit -m "refactor: rewrite drainBeeOutput to return accumulated logs as string"
```

---

## Task 6: Update `processBeeGroup` to record executions

**Files:**
- Modify: `internal/bee/feeder.go`
- Modify: `internal/bee/feeder_test.go` (new tests)

- [ ] **Step 1: Write failing tests for execution recording**

Add to `internal/bee/feeder_test.go`:

```go
// TestFeeder_CreatesExecutionOnBeeRun verifies that each processBeeGroup call
// creates one row in executions with status=completed and non-empty logs.
func TestFeeder_CreatesExecutionOnBeeRun(t *testing.T) {
    db, ms, ts, ss, es := setupFeederDB(t)
    insertMessage(t, db, "m1", "feishu:c:u", "hello bee")

    runner := &mockBeeRunner{}
    f := newFeeder(ms, ts, ss, es, runner)

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    go f.Run(ctx)
    time.Sleep(700 * time.Millisecond)

    rows, err := db.Query(`SELECT id, worker_id, status, logs FROM executions`)
    if err != nil {
        t.Fatalf("query executions: %v", err)
    }
    defer rows.Close()

    var execs []struct {
        id       string
        workerID *string
        status   string
        logs     string
    }
    for rows.Next() {
        var e struct {
            id       string
            workerID *string
            status   string
            logs     string
        }
        if err := rows.Scan(&e.id, &e.workerID, &e.status, &e.logs); err != nil {
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
}

// TestFeeder_ExecutionFailedOnBeeError verifies that a bee OutputError results
// in an execution row with status=failed.
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
    err := db.QueryRow(`SELECT status FROM executions`).Scan(&status)
    if err != nil {
        t.Fatalf("query executions: %v", err)
    }
    if status != "failed" {
        t.Errorf("expected status=failed, got %q", status)
    }
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/bee/... -run "TestFeeder_CreatesExecution|TestFeeder_ExecutionFailed" -v
```

Expected: FAIL — execution rows are not being created yet.

- [ ] **Step 3: Update `processBeeGroup` to create and update execution records**

Replace the body of `processBeeGroup` in `internal/bee/feeder.go`:

```go
func (f *Feeder) processBeeGroup(ctx context.Context, sessionKey string, msgs []store.ClaimedMessage) {
    // Look up existing session for this sessionKey
    sessionID, err := f.sessionStore.GetSessionContext(ctx, sessionKey, store.BeeAgentID)
    if err != nil {
        slog.Error("get session context", "component", "feeder", "sessionKey", sessionKey, "error", err)
        f.rollback(ctx, msgs)
        return
    }
    resume := sessionID != ""
    if sessionID == "" {
        sessionID = uuid.New().String()
    }

    prompt := buildPrompt(msgs)
    beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Feeder.Timeout)
    defer cancel()

    proc, outputCh, err := f.runner.Run(beeCtx, f.workDir, prompt, sessionID, resume)
    if err != nil {
        slog.Error("bee run failed", "component", "feeder", "sessionKey", sessionKey, "error", err)
        f.rollback(ctx, msgs)
        return
    }

    // Create execution record only after process starts successfully.
    exec, execErr := f.execStore.CreateBeeExecution(sessionID, prompt)
    if execErr != nil {
        slog.Error("create bee execution", "component", "feeder", "sessionKey", sessionKey, "error", execErr)
        // non-fatal: continue without execution tracking
    }
    if execErr == nil && proc != nil {
        if pidErr := f.execStore.UpdatePID(exec.ID, proc.PID()); pidErr != nil {
            slog.Error("update execution pid", "component", "feeder", "error", pidErr)
        }
    }

    logs, drainErr := f.drainBeeOutput(outputCh)

    if execErr == nil {
        if logsErr := f.execStore.UpdateLogs(exec.ID, logs); logsErr != nil {
            slog.Error("update execution logs", "component", "feeder", "error", logsErr)
        }
        finalStatus := model.ExecStatusCompleted
        resultMsg := ""
        if drainErr != nil {
            finalStatus = model.ExecStatusFailed
            resultMsg = drainErr.Error()
        }
        if resErr := f.execStore.UpdateResult(exec.ID, resultMsg, finalStatus); resErr != nil {
            slog.Error("update execution result", "component", "feeder", "error", resErr)
        }
    }

    if drainErr != nil {
        slog.Error("bee run failed", "component", "feeder", "sessionKey", sessionKey, "error", drainErr)
        f.rollback(ctx, msgs)
        return
    }

    // Persist session_id before marking messages processed.
    if resume {
        currentID, checkErr := f.sessionStore.GetSessionContext(ctx, sessionKey, store.BeeAgentID)
        if checkErr == nil && currentID == "" {
            slog.Info("session cleared during bee execution, skipping context upsert",
                "component", "feeder", "sessionKey", sessionKey)
        } else {
            if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID); err != nil {
                slog.Error("upsert session context", "component", "feeder", "sessionKey", sessionKey, "error", err)
            }
        }
    } else {
        if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID); err != nil {
            slog.Error("upsert session context", "component", "feeder", "sessionKey", sessionKey, "error", err)
        }
    }

    msgIDs := make([]string, len(msgs))
    for i, m := range msgs {
        msgIDs[i] = m.ID
    }
    if err := f.msgStore.MarkBeeProcessed(ctx, msgIDs); err != nil {
        slog.Error("mark bee_processed", "component", "feeder", "sessionKey", sessionKey, "error", err)
    }
}
```

Add `"github.com/theopenbee/openbee/internal/model"` to imports in `feeder.go` if not already present.

- [ ] **Step 4: Run new tests**

```bash
go test ./internal/bee/... -run "TestFeeder_CreatesExecution|TestFeeder_ExecutionFailed" -v
```

Expected: PASS

- [ ] **Step 5: Run full test suite**

```bash
go test ./... 2>&1 | tail -30
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/bee/feeder.go internal/bee/feeder_test.go
git commit -m "feat: record bee executions in executions table"
```

---

## Task 7: Final verification and cleanup

- [ ] **Step 1: Confirm `bee-logs` directory is no longer created**

The old code created `~/.openbee/bee-logs/`. Verify no reference remains:

```bash
grep -r "bee-logs" /Users/tengyongzhi/work/theopenbee/openbee/ --include="*.go"
```

Expected: no results.

- [ ] **Step 2: Run full test suite one final time**

```bash
go test ./... -count=1 2>&1
```

Expected: all PASS, no failures.

- [ ] **Step 3: Build the binary**

```bash
go build ./cmd/... 2>&1
```

Expected: builds cleanly.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "chore: verify bee-logs removed, all tests pass"
```
