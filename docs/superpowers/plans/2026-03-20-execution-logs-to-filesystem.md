# Execution Logs: DB → Filesystem Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move execution log storage from `bee_executions.logs` DB column to `~/.openbee/logs/<YYYY-MM-DD>/<execution_id>.log`, remove the `logs` column, add `log_path`, and remove the `get_execution_logs` MCP tool so AI reads log files directly.

**Architecture:** Two DB migrations add `log_path` and drop `logs` via table-rename. A new `WriteLog` method on `ExecutionStore` writes files and records the path. The `get_execution_logs` MCP tool and its live-log subscriber infrastructure are deleted; `claudemd/bee.go` tells AI to read `log_path` files directly.

**Tech Stack:** Go, SQLite (`database/sql`), `os.MkdirAll` + `os.WriteFile`

**Spec:** `docs/superpowers/specs/2026-03-20-execution-logs-to-filesystem-design.md`

---

## Chunk 1: Schema + Model + Config

### Task 1: DB migrations

**Files:**
- Modify: `internal/store/db.go`

- [ ] **Step 1: Add migration 19 (add log_path)**

  Open `internal/store/db.go`. After the last migration entry (currently version 18), add:

  ```go
  {
      version: 19,
      name:    "20260320_add_log_path_to_executions",
      sql:     `ALTER TABLE bee_executions ADD COLUMN log_path TEXT NOT NULL DEFAULT ''`,
  },
  ```

- [ ] **Step 2: Add migration 20 (drop logs via table rename)**

  Immediately after migration 19, add:

  ```go
  {
      version: 20,
      name:    "20260320_drop_logs_from_executions",
      sql: `CREATE TABLE bee_executions_new (
      id             TEXT PRIMARY KEY,
      worker_id      TEXT,
      session_id     TEXT NOT NULL,
      status         TEXT NOT NULL DEFAULT 'pending',
      ai_process_pid INTEGER NOT NULL DEFAULT 0,
      trigger_input  TEXT NOT NULL DEFAULT '',
      result         TEXT NOT NULL DEFAULT '',
      log_path       TEXT NOT NULL DEFAULT '',
      started_at     INTEGER,
      completed_at   INTEGER
  );
  INSERT INTO bee_executions_new
      SELECT id, worker_id, session_id, status, ai_process_pid,
             trigger_input, result, log_path, started_at, completed_at
      FROM bee_executions;
  DROP TABLE bee_executions;
  ALTER TABLE bee_executions_new RENAME TO bee_executions;
  CREATE INDEX idx_executions_worker_id ON bee_executions(worker_id);
  CREATE INDEX idx_executions_session_id ON bee_executions(session_id)`,
  },
  ```

  Note: do **not** use `IF NOT EXISTS` on the index creations — the old indexes are dropped with the old table in the same migration.

- [ ] **Step 3: Verify migrations compile**

  ```bash
  cd /Users/tengteng/work/theopenbee/openbee
  go build ./internal/store/...
  ```
  Expected: no errors.

- [ ] **Step 4: Commit**

  ```bash
  git add internal/store/db.go
  git commit -m "feat(store): add migrations 19-20 to add log_path and drop logs column"
  ```

---

### Task 2: Model update

**Files:**
- Modify: `internal/model/execution.go`

- [ ] **Step 1: Replace `Logs` with `LogPath`**

  In `internal/model/execution.go`, change:
  ```go
  Logs         string          `json:"logs,omitempty" db:"logs"`
  ```
  to:
  ```go
  LogPath      string          `json:"log_path,omitempty" db:"log_path"`
  ```

- [ ] **Step 2: Build to find all compilation errors**

  ```bash
  go build ./...
  ```
  Expected: compilation errors in `execution_store.go`, `feeder_test.go`. Note them — they are fixed in later tasks.

- [ ] **Step 3: Commit model change**

  ```bash
  git add internal/model/execution.go
  git commit -m "feat(model): replace Logs with LogPath on WorkerExecution"
  ```

---

### Task 3: Config — add DefaultLogsDir

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add DefaultLogsDir function**

  After `DefaultWorkerBaseDir()` in `internal/config/config.go`, add:

  ```go
  // DefaultLogsDir returns the execution log directory: ~/.openbee/logs
  func DefaultLogsDir() string {
      home, _ := os.UserHomeDir()
      return filepath.Join(home, ".openbee", "logs")
  }
  ```

- [ ] **Step 2: Build config package**

  ```bash
  go build ./internal/config/...
  ```
  Expected: no errors.

- [ ] **Step 3: Commit**

  ```bash
  git add internal/config/config.go
  git commit -m "feat(config): add DefaultLogsDir returning ~/.openbee/logs"
  ```

