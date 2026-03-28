# Tasks Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Tasks management page to the OpenBee web UI that surfaces `scheduled` (cron) and `countdown` tasks, with cancel and batch-cancel support.

**Architecture:** Backend adds 3 new API endpoints (`GET /api/tasks`, `DELETE /api/tasks/:id`, `POST /api/workers/:id/tasks/cancel-all`) served by a new `task_handler.go`. Frontend adds a shared `<TaskList>` component used by both a new global `/tasks` page and a new Tasks tab on the Worker detail page.

**Tech Stack:** Go/Gin (backend), React + react-query + shadcn/ui + react-i18next (frontend), SQLite via existing TaskStore/WorkerStore.

---

## File Map

| File | Action |
|------|--------|
| `internal/store/task_store.go` | Modify — add `Limit`/`Offset` to `TaskFilter`, add `CountTasks` method |
| `internal/api/task_handler.go` | Create — `listTasks`, `cancelTask`, `cancelWorkerTasks` handlers |
| `internal/api/router.go` | Modify — add `TaskStore` to `ServerParams`, add `registerTaskRoutes` |
| `internal/app/app.go` | Modify — pass `taskStore` into `ServerParams` |
| `internal/api/task_handler_test.go` | Create — handler tests |
| `web/src/lib/types.ts` | Modify — add `Task`, `TaskType`, `TaskStatus` |
| `web/src/lib/api.ts` | Modify — add `api.tasks.*` methods |
| `web/src/hooks/use-tasks.ts` | Create — react-query hooks |
| `web/src/locales/en.json` | Modify — add tasks i18n keys |
| `web/src/locales/zh.json` | Modify — add tasks i18n keys |
| `web/src/components/task-list.tsx` | Create — shared TaskList component |
| `web/src/pages/tasks.tsx` | Create — global Tasks page |
| `web/src/components/nav.tsx` | Modify — add Tasks nav link |
| `web/src/app.tsx` | Modify — add `/tasks` route |
| `web/src/pages/worker-detail.tsx` | Modify — add Tasks tab |

---

## Task 1: Add Pagination Support to TaskStore

**Files:**
- Modify: `internal/store/task_store.go`
- Test: `internal/store/task_store_test.go`

- [ ] **Step 1: Write failing tests for CountTasks and paginated List**

Add to `internal/store/task_store_test.go`:

```go
func TestTaskStore_CountTasks(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	for i := 0; i < 3; i++ {
		ts.Create(ctx, model.Task{
			MessageID: "m1", WorkerID: "w1", Instruction: "task",
			Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
	}
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "countdown",
		Type: model.TaskTypeCountdown, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	count, err := ts.CountTasks(ctx, TaskFilter{Type: "scheduled"})
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if count != 3 {
		t.Errorf("want 3, got %d", count)
	}

	count, err = ts.CountTasks(ctx, TaskFilter{Type: "scheduled,countdown"})
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if count != 4 {
		t.Errorf("want 4, got %d", count)
	}
}

func TestTaskStore_List_Pagination(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	for i := 0; i < 5; i++ {
		ts.Create(ctx, model.Task{
			MessageID: "m1", WorkerID: "w1", Instruction: "task",
			Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
	}

	page1, err := ts.List(ctx, TaskFilter{Type: "scheduled", Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("want 2, got %d", len(page1))
	}

	page2, err := ts.List(ctx, TaskFilter{Type: "scheduled", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("want 2, got %d", len(page2))
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /path/to/openbee
go test ./internal/store/... -run "TestTaskStore_CountTasks|TestTaskStore_List_Pagination" -v
```

Expected: FAIL — `CountTasks` and `Limit`/`Offset` fields not defined yet.

- [ ] **Step 3: Add `Limit`/`Offset` to `TaskFilter` and `CountTasks` method, update `List`**

In `internal/store/task_store.go`, update the `TaskFilter` struct:

```go
// TaskFilter specifies filtering criteria for List.
// message_id and session_key are mutually exclusive.
// At least one of message_id, session_key, or worker_id must be non-empty.
type TaskFilter struct {
	MessageID  string
	SessionKey string
	WorkerID   string
	Status     string // comma-separated, e.g. "pending,running"
	Type       string // comma-separated, e.g. "immediate,countdown"
	Limit      int    // 0 means no limit
	Offset     int
}
```

At the end of the `List` method, replace the current `q += \` ORDER BY t.created_at DESC\`` line with:

```go
	q += ` ORDER BY t.created_at DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
		if f.Offset > 0 {
			q += ` OFFSET ?`
			args = append(args, f.Offset)
		}
	}
```

Add `CountTasks` method after the `List` method:

```go
// CountTasks returns the number of tasks matching the given filter (ignores Limit/Offset).
func (s *TaskStore) CountTasks(ctx context.Context, f TaskFilter) (int, error) {
	q := `SELECT COUNT(*) FROM bee_tasks t`
	if f.SessionKey != "" {
		q += ` JOIN bee_platform_messages pm ON t.message_id = pm.id`
	}
	q += ` WHERE 1=1`
	var args []any
	if f.MessageID != "" {
		q += ` AND t.message_id = ?`
		args = append(args, f.MessageID)
	}
	if f.SessionKey != "" {
		q += ` AND pm.session_key = ?`
		args = append(args, f.SessionKey)
	}
	if f.WorkerID != "" {
		q += ` AND t.worker_id = ?`
		args = append(args, f.WorkerID)
	}
	q, args = appendCSVFilter(q, args, "status", f.Status)
	q, args = appendCSVFilter(q, args, "type", f.Type)
	var count int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&count)
	return count, err
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/store/... -run "TestTaskStore_CountTasks|TestTaskStore_List_Pagination" -v
```

Expected: PASS

- [ ] **Step 5: Run full store test suite to confirm no regressions**

```bash
go test ./internal/store/... -v
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/task_store.go internal/store/task_store_test.go
git commit -m "feat(store): add CountTasks and pagination support to TaskFilter"
```

---

## Task 2: Backend — task_handler.go + Router + App Wiring

**Files:**
- Create: `internal/api/task_handler.go`
- Create: `internal/api/task_handler_test.go`
- Modify: `internal/api/router.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write failing handler tests**

Create `internal/api/task_handler_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
)

func newTestServerWithTasks(t *testing.T) (*Server, *store.TaskStore, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','Worker1','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m1','s1','feishu','hi','','',1,1,1)`)

	taskStore := store.NewTaskStore(db)
	workerStore := store.NewWorkerStore(db)

	router := gin.New()
	s := &Server{
		router: router,
		ServerParams: ServerParams{
			TaskStore:   taskStore,
			WorkerStore: workerStore,
		},
	}
	s.registerTaskRoutes(router.Group("/api"))
	return s, taskStore, func() { db.Close() }
}

func TestListTasks_FiltersByTypeAndStatus(t *testing.T) {
	s, ts, cleanup := newTestServerWithTasks(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	// Create scheduled + countdown tasks
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "cron job",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: "0 * * * *", CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "countdown job",
		Type: model.TaskTypeCountdown, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	// Immediate task — should NOT appear in default list
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "immediate",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("want total=2, got %d", resp.Total)
	}
	for _, item := range resp.Items {
		if item["type"] == "immediate" {
			t.Error("immediate task should not appear in default list")
		}
	}
}

