# /stop Cancel Platform Messages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When `/stop` is issued, cancel all `received` (and their associated `merged`) messages for the session in `bee_platform_messages`, preventing accumulated messages from entering the Feeder processing queue.

**Architecture:** Add a `CancelReceivedBySessionKey` method to `MessageStore`, expose it behind a new `messageCanceller` interface in `CommandInterceptor`, call it as a new step in `handleStop`. Wire the real `MessageStore` as the `messageCanceller` in `internal/app/app.go`.

**Tech Stack:** Go, SQLite (via `modernc.org/sqlite`), standard `database/sql`

---

## File Map

| File | Change |
|---|---|
| `internal/infra/store/message_store.go` | Add `MsgStatusCancelled` constant + `CancelReceivedBySessionKey` method |
| `internal/infra/store/message_store_test.go` | Add `TestMessageStore_CancelReceivedBySessionKey` |
| `internal/domain/bee/command_interceptor.go` | Add `messageCanceller` interface, update struct/constructor/`handleStop` |
| `internal/domain/bee/command_interceptor_test.go` | Add `mockMsgCanceller`, update `setupCommandInterceptorTest`, add new test |
| `internal/app/app.go` | Pass `s.msgStore` as `messageCanceller` to `NewCommandInterceptor` |

---

### Task 1: Add `CancelReceivedBySessionKey` to MessageStore (TDD)

**Files:**
- Modify: `internal/infra/store/message_store.go`
- Test: `internal/infra/store/message_store_test.go`

- [ ] **Step 1: Write the failing test**

Open `internal/infra/store/message_store_test.go` and add at the end of the file:

