# Cancel Task Notification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Send a distinct "task was cancelled" platform notification when a running worker task is cancelled, so the bee always receives feedback.

**Architecture:** Extend `task.FailureNotifier` with `NotifyTaskCancelled`, implement it in `PlatformFailureNotifier` using a new i18n key, then add a `notifyCancel()` helper to `TaskDispatcher` and call it in both cancellation paths.

**Tech Stack:** Go, sqlx (test DB), `internal/infra/i18n` for user-facing strings

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/i18n/messages.go` | Add `TaskCancelled` field to `FailureNotifierMessages` |
| `internal/infra/i18n/locales/en.yaml` | Add `task_cancelled` string |
| `internal/infra/i18n/locales/zh.yaml` | Add `task_cancelled` string |
| `internal/domain/task/dispatcher.go` | Extend `FailureNotifier` interface, add `notifyCancel()`, patch two cancel paths |
| `internal/domain/task/failure_notifier.go` | Add `NotifyTaskCancelled` method to `PlatformFailureNotifier` |
| `internal/domain/task/dispatcher_test.go` | Update `mockFailureNotifier`, add two cancel dispatcher tests |
| `internal/domain/task/failure_notifier_test.go` | Add `TestPlatformFailureNotifier_CancelSuccess` |

---

## Task 1: i18n — add task_cancelled strings

**Files:**
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`

- [ ] **Step 1: Add `TaskCancelled` field to `FailureNotifierMessages` in `messages.go`**

  Current struct (lines 333–338):
  ```go
  type FailureNotifierMessages struct {
      TaskFailed  string `yaml:"task_failed"`
      ParseFailed string `yaml:"parse_failed"`
      WorkerLine  string `yaml:"worker_line"`
      Failed      string `yaml:"failed"`
  }
  ```

  Replace with:
  ```go
  type FailureNotifierMessages struct {
      TaskFailed    string `yaml:"task_failed"`
      TaskCancelled string `yaml:"task_cancelled"`
      ParseFailed   string `yaml:"parse_failed"`
      WorkerLine    string `yaml:"worker_line"`
      Failed        string `yaml:"failed"`
  }
  ```

- [ ] **Step 2: Add `task_cancelled` to `en.yaml`**

  In `internal/infra/i18n/locales/en.yaml`, under `runtime.failure_notifier:`, add one line after `task_failed`:
  ```yaml
  failure_notifier:
    task_failed: "❌ Task execution failed"
    task_cancelled: "🚫 Task was cancelled"
    parse_failed: "\nmessage parse failed"
    worker_line: "\nWorker: %s"
    failed: "\nError: %s"
  ```

- [ ] **Step 3: Add `task_cancelled` to `zh.yaml`**

  In `internal/infra/i18n/locales/zh.yaml`, under `runtime.failure_notifier:`:
  ```yaml
  failure_notifier:
    task_failed: "❌ 任务执行失败"
    task_cancelled: "🚫 任务已被取消"
    parse_failed: "\n消息解析失败"
    worker_line: "\nWorker：%s"
    failed: "\n错误：%s"
  ```

- [ ] **Step 4: Verify build**

  ```bash
  go build ./...
  ```
  Expected: no errors.

- [ ] **Step 5: Commit**

  ```bash
  git add internal/infra/i18n/messages.go \
          internal/infra/i18n/locales/en.yaml \
          internal/infra/i18n/locales/zh.yaml
  git commit -m "feat(i18n): add task_cancelled notification string"
  ```

---

## Task 2: Extend `FailureNotifier` interface + stub + update mock

**Files:**
- Modify: `internal/domain/task/dispatcher.go` (interface only)
- Modify: `internal/domain/task/failure_notifier.go` (stub implementation)
- Modify: `internal/domain/task/dispatcher_test.go` (mock update)

- [ ] **Step 1: Extend `FailureNotifier` interface in `dispatcher.go`**

  Find (lines ~51–57):
  ```go
  // FailureNotifier sends failure notifications to users when a worker execution
  // fails abnormally.
  type FailureNotifier interface {
      NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
  }
  ```

  Replace with:
  ```go
  // FailureNotifier sends failure and cancellation notifications to users.
  type FailureNotifier interface {
      NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
      NotifyTaskCancelled(ctx context.Context, messageID string, workerName string) error
  }
  ```

