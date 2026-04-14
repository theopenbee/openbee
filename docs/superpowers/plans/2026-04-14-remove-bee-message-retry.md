# Remove Bee Message Retry Mechanism Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the bee message retry mechanism so that any bee execution failure immediately marks the message as `failed` and notifies the user.

**Architecture:** Pure deletion refactor across 6 task groups — no new logic, only removal. Add `MarkFailed()` to `MessageStore`, rewrite `rollback()` → `failMessages()` in `Feeder`, strip `RetryCount`/`MaxRetries` from `FailureInfo`, clean up `MessageStore`, add DB migration to drop `retry_count` column, and remove i18n retry strings.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), standard library testing.

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/store/message_store.go` | Add `MarkFailed()`, remove `RollbackWithRetry()`, remove `RetryCount` from `ClaimedMessage` + SELECT |
| `internal/infra/store/message_store_test.go` | Delete `RollbackWithRetry` tests, add `MarkFailed` test |
| `internal/domain/bee/feeder.go` | Rename `rollback()` → `failMessages()`, remove retry logic |
| `internal/domain/bee/feeder_test.go` | Replace exhaust-retries test with immediate-fail test |
| `internal/domain/bee/constants.go` | Remove `MaxRetries = 3` |
| `internal/infra/model/execution.go` | Remove `RetryCount` and `MaxRetries` from `FailureInfo` |
| `internal/domain/task/failure_notifier.go` | Remove `RetryCount` conditional branch |
| `internal/domain/task/dispatcher.go` | Remove `RetryCount: -1` from both `FailureInfo` literals |
| `internal/domain/task/failure_notifier_test.go` | Delete `WithRetry` test, remove `RetryCount` from all `FailureInfo` literals |
| `internal/infra/store/db.go` | Add migration 31: `DROP COLUMN retry_count` |
| `internal/infra/i18n/messages.go` | Remove `RetriedCount` field from `FailureNotifierMessages` |
| `internal/infra/i18n/locales/en.yaml` | Remove `retried_count` line |
| `internal/infra/i18n/locales/zh.yaml` | Remove `retried_count` line |

---

### Task 1: Add `MarkFailed` to `MessageStore`

**Files:**
- Modify: `internal/infra/store/message_store.go` (after line 179, after `MarkBeeProcessed`)
- Modify: `internal/infra/store/message_store_test.go`

- [ ] **Step 1: Write the failing test**

Add at the end of `internal/infra/store/message_store_test.go`:

```go
func TestMessageStore_MarkFailed(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "m1", "feishu:c:u", "feishu", "hello", "", "", 0) //nolint
	s.UpdateStatusBatch(ctx, []string{"m1"}, "feeding")              //nolint

	if err := s.MarkFailed(ctx, []string{"m1"}); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	var status string
	s.db.QueryRowContext(ctx, `SELECT status FROM bee_platform_messages WHERE id = 'm1'`).Scan(&status) //nolint
	if status != "failed" {
		t.Errorf("expected status=failed, got %q", status)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/infra/store/... -run TestMessageStore_MarkFailed -v
```

Expected: `FAIL` — `s.MarkFailed undefined`

- [ ] **Step 3: Add `MarkFailed` implementation**

In `internal/infra/store/message_store.go`, add after the `MarkBeeProcessed` method (after line 179):

```go
// MarkFailed sets status to 'failed' for the given message IDs.
func (s *MessageStore) MarkFailed(ctx context.Context, ids []string) error {
	return s.UpdateStatusBatch(ctx, ids, MsgStatusFailed)
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/infra/store/... -run TestMessageStore_MarkFailed -v
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/message_store.go internal/infra/store/message_store_test.go
git commit -m "feat: add MarkFailed to MessageStore"
```

---

### Task 2: Replace `rollback()` with `failMessages()` in `Feeder`

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/bee/feeder_test.go`
- Modify: `internal/domain/bee/constants.go`

- [ ] **Step 1: Replace the exhaust-retries test with an immediate-fail test**

In `internal/domain/bee/feeder_test.go`, replace the entire `TestFeeder_ExhaustsRetries_MarksFailedAndNotifies` function (lines 428–467) with:

```go
func TestFeeder_ImmediateFailure_MarksFailedAndNotifies(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	notifier := &mockFailureNotifier{}
	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, bee.WithFailureNotifier(notifier))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go f.Run(ctx)

	// One poll cycle is enough — failure must be immediate, no retries.
	time.Sleep(bee.PollInterval + 500*time.Millisecond)

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	if status != "failed" {
		t.Errorf("expected status=failed immediately after one failure, got %q", status)
	}

	calls := notifier.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected notifier called once, got %d calls", len(calls))
	}
	if calls[0].messageID != "m1" {
		t.Errorf("expected notifier called with messageID m1, got %q", calls[0].messageID)
	}
	if calls[0].info.Reason == "" {
		t.Error("expected non-empty Reason in FailureInfo")
	}
}
```

Also remove the import of `bee.MaxRetries` if it appears anywhere in the file — after this step the constant still exists, so compilation is fine.

- [ ] **Step 2: Run to verify the new test fails**

```bash
go test ./internal/domain/bee/... -run TestFeeder_ImmediateFailure -v
```

Expected: `FAIL` — status is `received` (not `failed`) because the old code still retries.

- [ ] **Step 3: Rewrite `rollback()` → `failMessages()` in `feeder.go`**

In `internal/domain/bee/feeder.go`, replace the entire `rollback` method (lines 253–281) with:

```go
func (f *Feeder) failMessages(ctx context.Context, msgs []store.ClaimedMessage, reason string) {
	ids := messageIDs(msgs)
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Error("fail messages delete tasks", zap.Error(err))
	}
	if err := f.msgStore.MarkFailed(ctx, ids); err != nil {
		log.Error("mark messages failed", zap.Error(err))
		return
	}
	if f.failureNotifier == nil {
		return
	}
	for _, m := range msgs {
		log.Warn("message failed", zap.String("messageID", m.ID))
		if notifyErr := f.failureNotifier.NotifyTaskFailure(ctx, m.ID, model.FailureInfo{Reason: reason}); notifyErr != nil {
			log.Error("notify bee failure", zap.String("messageID", m.ID), zap.Error(notifyErr))
		}
	}
}
```

Then rename all four call sites of `f.rollback(` to `f.failMessages(` within `processBeeGroup` (lines ~155, ~185, ~193, ~229 in the original file). Do a search-and-replace for `f.rollback(` → `f.failMessages(`.

- [ ] **Step 4: Remove `MaxRetries` from `constants.go`**

In `internal/domain/bee/constants.go`, remove lines 12–14:

```go
	// MaxRetries is the maximum number of times a message can be retried after failure.
	// Once retry_count reaches MaxRetries the message is permanently marked 'failed'.
	MaxRetries = 3
```

The file should now contain only `PollInterval` and `QueueWarnThreshold`.

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/domain/bee/... -v
```

Expected: all tests `PASS`. The new `TestFeeder_ImmediateFailure_MarksFailedAndNotifies` should now pass.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_test.go internal/domain/bee/constants.go
git commit -m "refactor: remove bee message retry logic, fail immediately on error"
```

---

### Task 3: Remove `RetryCount`/`MaxRetries` from `FailureInfo` and all callers

All four files must be updated in the same step because removing fields from a struct breaks compilation in callers.

**Files:**
- Modify: `internal/infra/model/execution.go`
- Modify: `internal/domain/task/failure_notifier.go`
- Modify: `internal/domain/task/dispatcher.go`
- Modify: `internal/domain/task/failure_notifier_test.go`

- [ ] **Step 1: Update `failure_notifier_test.go`**

a. Delete the entire `TestPlatformFailureNotifier_StructuredFormat_WithRetry` function (lines 159–190).

b. In `TestPlatformFailureNotifier_StructuredFormat_NoRetry`, rename the function to `TestPlatformFailureNotifier_StructuredFormat`, and remove the `RetryCount: -1` field from the `FailureInfo` literal. The function should look like:

```go
func TestPlatformFailureNotifier_StructuredFormat(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-5", "sess-5", "test", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	info := model.FailureInfo{
		Reason:     "launch failed",
		WorkerName: "worker-abc",
	}
	if err := notifier.NotifyTaskFailure(ctx, "msg-5", info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	content := sender.sent[0].Content
	if strings.Contains(content, "Retried") {
		t.Errorf("expected no retry line in content, got: %s", content)
	}
	if !strings.Contains(content, "worker-abc") {
		t.Errorf("expected WorkerName in content, got: %s", content)
	}
}
```

c. In all remaining test functions (`TestPlatformFailureNotifier_Success`, `TestPlatformFailureNotifier_MessageNotFound`, `TestPlatformFailureNotifier_UnknownPlatform`, `TestPlatformFailureNotifier_TruncatesLongMessage`), remove the `RetryCount: -1` field from each `FailureInfo` literal. For example:

```go
// before
info := model.FailureInfo{
    Reason:     "API Error: content filtered",
    WorkerName: "my-worker",
    RetryCount: -1,
}

// after
info := model.FailureInfo{
    Reason:     "API Error: content filtered",
    WorkerName: "my-worker",
}
```

- [ ] **Step 2: Remove `RetryCount` and `MaxRetries` from `FailureInfo`**

In `internal/infra/model/execution.go`, replace the `FailureInfo` struct (lines 27–32) with:

```go
// FailureInfo carries context for a task failure notification sent to the user.
type FailureInfo struct {
	Reason     string // raw error (exec.Result or err.Error())
	WorkerName string // worker or bee name for identification
}
```

- [ ] **Step 3: Simplify `failure_notifier.go`**

In `internal/domain/task/failure_notifier.go`, replace lines 38–51 (the `workerLine` and `content` block) with:

```go
	m := i18n.M.Runtime.FailureNotifier
	var workerLine string
	if info.WorkerName != "" {
		workerLine = fmt.Sprintf(m.WorkerLine, info.WorkerName)
	} else {
		workerLine = m.ParseFailed
	}
	content := m.TaskFailed + workerLine + fmt.Sprintf(m.Failed, info.Reason)
```

- [ ] **Step 4: Remove `RetryCount: -1` from `dispatcher.go`**

In `internal/domain/task/dispatcher.go`, find and update both `FailureInfo` literals:

First (around line 281):
```go
// before
d.notifyFailure(taskCtx, task.MessageID, model.FailureInfo{
    Reason:     err.Error(),
    WorkerName: task.WorkerID,
    RetryCount: -1,
})

// after
d.notifyFailure(taskCtx, task.MessageID, model.FailureInfo{
    Reason:     err.Error(),
    WorkerName: task.WorkerID,
})
```

Second (around line 398):
```go
// before
d.notifyFailure(ctx, task.MessageID, model.FailureInfo{
    Reason:     exec.Result,
    WorkerName: workerName(exec.WorkerName, task.WorkerID),
    RetryCount: -1,
})

// after
d.notifyFailure(ctx, task.MessageID, model.FailureInfo{
    Reason:     exec.Result,
    WorkerName: workerName(exec.WorkerName, task.WorkerID),
})
```

- [ ] **Step 5: Run all tests**

```bash
go test ./internal/domain/task/... ./internal/infra/model/... -v
```

Expected: all tests `PASS`.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/model/execution.go \
        internal/domain/task/failure_notifier.go \
        internal/domain/task/dispatcher.go \
        internal/domain/task/failure_notifier_test.go
git commit -m "refactor: remove RetryCount/MaxRetries from FailureInfo"
```

---

### Task 4: Remove `RollbackWithRetry` and `RetryCount` from `MessageStore`

**Files:**
- Modify: `internal/infra/store/message_store.go`
- Modify: `internal/infra/store/message_store_test.go`

- [ ] **Step 1: Delete `RollbackWithRetry` tests from `message_store_test.go`**

Delete the two functions:
- `TestMessageStore_RollbackWithRetry_BelowLimit` (lines 359–382)
- `TestMessageStore_RollbackWithRetry_ExhaustsRetries` (lines 384–405)

- [ ] **Step 2: Remove `RetryCount` from `ClaimedMessage` struct**

In `internal/infra/store/message_store.go`, update `ClaimedMessage` (lines 106–113) to:

```go
// ClaimedMessage is a bee_platform_messages row claimed by the Feeder.
type ClaimedMessage struct {
	ID         string
	SessionKey string
	Platform   string
	Content    string
}
```

- [ ] **Step 3: Remove `retry_count` from `ClaimBatch` SELECT and `Scan`**

In `ClaimBatch`, update the query (line 126) from:

```go
`SELECT id, session_key, platform, content, retry_count
```

to:

```go
`SELECT id, session_key, platform, content
```

Update the `Scan` call (line 146) from:

```go
if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.RetryCount); err != nil {
```

to:

```go
if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content); err != nil {
```

- [ ] **Step 4: Delete `RollbackWithRetry` method**

In `internal/infra/store/message_store.go`, delete the entire `RollbackWithRetry` method (lines 181–205, including its comment block).

- [ ] **Step 5: Run all store tests**

```bash
go test ./internal/infra/store/... -v
```

Expected: all tests `PASS`. Verify `TestMessageStore_MarkFailed` still passes.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/message_store.go internal/infra/store/message_store_test.go
git commit -m "refactor: remove RollbackWithRetry and RetryCount from MessageStore"
```

---

### Task 5: Add DB migration to drop `retry_count` column

**Files:**
- Modify: `internal/infra/store/db.go`

- [ ] **Step 1: Add migration 31**

In `internal/infra/store/db.go`, append the following entry to the `migrations` slice (after the last entry, currently version 30):

```go
{
    version: 31,
    name:    "drop_retry_count_from_platform_messages",
    sql:     `ALTER TABLE bee_platform_messages DROP COLUMN retry_count`,
},
```

- [ ] **Step 2: Run all tests**

```bash
go test ./internal/infra/store/... -v
```

Expected: all tests `PASS`. The migration runs automatically via `InitDB` in each test's `setupMessageStore` / `setupFeederDB` helper.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/db.go
git commit -m "feat: migration 31 — drop retry_count from bee_platform_messages"
```

---

### Task 6: Remove `retried_count` from i18n

**Files:**
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`

- [ ] **Step 1: Remove `RetriedCount` field from `FailureNotifierMessages`**

In `internal/infra/i18n/messages.go`, delete line 286:

```go
RetriedCount string `yaml:"retried_count"` // suffix with retry info; contains %d %d %s
```

- [ ] **Step 2: Remove `retried_count` from `en.yaml`**

In `internal/infra/i18n/locales/en.yaml`, delete line 212:

```yaml
    retried_count: "\nRetried: %d/%d\nError: %s"
```

- [ ] **Step 3: Remove `retried_count` from `zh.yaml`**

In `internal/infra/i18n/locales/zh.yaml`, delete line 212:

```yaml
    retried_count: "\n已重试：%d/%d 次\n错误：%s"
```

- [ ] **Step 4: Run all tests**

```bash
go test ./... 
```

Expected: all tests `PASS`. Verify the full test suite is green.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/i18n/messages.go \
        internal/infra/i18n/locales/en.yaml \
        internal/infra/i18n/locales/zh.yaml
git commit -m "refactor: remove retried_count from i18n after retry mechanism removal"
```