```go
func TestMessageStore_CancelReceivedBySessionKey(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	now := time.Now().UnixMilli()

	// Session under test: one primary (received) with one merged sub-message
	primaryID := "primary-1"
	mergedID := "merged-1"
	// Session under test: a second standalone received message (no merged)
	primary2ID := "primary-2"
	// Another session — must NOT be affected
	otherID := "other-session-msg"
	// A feeding message in the same session — must NOT be affected
	feedingID := "feeding-1"

	seed := []BatchMsg{
		{ID: mergedID, SessionKey: "s1", Platform: "test", Content: "part1", MessageTime: now, Status: MsgStatusMerged, MergedInto: primaryID},
		{ID: primaryID, SessionKey: "s1", Platform: "test", Content: "full", MessageTime: now, Status: MsgStatusReceived},
		{ID: primary2ID, SessionKey: "s1", Platform: "test", Content: "another", MessageTime: now + 1, Status: MsgStatusReceived},
		{ID: otherID, SessionKey: "s2", Platform: "test", Content: "other", MessageTime: now, Status: MsgStatusReceived},
		{ID: feedingID, SessionKey: "s1", Platform: "test", Content: "feeding", MessageTime: now + 2, Status: MsgStatusFeeding},
	}
	if _, err := s.CreateBatch(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	n, err := s.CancelReceivedBySessionKey(ctx, "s1")
	if err != nil {
		t.Fatalf("CancelReceivedBySessionKey: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 received rows cancelled, got %d", n)
	}

	check := func(id, wantStatus string) {
		t.Helper()
		var got string
		if err := s.db.QueryRowContext(ctx, `SELECT status FROM bee_platform_messages WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("query %s: %v", id, err)
		}
		if got != wantStatus {
			t.Errorf("id=%s: want status=%q, got %q", id, wantStatus, got)
		}
	}

	check(primaryID, MsgStatusCancelled)
	check(primary2ID, MsgStatusCancelled)
	check(mergedID, MsgStatusCancelled)   // merged sub-message also cancelled
	check(otherID, MsgStatusReceived)     // other session unaffected
	check(feedingID, MsgStatusFeeding)    // feeding unaffected
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feature/stop-command
go test ./internal/infra/store/... -run TestMessageStore_CancelReceivedBySessionKey -v
```

Expected: compile error — `MsgStatusCancelled` and `CancelReceivedBySessionKey` undefined.

- [ ] **Step 3: Add `MsgStatusCancelled` constant**

In `internal/infra/store/message_store.go`, add to the `const` block after `MsgStatusFailed`:

```go
MsgStatusCancelled    = "cancelled"
```

- [ ] **Step 4: Add `CancelReceivedBySessionKey` method**

In `internal/infra/store/message_store.go`, add after `MarkFailed`:

```go
// CancelReceivedBySessionKey cancels all 'received' messages for the given
// session and their associated 'merged' sub-messages.
// Returns the number of 'received' rows cancelled (not counting merged rows).
func (s *MessageStore) CancelReceivedBySessionKey(ctx context.Context, sessionKey string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UnixMilli()

	// Step 1: cancel all 'received' rows for the session; collect their IDs.
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM bee_platform_messages WHERE session_key = ? AND status = ?`,
		sessionKey, MsgStatusReceived)
	if err != nil {
		return 0, fmt.Errorf("select received: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, tx.Commit()
	}

	args := make([]any, 0, 2+len(ids))
	args = append(args, MsgStatusCancelled, now)
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE bee_platform_messages SET status = ?, updated_at = ? WHERE id IN (`+inPlaceholders(len(ids))+`)`,
		args...)
	if err != nil {
		return 0, fmt.Errorf("cancel received: %w", err)
	}
	n, _ := res.RowsAffected()

	// Step 2: cancel associated 'merged' sub-messages.
	mergedArgs := make([]any, 0, 2+len(ids))
	mergedArgs = append(mergedArgs, MsgStatusCancelled, now)
	for _, id := range ids {
		mergedArgs = append(mergedArgs, id)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE bee_platform_messages SET status = ?, updated_at = ? WHERE status = 'merged' AND merged_into IN (`+inPlaceholders(len(ids))+`)`,
		mergedArgs...); err != nil {
		return 0, fmt.Errorf("cancel merged: %w", err)
	}

	return n, tx.Commit()
}
```

- [ ] **Step 5: Run the test to verify it passes**

```bash
go test ./internal/infra/store/... -run TestMessageStore_CancelReceivedBySessionKey -v
```

Expected: `PASS`

- [ ] **Step 6: Run the full store test suite**

```bash
go test ./internal/infra/store/... -v
```

Expected: all tests `PASS`

- [ ] **Step 7: Commit**

```bash
git add internal/infra/store/message_store.go internal/infra/store/message_store_test.go
git commit -m "feat(store): add CancelReceivedBySessionKey to MessageStore"
```

---

### Task 2: Add `messageCanceller` interface and update `CommandInterceptor` (TDD)

**Files:**
- Modify: `internal/domain/bee/command_interceptor.go`
- Test: `internal/domain/bee/command_interceptor_test.go`

- [ ] **Step 1: Write the failing test**

Open `internal/domain/bee/command_interceptor_test.go`.

Add `mockMsgCanceller` after `mockSessionClearer`:

```go
type mockMsgCanceller struct {
	mu        sync.Mutex
	cancelled []string // session keys passed
	n         int64
	err       error
}

func (m *mockMsgCanceller) CancelReceivedBySessionKey(_ context.Context, sessionKey string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelled = append(m.cancelled, sessionKey)
	return m.n, m.err
}
```

Update `setupCommandInterceptorTest` to include `mockMsgCanceller`. Replace the function with:

```go
func setupCommandInterceptorTest(t *testing.T) (
	*store.SessionStore,
	*store.ExecutionStore,
	*store.TaskStore,
	*mockExecStopper,
	*mockSessionClearer,
	*mockMsgCanceller,
	*mockSender,
	*bee.CommandInterceptor,
) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ss := store.NewSessionStore(db)
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTaskStore(db)

	stopper := &mockExecStopper{}
	clearer := &mockSessionClearer{}
	canceller := &mockMsgCanceller{}
	sender := &mockSender{}
	senders := map[string]platform.PlatformSenderAdapter{"local": sender}

	ci := bee.NewCommandInterceptor(ss, es, ts, stopper, clearer, canceller, senders, "claude-code")
	return ss, es, ts, stopper, clearer, canceller, sender, ci
}
```

Update existing tests that call `setupCommandInterceptorTest` — they destructure 7 return values; now there are 8. Update each call site:

```go
// Before (each test that ignores most returns):
_, _, _, _, clearer, sender, ci := setupCommandInterceptorTest(t)
// After:
_, _, _, _, clearer, _, sender, ci := setupCommandInterceptorTest(t)

// Before:
_, _, _, stopper, clearer, sender, ci := setupCommandInterceptorTest(t)
// After:
_, _, _, stopper, clearer, _, sender, ci := setupCommandInterceptorTest(t)

// Before:
ss, es, _, stopper, clearer, sender, ci := setupCommandInterceptorTest(t)
// After:
ss, es, _, stopper, clearer, _, sender, ci := setupCommandInterceptorTest(t)

// Before:
_, _, _, _, _, _, ci := setupCommandInterceptorTest(t)
// After:
_, _, _, _, _, _, _, ci := setupCommandInterceptorTest(t)

// Before:
ss, es, _, stopper, _, sender, ci := setupCommandInterceptorTest(t)
// After:
ss, es, _, stopper, _, _, sender, ci := setupCommandInterceptorTest(t)
```

Add a new test at the end of the file:

```go
func TestCommandInterceptor_Stop_CancelsReceivedMessages(t *testing.T) {
	_, _, _, _, clearer, canceller, sender, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	// Simulate 3 received messages pending
	canceller.n = 3

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "/stop"}}
	handled, err := ci.Intercept(ctx, "local:1", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("expected handled=true for /stop")
	}

	canceller.mu.Lock()
	defer canceller.mu.Unlock()
	if len(canceller.cancelled) == 0 || canceller.cancelled[0] != "local:1" {
		t.Errorf("expected CancelReceivedBySessionKey called with 'local:1', got %v", canceller.cancelled)
	}
	if len(clearer.cleared) == 0 {
		t.Error("expected ClearSession called")
	}
	if len(sender.sent) == 0 {
		t.Fatal("expected reply sent")
	}
	// 3 pending messages cancelled → should report "stopped"
	if sender.sent[0].Content != "已停止当前会话的所有任务" {
		t.Errorf("unexpected reply: %q", sender.sent[0].Content)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/domain/bee/... -run TestCommandInterceptor -v
```

Expected: compile errors — `NewCommandInterceptor` still has the old signature.

- [ ] **Step 3: Add `messageCanceller` interface and update `CommandInterceptor`**

In `internal/domain/bee/command_interceptor.go`:

**Add the interface** after `sessionClearer`:

```go
// messageCanceller cancels unprocessed platform messages for a session.
type messageCanceller interface {
	CancelReceivedBySessionKey(ctx context.Context, sessionKey string) (int64, error)
}
```

**Add field to struct** after `dispatcher`:

```go
msgCanceller  messageCanceller
```

**Update `NewCommandInterceptor` signature** — add `canceller messageCanceller` after `clearer sessionClearer`:

```go
func NewCommandInterceptor(
	ss *store.SessionStore,
	es *store.ExecutionStore,
	ts *store.TaskStore,
	stopper executionStopper,
	clearer sessionClearer,
	canceller messageCanceller,
	senders map[string]platform.PlatformSenderAdapter,
	engine string,
) *CommandInterceptor {
	return &CommandInterceptor{
		sessionStore: ss,
		execStore:    es,
		taskStore:    ts,
		execStopper:  stopper,
		dispatcher:   clearer,
		msgCanceller: canceller,
		senders:      senders,
		engine:       engine,
	}
}
```

**Update `handleStop`** — add the new step after `c.taskStore.CancelBySessionKey` and before `c.dispatcher.ClearSession`:

```go
n, err = c.msgCanceller.CancelReceivedBySessionKey(ctx, sessionKey)
if err != nil {
	log.Warn("stop command: cancel platform messages", zap.String("sessionKey", sessionKey), zap.Error(err))
} else if n > 0 {
	stopped = true
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/domain/bee/... -run TestCommandInterceptor -v
```

Expected: all `PASS`

- [ ] **Step 5: Run the full bee domain test suite**

```bash
go test ./internal/domain/bee/... -v
```

Expected: all `PASS`

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/command_interceptor.go internal/domain/bee/command_interceptor_test.go
git commit -m "feat(bee): /stop cancels unprocessed bee_platform_messages for session"
```

---

### Task 3: Wire-up in app.go

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Update `NewCommandInterceptor` call**

In `internal/app/app.go` at the line:

```go
ci := bee.NewCommandInterceptor(s.sessionStore, s.execStore, s.taskStore, mgr, disp, sendersByPlatform, cfg.Bee.EffectiveEngine())
```

Add `s.msgStore` as the `messageCanceller` argument (after `disp`):

```go
ci := bee.NewCommandInterceptor(s.sessionStore, s.execStore, s.taskStore, mgr, disp, s.msgStore, sendersByPlatform, cfg.Bee.EffectiveEngine())
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: all `PASS`

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "chore(app): wire MessageStore as messageCanceller in CommandInterceptor"
```