- [ ] **Step 2: Add stub `NotifyTaskCancelled` to `PlatformFailureNotifier` in `failure_notifier.go`**

  After the closing brace of `NotifyTaskFailure`, add:
  ```go
  // NotifyTaskCancelled sends a cancellation notification to the originating message thread.
  // Full implementation comes after i18n wiring; this stub satisfies the interface.
  func (n *PlatformFailureNotifier) NotifyTaskCancelled(_ context.Context, _ string, _ string) error {
      return nil
  }
  ```

- [ ] **Step 3: Update `mockFailureNotifier` in `dispatcher_test.go` to track cancel calls**

  Find (lines ~153–182):
  ```go
  type mockFailureNotifier struct {
      mu    sync.Mutex
      calls []failureCall
  }

  type failureCall struct {
      messageID string
      info      model.FailureInfo
  }

  func (n *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID string, info model.FailureInfo) error {
      n.mu.Lock()
      defer n.mu.Unlock()
      n.calls = append(n.calls, failureCall{messageID: messageID, info: info})
      return nil
  }

  func (n *mockFailureNotifier) waitForCall(timeout time.Duration) bool {
      deadline := time.Now().Add(timeout)
      for time.Now().Before(deadline) {
          n.mu.Lock()
          count := len(n.calls)
          n.mu.Unlock()
          if count > 0 {
              return true
          }
          time.Sleep(10 * time.Millisecond)
      }
      return false
  }
  ```

  Replace with:
  ```go
  type mockFailureNotifier struct {
      mu          sync.Mutex
      calls       []failureCall
      cancelCalls []cancelCall
  }

  type failureCall struct {
      messageID string
      info      model.FailureInfo
  }

  type cancelCall struct {
      messageID  string
      workerName string
  }

  func (n *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID string, info model.FailureInfo) error {
      n.mu.Lock()
      defer n.mu.Unlock()
      n.calls = append(n.calls, failureCall{messageID: messageID, info: info})
      return nil
  }

  func (n *mockFailureNotifier) NotifyTaskCancelled(_ context.Context, messageID string, workerName string) error {
      n.mu.Lock()
      defer n.mu.Unlock()
      n.cancelCalls = append(n.cancelCalls, cancelCall{messageID: messageID, workerName: workerName})
      return nil
  }

  func (n *mockFailureNotifier) waitForCall(timeout time.Duration) bool {
      deadline := time.Now().Add(timeout)
      for time.Now().Before(deadline) {
          n.mu.Lock()
          count := len(n.calls)
          n.mu.Unlock()
          if count > 0 {
              return true
          }
          time.Sleep(10 * time.Millisecond)
      }
      return false
  }

  func (n *mockFailureNotifier) waitForCancelCall(timeout time.Duration) bool {
      deadline := time.Now().Add(timeout)
      for time.Now().Before(deadline) {
          n.mu.Lock()
          count := len(n.cancelCalls)
          n.mu.Unlock()
          if count > 0 {
              return true
          }
          time.Sleep(10 * time.Millisecond)
      }
      return false
  }
  ```

- [ ] **Step 4: Run existing tests to confirm nothing broke**

  ```bash
  go test ./internal/domain/task/... -count=1
  ```
  Expected: all existing tests PASS.

- [ ] **Step 5: Commit**

  ```bash
  git add internal/domain/task/dispatcher.go \
          internal/domain/task/failure_notifier.go \
          internal/domain/task/dispatcher_test.go
  git commit -m "feat(task): extend FailureNotifier interface with NotifyTaskCancelled"
  ```

---

## Task 3: Dispatcher cancel notification (TDD)

**Files:**
- Modify: `internal/domain/task/dispatcher_test.go` (add failing tests)
- Modify: `internal/domain/task/dispatcher.go` (notifyCancel + patch paths)

### Step group A — write failing tests

