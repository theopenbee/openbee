# Session Clear — Running Task Detection Design

**Date:** 2026-04-13  
**Branch:** feature/worker-permission-scopes  
**Status:** Approved

## Background

When a user requests a session clear, the current flow is:

1. Bee agent runs `openbee ctl task list --session-key <key> --status pending,running` to check for active tasks
2. If tasks exist, Bee notifies the user and waits for confirmation
3. Bee calls `clear_session` (which has its own confirmation step for multiple linked workers)

This two-step pre-query adds an unnecessary round trip in the common case where no tasks are running.

## Goal

Move the running-task detection into the `clear_session` backend operation, eliminating the pre-query step from the Bee skill. The backend returns a `requires_confirmation` response when tasks are detected, and the Bee handles it the same way it already handles the multi-worker confirmation.

**Benefit:** Reduces one API call in the common case (no running tasks), makes the backend more self-contained, and simplifies the Bee skill logic.

## Scope

| File | Change |
|------|--------|
| `internal/mcp/tools.go` | Add running-task detection inside `toolClearSession()` |
| `internal/mcp/tools_test.go` | Add test cases for the new detection logic |
| `internal/infra/skillinstall/skills/openbee-bee/references/session-management.md` | Remove manual `task list` pre-query step; update response handling |
| i18n message files | Add `ClearSessionTasksConfirm` message key (parallel to existing `ClearSessionConfirm`) |

**Not changed:** `taskStore`, `sessionStore`, `dispatcher`, `clear_worker_session` tool, or the existing multi-worker confirmation logic (beyond adding a `reason` field).

## Architecture

### New Data Flow

```
Bee calls clear_session(force=false)
    │
    ▼
[NEW] Detect pending/running tasks
    │ Tasks found?
    ├─ Yes → return requires_confirmation=true, reason="running_tasks", running_tasks=[...]
    │         Bee shows task list to user → user confirms → Bee retries with force=true
    │
    ▼ (no tasks, or force=true)
[EXISTING] Detect multiple linked workers
    │ Multiple workers found (and force=false)?
    ├─ Yes → return requires_confirmation=true, reason="multiple_workers", linked_workers=[...]
    │
    ▼ (single worker, or force=true)
Execute clear (existing logic unchanged)
```

### Execution Order (force=false)

1. Task detection — higher priority; intercepts before worker check
2. Worker detection — existing logic (unchanged)
3. Clear execution — existing logic (unchanged)

`force=true` skips both checks entirely.

## Backend Implementation

### Location

`internal/mcp/tools.go` — `toolClearSession()` function (~line 534)

### Change

Insert the following block **before** the existing worker detection, inside the `if !params.Force` block:

```go
// Detect pending/running tasks; require confirmation before cancelling them.
activeTasks, err := s.taskStore.ListBySessionKey(ctx, params.SessionKey, "pending,running", "")
if err != nil {
    return nil, fmt.Errorf("list active tasks: %w", err)
}
if len(activeTasks) > 0 {
    taskSummaries := make([]map[string]string, 0, len(activeTasks))
    for _, t := range activeTasks {
        taskSummaries = append(taskSummaries, map[string]string{
            "task_id":     t.ID,
            "instruction": t.Instruction,
            "status":      t.Status,
        })
    }
    return map[string]any{
        "requires_confirmation": true,
        "reason":               "running_tasks",
        "running_tasks":        taskSummaries,
        "message":              fmt.Sprintf(i18n.M.Runtime.MCP.ClearSessionTasksConfirm, len(activeTasks)),
    }, nil
}
```

Also add `reason: "multiple_workers"` to the existing worker confirmation response so Bee can distinguish between the two cases.

### Response Format (running tasks detected)

```json
{
  "requires_confirmation": true,
  "reason": "running_tasks",
  "running_tasks": [
    {"task_id": "abc123", "instruction": "Analyze this report", "status": "running"},
    {"task_id": "def456", "instruction": "Update database config", "status": "pending"}
  ],
  "message": "2 tasks are currently running. Clearing the session will terminate them. Continue?"
}
```

### New Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `reason` | string | `"running_tasks"` or `"multiple_workers"` — lets Bee distinguish confirmation type |
| `running_tasks` | array | Task summary list (present when `reason == "running_tasks"`) |
| `running_tasks[].task_id` | string | Task ID |
| `running_tasks[].instruction` | string | Task instruction text |
| `running_tasks[].status` | string | `"pending"` or `"running"` |

## Bee Skill Changes

### File

`internal/infra/skillinstall/skills/openbee-bee/references/session-management.md`

### Before (step 1)

```
1. Run `openbee ctl task list --session-key <key> --status pending,running` to check
   for active tasks. If any exist, notify the user and wait for confirmation before proceeding.

2. Run `openbee ctl session clear --session-key <key>` ...
```

### After (step 1 removed, step 2 becomes step 1)

```
1. Run `openbee ctl session clear --session-key <key>` (without --force by default):

   - If it returns `requires_confirmation=true`:
     - If `reason == "running_tasks"`: show the user the running task list and ask:
       "N tasks are currently running (Tasks: ...). Clearing the session will terminate
       them. Continue?" After user confirms, re-run with --force.
     - If `reason == "multiple_workers"` (or no reason field): show the user the list
       of affected workers and ask for confirmation. After user confirms, re-run with --force.
   - If it returns `cleared=true`: inform the user "Session cleared. All worker contexts
     have been reset; you can start a new conversation."
```

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| `force=false`, tasks running | Return `requires_confirmation`, do not clear |
| `force=true`, tasks running | Skip detection, execute clear immediately |
| `force=false`, no tasks, multiple workers | Existing worker confirmation (unchanged) |
| `force=false`, no tasks, single worker | Clear immediately (unchanged) |
| `force=true`, any state | Skip all checks, clear immediately (unchanged) |
| Task query error | Return error, do not proceed with clear |

## Test Coverage

New test cases to add in `internal/mcp/tools_test.go`:

1. `TestCallTool_ClearSession_RunningTasksRequiresConfirmation` — returns confirmation when running tasks exist
2. `TestCallTool_ClearSession_PendingTasksRequiresConfirmation` — returns confirmation when pending tasks exist
3. `TestCallTool_ClearSession_ForceSkipsTaskCheck` — `force=true` bypasses task detection and clears
4. `TestCallTool_ClearSession_NoTasksProceeds` — no tasks present, falls through to existing logic (regression)