---

## Chunk 2: ExecutionStore — WriteLog

### Task 4: Replace UpdateLogs with WriteLog

**Files:**
- Modify: `internal/store/execution_store.go`
- Modify: `internal/store/execution_store_test.go`

- [ ] **Step 1: Write failing test for WriteLog**

  In `internal/store/execution_store_test.go`, **delete** the entire `TestExecutionStore_GetLogsByID` function (lines 180–211). Then add this new test at the end of the file:

  ```go
  func TestExecutionStore_WriteLog(t *testing.T) {
      db, err := InitDB(t.TempDir() + "/test.db")
      if err != nil {
          t.Fatal(err)
      }
      defer db.Close()

      logsDir := t.TempDir()
      es := NewExecutionStore(db, logsDir)

      exec, _ := es.CreateBeeExecution("session1", "test prompt")

      content := "line1\nline2\nline3"
      logPath, err := es.WriteLog(exec.ID, exec.StartedAt, content)
      if err != nil {
          t.Fatalf("WriteLog: %v", err)
      }
      if logPath == "" {
          t.Fatal("expected non-empty logPath")
      }

      // File must exist and contain the content
      got, err := os.ReadFile(logPath)
      if err != nil {
          t.Fatalf("read log file: %v", err)
      }
      if string(got) != content {
          t.Errorf("expected %q, got %q", content, string(got))
      }

      // DB must have log_path set
      updated, err := es.GetByID(exec.ID)
      if err != nil {
          t.Fatal(err)
      }
      if updated.LogPath != logPath {
          t.Errorf("expected LogPath %q, got %q", logPath, updated.LogPath)
      }
  }

  func TestExecutionStore_WriteLog_NilStartedAt(t *testing.T) {
      db, err := InitDB(t.TempDir() + "/test.db")
      if err != nil {
          t.Fatal(err)
      }
      defer db.Close()

      logsDir := t.TempDir()
      es := NewExecutionStore(db, logsDir)

      exec, _ := es.CreateBeeExecution("session2", "test")

      // Pass nil startedAt — must not panic, must write file
      logPath, err := es.WriteLog(exec.ID, nil, "content")
      if err != nil {
          t.Fatalf("WriteLog with nil startedAt: %v", err)
      }
      if logPath == "" {
          t.Fatal("expected non-empty logPath")
      }
  }
  ```

  Add `"os"` to the imports at the top of the test file if not already present.

- [ ] **Step 2: Run test to verify it fails**

  ```bash
  go test ./internal/store/... -run TestExecutionStore_WriteLog -v
  ```
  Expected: FAIL — `NewExecutionStore` does not accept a second argument yet.

- [ ] **Step 3: Implement WriteLog in ExecutionStore**

  In `internal/store/execution_store.go`:

  **3a.** Update imports — add `"os"`, `"path/filepath"`, `"time"`. Remove any unused imports after the next changes.

  **3b.** Update the struct to carry `logsDir`:

  **Important:** After step 3c (constructor change), all existing `NewExecutionStore(db)` calls in `execution_store_test.go` (8 instances) must also be updated to `NewExecutionStore(db, t.TempDir())` — otherwise Step 4 will fail to compile. Update them at the same time as step 3c.



  ```go
  type ExecutionStore struct {
      db      *sql.DB
      logsDir string
  }
  ```

  **3c.** Update the constructor:

  ```go
  func NewExecutionStore(db *sql.DB, logsDir string) *ExecutionStore {
      return &ExecutionStore{db: db, logsDir: logsDir}
  }
  ```

  **3d.** Update `execSelect` — replace `e.logs` with `e.log_path`:

  ```go
  const execSelect = `
  SELECT e.id, e.worker_id, e.session_id, e.trigger_input, e.status, e.result, e.log_path,
         e.ai_process_pid, e.started_at, e.completed_at, COALESCE(w.name, '')
  FROM bee_executions e
  LEFT JOIN bee_workers w ON w.id = e.worker_id`
  ```

  **3e.** Update `scanExecution` — replace `&e.Logs` with `&e.LogPath`:

  ```go
  func scanExecution(scanner interface{ Scan(...any) error }) (model.WorkerExecution, error) {
      var e model.WorkerExecution
      err := scanner.Scan(&e.ID, &e.WorkerID, &e.SessionID, &e.TriggerInput, &e.Status, &e.Result, &e.LogPath, &e.AIProcessPID, &e.StartedAt, &e.CompletedAt, &e.WorkerName)
      return e, err
  }
  ```

  **3f.** Delete the `UpdateLogs` method entirely.

  **3g.** Delete the `GetLogsByID` method entirely.

  **3h.** Add the `WriteLog` method after `UpdatePID`:

  ```go
  // WriteLog writes content to a date-partitioned log file and records the path in the DB.
  // startedAt is used to determine the date directory; falls back to time.Now() if nil.
  func (s *ExecutionStore) WriteLog(id string, startedAt *int64, content string) (string, error) {
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
      if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
          return "", fmt.Errorf("write log file: %w", err)
      }
      if _, err := s.db.Exec(`UPDATE bee_executions SET log_path=? WHERE id=?`, logPath, id); err != nil {
          return "", fmt.Errorf("update log_path: %w", err)
      }
      return logPath, nil
  }
  ```

