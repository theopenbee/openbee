# Worker Per-Engine Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow each Worker agent to independently select its AI engine (claude/codex/pi), while engine binary paths and global config remain shared.

**Architecture:** Add `engine` field to the `Worker` model and DB. At startup, build one `EngineAdapter` per engine type and store them in a `map[string]EngineAdapter` passed to `worker.Manager`. The manager picks the correct adapter per worker at execute time, falling back to the global default for legacy workers with an empty engine field.

**Tech Stack:** Go (backend), SQLite migrations, React + TypeScript (frontend), base-ui Select component

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/model/worker.go` | Add `Engine string` field |
| `internal/infra/store/db.go` | Add migration version 36 |
| `internal/infra/store/worker_store.go` | Update columns, scan, Create, Update |
| `internal/infra/config/config.go` | Add `EngineConfigRawFor(name string)` method |
| `internal/app/app.go` | Replace `buildEngine` with `buildAllEngines` |
| `internal/domain/worker/manager.go` | engines map, resolveEngine, CreateWorkerParams, ExecuteWorker, launchRuntime, monitorExecution |
| `internal/domain/worker/manager_test.go` | Add resolveEngine tests |
| `internal/api/worker_handler.go` | Engine validation, create/update request changes |
| `web/src/lib/types.ts` | Add `engine` field to `Worker` interface |
| `web/src/hooks/use-workers.ts` | Add `engine` to create mutation type |
| `web/src/components/create-worker-sheet.tsx` | Engine dropdown (required) |
| `web/src/pages/worker-detail.tsx` | Engine display + edit |
| `web/src/locales/en.json` | i18n keys for engine field |
| `web/src/locales/zh.json` | i18n keys for engine field (Chinese) |

---

## Task 1: Data Model — Add engine field to Worker

**Files:**
- Modify: `internal/infra/model/worker.go`

- [ ] **Step 1: Add `Engine` field**

Open `internal/infra/model/worker.go`. The current `Worker` struct ends with `UpdatedAt`. Add `Engine` after `PermissionScopes`:

```go
type Worker struct {
	ID                  string       `json:"id" db:"id"`
	Name                string       `json:"name" db:"name"`
	Description         string       `json:"description" db:"description"`
	Memory              string       `json:"memory" db:"memory"`
	WorkDir             string       `json:"work_dir" db:"work_dir"`
	Engine              string       `json:"engine" db:"engine"`
	Status              WorkerStatus `json:"status" db:"status"`
	PermissionScopes    string       `json:"permission_scopes" db:"permission_scopes"`
	CreatedAt           int64        `json:"created_at" db:"created_at"`
	UpdatedAt           int64        `json:"updated_at" db:"updated_at"`
}
```

- [ ] **Step 2: Build to verify**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./internal/infra/model/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/infra/model/worker.go
git commit -m "feat(model): add engine field to Worker"
```

---

## Task 2: DB Migration — Add engine column

**Files:**
- Modify: `internal/infra/store/db.go`
- Modify: `internal/infra/store/worker_store.go`

- [ ] **Step 1: Write the test**

Open `internal/infra/store/worker_store_test.go`. Find an existing test that creates a worker and checks its fields. Add a new test:

```go
func TestWorkerStore_EngineField(t *testing.T) {
	dir := t.TempDir()
	db, err := store.InitDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ws := store.NewWorkerStore(db)

	w, err := ws.Create(model.Worker{
		Name:    "test-worker",
		WorkDir: dir,
		Engine:  "codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.Engine != "codex" {
		t.Errorf("expected engine %q, got %q", "codex", w.Engine)
	}

	got, err := ws.GetByID(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Engine != "codex" {
		t.Errorf("after get: expected engine %q, got %q", "codex", got.Engine)
	}

	updated, err := ws.Update(model.Worker{
		ID:      w.ID,
		Name:    w.Name,
		WorkDir: w.WorkDir,
		Engine:  "pi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Engine != "pi" {
		t.Errorf("after update: expected engine %q, got %q", "pi", updated.Engine)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/infra/store/... -run TestWorkerStore_EngineField -v
```

