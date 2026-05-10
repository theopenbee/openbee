# Spec: Show task & worker details in `/clear` confirmation prompt

Date: 2026-05-10
Scope: `/clear` (global form only) confirmation message

## Background

When a user runs `/clear` and there are running tasks, the handler asks for confirmation with this template (`internal/infra/i18n/locales/en.yaml:256`, mirrored in `zh.yaml:256`):

```
⚠️ Will clear session contexts: 关羽 (claude), 马超 (claude), 张辽 (claude), bee (claude)
This will also stop 2 running task(s).
Send /clear again within 30s to confirm.
```

The line `This will also stop 2 running task(s).` only reports a count. The user cannot see which tasks will be killed or which worker each task belongs to, so they cannot make an informed decision before re-sending `/clear` to confirm.

## Goal

Make the confirmation prompt list each running task with its owning worker, instruction excerpt, runtime, and execution ID — same format already used by `/status`.

Out of scope (per user direction):
- `worker_cleared`, `cleared`, `cleared_with_tasks` templates remain unchanged.
- `/clear {workerName}` (single-worker form) is unchanged.

## Proposed output

```
⚠️ Will clear session contexts:
  - 关羽 (claude)
  - 马超 (claude)
  - 张辽 (claude)
  - bee (claude)

This will also stop 2 running task(s):
  - [关羽] 帮我写一个排序算法   running for 3m   exec: a1b2c3d4
  - [bee] 总结今天的会议纪要   running for 12s   exec: e5f6g7h8

Send /clear again within 30s to confirm.
```

Both the agents block and the tasks block are multi-line. Each task line uses the same shape as `status_command.task_line`:

```
  - [{worker}] {instruction}   running for {duration}   exec: {execID}
```

`{instruction}` is truncated to `maxInstructionRunes` (40 runes, defined in `status.go`). `{execID}` is truncated to 8 chars by `shortExecID`. Runtime is rendered by `formatRelative`.

## Design

### i18n (`internal/infra/i18n/messages.go` + locale files)

Replace the single `confirm_all_with_tasks` template with a structured set so each block can be assembled programmatically. New fields under `clear_command`:

- `confirm_header`: `"⚠️ Will clear session contexts:"` / `"⚠️ 将清除以下会话上下文："`
- `confirm_agent_line`: `"  - %s (%s)"` — `(name, engine)`
- `confirm_tasks_header`: `"This will also stop %d running task(s):"` / `"同时将终止 %d 个运行中任务："`
- `confirm_footer`: `"Send /clear again within 30s to confirm."` / `"30s 内再发一次 /clear 确认。"`

Reuse `status_command.task_line` for the per-task line — same wording in both locales, no duplication.

Delete the now-unused `confirm_all_with_tasks` field. (Single-edit cleanup; no callers will remain.)

### `clear.go` changes

1. Replace the existing `WorkerNameLookup` interface (only has `ListByName`) with a wider interface that also includes `GetByIDs([]string) ([]model.Worker, error)` — needed to resolve `task.WorkerID` to a display name. Real implementation (`store.WorkerStore`) already satisfies both methods.

2. Add a `now func() time.Time` field to `ClearCommandHandler` (default `time.Now`) so task runtime is testable.

3. In `handleClearAll`, when `len(runningTasks) > 0 && !confirmed`, build the prompt by joining:
   - `m.ConfirmHeader`
   - one `m.ConfirmAgentLine` per agent
   - blank line
   - `fmt.Sprintf(m.ConfirmTasksHeader, len(runningTasks))`
   - one `status_command.task_line` per task (reusing `resolveWorkerNames`, `workerNameOrFallback`, `formatRelative`, `shortExecID`, `maxInstructionRunes`, `utils.TruncateRunes` already in the same package)
   - blank line
   - `m.ConfirmFooter`

4. Move `resolveWorkerNames` from `*StatusCommandHandler` to a free function `resolveWorkerNames(workers WorkerByIDsLookup, tasks []model.Task) map[string]string` in a new shared file `internal/domain/command/task_format.go`, plus a small `WorkerByIDsLookup` interface (just `GetByIDs`). `StatusCommandHandler.resolveWorkerNames` becomes a thin wrapper or is replaced by direct calls.

   This avoids cross-handler coupling and keeps the formatter logic in one place.

5. `formatAgentList` (current single-line `"%s (%s), %s (%s)…"` joiner) is removed since it has only one caller and the new multi-line layout doesn't need it. The success-path templates (`cleared`, `cleared_with_tasks`) still receive the old comma-joined string — keep a small inline join there, OR also switch their `%s` to a multi-line list. **Per scope, those templates stay unchanged**, so we keep the comma-joined format for them only (inline `strings.Join`, no helper).

### Tests (new file `internal/domain/command/clear_test.go`)

Cover:
1. `/clear` with running tasks → confirmation prompt contains one line per agent, one line per task in the documented format, ends with the 30s footer.
2. `/clear` confirmed within 30s with running tasks → tasks are stopped and cleared message returned (regression — current behaviour).
3. Worker name resolves to a fallback (raw ID) when `GetByIDs` returns an empty map.

Use the same fake-store pattern as `status_test.go` (`fakeStatusTaskLister`, `fakeWorkerLookup`, etc.) to keep style consistent.

### Migration / compat

No persisted data changes. The only externally-visible change is the prompt text. No flag, no rollout — just ship.

## Risks & non-risks

- **Risk**: long instruction strings could push the prompt past platform message limits. Mitigation: existing `TruncateRunes(40)` already in use by `/status`; same applies here.
- **Non-risk**: removing `confirm_all_with_tasks` is safe; the only producer is `clear.go` and the only consumer is the same handler — caught by the i18n field rename + compile.

## Files touched

- `internal/infra/i18n/messages.go`
- `internal/infra/i18n/locales/en.yaml`
- `internal/infra/i18n/locales/zh.yaml`
- `internal/domain/command/clear.go`
- `internal/domain/command/status.go` (refactor `resolveWorkerNames` out)
- `internal/domain/command/task_format.go` (new, shared formatter helpers)
- `internal/domain/command/clear_test.go` (new)
