# Stale Message Handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a delayed message arrives after a newer message in the same session has already been processed, mark it as `stale` and skip it.

**Architecture:** Add a `MsgStatusStale` constant and extend `ClaimBatch` to run a single UPDATE inside the existing transaction that marks any `received` message as `stale` when its session already has a `bee_processed` message with a higher `received_at`. No other files change.

**Tech Stack:** Go, SQLite (`database/sql`), existing test helper `setupMessageStore`.

---

### Task 1: Add stale status constant and test coverage

**Files:**
- Modify: `internal/infra/store/message_store.go:12-18`
- Test: `internal/infra/store/message_store_test.go`

- [ ] **Step 1: Write the failing test**

Add the following test at the end of `internal/infra/store/message_store_test.go`:

```go
func TestMessageStore_ClaimBatch_StalesLateArrivingMessage(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewMessageStore(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	// Message B arrived first and was processed (received_at = now+1000).
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgB', 'sk1', 'feishu', 'newer', 'bee_processed', ?, ?, ?)`, now+1000, now, now)
	// Message A arrives late (received_at = now, older than B).
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgA', 'sk1', 'feishu', 'older', 'received', ?, ?, ?)`, now, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 claimed messages (msgA is stale), got %d", len(msgs))
	}

	var status string
	if err := db.QueryRowContext(ctx,
		`SELECT status FROM bee_platform_messages WHERE id = 'msgA'`,
	).Scan(&status); err != nil {
		t.Fatalf("scan msgA status: %v", err)
	}
	if status != MsgStatusStale {
		t.Errorf("msgA: want status=%q, got %q", MsgStatusStale, status)
	}
}

func TestMessageStore_ClaimBatch_DoesNotStaleMessageWithNoNewerProcessed(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewMessageStore(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	// One received message, no bee_processed in the session — should be claimed normally.
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgA', 'sk1', 'feishu', 'hello', 'received', ?, ?, ?)`, now, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 claimed message, got %d", len(msgs))
	}
	if msgs[0].ID != "msgA" {
		t.Errorf("expected msgA to be claimed, got %q", msgs[0].ID)
	}
}

func TestMessageStore_ClaimBatch_DoesNotStaleMessageNewerThanProcessed(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewMessageStore(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	// Message A processed (received_at = now), message B received (received_at = now+1000).
	// B is newer than A — it should be claimed, not staled.
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgA', 'sk1', 'feishu', 'older', 'bee_processed', ?, ?, ?)`, now, now, now)
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgB', 'sk1', 'feishu', 'newer', 'received', ?, ?, ?)`, now+1000, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 claimed message, got %d", len(msgs))
	}
	if msgs[0].ID != "msgB" {
		t.Errorf("expected msgB to be claimed, got %q", msgs[0].ID)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/infra/store/... -run "TestMessageStore_ClaimBatch_Stales|TestMessageStore_ClaimBatch_DoesNot" -v
```

Expected: FAIL — `MsgStatusStale` undefined, and stale assertions fail.

- [ ] **Step 3: Add MsgStatusStale constant**

In `internal/infra/store/message_store.go`, extend the `const` block (lines 12–18):

```go
const (
	MsgStatusReceived     = "received"
	MsgStatusFeeding      = "feeding"
	MsgStatusMerged       = "merged"
	MsgStatusBeeProcessed = "bee_processed"
	MsgStatusFailed       = "failed"
	MsgStatusStale        = "stale"
)
```

- [ ] **Step 4: Add stale UPDATE inside ClaimBatch transaction**

In `internal/infra/store/message_store.go`, immediately after the `tx, err := s.db.BeginTx(...)` / `defer tx.Rollback()` lines (currently lines 118–122), add the following UPDATE before the existing SELECT:

```go
// Mark received messages as stale when a newer bee_processed message exists in the same session.
if _, err := tx.ExecContext(ctx,
    `UPDATE bee_platform_messages
     SET    status = ?, updated_at = ?
     WHERE  status = ?
       AND  EXISTS (
              SELECT 1
              FROM   bee_platform_messages b2
              WHERE  b2.session_key  = bee_platform_messages.session_key
                AND  b2.status       = ?
                AND  b2.received_at  > bee_platform_messages.received_at
            )`,
    MsgStatusStale, time.Now().UnixMilli(), MsgStatusReceived, MsgStatusBeeProcessed,
); err != nil {
    return nil, fmt.Errorf("mark stale: %w", err)
}
```

The complete `ClaimBatch` function body after this change:

```go
func (s *MessageStore) ClaimBatch(ctx context.Context, batchSize int) ([]ClaimedMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Mark received messages as stale when a newer bee_processed message exists in the same session.
	if _, err := tx.ExecContext(ctx,
		`UPDATE bee_platform_messages
		 SET    status = ?, updated_at = ?
		 WHERE  status = ?
		   AND  EXISTS (
		          SELECT 1
		          FROM   bee_platform_messages b2
		          WHERE  b2.session_key  = bee_platform_messages.session_key
		            AND  b2.status       = ?
		            AND  b2.received_at  > bee_platform_messages.received_at
		        )`,
		MsgStatusStale, time.Now().UnixMilli(), MsgStatusReceived, MsgStatusBeeProcessed,
	); err != nil {
		return nil, fmt.Errorf("mark stale: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, session_key, platform, content
		 FROM bee_platform_messages m
		 WHERE status = 'received'
		   AND session_key NOT IN (
		       SELECT session_key FROM bee_platform_messages WHERE status = 'feeding'
		   )
		   AND received_at = (
		       SELECT MIN(received_at)
		       FROM bee_platform_messages m2
		       WHERE m2.session_key = m.session_key
		         AND m2.status = 'received'
		   )
		 ORDER BY received_at ASC
		 LIMIT ?`, batchSize)
	if err != nil {
		return nil, fmt.Errorf("select batch: %w", err)
	}
	var msgs []ClaimedMessage
	for rows.Next() {
		var m ClaimedMessage
		if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan: %w", err)
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}

	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	args := make([]any, 0, len(ids)+2)
	args = append(args, MsgStatusFeeding, time.Now().UnixMilli())
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE bee_platform_messages SET status = ?, updated_at = ? WHERE id IN (`+inPlaceholders(len(ids))+`)`, args...); err != nil {
		return nil, fmt.Errorf("update feeding: %w", err)
	}
	return msgs, tx.Commit()
}
```

- [ ] **Step 5: Run all store tests to verify everything passes**

```bash
go test ./internal/infra/store/... -v
```

Expected: All tests PASS including the three new ones.

- [ ] **Step 6: Run full test suite**

```bash
go test ./...
```

Expected: All tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/infra/store/message_store.go internal/infra/store/message_store_test.go
git commit -m "feat: mark stale late-arriving messages in ClaimBatch"
```