Expected: FAIL — compile error or column-not-found error.

- [ ] **Step 3: Add migration to db.go**

In `internal/infra/store/db.go`, after migration version 35 (the last entry), add:

```go
{
    version: 36,
    name:    "add_engine_to_bee_workers",
    sql:     `ALTER TABLE bee_workers ADD COLUMN engine TEXT NOT NULL DEFAULT ''`,
},
```

- [ ] **Step 4: Update worker_store.go — columns constants**

In `internal/infra/store/worker_store.go`, update the two column constants:

```go
const (
	workerColumns        = `id, name, description, memory, work_dir, engine, status, permission_scopes, created_at, updated_at`
	workerColumnsAliased = `w.id, w.name, w.description, w.memory, w.work_dir, w.engine, w.status, w.permission_scopes, w.created_at, w.updated_at`
)
```

- [ ] **Step 5: Update worker_store.go — scanWorker**

```go
func scanWorker(scanner interface{ Scan(...any) error }) (model.Worker, error) {
	var w model.Worker
	err := scanner.Scan(
		&w.ID, &w.Name, &w.Description, &w.Memory,
		&w.WorkDir, &w.Engine, &w.Status, &w.PermissionScopes, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return model.Worker{}, err
	}
	return w, nil
}
```

- [ ] **Step 6: Update worker_store.go — Create**

Replace the `INSERT` statement in `Create`:

```go
_, err := s.db.Exec(
    `INSERT INTO bee_workers (id, name, description, memory, work_dir, engine, status, permission_scopes, created_at, updated_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    w.ID, w.Name, w.Description, w.Memory, w.WorkDir, w.Engine,
    w.Status, w.PermissionScopes, w.CreatedAt, w.UpdatedAt,
)
```

- [ ] **Step 7: Update worker_store.go — Update**

Replace the `UPDATE` statement in `Update`:

```go
_, err := s.db.Exec(
    `UPDATE bee_workers SET name=?, description=?, memory=?, work_dir=?, engine=?, status=?, permission_scopes=?, updated_at=?
     WHERE id=?`,
    w.Name, w.Description, w.Memory, w.WorkDir, w.Engine,
    w.Status, w.PermissionScopes, w.UpdatedAt, w.ID,
)
```

- [ ] **Step 8: Run test to verify it passes**

```bash
go test ./internal/infra/store/... -run TestWorkerStore_EngineField -v
```

Expected: PASS.

- [ ] **Step 9: Run all store tests**

```bash
go test ./internal/infra/store/... -v
```

Expected: all PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/infra/store/db.go internal/infra/store/worker_store.go internal/infra/store/worker_store_test.go
git commit -m "feat(store): add engine column to bee_workers (migration v36)"
```

---

## Task 3: Config — Add EngineConfigRawFor helper

**Files:**
- Modify: `internal/infra/config/config.go`

- [ ] **Step 1: Add EngineConfigRawFor method**

In `internal/infra/config/config.go`, after the existing `EngineConfigRaw()` method, add:

```go
// EngineConfigRawFor returns the raw config map for the named engine.
// Used when building all engine adapters at startup.
func (b BeeConfig) EngineConfigRawFor(name string) map[string]any {
	switch name {
	case "claude":
		return map[string]any{"path": b.Claude.Path}
	case "codex":
		return map[string]any{"path": b.Codex.Path}
	case "pi":
		return map[string]any{"path": b.Pi.Path, "env": b.Pi.Env}
	default:
		return nil
	}
}
```

- [ ] **Step 2: Build to verify**

```bash
go build ./internal/infra/config/...
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/config/config.go
git commit -m "feat(config): add EngineConfigRawFor to support per-engine adapter init"
```

---

## Task 4: App Wiring — Pre-build all engine adapters

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Replace buildEngine with buildAllEngines**

In `internal/app/app.go`, replace the `buildEngine` function:

```go
// buildAllEngines constructs one EngineAdapter per supported engine type.
// All adapters are built at startup and shared safely across concurrent workers.
func buildAllEngines(cfg config.BeeConfig) (map[string]ai.EngineAdapter, error) {
	names := []string{ai.EngineClaude, ai.EngineCodex, ai.EnginePi}
	result := make(map[string]ai.EngineAdapter, len(names))
	for _, name := range names {
		adapter, err := ai.New(name, ai.EngineConfig{
			OpenbeeURL: cfg.MCPBaseURL,
			Raw:        cfg.EngineConfigRawFor(name),
		})
		if err != nil {
			return nil, fmt.Errorf("init engine %q: %w", name, err)
		}
		result[name] = adapter
	}
	return result, nil
}
```

- [ ] **Step 2: Update BuildApp to use buildAllEngines**

In `BuildApp`, replace:
```go
engine, err := buildEngine(cfg.Bee)
if err != nil {
    return nil, fmt.Errorf("init engine: %w", err)
}
```

With:
```go
engines, err := buildAllEngines(cfg.Bee)
if err != nil {
    return nil, fmt.Errorf("init engines: %w", err)
}
```

- [ ] **Step 3: Update buildBee call to pass the global engine**

Change:
```go
feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engine, envSvc)
```

To:
```go
feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engines[cfg.Bee.EffectiveEngine()], envSvc)
```

- [ ] **Step 4: Update buildWorkerManager to pass engines map**

Change:
```go
mgr := buildWorkerManager(cfg.Bee, s, engine, envSvc)
```

To:
```go
mgr := buildWorkerManager(cfg.Bee, s, engines, envSvc)
```

- [ ] **Step 5: Update buildWorkerManager signature**

```go
func buildWorkerManager(bc config.BeeConfig, s appStores, engines map[string]ai.EngineAdapter, envSvc *env.Service) *worker.Manager {
	return worker.NewManager(config.DefaultWorkerBaseDir(), bc, s.workerStore, s.execStore, engines, envSvc)
}
```

