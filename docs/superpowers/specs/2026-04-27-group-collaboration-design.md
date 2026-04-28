# Group Collaboration Design

- **Date**: 2026-04-27
- **Status**: Draft, awaiting user review
- **Author**: brainstormed via 貂蝉 (worker agent)

## 1. Goal

Introduce a `Group` (小组) concept so that multiple Workers can collaborate on a single user request under a coordinator (the Group itself). When a task is dispatched to a Group, the Group analyzes the request, breaks it into sub-tasks, dispatches them to its member Workers, monitors execution, reports progress to the user, and finally summarizes the result.

This document captures the architectural design only. A separate implementation plan will be produced after user approval.

## 2. Key Decisions

The decisions below were validated interactively before this document was written. Each is load-bearing for the rest of the design.

| # | Topic | Decision |
|---|---|---|
| 1 | Group vs Department | New independent `Group` entity. Department remains an org-chart-only label and is untouched. |
| 2 | Task entry into a Group | Both supported: `@groupName` direct dispatch (fast path via Feeder) and Bee intent routing (LLM picks the Group). |
| 3 | Group coordinator runtime form | Group is itself an Agent (peer to Worker, sharing Worker infrastructure). |
| 4 | Sub-task data model | Add `parent_task_id` and `root_task_id` self-referencing columns to `bee_tasks`. |
| 5 | Progress reporting | Group is the **only** voice to the user. Worker `message send` issued from a sub-task context is rerouted to the Group, not the IM platform. |
| 6 | Coordinator runtime model | Event-driven resume. Group agent suspends after each turn; dispatcher resumes it on sub-task state changes. |
| 7 | Shared workspace | None for MVP. Members keep their own `work_dir`; Group has its own `work_dir` only for the agent process. |

Defaults that were silently approved:
- Group reuses Worker model fields: `engine`, `engine_args`, `permission_scopes`, `status`.
- Management surface: `openbee ctl group ...` CLI tree (mirrors `worker`).
- Member roster is injected dynamically into the Group persona at each agent launch.
- Group name shares the global namespace with Worker and Bot names (avoids `@xxx` routing ambiguity).
- Default sub-task failure handling: dispatcher marks failed and resumes the Group; the Group decides retry / reassign / escalate.
- A single Group runs one root task per `sessionKey` at a time (existing dispatcher per-`workerID` queue applies).
- Web Console support is out of scope for the MVP; backend + CLI only.

## 3. Architecture Overview

```
┌──────────────────────────────────────────────────────────┐
│                       IM Layer                           │
│        (Lark / DingTalk / WeCom / WeChat / Telegram)     │
└──────────────────────────────────────────────────────────┘
                          ↕ message
┌──────────────────────────────────────────────────────────┐
│                 Bee  (singleton router)                  │
│  - LLM intent routing → Worker / Group                   │
│  - Direct dispatch shortcut: @workerName / @groupName    │
└──────────────────────────────────────────────────────────┘
              ↓ task                  ↓ task
   ┌───────────────────┐    ┌───────────────────────────┐
   │ Worker (Agent)    │    │ Group (Agent)             │
   │ - executes task   │←───│ - receives root task      │
   │ - exits when done │    │ - splits into sub-tasks   │
   │ - reports via ctl │    │ - event-driven resume     │
   └───────────────────┘    │ - aggregates and replies  │
                            └───────────────────────────┘
                                       ↕ ctl commands
                            ┌───────────────────────────┐
                            │ TaskDispatcher (existing) │
                            │ - per-agent serial queue  │
                            │ - new: sub-task event →   │
                            │   parent session resume   │
                            └───────────────────────────┘
```

### 3.1 Concept boundaries

| Entity | What it is | What it is **not** |
|---|---|---|
| Group | An Agent that owns constraints + member roster, and the right to dispatch sub-tasks. | Not a Department; not a passive worker tag. |
| Root task | The top-level task assigned to a Group; `parent_task_id = ''`. | Not executed by a Worker directly. |
| Sub-task | A task created by the Group agent for a member Worker; `parent_task_id != ''`. | Never directly notifies the user. |
| `subtask_event` | XML block injected into the prompt when the Group session is resumed. | Not a stored entity; assembled on the fly from the current `tasks` snapshot. |

### 3.2 Module change list

