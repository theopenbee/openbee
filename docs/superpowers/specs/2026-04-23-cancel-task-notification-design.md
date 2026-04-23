# Cancel Task Notification Design

**Date:** 2026-04-23  
**Branch:** fix/cancel-task-dispatcher  
**Status:** Approved

## Problem

When a worker task is cancelled while it is actively executing, the bee/user receives no notification. Two code paths in `TaskDispatcher` both silently return after killing the worker process:

- **Path 1** (`dispatcher.go:302–306`): Cancellation arrives after `resolveExecution` completes but before `waitForResult` starts.
- **Path 2** (`dispatcher.go:420–423`): Cancellation detected via `ctx.Done()` inside the `waitForResult` polling loop.

In contrast, the `ExecStatusFailed` branch calls `notifyFailure()` — both cancellation paths bypass this entirely.

## Goal

Send a distinct "task was cancelled" platform notification to the originating message thread whenever a running or about-to-run worker task is cancelled, matching the user-facing clarity of the existing failure notification.

## Design

### 1. Interface — `task.FailureNotifier`

Add one method to the existing interface in `internal/domain/task/dispatcher.go`:

```go
type FailureNotifier interface {
    NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
    NotifyTaskCancelled(ctx context.Context, messageID string, workerName string) error
}
```

`bee.FailureNotifier` (in `internal/domain/bee/feeder.go`) is a **separate interface** used only for message-level failures. It is not changed.

### 2. Implementation — `PlatformFailureNotifier`

Add `NotifyTaskCancelled` to `internal/domain/task/failure_notifier.go`. The method follows the same shape as `NotifyTaskFailure`:

1. Look up the originating message by `messageID`.
2. Find the platform sender.
3. Build content: `task_cancelled` prefix + optional `worker_line` (when `workerName` is non-empty). No error-reason suffix — cancellation is not an error.
4. Truncate to 500 runes, send via the platform adapter.

### 3. Dispatcher helper — `notifyCancel()`

Add to `internal/domain/task/dispatcher.go`, parallel to the existing `notifyFailure()`:

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

### 4. Cancellation paths

**Path 1** — early cancel after `resolveExecution` (`executeAsync`):

```go
if taskCtx.Err() != nil {
    d.manager.CancelExecution(context.Background(), exec.ID)
    d.notifyCancel(context.Background(), task.MessageID, task.WorkerID)
    return
}
```

**Path 2** — cancel during polling (`waitForResult`):

```go
case <-ctx.Done():
    d.manager.CancelExecution(context.Background(), executionID)
    d.notifyCancel(context.Background(), task.MessageID, workerName(exec.WorkerName, task.WorkerID))
    return
```

Both paths use `context.Background()` for the notification call (the task context is already cancelled).

### 5. i18n

**`internal/infra/i18n/messages.go`** — extend `FailureNotifierMessages`:

```go
type FailureNotifierMessages struct {
    TaskFailed    string `yaml:"task_failed"`
    TaskCancelled string `yaml:"task_cancelled"` // new
    ParseFailed   string `yaml:"parse_failed"`
    WorkerLine    string `yaml:"worker_line"`
    Failed        string `yaml:"failed"`
}
```

**`locales/en.yaml`:**
```yaml
failure_notifier:
  task_failed: "❌ Task execution failed"
  task_cancelled: "🚫 Task was cancelled"
```

**`locales/zh.yaml`:**
```yaml
failure_notifier:
  task_failed: "❌ 任务执行失败"
  task_cancelled: "🚫 任务已被取消"
```

Existing `worker_line`, `parse_failed`, and `failed` keys are unchanged and not used by the cancel notification.

### 6. Tests

**`internal/domain/task/dispatcher_test.go`:**

- Add `NotifyTaskCancelled` stub to `mockFailureNotifier` (required to satisfy the updated interface).
- New test `TestDispatch_CancelWhileExecuting`: dispatch a task, let it reach `waitForResult`, cancel it, assert `NotifyTaskCancelled` is called with the correct `messageID` and `workerName`.
- New test `TestDispatch_CancelBetweenResolveAndWait`: cancel the task context immediately after `resolveExecution` returns, assert `NotifyTaskCancelled` is called.

**`internal/domain/task/failure_notifier_test.go`:**

- New test `TestPlatformFailureNotifier_CancelSuccess`: verify the cancel notification message uses `task_cancelled` prefix and includes the `worker_line` when a worker name is provided.
- Reuse the existing `setupNotifier` helper.

**`internal/domain/bee/feeder_test.go`:** No changes — `bee.FailureNotifier` interface is unchanged.

## Affected Files

| File | Change |
|------|--------|
| `internal/domain/task/dispatcher.go` | Extend interface, add `notifyCancel()`, patch two cancellation paths |
| `internal/domain/task/failure_notifier.go` | Add `NotifyTaskCancelled` method |
| `internal/infra/i18n/messages.go` | Add `TaskCancelled` field to `FailureNotifierMessages` |
| `internal/infra/i18n/locales/en.yaml` | Add `task_cancelled` string |
| `internal/infra/i18n/locales/zh.yaml` | Add `task_cancelled` string |
| `internal/domain/task/dispatcher_test.go` | Update mock, add two cancel tests |
| `internal/domain/task/failure_notifier_test.go` | Add cancel notification test |