- [ ] **Step 6: Build to verify (will fail until manager is updated — that's ok)**

```bash
go build ./internal/app/... 2>&1 | head -20
```

Expected: compile errors in worker.NewManager signature — that's expected, resolved in the next task.

- [ ] **Step 7: Commit (after Task 5 passes build)**

Hold this commit until Task 5 is done and the build passes.

---

## Task 5: Worker Manager — engines map + resolveEngine

**Files:**
- Modify: `internal/domain/worker/manager.go`
- Modify: `internal/domain/worker/manager_test.go`

- [ ] **Step 1: Write tests for resolveEngine**

In `internal/domain/worker/manager_test.go`, add after the existing mock types:

```go
func newTestManager(t *testing.T, engines map[string]ai.EngineAdapter, defaultEngine string) *Manager {
	t.Helper()
	dir := t.TempDir()
	db, err := store.InitDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, dir)
	const testKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	envSvc, err := env.NewService(store.NewEnvConfigStore(db), store.NewDepartmentStore(db), testKey)
	if err != nil {
		t.Fatal(err)
	}
	bc := config.BeeConfig{}
	bc.MCP.TokenTTL = time.Minute
	m := &Manager{
		workerBaseDir:   dir,
		tokenSecret:     bc.MCP.TokenSecret,
		tokenTTL:        bc.MCP.TokenTTL,
		workerTimeout:   30 * time.Minute,
		workerStore:     ws,
		executionStore:  es,
		engines:         engines,
		defaultEngine:   defaultEngine,
		envService:      envSvc,
		activeProcesses: make(map[string]ai.Process),
	}
	return m
}

func TestManager_ResolveEngine_KnownEngine(t *testing.T) {
	claude := &mockEngine{}
	codex := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude, "codex": codex}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: "codex"}
	got := mgr.resolveEngine(w)
	if got != codex {
		t.Error("expected codex engine adapter")
	}
}

func TestManager_ResolveEngine_EmptyEngine_FallsBackToDefault(t *testing.T) {
	claude := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: ""}
	got := mgr.resolveEngine(w)
	if got != claude {
		t.Error("expected default claude engine adapter")
	}
}

func TestManager_ResolveEngine_UnknownEngine_FallsBackToDefault(t *testing.T) {
	claude := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: "unknown-engine"}
	got := mgr.resolveEngine(w)
	if got != claude {
		t.Error("expected fallback to default claude engine adapter")
	}
}
```

Also add the missing import `"path/filepath"` to the test file if not already present.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/domain/worker/... -run "TestManager_ResolveEngine" -v
```

Expected: FAIL — compile errors because `Manager` still has `engine` field not `engines`.

- [ ] **Step 3: Update Manager struct**

In `internal/domain/worker/manager.go`, replace:
```go
engine         ai.EngineAdapter
```

With:
```go
engines        map[string]ai.EngineAdapter
defaultEngine  string
```

- [ ] **Step 4: Update NewManager signature and body**

```go
func NewManager(
	workerBaseDir string,
	bc config.BeeConfig,
	ws *store.WorkerStore,
	es *store.ExecutionStore,
	engines map[string]ai.EngineAdapter,
	envService *env.Service,
) *Manager {
	return &Manager{
		workerBaseDir:   workerBaseDir,
		tokenSecret:     bc.MCP.TokenSecret,
		tokenTTL:        bc.MCP.TokenTTL,
		workerTimeout:   bc.WorkerTimeout(),
		workerStore:     ws,
		executionStore:  es,
		engines:         engines,
		defaultEngine:   bc.EffectiveEngine(),
		envService:      envService,
		activeProcesses: make(map[string]ai.Process),
	}
}
```

- [ ] **Step 5: Add resolveEngine method**

After `NewManager`, add:

```go
// resolveEngine returns the EngineAdapter for the given worker.
// If the worker has no engine set, or the engine is unknown, falls back to the default.
func (m *Manager) resolveEngine(w model.Worker) ai.EngineAdapter {
	if w.Engine != "" {
		if e, ok := m.engines[w.Engine]; ok {
			return e
		}
		log.Warn("unknown engine on worker, falling back to default",
			zap.String("worker_id", w.ID), zap.String("engine", w.Engine))
	}
	return m.engines[m.defaultEngine]
}
```

- [ ] **Step 6: Update CreateWorkerParams and CreateWorker**

Add `Engine string` to `CreateWorkerParams`:

```go
type CreateWorkerParams struct {
	Name             string
	Description      string
	Memory           string
	WorkDir          string
	PermissionScopes string
	Engine           string
}
```

In `CreateWorker`, update the two places that use `m.engine`:

Replace:
```go
if err := m.engine.Prepare(p.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
    return model.Worker{}, fmt.Errorf("prepare worker workspace: %w", err)
}

return m.workerStore.Create(model.Worker{
    ID:               id,
    Name:             p.Name,
    Description:      p.Description,
    Memory:           p.Memory,
    WorkDir:          p.WorkDir,
    PermissionScopes: p.PermissionScopes,
})
```

With:
```go
workerModel := model.Worker{
    ID:               id,
    Name:             p.Name,
    Description:      p.Description,
    Memory:           p.Memory,
    WorkDir:          p.WorkDir,
    Engine:           p.Engine,
    PermissionScopes: p.PermissionScopes,
}
if err := m.resolveEngine(workerModel).Prepare(p.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
    return model.Worker{}, fmt.Errorf("prepare worker workspace: %w", err)
}

return m.workerStore.Create(workerModel)
```

- [ ] **Step 7: Update ExecuteWorker**

In `ExecuteWorker`, replace:
```go
if err := m.engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
    log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
}
```

With:
```go
if err := m.resolveEngine(worker).Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
    log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
}
```

- [ ] **Step 8: Update launchRuntime**

In `launchRuntime`, replace:
```go
proc, outputCh, err := m.engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
```

With:
```go
proc, outputCh, err := m.resolveEngine(worker).Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
```

- [ ] **Step 9: Update monitorExecution**

In `monitorExecution`, replace both `m.engine.ExtractResult(logPath)` calls:
```go
result := m.resolveEngine(worker).ExtractResult(logPath)
```

(There are two occurrences — one for `OutputDone` and one for `OutputError`. Update both.)

- [ ] **Step 10: Run manager tests**

```bash
go test ./internal/domain/worker/... -v
```

Expected: all PASS including the three new `TestManager_ResolveEngine_*` tests.

- [ ] **Step 11: Build everything**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 12: Commit**

```bash
git add internal/app/app.go internal/domain/worker/manager.go internal/domain/worker/manager_test.go
git commit -m "feat(worker): per-worker engine selection via pre-built engine map"
```

---

## Task 6: API — Engine validation and request changes

**Files:**
- Modify: `internal/api/worker_handler.go`

- [ ] **Step 1: Add engine validation helper**

At the top of `internal/api/worker_handler.go`, after the imports, add:

```go
var validEngines = map[string]bool{
	ai.EngineClaude: true,
	ai.EngineCodex:  true,
	ai.EnginePi:     true,
}