func TestCancelTask_PendingSucceeds(t *testing.T) {
	s, ts, cleanup := newTestServerWithTasks(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tasks/"+id, nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	task, _ := ts.GetByID(ctx, id)
	if task.Status != model.TaskStatusCancelled {
		t.Errorf("want cancelled, got %s", task.Status)
	}
}

func TestCancelTask_NonPendingReturns409(t *testing.T) {
	s, ts, cleanup := newTestServerWithTasks(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	// Mark it running
	ts.UpdateStatus(ctx, id, model.TaskStatusRunning)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tasks/"+id, nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestCancelWorkerTasks_CancelsAllPending(t *testing.T) {
	s, ts, cleanup := newTestServerWithTasks(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()
	for i := 0; i < 3; i++ {
		ts.Create(ctx, model.Task{
			MessageID: "m1", WorkerID: "w1", Instruction: "x",
			Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/workers/w1/tasks/cancel-all", nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	tasks, _ := ts.List(ctx, store.TaskFilter{WorkerID: "w1", Status: "cancelled"})
	if len(tasks) != 3 {
		t.Errorf("want 3 cancelled tasks, got %d", len(tasks))
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/api/... -run "TestListTasks|TestCancelTask|TestCancelWorkerTasks" -v
```

Expected: FAIL — `TaskStore` field and handler methods not defined yet.

- [ ] **Step 3: Add `TaskStore` to `ServerParams` in `router.go`**

In `internal/api/router.go`, add `TaskStore *store.TaskStore` to `ServerParams`:

```go
type ServerParams struct {
	WorkerStore      *store.WorkerStore
	ExecutionStore   *store.ExecutionStore
	TaskStore        *store.TaskStore
	Manager          *worker.Manager
	BeeMCPServer     *mcp.MCPServer
	WorkerMCPServer  *mcp.MCPServer
	BeeAPIKey        string
	WorkerAPIKey     string
	StaticFS         fs.FS
	LocalChatHandler *LocalChatHandler
	AuthHandler      *auth.AuthHandler
	JWTMiddleware    gin.HandlerFunc
	Language         string
}
```

In `setupRoutes`, add `s.registerTaskRoutes(api)` after `s.registerExecutionRoutes(api)`:

```go
func (s *Server) setupRoutes() error {
	s.registerAuthRoutes()
	s.router.GET("/api/config", s.getConfig)

	api := s.router.Group("/api")
	api.Use(s.JWTMiddleware)
	{
		s.registerWorkerRoutes(api)
		s.registerExecutionRoutes(api)
		s.registerTaskRoutes(api)
		s.registerLocalChatRoutes(api)
	}

	s.registerMCPRoutes()
	return s.registerStaticRoutes()
}
```

- [ ] **Step 4: Create `internal/api/task_handler.go`**

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
)

func (s *Server) registerTaskRoutes(api *gin.RouterGroup) {
	api.GET("/tasks", s.listTasks)
	api.DELETE("/tasks/:id", s.cancelTask)
	api.POST("/workers/:id/tasks/cancel-all", s.cancelWorkerTasks)
}

type taskResponse struct {
	ID          string  `json:"id"`
	WorkerID    string  `json:"worker_id"`
	WorkerName  string  `json:"worker_name"`
	Instruction string  `json:"instruction"`
	Type        string  `json:"type"`
	Status      string  `json:"status"`
	ScheduledAt *int64  `json:"scheduled_at"`
	CronExpr    string  `json:"cron_expr"`
	NextRunAt   *int64  `json:"next_run_at"`
	ExecutionID string  `json:"execution_id"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
}

func (s *Server) listTasks(c *gin.Context) {
	page, pageSize, offset := parsePagination(c)

	taskType := c.DefaultQuery("type", "scheduled,countdown")
	taskStatus := c.DefaultQuery("status", "pending,running")
	workerID := c.Query("worker_id")

	filter := store.TaskFilter{
		Type:     taskType,
		Status:   taskStatus,
		WorkerID: workerID,
		Limit:    pageSize,
		Offset:   offset,
	}

	total, err := s.TaskStore.CountTasks(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tasks, err := s.TaskStore.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build worker name map from all workers (1 query, avoids N+1)
	workers, err := s.WorkerStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	workerNames := make(map[string]string, len(workers))
	for _, w := range workers {
		workerNames[w.ID] = w.Name
	}

	items := make([]taskResponse, len(tasks))
	for i, t := range tasks {
		items[i] = taskResponse{
			ID:          t.ID,
			WorkerID:    t.WorkerID,
			WorkerName:  workerNames[t.WorkerID],
			Instruction: t.Instruction,
			Type:        t.Type,
			Status:      t.Status,
			ScheduledAt: t.ScheduledAt,
			CronExpr:    t.CronExpr,
			NextRunAt:   t.NextRunAt,
			ExecutionID: t.ExecutionID,
			CreatedAt:   t.CreatedAt,
			UpdatedAt:   t.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, paginatedResponse(items, total, page, pageSize))
}

func (s *Server) cancelTask(c *gin.Context) {
	id := c.Param("id")

	task, err := s.TaskStore.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if task.Status != model.TaskStatusPending {
		c.JSON(http.StatusConflict, gin.H{"error": "task is not in pending state"})
		return
	}

	if err := s.TaskStore.CancelTask(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) cancelWorkerTasks(c *gin.Context) {
	workerID := c.Param("id")

	if err := s.TaskStore.CancelByWorkerID(c.Request.Context(), workerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
```

- [ ] **Step 5: Wire `TaskStore` into `app.go`**

In `internal/app/app.go`, find the `api.NewServer(api.ServerParams{...})` call and add `TaskStore: s.taskStore`:

```go
return api.NewServer(api.ServerParams{
    WorkerStore:      s.workerStore,
    ExecutionStore:   s.execStore,
    TaskStore:        s.taskStore,
    Manager:          mgr,
    BeeMCPServer:     beeMCPSrv,
    WorkerMCPServer:  workerMCPSrv,
    BeeAPIKey:        mcpCfg.APIKey,
    WorkerAPIKey:     mcpCfg.WorkerAPIKey,
    StaticFS:         webui.DistFS,
    LocalChatHandler: localChat,
    AuthHandler:      authHandler,
    JWTMiddleware:    jwtMiddleware,
    Language:         language,
})
```

- [ ] **Step 7: Run handler tests**

```bash
go test ./internal/api/... -run "TestListTasks|TestCancelTask|TestCancelWorkerTasks" -v
```

Expected: PASS

- [ ] **Step 8: Run full backend test suite**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all PASS (or only pre-existing failures)

- [ ] **Step 9: Commit**

```bash
git add internal/api/task_handler.go internal/api/task_handler_test.go \
        internal/api/router.go internal/app/app.go
git commit -m "feat(api): add task list, cancel, and batch-cancel endpoints"
```

---

## Task 3: Frontend Types + API Client

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`

- [ ] **Step 1: Add Task types to `web/src/lib/types.ts`**

Append to the end of the file:

```ts
export type TaskType = "immediate" | "countdown" | "scheduled"
export type TaskStatus = "pending" | "running" | "completed" | "failed" | "cancelled"

export interface Task {
  id: string
  worker_id: string
  worker_name: string
  instruction: string
  type: TaskType
  status: TaskStatus
  scheduled_at: number | null
  cron_expr: string
  next_run_at: number | null
  execution_id: string
  created_at: number
  updated_at: number
}
```

- [ ] **Step 2: Add `api.tasks` methods to `web/src/lib/api.ts`**

In `web/src/lib/api.ts`, add the `Task` import to the top-level import from `./types`:

```ts
import type { Worker, WorkerExecution, PaginatedResponse, LocalChatSession, ChatMessage, Task } from "./types"
```

Then add a `tasks` section inside the `api` object (after `localChat`):

```ts
  tasks: {
    list: (params: { workerID?: string; page?: number; pageSize?: number } = {}) => {
      const { workerID, page = 1, pageSize = 20 } = params
      const qs = new URLSearchParams({
        type: "scheduled,countdown",
        status: "pending,running",
        page: String(page),
        page_size: String(pageSize),
      })
      if (workerID) qs.set("worker_id", workerID)
      return fetchAPI<PaginatedResponse<Task>>(`/tasks?${qs}`)
    },
    cancel: (id: string) =>
      fetchAPI(`/tasks/${id}`, { method: "DELETE" }),
    cancelAll: (workerID: string) =>
      fetchAPI(`/workers/${workerID}/tasks/cancel-all`, { method: "POST" }),
  },
```

- [ ] **Step 3: Build frontend to check for type errors**

```bash
cd web && npm run build 2>&1 | tail -20
```

Expected: no TypeScript errors related to the new types.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts
git commit -m "feat(web): add Task types and api.tasks client methods"
```

---

## Task 4: use-tasks Hook

**Files:**
- Create: `web/src/hooks/use-tasks.ts`

- [ ] **Step 1: Create `web/src/hooks/use-tasks.ts`**

```ts
import { useMutation, useQuery, useQueryClient, keepPreviousData } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useTasks(params: { workerID?: string; page?: number; pageSize?: number } = {}) {
  return useQuery({
    queryKey: ["tasks", params],
    queryFn: () => api.tasks.list(params),
    placeholderData: keepPreviousData,
    refetchInterval: 30_000,
  })
}

export function useCancelTask() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.tasks.cancel(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["tasks"] }),
  })
}

export function useCancelWorkerTasks() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (workerID: string) => api.tasks.cancelAll(workerID),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["tasks"] }),
  })
}
```

- [ ] **Step 2: Build to confirm no type errors**

```bash
cd web && npm run build 2>&1 | tail -20
```

Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add web/src/hooks/use-tasks.ts
git commit -m "feat(web): add useTasks, useCancelTask, useCancelWorkerTasks hooks"
```

---

## Task 5: i18n Keys

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add tasks keys to `web/src/locales/en.json`**

In the `"nav"` object, add one new key:
```json
"tasks": "Tasks"
```

Add a new top-level `"tasks"` object (alongside the existing `"nav"`, `"workers"`, etc. keys):
```json
"tasks": {
  "title": "Tasks",
  "columns": {
    "type": "Type",
    "worker": "Worker",
    "instruction": "Instruction",
    "status": "Status",
    "timeInfo": "Time Info",
    "actions": "Actions"
  },
  "types": {
    "scheduled": "Scheduled",
    "countdown": "Countdown"
  },
  "nextRun": "Next run",
  "triggerAt": "Trigger at",
  "cancelTask": "Cancel",
  "cancelAll": "Cancel All Pending",
  "cancelSuccess": "Task cancelled",
  "cancelAllSuccess": "All pending tasks cancelled",
  "cancelNoPending": "No pending tasks to cancel",
  "cancelFailed": "Cannot cancel: task is not pending"
}
```

In the existing `"emptyState"` object, add two new keys:
```json
"noTasks": "No active tasks",
"noTasksDesc": "Scheduled and countdown tasks will appear here"
```

- [ ] **Step 2: Add tasks keys to `web/src/locales/zh.json`**

In the `"nav"` object, add one new key:
```json
"tasks": "任务"
```

Add a new top-level `"tasks"` object:
```json
"tasks": {
  "title": "任务",
  "columns": {
    "type": "类型",
    "worker": "工作者",
    "instruction": "指令",
    "status": "状态",
    "timeInfo": "时间信息",
    "actions": "操作"
  },
  "types": {
    "scheduled": "定时任务",
    "countdown": "倒计时任务"
  },
  "nextRun": "下次运行",
  "triggerAt": "触发时间",
  "cancelTask": "取消",
  "cancelAll": "取消所有待执行任务",
  "cancelSuccess": "任务已取消",
  "cancelAllSuccess": "所有待执行任务已取消",
  "cancelNoPending": "当前没有可取消的任务",
  "cancelFailed": "任务已在运行或已完成，无法取消"
}
```

In the existing `"emptyState"` object, add two new keys:
```json
"noTasks": "暂无任务",
"noTasksDesc": "定时任务和倒计时任务将在此显示"
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat(i18n): add tasks page translations (en + zh)"
```

---

## Task 6: TaskList Component

**Files:**
- Create: `web/src/components/task-list.tsx`

- [ ] **Step 1: Create `web/src/components/task-list.tsx`**

```tsx
import { useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useTasks, useCancelTask, useCancelWorkerTasks } from "@/hooks/use-tasks"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { StatusBadge } from "@/components/status-badge"
import { EmptyState } from "@/components/empty-state"
import { SkeletonTable } from "@/components/skeleton-loader"
import { PaginationControls } from "@/components/pagination-controls"
import { Badge } from "@/components/ui/badge"
import type { Task } from "@/lib/types"

const PAGE_SIZE = 20

interface TaskListProps {
  workerId?: string
}

function TypeBadge({ type }: { type: Task["type"] }) {
  const { t } = useTranslation()
  return (
    <Badge variant={type === "scheduled" ? "secondary" : "outline"}>
      {t(`tasks.types.${type}`)}
    </Badge>
  )
}

function TimeInfo({ task }: { task: Task }) {
  const { t } = useTranslation()
  if (task.type === "countdown" && task.scheduled_at) {
    return (
      <div className="text-sm text-muted-foreground">
        <span className="text-xs font-medium text-foreground">{t("tasks.triggerAt")}</span>
        <br />
        {new Date(task.scheduled_at).toLocaleString()}
      </div>
    )
  }
  if (task.type === "scheduled") {
    return (
      <div className="text-sm text-muted-foreground">
        {task.next_run_at && (
          <>
            <span className="text-xs font-medium text-foreground">{t("tasks.nextRun")}</span>
            <br />
            {new Date(task.next_run_at).toLocaleString()}
          </>
        )}
        {task.cron_expr && (
          <div className="font-mono text-xs mt-1">{task.cron_expr}</div>
        )}
      </div>
    )
  }
  return <span className="text-muted-foreground">—</span>
}

export function TaskList({ workerId }: TaskListProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const { data, error, isLoading } = useTasks({ workerID: workerId, page, pageSize: PAGE_SIZE })
  const cancelTask = useCancelTask()
  const cancelAll = useCancelWorkerTasks()

  const tasks = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))

  const handleCancel = async (id: string) => {
    try {
      await cancelTask.mutateAsync(id)
    } catch {
      // error handled by react-query, toast can be added here if desired
    }
  }

  const handleCancelAll = async (wid: string) => {
    try {
      await cancelAll.mutateAsync(wid)
    } catch {
      // error handled by react-query
    }
  }

  return (
    <div>
      {workerId && (
        <div className="flex justify-end mb-4">
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleCancelAll(workerId)}
            disabled={cancelAll.isPending}
          >
            {t("tasks.cancelAll")}
          </Button>
        </div>
      )}

      {error && <p className="text-destructive mb-4">{error.message}</p>}

      {isLoading ? (
        <SkeletonTable />
      ) : tasks.length === 0 && !error ? (
        <EmptyState
          title={t("emptyState.noTasks")}
          description={t("emptyState.noTasksDesc")}
        />
      ) : (
        <>
          <div className="rounded-xl bg-card ring-1 ring-foreground/5 overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                  <TableHead>{t("tasks.columns.type")}</TableHead>
                  {!workerId && <TableHead>{t("tasks.columns.worker")}</TableHead>}
                  <TableHead>{t("tasks.columns.instruction")}</TableHead>
                  <TableHead>{t("tasks.columns.status")}</TableHead>
                  <TableHead>{t("tasks.columns.timeInfo")}</TableHead>
                  <TableHead>{t("tasks.columns.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tasks.map((task) => (
                  <TableRow key={task.id} className="hover:bg-primary/5 transition-colors">
                    <TableCell>
                      <TypeBadge type={task.type} />
                    </TableCell>
                    {!workerId && (
                      <TableCell>
                        {task.worker_id ? (
                          <Link
                            to={`/workers/${task.worker_id}`}
                            className="text-sm hover:text-primary transition-colors"
                          >
                            {task.worker_name || task.worker_id.slice(0, 8) + "..."}
                          </Link>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                    )}
                    <TableCell className="max-w-xs">
                      <p className="text-sm truncate" title={task.instruction}>
                        {task.instruction}
                      </p>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={task.status} />
                    </TableCell>
                    <TableCell>
                      <TimeInfo task={task} />
                    </TableCell>
                    <TableCell>
                      {task.status === "pending" && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleCancel(task.id)}
                          disabled={cancelTask.isPending}
                          className="text-destructive hover:text-destructive"
                        >
                          {t("tasks.cancelTask")}
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <PaginationControls page={page} totalPages={totalPages} onPageChange={setPage} />
        </>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Build to confirm no errors**

```bash
cd web && npm run build 2>&1 | tail -20
```

Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/task-list.tsx
git commit -m "feat(web): add shared TaskList component"
```

---

## Task 7: Global Tasks Page + Nav + Route

**Files:**
- Create: `web/src/pages/tasks.tsx`
- Modify: `web/src/components/nav.tsx`
- Modify: `web/src/app.tsx`

- [ ] **Step 1: Create `web/src/pages/tasks.tsx`**

```tsx
import { useTranslation } from "react-i18next"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { TaskList } from "@/components/task-list"

export function Tasks() {
  const { t } = useTranslation()
  return (
    <FadeIn>
      <PageHeader title={t("tasks.title")} />
      <TaskList />
    </FadeIn>
  )
}
```

- [ ] **Step 2: Add Tasks link to `web/src/components/nav.tsx`**

Add `Clock` to the lucide-react import:
```tsx
import { LayoutDashboard, Bot, Activity, MessageCircle, Github, Clock } from "lucide-react"
```

Add a Tasks entry to the `links` array after the executions entry:
```tsx
const links = [
  { href: "/", label: t("nav.dashboard"), icon: LayoutDashboard },
  { href: "/workers", label: t("nav.workers"), icon: Bot },
  { href: "/executions", label: t("nav.executions"), icon: Activity },
  { href: "/tasks", label: t("nav.tasks"), icon: Clock },
  { href: "/local-chat", label: t("localChat.title"), icon: MessageCircle },
]
```

- [ ] **Step 3: Add `/tasks` route to `web/src/app.tsx`**

Add the lazy import:
```tsx
const Tasks = lazy(() => import("@/pages/tasks").then(m => ({ default: m.Tasks })))
```

Add the route inside the auth guard:
```tsx
<Route path="/tasks" element={<Tasks />} />
```

- [ ] **Step 4: Build to confirm no errors**

```bash
cd web && npm run build 2>&1 | tail -20
```

Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/tasks.tsx web/src/components/nav.tsx web/src/app.tsx
git commit -m "feat(web): add global Tasks page with nav link and route"
```

---

## Task 8: Worker Detail Tasks Tab

**Files:**
- Modify: `web/src/pages/worker-detail.tsx`

- [ ] **Step 1: Add Tasks tab to `web/src/pages/worker-detail.tsx`**

Add the import at the top of the file:
```tsx
import { TaskList } from "@/components/task-list"
```

In the `<Tabs defaultValue="sessions">` section, add a new trigger after the `info` trigger:
```tsx
<TabsList variant="line">
  <TabsTrigger value="sessions">{t("workerDetail.sessions")}</TabsTrigger>
  <TabsTrigger value="tasks">{t("nav.tasks")}</TabsTrigger>
  <TabsTrigger value="info">{t("executionDetail.info")}</TabsTrigger>
</TabsList>
```

Add the tab content between the `sessions` and `info` tab contents:
```tsx
<TabsContent value="tasks" className="mt-6">
  <TaskList workerId={id!} />
</TabsContent>
```

- [ ] **Step 2: Build to confirm no errors**

```bash
cd web && npm run build 2>&1 | tail -20
```

Expected: clean build.

- [ ] **Step 3: Run full backend tests one final time**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/worker-detail.tsx
git commit -m "feat(web): add Tasks tab to Worker detail page"
```