- [ ] **Step 4: Run the new test**

  ```bash
  go test ./internal/store/... -run TestExecutionStore_WriteLog -v
  ```
  Expected: PASS.

- [ ] **Step 5: Fix NewExecutionStore call sites outside store package**

  `NewExecutionStore` now requires a `logsDir` argument. Fix these specific files:

  - `internal/app/app.go` — find `NewExecutionStore(db)`, replace with `NewExecutionStore(db, config.DefaultLogsDir())`
  - `internal/mcp/tools_test.go` — has 3 calls; replace each `NewExecutionStore(db)` with `NewExecutionStore(db, t.TempDir())`
  - `internal/bee/feeder_test.go` — has 1 call in `setupFeederDB`; replace with `NewExecutionStore(db, t.TempDir())`

  Verify nothing was missed:
  ```bash
  grep -rn "NewExecutionStore" internal/
  ```

  Run:
  ```bash
  go build ./...
  ```
  Expected: no errors.

- [ ] **Step 6: Run all store tests**

  ```bash
  go test ./internal/store/... -v
  ```
  Expected: all pass.

- [ ] **Step 7: Commit**

  ```bash
  git add internal/store/execution_store.go internal/store/execution_store_test.go internal/app/app.go
  git commit -m "feat(store): replace UpdateLogs/GetLogsByID with WriteLog writing to filesystem"
  ```

---

## Chunk 3: Callers + Subscriber Cleanup

### Task 5: Update feeder.go

**Files:**
- Modify: `internal/bee/feeder.go`
- Modify: `internal/bee/feeder_test.go`

- [ ] **Step 1: Update feeder.go call site**

  In `internal/bee/feeder.go`, find:
  ```go
  if logsErr := f.execStore.UpdateLogs(exec.ID, logs); logsErr != nil {
      log.Error("update execution logs", zap.Error(logsErr))
  }
  ```
  Replace with:
  ```go
  if _, logsErr := f.execStore.WriteLog(exec.ID, exec.StartedAt, logs); logsErr != nil {
      log.Error("write execution logs", zap.Error(logsErr))
  }
  ```

- [ ] **Step 2: Update feeder_test.go**

  Note: `setupFeederDB`'s `NewExecutionStore` call was already fixed in Task 4 Step 5.

  In `TestFeeder_CreatesExecutionOnBeeRun`, update the SQL query:
  ```go
  // Before:
  rows, err := db.Query(`SELECT id, worker_id, status, logs FROM bee_executions`)
  // After:
  rows, err := db.Query(`SELECT id, worker_id, status, log_path FROM bee_executions`)
  ```

  There are **two** anonymous struct declarations that both need the `logs string` field renamed to `logPath string`:

  ```go
  // Outer declaration (var execs []struct{...}):
  var execs []struct {
      id       string
      workerID *string
      status   string
      logPath  string   // was: logs string
  }
  // Inner declaration (var e struct{...} inside rows.Next()):
  var e struct {
      id       string
      workerID *string
      status   string
      logPath  string   // was: logs string
  }
  ```

  Update the `Scan` call: `&e.logPath` (was `&e.logs`).

  Also update the comment on line 221 from `non-empty logs` to `non-empty log_path`.

- [ ] **Step 3: Build and test**

  ```bash
  go build ./internal/bee/...
  go test ./internal/bee/... -v
  ```
  Expected: all pass.

- [ ] **Step 4: Commit**

  ```bash
  git add internal/bee/feeder.go internal/bee/feeder_test.go
  git commit -m "feat(bee): use WriteLog instead of UpdateLogs in feeder"
  ```

---

### Task 6: Update worker/manager.go — WriteLog + subscriber cleanup

**Files:**
- Modify: `internal/worker/manager.go`