| Module | Change | Notes |
|---|---|---|
| `internal/domain/group/` | New package | `Group` entity, Manager (CRUD), member operations |
| `internal/infra/model/group.go` | New | `Group`, `WorkerGroup` |
| `internal/infra/store/group_store.go` | New | DDL + CRUD |
| `internal/infra/store/task_store.go` | Modified | New columns; new query methods (`ListByRoot`, `ListChildrenStatus`) |
| `internal/infra/store/db.go` (migrations) | Modified | New migrations for groups + worker_groups + task columns |
| `internal/domain/task/dispatcher.go` | Modified | Branch on `agent_kind`; new sub-task event → parent resume path; new recovery on startup |
| `internal/domain/bee/feeder.go` | Modified | `tryDirectDispatch` looks up by group name as well |
| `internal/ai/persona.go` | Modified | Add `GroupPersona` (constraints + member roster) |
| `cmd/openbee/ctl_group.go` | New | `openbee ctl group ...` command tree |
| `cmd/openbee/ctl_task.go` | Modified | Add 5 sub-commands: `dispatch-subtask`, `subtasks`, `suspend`, `mark-success`, `mark-failed` |
| `cmd/openbee/ctl_message.go` (server side) | Modified | `message send` reroutes when current task has `parent_task_id` |
| `internal/api/...` | Modified | REST resources for Group (CRUD + member ops); minimal MVP surface |

### 3.3 Invariants

- Worker behaviour is fully backward compatible. A Worker with no Group association is unchanged.
- Bee remains a singleton.
- Per-`sessionKey` serialization continues to apply independently in the Bee, Worker, and Group dimensions.
- Sub-tasks **never** reach the user channel directly. This is a strong guarantee.

## 4. Data Model

DDL follows existing `bee_` prefix and column conventions.

### 4.1 New table: `bee_groups`

```sql
CREATE TABLE IF NOT EXISTS bee_groups (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,            -- shares namespace with worker / bot
    description       TEXT NOT NULL DEFAULT '', -- injected into persona
    constraints       TEXT NOT NULL DEFAULT '', -- injected into persona
    work_dir          TEXT NOT NULL,            -- group agent's own work_dir
    engine            TEXT NOT NULL DEFAULT '',
    engine_args       TEXT NOT NULL DEFAULT '{}',
    status            TEXT NOT NULL DEFAULT 'idle'
                          CHECK(status IN ('idle','working','error')),
    permission_scopes TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_groups_name_lower ON bee_groups (LOWER(name));
```

### 4.2 New table: `bee_worker_groups`

```sql
CREATE TABLE IF NOT EXISTS bee_worker_groups (
    worker_id  TEXT NOT NULL REFERENCES bee_workers(id),
    group_id   TEXT NOT NULL REFERENCES bee_groups(id),
    role       TEXT NOT NULL DEFAULT 'member',  -- reserved for future leader/member etc.
    created_at INTEGER NOT NULL,
    PRIMARY KEY (worker_id, group_id)
);
CREATE INDEX IF NOT EXISTS idx_worker_groups_worker ON bee_worker_groups(worker_id);
CREATE INDEX IF NOT EXISTS idx_worker_groups_group  ON bee_worker_groups(group_id);
```

### 4.3 `bee_tasks` evolution

```sql
ALTER TABLE bee_tasks ADD COLUMN parent_task_id TEXT NOT NULL DEFAULT '';
ALTER TABLE bee_tasks ADD COLUMN root_task_id   TEXT NOT NULL DEFAULT '';
ALTER TABLE bee_tasks ADD COLUMN agent_kind     TEXT NOT NULL DEFAULT 'worker'
                       CHECK(agent_kind IN ('worker','group'));

UPDATE bee_tasks SET root_task_id = id WHERE root_task_id = '';

CREATE INDEX IF NOT EXISTS idx_tasks_parent ON bee_tasks(parent_task_id) WHERE parent_task_id != '';
CREATE INDEX IF NOT EXISTS idx_tasks_root   ON bee_tasks(root_task_id);
```

Field semantics:
- `parent_task_id = ''` → root task (the entry from the user).
- `parent_task_id != ''` → sub-task created by a Group agent.
- `root_task_id` always points to the root of the task tree; root tasks self-reference. Enables `WHERE root_task_id = ?` for cheap tree summaries.
- `agent_kind` tells the dispatcher which execution path to take.
- Existing `worker_id` column carries the Group ID when `agent_kind = 'group'`. This is a deliberate MVP shortcut to avoid a second schema change; if Web Console later needs typed listing, a generic `agent_id` column may be added in a follow-up.

