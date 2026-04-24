# Legacy Session Tombstone Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Write a tombstone record (`model="unknown"`, all tokens=0) to `bee_token_stats` whenever all parsers return `ErrSessionDataNotFound`, so legacy sessions stop being re-queried on every sync cycle.

**Architecture:** Remove the `engine == ""` early-return branch in `syncSession`; both the legacy-session path and the known-engine-no-data path now fall through to a single `storeUsages` call that writes the tombstone. The tombstone's `synced_at` timestamp causes `collectSessions` to exclude these sessions in all future rounds.

**Tech Stack:** Go, SQLite (`database/sql`), `go.uber.org/zap` for logging.

---

## File Map

| File | Action | What changes |
|---|---|---|
| `internal/tokenstat/syncer.go` | Modify lines 145–151 | Remove `engine==""` branch; unified tombstone write |
| `internal/tokenstat/syncer_test.go` | Modify — add 2 test funcs | Cover legacy + known-engine no-data paths |

---

### Task 1: Write the two failing tests

**Files:**
- Modify: `internal/tokenstat/syncer_test.go`

- [ ] **Step 1.1: Add `TestSyncer_SyncOnce_LegacyExecutionNoDataWritesTombstone`**

Append to the end of `internal/tokenstat/syncer_test.go`:

```go
func TestSyncer_SyncOnce_LegacyExecutionNoDataWritesTombstone(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "worker-legacy", "")
	insertTestExecution(t, db, "worker-legacy", "legacy-no-data-session", time.Now().UnixMilli())

	// point all parsers at empty dirs so every parser returns ErrSessionDataNotFound
	emptyDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", emptyDir)
	t.Setenv("HOME", emptyDir)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("legacy-no-data-session")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 tombstone record, got %d", len(stats))
	}
	if stats[0].Model != "unknown" {
		t.Errorf("Model: want unknown, got %s", stats[0].Model)
	}
	if stats[0].TotalTokens != 0 {
		t.Errorf("TotalTokens: want 0, got %d", stats[0].TotalTokens)
	}

	// second SyncOnce must not produce a second record (synced_at now > completed_at)
	syncer.SyncOnce(context.Background())
	stats2, err := tokenStore.GetBySessionID("legacy-no-data-session")
	if err != nil {
		t.Fatalf("GetBySessionID (2nd): %v", err)
	}
	if len(stats2) != 1 {
		t.Errorf("expected still 1 record after second sync, got %d", len(stats2))
	}
}
```

- [ ] **Step 1.2: Add `TestSyncer_SyncOnce_KnownEngineNoDataWritesTombstone`**

Append immediately after the function above:

```go
func TestSyncer_SyncOnce_KnownEngineNoDataWritesTombstone(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "worker-claude", "claude")
	insertTestExecutionWithEngine(t, db, "worker-claude", "claude-no-data-session", "claude", time.Now().UnixMilli())

	// point all parsers at empty dirs so every parser returns ErrSessionDataNotFound
	emptyDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", emptyDir)
	t.Setenv("HOME", emptyDir)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("claude-no-data-session")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 tombstone record, got %d", len(stats))
	}
	if stats[0].Model != "unknown" {
		t.Errorf("Model: want unknown, got %s", stats[0].Model)
	}
	if stats[0].TotalTokens != 0 {
		t.Errorf("TotalTokens: want 0, got %d", stats[0].TotalTokens)
	}
}
```

- [ ] **Step 1.3: Run the new tests to confirm they fail**

```bash
go test ./internal/tokenstat/... -run "TestSyncer_SyncOnce_LegacyExecutionNoDataWritesTombstone|TestSyncer_SyncOnce_KnownEngineNoDataWritesTombstone" -v
```

Expected: both tests FAIL — `expected 1 tombstone record, got 0`

---

### Task 2: Implement the tombstone write in `syncSession`

**Files:**
- Modify: `internal/tokenstat/syncer.go` lines 145–151

- [ ] **Step 2.1: Replace the legacy-session branch with a unified tombstone write**

In `internal/tokenstat/syncer.go`, find this block (starting around line 145):

```go
	if firstErr != nil {
		return firstErr
	}
	if engine == "" { // legacy execution without engine hint; missing data is expected
		return nil
	}
	return fmt.Errorf("no token session data found for %s", sessionID)
```

Replace it with:

```go
	if firstErr != nil {
		return firstErr
	}
	if engine == "" {
		logger.Debug("tokenstat: legacy session has no data, writing tombstone",
			zap.String("session_id", sessionID))
	} else {
		logger.Warn("tokenstat: no token data found for session, writing tombstone",
			zap.String("session_id", sessionID),
			zap.String("engine", engine))
	}
	return s.storeUsages([]SessionTokenUsage{{SessionID: sessionID, Model: "unknown"}})
```

- [ ] **Step 2.2: Remove the now-unused `fmt` import if `fmt` is no longer referenced**

Check `syncer.go` for any remaining `fmt.` usage. If `fmt.Errorf` was the only usage, remove `"fmt"` from the import block. Run:

```bash
go build ./internal/tokenstat/...
```

Expected: compiles with no errors. If you see `imported and not used: "fmt"`, remove it from the import block.

- [ ] **Step 2.3: Run the new tests to confirm they pass**

```bash
go test ./internal/tokenstat/... -run "TestSyncer_SyncOnce_LegacyExecutionNoDataWritesTombstone|TestSyncer_SyncOnce_KnownEngineNoDataWritesTombstone" -v
```

Expected: both tests PASS.

- [ ] **Step 2.4: Run the full tokenstat test suite to check for regressions**

```bash
go test ./internal/tokenstat/... -v
```

Expected: all tests PASS. Pay special attention to `TestSyncer_SyncOnce_LegacyExecutionFallsBackToKimi` and `TestSyncer_SyncOnce_LegacyExecutionWithoutEngineFallsBackAcrossParsers` — these test legacy sessions that DO have data, so they must still succeed with real data (not a tombstone).

- [ ] **Step 2.5: Commit**

```bash
git add internal/tokenstat/syncer.go internal/tokenstat/syncer_test.go
git commit -m "fix(tokenstat): write tombstone for sessions with no token data

Legacy sessions (engine='') and sessions where all parsers return
ErrSessionDataNotFound now write a tombstone record (model=unknown,
tokens=0) to bee_token_stats. This sets synced_at so collectSessions
stops re-querying them on every sync cycle."
```