func validateEngine(name string) error {
	if !validEngines[name] {
		return fmt.Errorf("unknown engine %q, valid values: claude, codex, pi", name)
	}
	return nil
}
```

Add `ai "github.com/theopenbee/openbee/internal/ai"` and `"fmt"` to the imports.

- [ ] **Step 2: Update createWorkerRequest**

```go
type createWorkerRequest struct {
	Name             string `json:"name" binding:"required"`
	Engine           string `json:"engine" binding:"required"`
	Description      string `json:"description"`
	Memory           string `json:"memory"`
	WorkDir          string `json:"work_dir"`
	PermissionScopes string `json:"permission_scopes"`
}
```

- [ ] **Step 3: Update Create handler to validate engine and pass it to manager**

In the `Create` handler, after the `ShouldBindJSON` block, add engine validation:

```go
if err := validateEngine(req.Engine); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
}
```

And pass `Engine` to `CreateWorkerParams`:

```go
w, err := h.manager.CreateWorker(worker.CreateWorkerParams{
    Name:             req.Name,
    Engine:           req.Engine,
    Description:      req.Description,
    Memory:           req.Memory,
    WorkDir:          req.WorkDir,
    PermissionScopes: req.PermissionScopes,
})
```

- [ ] **Step 4: Update Update handler**

In the `Update` handler, add `Engine *string` to the patch request struct:

```go
var req struct {
    Name             *string `json:"name"`
    Description      *string `json:"description"`
    Memory           *string `json:"memory"`
    PermissionScopes *string `json:"permission_scopes"`
    Engine           *string `json:"engine"`
}
```

After the existing field assignments, add engine handling:

```go
if req.Engine != nil {
    if err := validateEngine(*req.Engine); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    w.Engine = *req.Engine
}
```

- [ ] **Step 5: Build to verify**

```bash
go build ./internal/api/...
```

Expected: no output.

- [ ] **Step 6: Run all tests**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/worker_handler.go
git commit -m "feat(api): require and validate engine field on create/update worker"
```

---

## Task 7: Frontend — Types and API hook

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/hooks/use-workers.ts`

- [ ] **Step 1: Add engine to Worker interface**

In `web/src/lib/types.ts`, update `Worker`:

```typescript
export interface Worker {
  id: string
  name: string
  description: string
  memory: string
  work_dir: string
  engine: string
  permission_scopes?: string
  status: WorkerStatus
  departments?: DepartmentBrief[]
  created_at: number
  updated_at: number
}
```

- [ ] **Step 2: Add engine to useCreateWorker mutation type**

In `web/src/hooks/use-workers.ts`, update `useCreateWorker` mutation fn type:

```typescript
export function useCreateWorker() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: {
      name: string
      engine: string
      description: string
      memory?: string
      work_dir?: string
      permission_scopes?: string
    }) => api.workers.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workers"] })
    },
  })
}
```

- [ ] **Step 3: Build TypeScript**

```bash
cd web && pnpm tsc --noEmit 2>&1 | head -30
```

Expected: no errors (or only pre-existing errors unrelated to Worker).

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/types.ts web/src/hooks/use-workers.ts
git commit -m "feat(frontend/types): add engine field to Worker interface and create hook"
```