- [ ] **Step 1: Add `TestTaskDispatcher_CancelWhileWaitingForResult_NotifiesCancel`**

  This tests Path 2: task is executing and polling; cancel fires in `waitForResult`.
  Append to `dispatcher_test.go`:

  ```go
  func TestTaskDispatcher_CancelWhileWaitingForResult_NotifiesCancel(t *testing.T) {
      // ExecuteWorker returns immediately; polling always sees ExecStatusRunning.
      mgr := &mockExecManager{
          execResult: model.WorkerExecution{ID: "exec-poll-cancel"},
      }
      eq := &mockExecutionQuerier{
          result: model.WorkerExecution{ID: "exec-poll-cancel", Status: model.ExecStatusRunning},
      }
      fn := &mockFailureNotifier{}
      d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore(), task.WithFailureNotifier(fn))

      ctx, cancel := context.WithCancel(context.Background())
      defer cancel()
      go d.Run(ctx)

      t1 := immediateTask("s1", "w1", "long task")
      t1.TaskID = "task-poll-cancel"
      t1.MessageID = "msg-poll-cancel"
      in <- t1

      // Wait for ExecuteWorker to be called (task is now in waitForResult polling loop).
      if !waitForExecCount(mgr, 1, 2*time.Second) {
          t.Fatal("ExecuteWorker was not called within timeout")
      }

      // Cancel the task while it is being polled.
      if err := d.CancelTask(context.Background(), "task-poll-cancel"); err != nil {
          t.Fatalf("CancelTask: %v", err)
      }

      // Expect cancel notification to arrive.
      if !fn.waitForCancelCall(2 * time.Second) {
          t.Fatal("expected NotifyTaskCancelled to be called, but it was not")
      }
      fn.mu.Lock()
      defer fn.mu.Unlock()
      got := fn.cancelCalls[0]
      if got.messageID != "msg-poll-cancel" {
          t.Errorf("expected messageID=msg-poll-cancel, got %q", got.messageID)
      }
      if got.workerName != "w1" {
          t.Errorf("expected workerName=w1, got %q", got.workerName)
      }
  }
  ```

- [ ] **Step 2: Add `TestTaskDispatcher_CancelDuringResolve_NotifiesCancel`**

  This tests Path 1: cancel fires while `ExecuteWorker` is blocking (before `waitForResult`).
  Append to `dispatcher_test.go`:

  ```go
  func TestTaskDispatcher_CancelDuringResolve_NotifiesCancel(t *testing.T) {
      // cancelTrackingExecManager blocks ExecuteWorker until its ctx is cancelled.
      var cancelCount int64
      mgr := &cancelTrackingExecManager{cancelCount: &cancelCount}
      eq := &mockExecutionQuerier{
          result: model.WorkerExecution{ID: "exec-tracked", Status: model.ExecStatusCompleted},
      }
      fn := &mockFailureNotifier{}
      d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore(), task.WithFailureNotifier(fn))

      ctx, cancel := context.WithCancel(context.Background())
      defer cancel()
      go d.Run(ctx)

      t1 := immediateTask("s1", "w1", "blocked task")
      t1.TaskID = "task-resolve-cancel"
      t1.MessageID = "msg-resolve-cancel"
      in <- t1

      // Give the dispatcher time to start ExecuteWorker (which blocks on ctx.Done).
      time.Sleep(50 * time.Millisecond)

      // Cancel the task; this unblocks ExecuteWorker via taskCtx cancellation.
      if err := d.CancelTask(context.Background(), "task-resolve-cancel"); err != nil {
          t.Fatalf("CancelTask: %v", err)
      }

      // Expect cancel notification to arrive.
      if !fn.waitForCancelCall(2 * time.Second) {
          t.Fatal("expected NotifyTaskCancelled to be called, but it was not")
      }
      fn.mu.Lock()
      defer fn.mu.Unlock()
      got := fn.cancelCalls[0]
      if got.messageID != "msg-resolve-cancel" {
          t.Errorf("expected messageID=msg-resolve-cancel, got %q", got.messageID)
      }
  }
  ```