### 4.4 New task status

```
existing:  pending → running → {completed | failed | cancelled}
new:                 ↓
              waiting_subtasks   ← Group agent exited but root task not finished
                    ↓ (sub-task event triggers resume)
                 running ← resumed
                    ↓
              {completed | failed | cancelled}
```

`waiting_subtasks` is the only new state introduced.

### 4.5 Tables that are **not** changed

- `bee_session_contexts`: already keyed by `(session_key, agent_id, engine)`. Group ID slots into `agent_id` directly.
- `bee_executions`: `worker_id` column stores the Group ID for Group executions (mirrors the `bee_tasks.worker_id` shortcut).
- `bee_platform_messages`: a root task corresponds to a single inbound message; sub-tasks have no inbound message of their own.

### 4.6 Migration ordering

Append after the current maximum migration version in `db.go`:

| version | name | purpose |
|---|---|---|
| N+1 | `create_table_bee_groups` | groups table |
| N+2 | `create_index_groups_name_lower` | name index |
| N+3 | `create_table_bee_worker_groups` | membership table + indexes |
| N+4 | `add_parent_root_to_tasks` | tasks columns |
| N+5 | `backfill_root_task_id_self_reference` | populate root for existing rows |
| N+6 | `add_agent_kind_to_tasks` | `agent_kind` column |
| N+7 | `create_index_tasks_parent_root` | tasks indexes |

## 5. Runtime Sequence

### 5.1 CLI surface

#### Management commands (operator / external use)

```
openbee ctl group create  --name <n> --description <d> [--constraints <c>] [--engine <e>] [--engine-args ...]
openbee ctl group list    [--name ...]
openbee ctl group get     <id>
openbee ctl group update  <id> [--name|--description|--constraints|--engine|--engine-args ...]
openbee ctl group delete  <id> [--delete-work-dir]

openbee ctl group member add    --group <id> --worker <id>
openbee ctl group member remove --group <id> --worker <id>
openbee ctl group member list   --group <id>
```

#### Runtime commands (used by the Group agent during execution)

```
# Coordinator splits and dispatches a sub-task
openbee ctl task dispatch-subtask --parent-task-id <root> --worker-id <w> --stdin
# stdin = the sub-task instruction
# Server creates: tasks(parent_task_id=root, root_task_id=root,
#                       worker_id=w, agent_kind='worker', status='pending')
# Output: subtask_id

# Snapshot of all sub-tasks under a root
openbee ctl task subtasks --task-id <root>
# Output: JSON list [{id, worker_id, status, result, started_at, completed_at}, ...]

# Coordinator politely yields the turn, awaiting events
openbee ctl task suspend --task-id <root>
# Server: tasks(root).status = 'waiting_subtasks'; CLI returns success;
# the agent process exits naturally afterwards.

# Coordinator declares the root task finished
openbee ctl task mark-success --task-id <root> [--stdin]
# Server: tasks(root).status = 'completed'; optional stdin → result; session released.

# Coordinator declares the root task failed
openbee ctl task mark-failed --task-id <root> [--stdin]
```

#### Worker side: `message send` is **not** changed at the CLI level

```
openbee ctl message send --message-id <id> --stdin
```

Server-side increment: when handling `message send`, look up the task that owns this message:
- If `task.parent_task_id == ''` (root or non-grouped task) → existing behaviour, send to IM platform.
- If `task.parent_task_id != ''` (sub-task) → do **not** send to IM. Instead, trigger a resume of the parent (Group) session so the next turn's prompt includes the latest snapshot.

Worker code is therefore unchanged; sub-tasks are transparent to Workers — only the audience changes.

### 5.2 End-to-end sequence (happy path)