- [ ] **Step 1: Remove subscriber fields from Manager struct**

  In `internal/worker/manager.go`, remove from the `Manager` struct:
  ```go
  logSubscribers   map[string][]chan claude.Output
  liveLogSnapshots map[string]string
  ```

- [ ] **Step 2: Remove subscriber initialization from NewManager**

  Remove from `NewManager`:
  ```go
  logSubscribers:   make(map[string][]chan claude.Output),
  liveLogSnapshots: make(map[string]string),
  ```

- [ ] **Step 3: Remove subscriber broadcast and liveLogSnapshots update from monitorExecution**

  These two blocks are in different locations in `monitorExecution`:

  **Block A** — the subscriber broadcast, which appears **before** the `switch out.Type` statement (remove entirely):
  ```go
  // Broadcast to WebSocket subscribers
  m.mu.RLock()
  subs := m.logSubscribers[exec.ID]
  m.mu.RUnlock()

  for _, sub := range subs {
      select {
      case sub <- out:
      default:
      }
  }
  ```

  **Block B** — the `liveLogSnapshots` update, which is **inside** `case claude.OutputStdout`, after the `rawLogsBuilder.WriteString` calls (remove these 3 lines only):
  ```go
  m.mu.Lock()
  m.liveLogSnapshots[exec.ID] = rawLogsBuilder.String()
  m.mu.Unlock()
  ```

- [ ] **Step 4: Remove subscriber cleanup from monitorExecution's cleanup section**

  Find and remove from the cleanup block after `delete(m.activeProcesses, exec.ID)`:
  ```go
  delete(m.liveLogSnapshots, exec.ID)
  for _, sub := range m.logSubscribers[exec.ID] {
      close(sub)
  }
  delete(m.logSubscribers, exec.ID)
  ```

- [ ] **Step 5: Delete GetExecutionLogs and SubscribeLogs methods**

  Delete:
  ```go
  // GetExecutionLogs returns the current logs for a running execution.
  func (m *Manager) GetExecutionLogs(executionID string) string { ... }
  ```
  Delete:
  ```go
  func (m *Manager) SubscribeLogs(executionID string) <-chan claude.Output { ... }
  ```

- [ ] **Step 6: Replace UpdateLogs calls with WriteLog**

  In `monitorExecution`, replace both:
  ```go
  m.executionStore.UpdateLogs(exec.ID, rawLogs)
  ```
  with:
  ```go
  if _, err := m.executionStore.WriteLog(exec.ID, exec.StartedAt, rawLogs); err != nil {
      log.Error("write execution logs", zap.Error(err))
  }
  ```
  (There are two occurrences: `OutputDone` and `OutputError` branches.)

- [ ] **Step 7: Remove unused imports**

  If `sync` is still needed (for `activeProcesses` mutex), keep it. Otherwise remove. Check if `"strings"` is still needed for `rawLogsBuilder` — it is (strings.Builder). Clean up any other unused imports.

- [ ] **Step 8: Build and test**

  ```bash
  go build ./internal/worker/...
  go test ./internal/worker/... -v
  ```
  Expected: all pass.

- [ ] **Step 9: Commit**

  ```bash
  git add internal/worker/manager.go
  git commit -m "feat(worker): use WriteLog, remove live-log subscriber infrastructure"
  ```

  Note: after this commit, `go build ./...` will fail until Task 7 is complete because `internal/mcp/tools.go` still calls `s.manager.GetExecutionLogs(...)` which was just deleted. Proceed directly to Task 7.

---

## Chunk 4: MCP + API + claudemd Cleanup

### Task 7: Remove get_execution_logs MCP tool

**Files:**
- Modify: `internal/toolnames/toolnames.go`
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Remove constant from toolnames**

  In `internal/toolnames/toolnames.go`, delete the line:
  ```go
  GetExecutionLogs  = "get_execution_logs"
  ```

- [ ] **Step 2: Remove tool schema from toolSchemas()**

  In `internal/mcp/tools.go`, delete the entire schema block for `GetExecutionLogs` (lines ~169–180):
  ```go
  {
      Name:        toolnames.GetExecutionLogs,
      Description: "查看某个执行记录的最新日志。返回执行的最后N行日志。",
      InputSchema: map[string]any{ ... },
  },
  ```

- [ ] **Step 3: Remove case from CallTool switch**

  Delete:
  ```go
  case toolnames.GetExecutionLogs:
      return s.toolGetExecutionLogs(args)
  ```

- [ ] **Step 4: Delete toolGetExecutionLogs method**

  Delete the entire `func (s *MCPServer) toolGetExecutionLogs(args json.RawMessage) (any, error)` method.