- [ ] **Step 3: Run failing tests to confirm they fail**

  ```bash
  go test ./internal/domain/task/... -run "TestTaskDispatcher_Cancel.*NotifiesCancel" -v -count=1
  ```
  Expected: both tests FAIL with "expected NotifyTaskCancelled to be called, but it was not" (timeout).

### Step group B — implement

- [ ] **Step 4: Add `notifyCancel()` helper to `TaskDispatcher` in `dispatcher.go`**

  After the closing brace of `notifyFailure` (around line 451):
  ```go
  func (d *TaskDispatcher) notifyCancel(ctx context.Context, messageID, workerName string) {
      if d.failureNotifier == nil || messageID == "" {
          return
      }
      if err := d.failureNotifier.NotifyTaskCancelled(ctx, messageID, workerName); err != nil {
          log.Error("notify task cancel", zap.String("messageID", messageID), zap.Error(err))
      }
  }
  ```

- [ ] **Step 5: Patch Path 1 in `executeAsync`**

  Find (around line 302–306):
  ```go
  // Cancellation may have arrived while resolveExecution was in-flight; kill the
  // just-launched worker before entering waitForResult.
  if taskCtx.Err() != nil {
      d.manager.CancelExecution(context.Background(), exec.ID) //nolint:errcheck
      return
  }
  ```

  Replace with:
  ```go
  // Cancellation may have arrived while resolveExecution was in-flight; kill the
  // just-launched worker before entering waitForResult.
  if taskCtx.Err() != nil {
      d.manager.CancelExecution(context.Background(), exec.ID) //nolint:errcheck
      d.notifyCancel(context.Background(), task.MessageID, task.WorkerID)
      return
  }
  ```

- [ ] **Step 6: Patch Path 2 in `waitForResult`**

  Find (around line 418–424):
  ```go
  select {
  case <-ticker.C:
  case <-ctx.Done():
      // Task was cancelled — kill the worker process.
      d.manager.CancelExecution(context.Background(), executionID) //nolint:errcheck
      return
  }
  ```

  Replace with:
  ```go
  select {
  case <-ticker.C:
  case <-ctx.Done():
      // Task was cancelled — kill the worker process.
      d.manager.CancelExecution(context.Background(), executionID) //nolint:errcheck
      d.notifyCancel(context.Background(), task.MessageID, workerName(exec.WorkerName, task.WorkerID))
      return
  }
  ```

  Note: `exec` and `task` are both in scope in `waitForResult`. The method signature is:
  `func (d *TaskDispatcher) waitForResult(ctx context.Context, executionID string, task DispatchTask, engineName string)`
  and `exec` is the loop variable from `d.execStore.GetByID(executionID)`.

- [ ] **Step 7: Run the new tests to confirm they pass**

  ```bash
  go test ./internal/domain/task/... -run "TestTaskDispatcher_Cancel.*NotifiesCancel" -v -count=1
  ```
  Expected: both tests PASS.

- [ ] **Step 8: Run the full test suite to confirm no regressions**

  ```bash
  go test ./internal/domain/task/... -count=1
  ```
  Expected: all tests PASS.

- [ ] **Step 9: Commit**

  ```bash
  git add internal/domain/task/dispatcher.go \
          internal/domain/task/dispatcher_test.go
  git commit -m "feat(task): notify on task cancellation via notifyCancel"
  ```

---

## Task 4: Implement `PlatformFailureNotifier.NotifyTaskCancelled` (TDD)

**Files:**
- Modify: `internal/domain/task/failure_notifier_test.go` (add failing test)
- Modify: `internal/domain/task/failure_notifier.go` (replace stub with real impl)