---

## Task 8: Frontend — i18n keys

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add keys to en.json**

Inside the `"workers"` → `"form"` object in `web/src/locales/en.json`, add after `"sectionConfig"`:

```json
"engine": "AI Engine",
"engineHelper": "The AI agent engine this worker will use to execute tasks.",
"engineDefault": "Select engine",
```

Also add engine name labels inside the `"workers"` object (at the same level as `"form"`):

```json
"engines": {
  "claude": "Claude Code",
  "codex": "Codex",
  "pi": "Pi"
}
```

- [ ] **Step 2: Add keys to zh.json**

Inside `"workers"` → `"form"` in `web/src/locales/zh.json`, add after `"sectionConfig"`:

```json
"engine": "AI 引擎",
"engineHelper": "该员工执行任务所使用的 AI 引擎。",
"engineDefault": "选择引擎",
```

Also add inside `"workers"`:

```json
"engines": {
  "claude": "Claude Code",
  "codex": "Codex",
  "pi": "Pi"
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat(i18n): add engine field labels for worker form"
```

---

## Task 9: Frontend — Engine dropdown in create-worker-sheet

**Files:**
- Modify: `web/src/components/create-worker-sheet.tsx`

- [ ] **Step 1: Add Select imports**

In `create-worker-sheet.tsx`, add to the imports:

```typescript
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
```

- [ ] **Step 2: Add engine to WorkerInitialValues and workerToInitialValues**

Update `WorkerInitialValues`:

```typescript
export interface WorkerInitialValues {
  name: string
  description: string
  memory: string
  work_dir: string
  permission_scopes: string
  engine: string
  departmentIds: string[]
}
```

Update `workerToInitialValues`:

```typescript
export function workerToInitialValues(worker: Worker): WorkerInitialValues {
  return {
    name: worker.name,
    description: worker.description,
    memory: worker.memory,
    work_dir: worker.work_dir,
    permission_scopes: worker.permission_scopes ?? "",
    engine: worker.engine ?? "claude",
    departmentIds: worker.departments?.map((d) => d.id) ?? [],
  }
}
```

- [ ] **Step 3: Add engine state**

In the component body, after the existing state declarations, add:

```typescript
const [engine, setEngine] = useState("claude")
```

- [ ] **Step 4: Initialize engine in the useEffect**

Inside the `useEffect` that resets form on open, add:

```typescript
setEngine(iv?.engine ?? "claude")
```

- [ ] **Step 5: Pass engine in handleSubmit**

In `handleSubmit`, add `engine` to the `createWorker.mutateAsync` call:

```typescript
const worker = await createWorker.mutateAsync({
  name: name.trim(),
  engine,
  description,
  memory: memory || undefined,
  work_dir: workDir || undefined,
  permission_scopes: serializeScopes(selectedScopes) || undefined,
})
```

- [ ] **Step 6: Add engine Select in the sectionConfig block**

In the JSX, inside the `sectionConfig` div, add the engine selector before workDir:

```tsx
<div className="space-y-1.5">
  <Label htmlFor="cws-engine">
    {t("workers.form.engine")}
    <span className="ml-1 text-destructive" aria-hidden>*</span>
  </Label>
  <Select value={engine} onValueChange={setEngine}>
    <SelectTrigger id="cws-engine">
      <SelectValue placeholder={t("workers.form.engineDefault")} />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="claude">{t("workers.engines.claude")}</SelectItem>
      <SelectItem value="codex">{t("workers.engines.codex")}</SelectItem>
      <SelectItem value="pi">{t("workers.engines.pi")}</SelectItem>
    </SelectContent>
  </Select>
  <p className="text-xs text-muted-foreground">{t("workers.form.engineHelper")}</p>
</div>
```

- [ ] **Step 7: Disable submit when engine empty**

