# Add `total_tokens` to `bee_token_stats` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `total_tokens` stored column to `bee_token_stats` that persists `input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens`.

**Architecture:** The value is computed once in `syncer.go` at write time and stored as a plain integer column. No parser changes are needed; the four source fields already flow through to the syncer. The DB schema is modified directly in the existing `CREATE TABLE` statement (migration 41), not via `ALTER TABLE`.

**Tech Stack:** Go, SQLite (`database/sql`), `sqlx`-style struct tags (`db:"..."`), `go test`

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/store/db.go` | Add `total_tokens` column to migration 41 CREATE TABLE |
| `internal/infra/model/token_stats.go` | Add `TotalTokens int64` field to struct |
| `internal/infra/store/token_stats_store.go` | Add `total_tokens` to INSERT, ON CONFLICT UPDATE, SELECT, and Scan |
| `internal/infra/store/token_stats_store_test.go` | Update existing tests + add TotalTokens assertion |
| `internal/tokenstat/syncer.go` | Compute and assign `TotalTokens` in `storeUsages` |

---

## Task 1: Update DB schema (migration 41)

**Files:**
- Modify: `internal/infra/store/db.go:376-386`

- [ ] **Step 1: Add `total_tokens` column to CREATE TABLE**

In `db.go`, find the migration 41 `CREATE TABLE IF NOT EXISTS bee_token_stats` block (around line 376). Replace the closing section:

```sql
-- before
            cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
            synced_at             INTEGER NOT NULL
        );
```

```sql
-- after
            cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
            total_tokens          INTEGER NOT NULL DEFAULT 0,
            synced_at             INTEGER NOT NULL
        );
```

- [ ] **Step 2: Verify the file compiles**

```bash
go build ./internal/infra/store/...
```

Expected: no output (clean build).

---

## Task 2: Add `TotalTokens` to Go model struct

**Files:**
- Modify: `internal/infra/model/token_stats.go:8-12`

- [ ] **Step 1: Add field to TokenStats**

Replace the struct body so `TotalTokens` appears after `CacheReadTokens`:

```go
type TokenStats struct {
	ID                  string `json:"id" db:"id"`
	SessionID           string `json:"session_id" db:"session_id"`
	AgentType           string `json:"agent_type" db:"agent_type"`
	Model               string `json:"model" db:"model"`
	InputTokens         int64  `json:"input_tokens" db:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens" db:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens" db:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens" db:"cache_read_tokens"`
	TotalTokens         int64  `json:"total_tokens" db:"total_tokens"`
	SyncedAt            int64  `json:"synced_at" db:"synced_at"`
}
```

- [ ] **Step 2: Verify the file compiles**

```bash
go build ./internal/infra/model/...
```

Expected: no output.

---

## Task 3: Update store — INSERT, UPDATE, SELECT, Scan

**Files:**
- Modify: `internal/infra/store/token_stats_store.go:43-87`
- Test: `internal/infra/store/token_stats_store_test.go`

- [ ] **Step 1: Write a failing test for TotalTokens persistence**

In `token_stats_store_test.go`, add after `TestTokenStatsStore_Upsert_UpdatesOnConflict`:

```go
func TestTokenStatsStore_Upsert_TotalTokensStored(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	if err := s.Upsert(model.TokenStats{
		SessionID:           "session-total",
		AgentType:           "claude",
		Model:               "claude-3-5-sonnet",
		InputTokens:         100,
		OutputTokens:        200,
		CacheCreationTokens: 50,
		CacheReadTokens:     30,
		TotalTokens:         380,
		SyncedAt:            time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := s.GetBySessionID("session-total")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	if got[0].TotalTokens != 380 {
		t.Errorf("TotalTokens: want 380, got %d", got[0].TotalTokens)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/infra/store/... -run TestTokenStatsStore_Upsert_TotalTokensStored -v
```

Expected: FAIL — either compile error (field not in INSERT/SELECT) or assertion error.

- [ ] **Step 3: Update `upsertTokenStat` — INSERT and ON CONFLICT UPDATE**

Replace the `db.Exec` call in `upsertTokenStat` (lines 43-59):

```go
_, err := db.Exec(
    `INSERT INTO bee_token_stats
         (id, session_id, agent_type, model, input_tokens, output_tokens,
          cache_creation_tokens, cache_read_tokens, total_tokens, synced_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
     ON CONFLICT(session_id, model) DO UPDATE SET
         agent_type            = excluded.agent_type,
         input_tokens          = excluded.input_tokens,
         output_tokens         = excluded.output_tokens,
         cache_creation_tokens = excluded.cache_creation_tokens,
         cache_read_tokens     = excluded.cache_read_tokens,
         total_tokens          = excluded.total_tokens,
         synced_at             = excluded.synced_at`,
    stat.ID, stat.SessionID, stat.AgentType, stat.Model,
    stat.InputTokens, stat.OutputTokens,
    stat.CacheCreationTokens, stat.CacheReadTokens,
    stat.TotalTokens,
    stat.SyncedAt,
)
```

- [ ] **Step 4: Update `GetBySessionID` — SELECT and Scan**

Replace the SELECT query and Scan call in `GetBySessionID` (lines 64-81):

```go
rows, err := s.db.Query(
    `SELECT id, session_id, agent_type, model, input_tokens, output_tokens,
            cache_creation_tokens, cache_read_tokens, total_tokens, synced_at
     FROM bee_token_stats WHERE session_id = ?`,
    sessionID,
)
```

And update the `rows.Scan` call:

```go
if err := rows.Scan(
    &st.ID, &st.SessionID, &st.AgentType, &st.Model,
    &st.InputTokens, &st.OutputTokens,
    &st.CacheCreationTokens, &st.CacheReadTokens,
    &st.TotalTokens, &st.SyncedAt,
); err != nil {
    return nil, fmt.Errorf("scan token stats: %w", err)
}
```

- [ ] **Step 5: Run the new test to confirm it passes**

```bash
go test ./internal/infra/store/... -run TestTokenStatsStore_Upsert_TotalTokensStored -v
```

Expected: PASS.

- [ ] **Step 6: Run the full store test suite**

```bash
go test ./internal/infra/store/... -v
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/infra/store/db.go internal/infra/model/token_stats.go internal/infra/store/token_stats_store.go internal/infra/store/token_stats_store_test.go
git commit -m "feat(tokenstat): add total_tokens column to bee_token_stats"
```

---

## Task 4: Compute `TotalTokens` in syncer

**Files:**
- Modify: `internal/tokenstat/syncer.go:177-188`

- [ ] **Step 1: Assign TotalTokens in storeUsages**

In `syncer.go`, update the `model.TokenStats` literal inside the `for _, u := range usages` loop (lines 178-187):

```go
if err := s.tokenStore.UpsertTx(tx, model.TokenStats{
    SessionID:           u.SessionID,
    AgentType:           u.AgentType,
    Model:               u.Model,
    InputTokens:         u.InputTokens,
    OutputTokens:        u.OutputTokens,
    CacheCreationTokens: u.CacheCreationTokens,
    CacheReadTokens:     u.CacheReadTokens,
    TotalTokens:         u.InputTokens + u.OutputTokens + u.CacheCreationTokens + u.CacheReadTokens,
    SyncedAt:            now,
}); err != nil {
```

- [ ] **Step 2: Build the full package**

```bash
go build ./...
```

Expected: no output.

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tokenstat/syncer.go
git commit -m "feat(tokenstat): compute and persist total_tokens in syncer"
```