```
[user]                           [Bee/Dispatcher]                     [Group agent]            [Worker A]    [Worker B]
  │                                       │                                  │                       │             │
  │── @data-team please do X ─────────→  │                                  │                       │             │
  │                          (1) Direct dispatch → create root task          │                       │             │
  │                                tasks(R, agent_kind='group',               │                       │             │
  │                                       worker_id=group.data-team,          │                       │             │
  │                                       parent='', root=R, status=pending)  │                       │             │
  │                                       │                                  │                       │             │
  │                          (2) Dispatcher takes the group path,            │                       │             │
  │                                fresh session, Resume=false                │                       │             │
  │                                       │ ── persona + roster + root_task ─→│                       │             │
  │                                       │                                  │                       │             │
  │                                       │                          (3) ctl message send           │             │
  │  ←─── "Starting: 3 steps planned" ───────────────────────────────────────│                       │             │
  │                                       │                                  │                       │             │
  │                                       │                          (4) ctl task dispatch-subtask × 2             │
  │                                       │  ←──────────────────────────────│                       │             │
  │                          tasks(S1, parent=R, root=R, worker=A, kind=worker)                     │             │
  │                          tasks(S2, parent=R, root=R, worker=B, kind=worker)                     │             │
  │                                       │                                  │                       │             │
  │                                       │                          (5) ctl task suspend            │             │
  │                                       │  ←──────────────────────────────│                       │             │
  │                          tasks(R).status = 'waiting_subtasks'             │                       │             │
  │                                       │                          agent process exits; session kept            │
  │                                       │                                                          │             │
  │                          (6) Dispatcher enqueues S1 and S2; per-worker queues run them          │
  │                                       │ ─────── start S1 ──────────────────────────────────────→│             │
  │                                       │ ─────── start S2 ────────────────────────────────────────────────→    │
  │                                       │                                                  (S1 done)             │
  │                                       │ ←── S1 execution=completed ────────────────────────────│             │
  │                          (7) Dispatcher: S1.parent_task_id=R AND R.status==waiting_subtasks    │             │
  │                                → trigger resume of group session                                 │             │
  │                                       │ ─ resume(prompt=<subtask_event status=ok>S1 done…</…>) ─→               │
  │                                       │                          (8) Group decides:             │             │
  │                                       │                            ctl message send "1/2 done"                 │
  │  ←─── "✅ 1/2 data collection done" ────────────────────────────────────│                       │             │
  │                                       │                            ctl task suspend             │             │
  │                          tasks(R).status = 'waiting_subtasks' (again)     │                       │             │
  │                                       │                                                  (S2 done)             │
  │                                       │ ←── S2 execution=completed ──────────────────────────── │             │
  │                          (9) Same as (7)                                  │                       │             │
  │                                       │ ─ resume(<subtask_event status=ok>S2 done…</…>) ─────→   │             │
  │                                       │                          (10) All done, decides:        │             │
  │                                       │                            ctl message send "🎉 Final"   │             │
  │  ←─── "🎉 Done. Result: …" ─────────────────────────────────────────────│                       │             │
  │                                       │                            ctl task mark-success(R)     │             │
  │                                       │  ←──────────────────────────────│                       │             │
  │                          tasks(R).status = 'completed'; session released                                       │
```

### 5.3 Resume content is computed from the task table

When a sub-task transitions to a terminal state (or a Worker `message send` is rerouted), the dispatcher pushes a `DispatchTask` to the Group session containing the latest snapshot:

```
<subtask_event source=…>
  <root_task id=R status=waiting_subtasks/>
  <subtasks>
    <subtask id=S1 worker=A status=completed result=…/>
    <subtask id=S2 worker=B status=running/>
    …
  </subtasks>
  <recent>
    <event subtask=S1 kind=completed result=…/>
  </recent>
</subtask_event>
```

The snapshot is queried at trigger time; the design does not depend on a reliable event stream. A single trigger arriving at the Group session is sufficient — the prompt always reflects the current state.

### 5.4 SessionKey routing

| Trigger | sessionKey | agent_id (in `bee_session_contexts`) |
|---|---|---|
| User message → Bee | original IM session_key | `"bee"` |
| User message → Worker (direct or via Bee) | original IM session_key | `worker.id` |
| User message → Group (direct or via Bee) | original IM session_key | `group.id` |
| Group's sub-task → Worker | newly minted: `subtask:{root_task_id}` | `worker.id` |

The minted sub-task `sessionKey` prevents the user's prior chat history with the same Worker from leaking into the sub-task's context.

## 6. Error Handling

Faults are grouped by their source.

### 6.1 Worker / sub-task

