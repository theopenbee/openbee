# Design: Remove `mark_task_failed` Tool

**Date:** 2026-03-16

## Summary

Remove the `mark_task_failed` MCP tool. The LLM (worker) only needs to call `mark_task_success` when execution completes normally. Task failure state is automatically set by the dispatcher when it detects that a worker process has exited abnormally (`ExecStatusFailed`).

## Motivation

Currently the LLM is expected to call `mark_task_failed` to signal failure. This is unreliable because a crashed or abnormally-exited worker cannot call any MCP tool. The failure path should be handled by the system, not the LLM.

## Design

### Task Failure Flow (After Change)

```
Worker runs normally  → LLM calls mark_task_success → task: completed
Worker exits abnormally (ExecStatusFailed) → dispatcher calls FailTask → task: failed (or pending for scheduled)
```

### Files Changed

#### 1. `internal/toolnames/toolnames.go`
Delete the `MarkTaskFailed` constant.

#### 2. `internal/mcp/tools.go`
- Remove `mark_task_failed` entry from the tools slice returned by `Tools()`
- Remove `toolMarkTaskFailed` function
- Remove the `case toolnames.MarkTaskFailed` branch from `callTool`

#### 3. `internal/task_dispatcher/dispatcher.go`
- Add `FailTask(ctx context.Context, taskID string) error` to the `TaskStore` interface
- In `waitForResult`, when `ExecStatusFailed` is detected, call `d.taskStore.FailTask(ctx, taskID)`

#### 4. `internal/store/task_store.go`
Implement `FailTask(ctx, taskID)` with logic matching the removed `toolMarkTaskFailed`:
- If the task is a scheduled task with a cron expression → call `CompleteScheduledTask` (reset to pending for next run)
- Otherwise → `UpdateStatus(ctx, taskID, TaskStatusFailed)`

#### 5. `internal/claudemd/claudemd.go`
Update `workerRules()`:
- Remove all references to `mark_task_failed`
- Simplify task completion instruction: LLM only calls `mark_task_success` at the end

#### 6. Tests
- `internal/mcp/tools_test.go`: remove `mark_task_failed` test cases
- `internal/claudemd/claudemd_test.go`: update assertions to not expect `mark_task_failed`

## Edge Cases

### Scheduled Tasks
When a scheduled task's worker exits abnormally, `FailTask` calls `CompleteScheduledTask` (same as success path). This resets the task to `pending` so it will run again on the next scheduled trigger — consistent with existing behavior and avoids permanently failing recurring jobs.

### taskID is empty
`waitForResult` already guards with `if taskID != ""` before calling store methods. No change needed.

## Out of Scope
- No changes to `mark_task_success` behavior
- No changes to task scheduling or dispatch logic
- No new retry or alerting mechanism for failed tasks