- [ ] **Step 5: Remove manager.GetExecutionLogs reference in tools.go**

  The `toolGetExecutionLogs` method called `s.manager.GetExecutionLogs(...)`. That method is now gone from manager. Verify no other references remain:
  ```bash
  grep -rn "GetExecutionLogs" internal/
  ```
  Expected: no results.

- [ ] **Step 6: Remove test for the deleted tool**

  In `internal/mcp/tools_test.go`, delete the entire `TestCallTool_GetExecutionLogs` function (lines 544–554).

- [ ] **Step 7: Build and test**

  ```bash
  go build ./internal/mcp/... ./internal/toolnames/...
  go test ./internal/mcp/... -v
  ```
  Expected: all pass.

- [ ] **Step 8: Commit**

  ```bash
  git add internal/toolnames/toolnames.go internal/mcp/tools.go internal/mcp/tools_test.go
  git commit -m "feat(mcp): remove get_execution_logs tool"
  ```

---

### Task 8: Remove WebSocket logs API endpoint

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/execution_handler.go`

- [ ] **Step 1: Remove route from router.go**

  In `internal/api/router.go`, find and delete:
  ```go
  api.GET("/executions/:id/logs", s.streamLogs)
  ```

- [ ] **Step 2: Delete streamLogs handler**

  In `internal/api/execution_handler.go`, delete the entire `streamLogs` function:
  ```go
  func (s *Server) streamLogs(c *gin.Context) {
      ...
  }
  ```

- [ ] **Step 3: Check for unused imports in execution_handler.go**

  The `websocket` import was used only by `streamLogs`. Remove `"github.com/gorilla/websocket"` and the `upgrader` variable if no other handlers use them.

  ```bash
  go build ./internal/api/...
  ```
  Expected: no errors.

- [ ] **Step 4: Test**

  ```bash
  go test ./internal/api/... -v
  ```
  Expected: all pass.

- [ ] **Step 5: Commit**

  ```bash
  git add internal/api/router.go internal/api/execution_handler.go
  git commit -m "feat(api): remove WebSocket execution logs endpoint"
  ```

---

### Task 9: Update claudemd/bee.go

**Files:**
- Modify: `internal/claudemd/bee.go`

- [ ] **Step 1: Remove GetExecutionLogs tool reference**

  In `internal/claudemd/bee.go`, find and delete this line in the 状态工具 list:
  ```go
  - ` + toolnames.GetExecutionLogs + ` - 查看执行日志
  ```

- [ ] **Step 2: Update 使用场景 prose**

  Find:
  ```
  - 需要自我反思时，用 list_bee_executions 回顾历史，用 get_execution_logs 查看详情
  ```
  Replace with:
  ```
  - 需要自我反思时，用 list_bee_executions 回顾历史，直接读取 log_path 文件查看详情
  ```

- [ ] **Step 3: Add file-read guidance**

  After the 使用场景 bullet list (before the closing backtick of the function), add:
  ```
  - 查看执行日志时，从执行记录的 log_path 字段获取文件路径，然后直接读取该文件
  ```

- [ ] **Step 4: Build**

  ```bash
  go build ./internal/claudemd/...
  ```
  Expected: no errors.

- [ ] **Step 5: Commit**

  ```bash
  git add internal/claudemd/bee.go
  git commit -m "feat(claudemd): remove get_execution_logs ref, instruct AI to read log files"
  ```

---

## Chunk 5: Final Verification

### Task 10: Full build and test

- [ ] **Step 1: Full build**

  ```bash
  cd /Users/tengteng/work/theopenbee/openbee
  go build ./...
  ```
  Expected: no errors.

- [ ] **Step 2: Full test suite**

  ```bash
  go test ./...
  ```
  Expected: all pass.

- [ ] **Step 3: Verify no stale references**

  ```bash
  grep -rn "UpdateLogs\|GetLogsByID\|SubscribeLogs\|liveLogSnapshots\|logSubscribers\|GetExecutionLogs\|get_execution_logs" \
    internal/ --include="*.go"
  grep -rn '"logs"' internal/ --include="*.go"
  grep -rn '\.Logs[^P]' internal/ --include="*.go"
  ```
  Expected: no results for any of these.

- [ ] **Step 4: Verify log file is written during a manual run (optional smoke test)**

  Start the server locally and trigger a bee execution. Check that:
  - `~/.openbee/logs/<today>/` directory exists
  - A `.log` file exists for the execution ID
  - `bee_executions.log_path` column holds the file path

- [ ] **Step 5: Final commit if needed**

  If any final cleanups were made:
  ```bash
  git add -p
  git commit -m "chore: final cleanup after logs filesystem migration"
  ```
