# Remove `execution` Subcommand & Invert Task↔Execution Relationship

Date: 2026-05-28

## Background & Motivation

`openbee ctl` currently exposes an `execution` subcommand (`openbee ctl execution list`)
backed by the `list_executions` RPC tool. "Execution" is the runtime log record of a
task's run. Exposing it as a top-level, agent-facing command creates semantic ambiguity
for the LLM: it cannot reliably distinguish a *task* (the unit of work) from an
*execution* (a single run record of that work).

Decision: remove the standalone `execution` query surface, and instead surface execution
records nested under task queries, where they are unambiguously scoped to their parent task.

To make "executions belonging to a task" a first-class, queryable relationship, the
data model is inverted: executions point to their task, instead of a task pointing to a
single execution.

## Goals

1. Remove the `openbee ctl execution` CLI subcommand and its dedicated `list_executions`
   RPC tool (verified: the CLI is the only caller).
2. Invert the task↔execution relationship in the schema:
   - Add `task_id` to `bee_executions`.
   - Remove `execution_id` from `bee_tasks`.
3. `list_tasks` returns, for each task, the full history of its associated execution
   records (newest first).

## Non-Goals

- No change to the web frontend's session/execution HTTP APIs (`/sessions`, `/sessions/:id`,
  logs, stats). Those are independent of the `list_executions` RPC tool and remain.
- No change to bee-specific `list_bee_executions`.

## Confirmed Decisions

1. **`/status` & `/clear` Lark command output** (`command/task_format.go`): keep showing a
   short execution id per task, but derive it from the task's **latest / running**
   execution (one extra lookup), since the task no longer carries `execution_id`.
2. **`read:executions` scope**: deleted entirely. Embedded executions in `list_tasks` are
   covered by the existing `read:tasks` scope.
3. **HTTP `/tasks` `execution_id` field**: removed from the response (and from the web
   `Task` type). Verified the web UI does not read it.
4. **`list_tasks` executions**: full history per task (immediate/countdown usually 1,
   scheduled/cron may have many).

## Current State (as analyzed)

- `bee_executions` has no `task_id`. The only link is `bee_tasks.execution_id` →
  `bee_executions.id`, written by `TaskStore.SetExecution`, which **overwrites** on every
  dispatch. So scheduled/cron tasks lose all but their latest execution's back-link.
- `task.ExecutionID` is read in: `cancel_task` and `clear_session` (to stop the running
  process), `command/clear.go`, `command/task_format.go` (display), `api/task_handler.go`
  (HTTP response), and `waitForResult` via `GetTaskByExecutionID`.
- `ExecuteWorker` (the execution creator) is only called from the dispatcher (2 sites);
  `task_id` is available there.
- Direct `ALTER TABLE ... DROP COLUMN` is supported (see migration 31). Current max
  migration version is 44.

## Design

### Schema migrations

Migration 45 — `add_task_id_to_executions_and_backfill` (multi-statement):

```sql
ALTER TABLE bee_executions ADD COLUMN task_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_executions_task_id ON bee_executions(task_id);
UPDATE bee_executions
   SET task_id = (SELECT t.id FROM bee_tasks t WHERE t.execution_id = bee_executions.id)
 WHERE EXISTS (SELECT 1 FROM bee_tasks t WHERE t.execution_id = bee_executions.id);
```

The backfill preserves existing task↔execution links before the old column is dropped.

Migration 46 — `drop_execution_id_from_tasks`:

```sql
ALTER TABLE bee_tasks DROP COLUMN execution_id;
```

### Model changes

- `model.WorkerExecution`: add `TaskID string` (`json:"task_id,omitempty" db:"task_id"`).
- `model.Task`: remove `ExecutionID`.

### Store: `execution_store.go`

- `Create(workerID, taskID, triggerInput, sessionID, engine)` — add `taskID` param, write
  `task_id` in the INSERT.
- `execSelect` / `scanExecution`: include `task_id`.
- New `ListByTaskIDs(ctx, taskIDs []string) (map[string][]WorkerExecution, error)` —
  batch-load executions grouped by task, newest first; used by `list_tasks`.
- New `GetRunningByTaskID(ctx, taskID) (*WorkerExecution, error)` — used by the
  cancel/clear paths to find the live process to stop.

### Store: `task_store.go`

- Remove `execution_id` from all SELECT/INSERT column lists and row scans.
- Remove `SetExecution` and `GetTaskByExecutionID`.

### Dispatcher (`domain/task/dispatcher.go`)

- Thread `task.TaskID` into `ExecuteWorker` → `Create` so the execution row records its task.
- Replace `SetExecution(taskID, exec.ID, running)` with `UpdateStatus(taskID, running)`
  (the link now lives on the execution row).
- In `waitForResult`, replace `GetTaskByExecutionID(exec.ID)` with `exec.TaskID`.

### Scheduler (`domain/task/scheduler.go`)

- Replace `SetExecution(id, "", failed)` with `UpdateStatus(id, failed)`.

### Worker manager (`domain/worker/execution.go`)

- `ExecuteWorker(ctx, workerID, taskID, triggerInput, sessionID, resume)` — add `taskID`
  param, pass to `executionStore.Create`.

### Cancel / clear paths

Currently rely on `task.ExecutionID` to stop the live process. Change to look up the
running execution by task:

- `rpc/tools.go` `toolCancelTask`: `GetRunningByTaskID(taskID)` → stop + finalize.
- `rpc/tools.go` `toolClearSession`: for each task to stop, `GetRunningByTaskID`.
- `domain/command/clear.go`: same pattern.

### `list_tasks` response

Each task is returned with an attached `executions` array (newest first). Shape:

```json
{
  "id": "...", "worker_id": "...", "instruction": "...", "type": "...",
  "status": "...", "executions": [ { "id": "...", "status": "...", ... } ]
}
```

`toolListTasks` will batch-fetch via `ListByTaskIDs` and attach.

### Removals

- Delete `cmd/openbee/ctl_execution.go`.
- Delete `utils.ListExecutions` constant.
- Delete the `ListExecutions` dispatch case and `toolListExecutions` handler in `rpc/tools.go`.
- Delete `ScopeReadExecutions` and its tool mapping in `auth/scopes.go`.
- Remove `ExecutionID` from `api/task_handler.go` `taskResponse` and from web `Task` type
  (`web/src/lib/types.ts`).

### Command display (`command/task_format.go`)

- `shortExecID(t.ExecutionID)` → derive from the task's latest/running execution id. The
  formatter's caller will supply the resolved id (via `GetRunningByTaskID` or the latest
  from the task's executions), so the formatting layer stays free of store access.

## Testing

- Store tests: `Create` writes `task_id`; `ListByTaskIDs` groups & orders correctly;
  `GetRunningByTaskID` returns the live execution; remove/replace `SetExecution` and
  `GetTaskByExecutionID` tests.
- Migration test: backfill copies `execution_id` → `task_id`; `execution_id` column gone
  from `bee_tasks` afterward.
- Dispatcher/scheduler tests: status updates still happen; execution rows carry `task_id`.
- RPC tests: `list_tasks` returns embedded executions; cancel/clear still stop the live
  process via the new lookup; `list_executions` no longer dispatchable.
- Existing tests referencing `ExecutionID` / `SetExecution` / `GetTaskByExecutionID`
  updated or removed.

## CHANGELOG

Add an English entry noting removal of the `execution` subcommand and that task queries
now return associated execution records.