| Fault | Detection | Action |
|---|---|---|
| Worker process crash / abnormal exit | dispatcher `waitForResult` sees `execStatus=failed` | Mark sub-task failed → resume Group with `<subtask_event status=failed reason=…>` → Group decides retry / reassign / escalate. |
| Worker engine timeout | same as above | same; reason notes "timeout". |
| Worker fails to start (resource / permission) | dispatcher `resolveExecution` returns error | same; reason notes the startup error. |
| Sub-task cancelled by operator (`task cancel`) | existing cancel path | task=cancelled → `<subtask_event status=cancelled>` → Group decides. |

### 6.2 Group agent

| Fault | Detection | Action |
|---|---|---|
| Group agent process crash | dispatcher `waitForResult` sees failed for a group task | At service startup, `RecoverGroupTasks` scans tasks where status ∈ {`waiting_subtasks`, `running`} and `agent_kind='group'`. Instead of failing them, the dispatcher injects a `<recovery_event>` and resumes the agent. |
| Group engine timeout | same | Same recovery path. The persona prompt steers Group toward "single action per turn + suspend" to keep turns short. |
| "Phantom suspend": Group suspended but all sub-tasks are already terminal | dispatcher detects on the `suspend` write | Immediately trigger one resume with `<subtask_event status=all_done>` to avoid deadlock. |
| Group dispatched sub-tasks then crashed before suspending | execution=failed but child tasks remain pending | Recovery path; the recovery event indicates "previous turn dispatched N sub-tasks". |
| Group never suspends and never marks final | engine timeout | Falls back to the recovery path. |

### 6.3 Dispatcher / service

| Fault | Detection | Action |
|---|---|---|
| `openbee server` restart | startup `RecoverFeeding` (existing) + new `RecoverGroupTasks` | Pending sub-tasks remain pending and are re-enqueued naturally; `waiting_subtasks` roots take the recovery path. |
| Sub-task event `inCh` near full | existing log + drop pattern | Buffer sized at 256. The recovery model — snapshot from `tasks` table at trigger time — means a dropped trigger is harmless as long as another arrives later. |

> **Key design choice**: the prompt content for resume is computed from the live `tasks` table, not consumed from an event queue. This downgrades event-stream reliability concerns to "at-least-once trigger delivery", which the current channel pattern already guarantees in practice.

### 6.4 Naming / reference errors