Update the submit button's `disabled` prop to also require engine:

```tsx
disabled={createWorker.isPending || setWorkerDepts.isPending || !name.trim() || !engine}
```

- [ ] **Step 8: Build TypeScript**

```bash
cd web && pnpm tsc --noEmit 2>&1 | head -30
```

Expected: no new errors.

- [ ] **Step 9: Commit**

```bash
git add web/src/components/create-worker-sheet.tsx
git commit -m "feat(ui): add required engine selector to create worker form"
```

---

## Task 10: Frontend — Engine display and edit in worker-detail

**Files:**
- Modify: `web/src/pages/worker-detail.tsx`

- [ ] **Step 1: Add Select imports**

In `worker-detail.tsx`, add to the imports:

```typescript
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
```

- [ ] **Step 2: Add isEditingEngine and editEngine state**

In the component body, alongside the other `isEditing*` states:

```typescript
const [isEditingEngine, setIsEditingEngine] = useState(false)
const [editEngine, setEditEngine] = useState("")
```

- [ ] **Step 3: Add engine display and edit UI**

Find the area in the JSX where `description` is displayed and edited (around the `isEditingDesc` block). Add an analogous block for engine in the same section:

```tsx
{/* Engine */}
<div className="space-y-1.5">
  <div className="flex items-center gap-2">
    <span className="text-sm font-medium">{t("workers.form.engine")}</span>
    {!isEditingEngine && (
      <Button
        variant="ghost"
        size="icon"
        className="h-6 w-6"
        onClick={() => {
          setEditEngine(worker.engine ?? "claude")
          setIsEditingEngine(true)
        }}
      >
        <Pencil className="h-3 w-3" />
      </Button>
    )}
  </div>
  {isEditingEngine ? (
    <div className="flex items-center gap-2">
      <Select value={editEngine} onValueChange={setEditEngine}>
        <SelectTrigger className="h-8 text-sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="claude">{t("workers.engines.claude")}</SelectItem>
          <SelectItem value="codex">{t("workers.engines.codex")}</SelectItem>
          <SelectItem value="pi">{t("workers.engines.pi")}</SelectItem>
        </SelectContent>
      </Select>
      <Button
        size="icon"
        variant="ghost"
        className="h-8 w-8"
        disabled={!editEngine || updateWorker.isPending}
        onClick={async () => {
          await updateWorker.mutateAsync({ id: id!, data: { engine: editEngine } })
          setIsEditingEngine(false)
        }}
      >
        <Check className="h-4 w-4" />
      </Button>
      <Button
        size="icon"
        variant="ghost"
        className="h-8 w-8"
        onClick={() => setIsEditingEngine(false)}
      >
        <X className="h-4 w-4" />
      </Button>
    </div>
  ) : (
    <span className="text-sm text-muted-foreground">
      {t(`workers.engines.${worker.engine}`) || worker.engine || "—"}
    </span>
  )}
</div>
```

- [ ] **Step 4: Build TypeScript**

```bash
cd web && pnpm tsc --noEmit 2>&1 | head -30
```

Expected: no new errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/worker-detail.tsx
git commit -m "feat(ui): add engine display and inline edit to worker detail page"
```

---

## Task 11: End-to-end smoke test

- [ ] **Step 1: Start the dev server**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go run ./cmd/... &
cd web && pnpm dev &
```

- [ ] **Step 2: Create a worker via UI**

Open the web UI. Click "Create Worker". Verify:
- Engine dropdown appears and is required
- Can select `claude`, `codex`, or `pi`
- Submitting without engine is blocked
- Worker is created successfully with the selected engine

- [ ] **Step 3: Edit engine in worker detail**

Open the newly created worker's detail page. Verify:
- Engine is displayed correctly
- Clicking the pencil icon shows the dropdown
- Saving updates the engine
- Cancelling discards the change

- [ ] **Step 4: Run full test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./...
```

Expected: all PASS.

- [ ] **Step 5: Final commit if any loose changes**

```bash
git status
# commit any uncommitted changes
```
