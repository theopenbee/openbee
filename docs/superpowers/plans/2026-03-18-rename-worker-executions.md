# Rename worker_executions to executions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the SQLite table `worker_executions` to `executions` via migrations and update all SQL references.

**Architecture:** Add 5 sequential migrations (v16–v20) to rename the table and swap the indexes, then update all raw SQL strings in `execution_store.go`. A test in `db_test.go` also references the old name and must be updated.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), standard `database/sql`

---

## File Map

| File | Change |
|------|--------|
| `internal/store/db.go` | Add migrations v16–v20 |
| `internal/store/execution_store.go` | Replace 6 occurrences of `worker_executions` with `executions` |
| `internal/store/db_test.go` | Update `TestInitDB` to check for `executions` instead of `worker_executions` |
| `internal/store/execution_store_test.go` | Update 2 raw SQL queries (lines 82, 113) that reference `worker_executions` |

---

### Task 1: Update the failing test first (TDD)

**Files:**
- Modify: `internal/store/db_test.go:17`
- Modify: `internal/store/execution_store_test.go:82,113`

- [ ] **Step 1: Update `TestInitDB` to expect the new table name**

In `db_test.go` line 17, find:
```go
tables := []string{"workers", "worker_executions"}
```
Change to:
```go
tables := []string{"workers", "executions"}
```

- [ ] **Step 2: Update raw SQL queries in `execution_store_test.go`**

Line 82:
```go
// Before
err = db.QueryRow(`SELECT started_at FROM worker_executions WHERE id = ?`, exec.ID).Scan(&startedAt)
// After
err = db.QueryRow(`SELECT started_at FROM executions WHERE id = ?`, exec.ID).Scan(&startedAt)
```

Line 113:
```go
// Before
err = db.QueryRow(`SELECT completed_at FROM worker_executions WHERE id = ?`, exec.ID).Scan(&completedAt)
// After
err = db.QueryRow(`SELECT completed_at FROM executions WHERE id = ?`, exec.ID).Scan(&completedAt)
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee
go test ./internal/store/... -v
```
Expected: FAIL — `table executions not found` (migrations not yet added)

---

### Task 2: Add migration v16 — rename table

**Files:**
- Modify: `internal/store/db.go` (append to `migrations` slice)

- [ ] **Step 1: Append migration v16 to the `migrations` slice**

After the last migration entry (v15), add:
```go
{
    version: 16,
    name:    "20260318_rename_table_worker_executions_to_executions",
    sql:     `ALTER TABLE worker_executions RENAME TO executions`,
},
```

- [ ] **Step 2: Run tests to verify `TestInitDB` now passes**

```bash
go test ./internal/store/... -run TestInitDB -v
```
Expected: PASS — table `executions` now exists.
Run all store tests to check nothing else broke:
```bash
go test ./internal/store/... -v
```

---

### Task 3: Add migrations v17–v20 — rename indexes

**Files:**
- Modify: `internal/store/db.go` (append to `migrations` slice)

- [ ] **Step 1: Append migrations v17–v20**

```go
{
    version: 17,
    name:    "20260318_drop_index_worker_executions_worker_id",
    sql:     `DROP INDEX idx_worker_executions_worker_id`,
},
{
    version: 18,
    name:    "20260318_create_index_executions_worker_id",
    sql:     `CREATE INDEX IF NOT EXISTS idx_executions_worker_id ON executions(worker_id)`,
},
{
    version: 19,
    name:    "20260318_drop_index_worker_executions_session_id",
    sql:     `DROP INDEX idx_worker_executions_session_id`,
},
{
    version: 20,
    name:    "20260318_create_index_executions_session_id",
    sql:     `CREATE INDEX IF NOT EXISTS idx_executions_session_id ON executions(session_id)`,
},
```

- [ ] **Step 2: Run all store tests**

```bash
go test ./internal/store/... -v
```
Expected: all tests PASS, including `TestMigrations_TableExists` (migration count now 20).

- [ ] **Step 3: Commit migrations and test fix**

```bash
git add internal/store/db.go internal/store/db_test.go internal/store/execution_store_test.go
git commit -m "feat: rename worker_executions table to executions via migrations"
```

---

### Task 4: Update SQL in execution_store.go

**Files:**
- Modify: `internal/store/execution_store.go`

- [ ] **Step 1: Replace all 6 occurrences of `worker_executions` with `executions`**

Locations to change:

1. `execSelect` constant (line ~44):
   ```go
   // Before
   FROM worker_executions e
   // After
   FROM executions e
   ```

2. `Create` method (line ~31):
   ```go
   // Before
   `INSERT INTO worker_executions (id, worker_id, session_id, trigger_input, status, result, ai_process_pid, started_at)
   // After
   `INSERT INTO executions (id, worker_id, session_id, trigger_input, status, result, ai_process_pid, started_at)
   ```

3. `UpdateStatus` (line ~126):
   ```go
   // Before
   `UPDATE worker_executions SET status=? WHERE id=?`
   // After
   `UPDATE executions SET status=? WHERE id=?`
   ```

4. `UpdateLogs` (line ~131):
   ```go
   // Before
   `UPDATE worker_executions SET logs=? WHERE id=?`
   // After
   `UPDATE executions SET logs=? WHERE id=?`
   ```

5. `UpdateResult` (line ~136):
   ```go
   // Before
   `UPDATE worker_executions SET result=?, status=?, completed_at=? WHERE id=?`
   // After
   `UPDATE executions SET result=?, status=?, completed_at=? WHERE id=?`
   ```

6. `UpdatePID` (line ~141):
   ```go
   // Before
   `UPDATE worker_executions SET ai_process_pid=?, status=? WHERE id=?`
   // After
   `UPDATE executions SET ai_process_pid=?, status=? WHERE id=?`
   ```

- [ ] **Step 2: Run all store tests**

```bash
go test ./internal/store/... -v
```
Expected: all tests PASS.

- [ ] **Step 3: Run full build to check no compile errors**

```bash
go build ./...
```
Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add internal/store/execution_store.go
git commit -m "refactor: update SQL queries to reference renamed executions table"
```
