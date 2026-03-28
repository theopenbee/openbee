# Tasks Page Design

**Date:** 2026-03-28
**Status:** Approved

## Overview

Add a Tasks management page to the OpenBee web UI that surfaces `scheduled` (cron) and `countdown` tasks. The feature spans a new global Tasks page and an embedded Tasks tab on the Worker detail page.

## Requirements

- Display `scheduled` and `countdown` task types only; `immediate` tasks are excluded
- Completed countdown tasks are not shown
- Support cancelling individual `pending` tasks
- Support batch-cancelling all `pending` tasks for a given worker
- Auto-poll every 30 seconds to refresh task data
- Two entry points: global Tasks page + Worker detail Tasks tab

## Architecture

### Backend — 3 New API Endpoints

All endpoints are registered under `/api` with JWT authentication.

#### 1. List Tasks
```
GET /api/tasks
```
Query parameters:
- `worker_id` — filter by worker (optional)
- `type` — comma-separated, default `scheduled,countdown`
- `status` — comma-separated, default `pending,running`
- `page`, `page_size` — pagination

Response: `PaginatedResponse<Task>` where each `Task` includes a `worker_name` field (joined from workers table).

Implementation: new `TaskHandler.listTasks` method using existing `TaskStore.List()` with `TaskFilter`.

#### 2. Cancel Single Task
```
DELETE /api/tasks/:id
```
- Only cancels tasks with `status = pending`; returns `409 Conflict` for other statuses
- Uses existing `TaskStore.CancelTask()`

#### 3. Batch Cancel by Worker
```
POST /api/workers/:id/tasks/cancel-all
```
- Cancels all `pending` and `running` tasks for the given worker
- Uses existing `TaskStore.CancelByWorkerID()`

New file: `internal/api/task_handler.go`
Router change: `registerTaskRoutes()` added to `router.go` beside `registerWorkerRoutes`.

### Frontend — New Page + Shared Component

#### Shared Component: `<TaskList>`

Location: `web/src/components/task-list.tsx`

Props:
```ts
interface TaskListProps {
  workerId?: string  // if provided, filters to that worker only
}
```

Responsibilities:
- Fetches tasks via `GET /api/tasks` with appropriate filters
- Auto-polls with `refetchInterval: 30_000` (react-query)
- Renders paginated table with columns: Type, Worker, Instruction, Status, Time Info, Actions
- Cancel button (single task, pending only) via `DELETE /api/tasks/:id`
- Batch cancel button (all pending for worker) via `POST /api/workers/:id/tasks/cancel-all`
- New hook: `web/src/hooks/use-tasks.ts`

#### Time Info Column

- `scheduled` task: displays Next Run (`next_run_at`) + Cron expression (`cron_expr`)
- `countdown` task: displays Trigger Time (`scheduled_at`)

#### Global Tasks Page

Route: `/tasks`
File: `web/src/pages/tasks.tsx`

- Renders `<TaskList />` (no `workerId`)
- Added to nav bar with `Clock` icon, positioned after Sessions

#### Worker Detail Tasks Tab

File: `web/src/pages/worker-detail.tsx`

- New "Tasks" tab added alongside existing content
- Renders `<TaskList workerId={id} />`

### Frontend Type Extension

Add to `web/src/lib/types.ts`:
```ts
export type TaskType = "immediate" | "countdown" | "scheduled"
export type TaskStatus = "pending" | "running" | "completed" | "failed" | "cancelled"

export interface Task {
  id: string
  message_id: string
  worker_id: string
  worker_name?: string
  instruction: string
  type: TaskType
  status: TaskStatus
  scheduled_at: number | null   // ms — countdown trigger time
  cron_expr: string
  next_run_at: number | null    // ms — scheduled next run
  execution_id: string
  created_at: number
  updated_at: number
}
```

## Data Flow

```
Frontend (react-query, 30s poll)
  → GET /api/tasks?type=scheduled,countdown&status=pending,running
  → TaskHandler.listTasks
  → TaskStore.List(TaskFilter{Type: "scheduled,countdown", Status: "pending,running", WorkerID: ...})
  → SQLite bee_tasks JOIN workers

Cancel single:
  Frontend → DELETE /api/tasks/:id → TaskHandler.cancelTask → TaskStore.CancelTask()

Batch cancel:
  Frontend → POST /api/workers/:id/tasks/cancel-all → TaskHandler.cancelWorkerTasks → TaskStore.CancelByWorkerID()
```

## Error Handling

- Cancel non-pending task: backend returns `409 Conflict`; frontend shows toast "任务已在运行或已完成，无法取消"
- Batch cancel with 0 affected rows: frontend shows toast "当前没有可取消的任务"
- Poll failure: silent — react-query retries automatically, stale data remains visible

## Testing

- Backend: `internal/api/task_handler_test.go` covering list filtering, single cancel (including 409 case), and batch cancel
- Frontend: no new tests (consistent with existing project conventions)

## Files Changed

| File | Change |
|------|--------|
| `internal/api/task_handler.go` | New — task API handler |
| `internal/api/router.go` | Add `registerTaskRoutes()` |
| `web/src/lib/types.ts` | Add `Task`, `TaskType`, `TaskStatus` types |
| `web/src/hooks/use-tasks.ts` | New — react-query hook for tasks |
| `web/src/components/task-list.tsx` | New — shared TaskList component |
| `web/src/pages/tasks.tsx` | New — global Tasks page |
| `web/src/pages/worker-detail.tsx` | Add Tasks tab |
| `web/src/components/nav.tsx` | Add Tasks nav link |
| `web/src/app.tsx` | Add `/tasks` route |
| `web/src/locales/en.json` | Add tasks i18n keys |
| `web/src/locales/zh.json` | Add tasks i18n keys |