- [ ] **Step 1: Write the failing test**

  Append to `internal/domain/task/failure_notifier_test.go`:

  ```go
  func TestPlatformFailureNotifier_CancelSuccess(t *testing.T) {
      notifier, ms, sender := setupNotifier(t, "test")
      ctx := context.Background()

      _, err := ms.Create(ctx, "msg-cancel-1", "sess-cancel", "test", "cancel me", `{"raw":true}`, "", 0)
      if err != nil {
          t.Fatalf("create message: %v", err)
      }

      err = notifier.NotifyTaskCancelled(ctx, "msg-cancel-1", "my-worker")
      if err != nil {
          t.Fatalf("unexpected error: %v", err)
      }

      sender.mu.Lock()
      defer sender.mu.Unlock()
      if len(sender.sent) != 1 {
          t.Fatalf("expected 1 message sent, got %d", len(sender.sent))
      }
      content := sender.sent[0].Content
      if !strings.Contains(content, "Task was cancelled") {
          t.Errorf("expected cancel prefix in content, got: %s", content)
      }
      if !strings.Contains(content, "my-worker") {
          t.Errorf("expected worker name in content, got: %s", content)
      }
      if strings.Contains(content, "Task execution failed") {
          t.Errorf("cancel notification must not contain failure prefix, got: %s", content)
      }
      if strings.Contains(content, "Error:") {
          t.Errorf("cancel notification must not contain error line, got: %s", content)
      }
      if sender.sent[0].ReplyTo.Platform != "test" {
          t.Errorf("expected platform=test, got %s", sender.sent[0].ReplyTo.Platform)
      }
  }
  ```

- [ ] **Step 2: Run failing test to confirm it fails**

  ```bash
  go test ./internal/domain/task/... -run TestPlatformFailureNotifier_CancelSuccess -v -count=1
  ```
  Expected: FAIL — content does not contain "Task was cancelled" (stub returns nil without sending).

- [ ] **Step 3: Replace the stub with the real implementation in `failure_notifier.go`**

  Find and replace the stub `NotifyTaskCancelled`:
  ```go
  // NotifyTaskCancelled sends a cancellation notification to the originating message thread.
  // Full implementation comes after i18n wiring; this stub satisfies the interface.
  func (n *PlatformFailureNotifier) NotifyTaskCancelled(_ context.Context, _ string, _ string) error {
      return nil
  }
  ```

  Replace with:
  ```go
  func (n *PlatformFailureNotifier) NotifyTaskCancelled(ctx context.Context, messageID string, workerName string) error {
      stored, err := n.msgStore.GetByID(ctx, messageID)
      if err != nil {
          return fmt.Errorf("get message for cancel notification: %w", err)
      }

      sender, ok := n.senders[stored.Platform]
      if !ok {
          return fmt.Errorf("no sender for platform %q", stored.Platform)
      }

      m := i18n.M.Runtime.FailureNotifier
      content := m.TaskCancelled
      if workerName != "" {
          content += fmt.Sprintf(m.WorkerLine, workerName)
      }
      const maxRunes = 500
      runes := []rune(content)
      if len(runes) > maxRunes {
          content = string(runes[:maxRunes-1]) + "…"
      }

      outbound := platform.OutboundMessage{
          Content: content,
          ReplyTo: platform.InboundMessage{
              Platform:   stored.Platform,
              SessionKey: stored.SessionKey,
              Raw:        stored.Raw,
          },
          SourceType:   store.SourceTypeSystem,
          InboundMsgID: messageID,
      }
      if err := sender.Send(ctx, outbound); err != nil {
          log.Error("send cancel notification", zap.String("messageID", messageID), zap.Error(err))
          return fmt.Errorf("send cancel notification: %w", err)
      }
      return nil
  }
  ```

- [ ] **Step 4: Run the new test to confirm it passes**

  ```bash
  go test ./internal/domain/task/... -run TestPlatformFailureNotifier_CancelSuccess -v -count=1
  ```
  Expected: PASS.

- [ ] **Step 5: Run the full test suite**

  ```bash
  go test ./internal/domain/task/... -count=1
  ```
  Expected: all tests PASS.

- [ ] **Step 6: Run build check**

  ```bash
  go build ./...
  ```
  Expected: no errors.

- [ ] **Step 7: Commit**

  ```bash
  git add internal/domain/task/failure_notifier.go \
          internal/domain/task/failure_notifier_test.go
  git commit -m "feat(task): implement PlatformFailureNotifier.NotifyTaskCancelled"
  ```
