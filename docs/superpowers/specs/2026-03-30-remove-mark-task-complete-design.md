# Remove `mark_task_complete` Tool

**Date:** 2026-03-30

## Summary

Remove the `mark_task_complete` MCP tool and transfer task completion responsibility from the worker model to the task dispatcher. Task terminal state is determined entirely by the Claude Code process exit code: exit 0 → completed, non-0 → failed.

## Motivation

- The model is required to call `mark_task_complete` as its last action every run, creating cognitive overhead and extensive system prompt reminders to prevent forgetting.
- The exit code already carries the success/failure signal reliably — the model calling a tool to repeat this information is redundant.
- Removing the tool simplifies the worker system prompt and reduces the model's tool surface.

## Design

### Task completion ownership

Shifts from **model** to **dispatcher**. The dispatcher already owns failure handling (`FailTask` on non-0 exit). After this change it symmetrically owns both terminal states.

### `waitForResult` change

```
ExecStatusCompleted → taskStore.CompleteTask(ctx, taskID)   // new
ExecStatusFailed    → taskStore.FailTask(ctx, taskID)       // unchanged
```

### Scheduled task handling

`CompleteTask` in the store layer encapsulates the existing branching logic:
- If task is `TaskTypeScheduled` with a `CronExpr` → call `CompleteScheduledTask` (resets to pending for next run)
- Otherwise → call `UpdateStatus(completed)`

This logic was previously in `toolMarkTaskComplete`; it moves to the store where it belongs.

## Components Changed

| Component | Change |
|---|---|
| `task_dispatcher/dispatcher.go` — `TaskStore` interface | Add `CompleteTask(ctx, taskID string) error` |
| `task_dispatcher/dispatcher.go` — `waitForResult` | Call `taskStore.CompleteTask` on `ExecStatusCompleted` |
| `store` (concrete `TaskStore`) | Implement `CompleteTask` with scheduled/regular branching |
| `internal/mcp/tools.go` | Remove tool definition, handler, allowlist entry, dispatch case |
| `internal/toolnames/toolnames.go` | Remove `MarkTaskComplete` constant |
| `internal/claudemd/worker.go` | Remove all `mark_task_complete` references from system prompt |
| `internal/mcp/tools_test.go` | Remove `mark_task_complete` tests |
| `internal/claudemd/claudemd_test.go` | Remove `mark_task_complete` assertions |

## What Does Not Change

- `send_message` tool remains — the model still uses it to communicate results to users.
- Failure path in dispatcher is unchanged.
- Session context persistence on success is unchanged.
- Scheduled task reset semantics are preserved, just moved to the store layer.