| Fault | Detection | Action |
|---|---|---|
| `group create --name` collides with worker / bot | manager validation | Reject with the same error style as `validateWorkerName`. |
| Group dispatches to a worker that isn't a member | server-side check on `dispatch-subtask` | MVP: warn but allow (gives Group cross-team flexibility). Final stance to be confirmed during implementation. |
| Group dispatches against a root task it does not own | server-side check (root must match the Group's current session) | Reject; prevents privilege escalation. |

### 6.5 Notifications

| Fault | Detection | Action |
|---|---|---|
| Group `message send` fails (IM platform unreachable) | existing `outbound` store records status=failed | No additional handling. |
| Group calls `mark-failed` | `mark-failed` triggers `failureNotifier.NotifyTaskFailure` automatically | MUST: explicit user notification required as a safety net. |
| Group calls `mark-success` | does **not** auto-notify; the Group is expected to have already issued a final summary via `message send` | Keeps the user-facing voice exclusively in the Group's hands. |

### 6.6 Resource cleanup

| Scenario | Action |
|---|---|
| Delete Group with active root tasks | Reject (mirrors Worker delete policy). |
| Delete Group with `worker_groups` rows | Manual cascade cleanup. |
| Remove Worker from Group while it is running a sub-task for that Group | Reject; ask operator to wait or cancel sub-task first. |
| Orphaned `bee_session_contexts` (Group deleted) | `manager.DeleteGroup` calls `ClearSessionContexts(group.id)`. |

### 6.7 Cancellation cascade

| Trigger | Behaviour |
|---|---|
| User cancels root task | Cancel Group agent + cancel all sub-tasks in {pending, running} (recursive) → notify user "cancelled". |
| User cancels a single sub-task | Cancel only that sub-task → `<subtask_event status=cancelled>` → Group decides whether to continue or abort the root. |
| Group calls `mark-failed` early | Cancel all unfinished sub-tasks + notify user "failed". |

## 7. Testing Strategy

Existing repo conventions: Go test + real SQLite in-memory store + fakes for engine and execution. Each new store / manager gets a `_test.go`; dispatcher uses the `dispatcher_internal_test.go` style.

### 7.1 Unit tests (per package)

| File | Coverage |
|---|---|
| `store/group_store_test.go` | Group CRUD; `ExistsByName`; name uniqueness. |
| `store/group_store_test.go` | `worker_groups` add / remove / list; cascade cleanup on group delete and on worker delete. |
| `store/task_store_test.go` (incremental) | parent / root writes and queries; `ListByRoot(rootID)`; `ListChildrenStatus(parentID)`; agent_kind filter. |
| `domain/group/manager_test.go` | `CreateGroup` (collision, work_dir creation, persona prepare); `UpdateGroup`; `DeleteGroup` (rejects when root tasks active); member add / remove validation. |

### 7.2 Dispatcher behaviour tests (the heavyweight)

Following the existing fakes pattern (`fakeExecutionManager`, `fakeStore`, `fakeSessionStore`).

| Case | Key assertions |
|---|---|
| Dispatch group task → group agent starts → suspend → tasks.status='waiting_subtasks' | State correct; session preserved. |
| Sub-task completes → group session is resumed | Resume called once; prompt contains `<subtask_event>` populated from a `tasks` snapshot, not from an event stream. |
| Multiple sub-tasks complete in parallel → multiple resumes | Each resume occurs; same sessionKey serializes them. |
| Group `mark-success` → user notified + session cleared + root task = completed | All three side effects observed. |
| Group `mark-failed` → `failureNotifier` invoked + unfinished sub-tasks cancelled | Cascade cancel correct. |
| User cancels root task → group agent cancelled + all unfinished sub-tasks cancelled | All `cancelFuncs` triggered. |
| Cancel single sub-task → cancelled event injected → group resumed | Sub-task event routing correct. |
| Worker `message send` in sub-task context → no IM, transcribed event triggers group resume | `outboundStore` not called; group resumed. |
| Phantom suspend: group suspends after all sub-tasks already terminal | Immediate resume to prevent deadlock. |
| Service startup: `waiting_subtasks` root + session present → `RecoverGroupTasks` resumes | Single resume; prompt is `<recovery_event>`. |
| Service startup: `waiting_subtasks` root + session lost → mark failed | User notified of failure. |

### 7.3 Migration tests

| Case | Assertion |
|---|---|
| Fresh DB → migrations run → groups / worker_groups exist; tasks has the three new columns | Schema correct. |
| Pre-existing data (legacy `tasks` rows) → migrations run → `root_task_id` backfilled to self | UPDATE applied to all legacy rows. |
| Re-run migrations on already-migrated DB | `bee_migrations` is idempotent. |

### 7.4 End-to-end test

One integration test `e2e_group_test.go`:
- Boot an embedded server (existing test harness).
- Create a Group via CLI; add 2 worker members.
- Direct-dispatch `@group-name task` into `message_store`.
- Use a fake engine (scripted CLI replay) to simulate the Group's three turns: (a) initial split + suspend, (b) post-S1 progress message + suspend, (c) post-S2 final message + mark-success.
- Use the same fake to play the two Workers' completions.
- Assert: 3 outbound messages reach the user; all `tasks` rows are `completed`; session contexts are cleared.

A reusable scripted-engine fake will be introduced (or extended from any existing one) for this purpose.

### 7.5 Out of scope for tests

- Real LLM call accuracy (a prompt-tuning concern, not engineering).
- Real IM platform delivery (already covered by existing `outbound_messages` tests).
- Cross-process race conditions (the dispatcher is a single goroutine; no race exists at this layer).

## 8. Open Questions Deferred to Implementation

The following are intentionally not pinned in this spec; they will be resolved while implementing:

- Exact prompt text for the Group persona, the `<subtask_event>` wrapper, and the `<recovery_event>` wrapper.
- Whether `dispatch-subtask` rejects non-member workers (MVP leaning: warn only).
- Whether `bee_executions.worker_id` should grow a sibling `agent_kind` column for clarity.
- REST API shape for Group resources beyond minimal CRUD.

## 9. Out of Scope

- Web Console UI for Group (deferred to a separate iteration).
- Shared workspace between members (scenario 2 from the brainstorm). Schema and dispatcher leave a clean extension path: add a `runs/{root_task_id}` directory creation step and inject the path into sub-task instructions.
- Hierarchical Groups (a Group as a member of another Group). The data model permits it (`agent_kind=group` on a sub-task), but this is not exercised in the MVP and not tested.
- Multi-leader / role-based dispatch within a Group. The `worker_groups.role` column is reserved.
