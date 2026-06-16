# Worker WorkDir Update — Design

Date: 2026-06-15
Status: Approved (brainstorming)

## Background

A worker has a `WorkDir` field — the filesystem path where the AI engine
runs (`engine.Prepare` / `engine.Run`). Today the path is set at create time
(`openbee ctl worker create --work-dir <path>`, default
`workerBaseDir/<id>`) and cannot be changed afterwards. Users need to relocate
a worker's working directory after creation — for example to move it onto a
different disk, into a project repo, or to consolidate paths.

## Goal

Allow modifying a worker's `WorkDir` after creation, from both the CLI
(`openbee ctl worker update`) and the web UI (worker detail page).

## Non-Goals

- Do NOT migrate / move / copy any files between old and new directories.
- Do NOT pre-validate the new path (existence, writability, absolute-vs-relative).
- Do NOT pre-create or pre-`Prepare` the new path at update time.
- Do NOT reject updates while an execution is running on the worker.
- Do NOT add audit logging beyond what `workerStore.Update` already records.

## Behavior

- `worker update --work-dir <new>` updates only the database record. Old
  directory contents are left untouched.
- Validation is minimal: the new path, after trim, must be non-empty. Any
  syntactically legal non-empty path is accepted.
- The new directory is created lazily on the next execution: `execute.go`
  calls `os.MkdirAll(worker.WorkDir, 0755)` before `engine.Prepare`.
- If an execution is in flight when `WorkDir` is updated, it continues to use
  whatever path it had captured. The user is responsible for understanding
  this — we do not block the update.

## Changes

### Domain — `internal/domain/worker/worker.go`

1. `UpdateWorkerParams`: add `WorkDir *string` (`nil` = no change).
2. `HasChanges()`: include `WorkDir != nil`.
3. `Validate()`: if `WorkDir != nil`, trim and reject empty string with
   `ErrValidation` ("work_dir cannot be empty").
4. `ApplyTo(w)`: write the trimmed value to `w.WorkDir`.

### Domain — `internal/domain/worker/execution.go`

5. Before line 51's `engine.Prepare(worker.WorkDir, …)`, add:
   `os.MkdirAll(worker.WorkDir, 0755)`. Propagate the error if it fails
   (mark execution failed, worker errored — same pattern as `create runtime`
   failures).

### CLI — `cmd/openbee/internal/cli/ctlcmd/worker.go`

6. `newWorkerUpdateCommand`: register `--work-dir <path>` flag and wire it
   through `setIfFlagChanged(c, a, "work-dir", "work_dir", workDir)`.

### HTTP API

7. Update-worker handler / utils call: ensure `work_dir` is accepted on the
   request payload and forwarded to `UpdateWorkerParams.WorkDir`. (Confirm
   at implementation time whether the existing allowlist already passes
   unknown fields through; if not, add `work_dir`.)

### Web — `web/src/pages/worker-detail.tsx`

8. Convert the current read-only `WorkDir` display into an inline editor,
   mirroring the `isEditingConstraints` pattern already on the page:
   - Display mode: path + copy button + pencil/edit button.
   - Edit mode: text input + save / cancel buttons.
   - Save calls `useUpdateWorker` with `{ work_dir: <new> }`.

### Web — types & i18n

9. `web/src/lib/types.ts`, `web/src/lib/api.ts`: ensure
   `UpdateWorkerRequest` includes optional `work_dir: string`.
10. `web/src/locales/{zh,en}.json`: add labels for "Edit work dir",
    "Save", "Cancel", "Work directory updated" (reuse existing keys where
    possible).

### Skill docs — `internal/infra/skillinstall/skills/openbee-bee/`

11. `references/cli-reference.md` line 12 — add `[--work-dir <directory>]`
    to the `openbee ctl worker update` signature.
12. Same file — add a short example in the worker section:
    ```bash
    # Update a worker's working directory
    openbee ctl worker update <id> --work-dir /path/to/new/dir
    ```
13. Scan `SKILL.md` and `references/entity-relationships.md` for any place
    that enumerates worker mutability; if such a list exists, note that
    `WorkDir` is now mutable via `update`.

## Data Flow

CLI:
```
openbee ctl worker update <id> --work-dir /new
  → Runner → utils.UpdateWorker (HTTP)
  → handler → Manager.UpdateWorker(id, {WorkDir: &"/new"})
  → Validate → ApplyTo → workerStore.Update
```

Web:
```
worker detail page → save edit
  → useUpdateWorker mutation → API
  → same Manager.UpdateWorker path
  → React Query invalidates and refetches
```

Execution after update:
```
execute → os.MkdirAll(WorkDir) → engine.Prepare(WorkDir) → engine.Run(WorkDir, …)
```

## Error Handling

| Case | Behavior |
| --- | --- |
| `work_dir` trim → empty | `ErrValidation` ("work_dir cannot be empty") |
| `MkdirAll` fails at execute time | Existing failure path: execution → failed, worker → error |
| `engine.Prepare` fails | Existing behavior preserved: `log.Error`, continue to `Run` |
| Update during a running execution | Allowed; in-flight execution keeps the path it captured |

## Test Plan

Unit (`internal/domain/worker/`):
- `TestUpdateWorker_WorkDir_Success` — happy path persists the new value.
- `TestUpdateWorker_WorkDir_TrimWhitespace` — leading/trailing space stripped.
- `TestUpdateWorker_WorkDir_EmptyAfterTrim` — returns `ErrValidation`.
- `TestUpdateWorker_WorkDir_Nil_NoChange` — field untouched when `nil`.
- `TestUpdateWorker_WorkDir_OldDirUntouched` — files placed in the old
  directory before the update still exist after the update.
- `TestExecute_CreatesNewWorkDirIfMissing` — execute auto-creates the new
  directory when it does not exist yet.

CLI smoke:
- `worker create --work-dir /tmp/a` → `worker update <id> --work-dir /tmp/b`
  → `worker get <id>` reports `/tmp/b`.

Web manual:
- Detail page: edit → save → refresh → value persists → trigger an execution
  → confirm engine artifacts (e.g. `CLAUDE.md`) appear under the new path.

## YAGNI (Explicitly Excluded)

- File migration between old and new paths.
- Path existence / writability / format validation.
- Reject-update-when-running guard.
- Extra audit log entries.
