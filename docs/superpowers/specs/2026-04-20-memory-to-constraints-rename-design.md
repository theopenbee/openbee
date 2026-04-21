# Design: Rename `memory` Field to `constraints` in `bee_workers`

**Date:** 2026-04-20
**Status:** Approved

## Background

The `memory` field in `bee_workers` stores static per-worker instructions that are injected into every AI session. The name `memory` conflicts with the built-in memory capabilities of AI agents (Claude, Codex, etc.), causing confusion. The new name `constraints` (工作约束) clearly communicates that this field defines behavioral rules imposed on the worker, not agent-managed memory.

## Scope

Full rename across all layers: database, Go code, REST API, MCP tools, frontend UI, and i18n copy.

**Breaking changes:**
- REST API: JSON field `memory` → `constraints`
- MCP tools: tool parameter `memory` → `constraints`

## Changes by Layer

### 1. Database Migration (version 39)

File: `internal/infra/store/db.go`

```sql
ALTER TABLE bee_workers RENAME COLUMN memory TO constraints;
```

SQLite 3.25.0+ supports `RENAME COLUMN` natively. The project uses `modernc.org/sqlite v1.46.1` (SQLite 3.49.x), so no compatibility risk.

### 2. Go Model

File: `internal/infra/model/worker.go`

```go
// Before
Memory string `json:"memory" db:"memory"`

// After
Constraints string `json:"constraints" db:"constraints"`
```

### 3. Store Layer

File: `internal/infra/store/worker_store.go`

- INSERT statement: `memory` → `constraints`
- SELECT column list: `memory` → `constraints`
- UPDATE statement: `memory` → `constraints`
- Scan call: field pointer updated to `&w.Constraints`

### 4. Domain Layer

File: `internal/domain/worker/manager.go`

- `CreateWorkerParams.Memory` → `CreateWorkerParams.Constraints`
- Assignment `worker.Memory = params.Memory` → `worker.Constraints = params.Constraints`

File: `internal/domain/task/dispatcher.go`

- `ai.WorkerPersona(worker.Name, worker.Description, worker.Memory)` → `ai.WorkerPersona(worker.Name, worker.Description, worker.Constraints)`

### 5. AI Prompt Injection

File: `internal/ai/rules.go`

- Function signature: parameter `memory string` → `constraints string`
- Injected section header: `## Memory Constraints` → `## Work Constraints`

File: `internal/ai/rules_test.go`

- Update assertion strings from `## Memory Constraints` → `## Work Constraints`

### 6. REST API Handler

File: `internal/api/worker_handler.go`

- `createWorkerRequest.Memory` → `.Constraints`, JSON tag `"memory"` → `"constraints"`
- PATCH handler: field reference updated

### 7. MCP Tools

File: `internal/mcp/tools.go`

- `toolCreateWorker`: parameter `memory` → `constraints`
- `toolUpdateWorker`: parameter `memory` → `constraints`, `fieldsChanged` detection updated

File: `internal/mcp/tools_test.go`

- Test field references updated from `memory` → `constraints`

### 8. Frontend

File: `web/src/pages/worker-detail.tsx`

- State variables: `isEditingMemory` → `isEditingConstraints`, `editMemory` → `editConstraints`
- API call payload field: `memory` → `constraints`

File: `web/src/components/create-worker-sheet.tsx`

- Form field: `memory` → `constraints`
- API call payload field: `memory` → `constraints`

### 9. Internationalization

File: `web/src/locales/en.json`

| Key | Before | After |
|-----|--------|-------|
| `memory` (createWorker) | `Static Memory` | `Work Constraints` |
| `memoryPlaceholder` | `The instruction this worker will execute...` | `The work constraints for this worker...` |
| `memoryHelper` | `Persistent context loaded into the worker at the start of every session.` | `Work constraints injected into the worker at the start of every session.` |
| `memory` (workerDetail) | `Static Memory` | `Work Constraints` |
| `noMemory` | `No static memory configured` | `No work constraints configured` |
| `editMemory` | `Edit static memory` | `Edit work constraints` |

File: `web/src/locales/zh.json`

| Key | Before | After |
|-----|--------|-------|
| `memory` (createWorker) | `静态记忆` | `工作约束` |
| `memoryPlaceholder` | `这个员工执行的指令...` | `这个员工的工作约束...` |
| `memoryHelper` | `在每次会话开始时加载到员工中的持久化上下文。` | `每次会话开始时注入到员工中的工作约束。` |
| `memory` (workerDetail) | `静态记忆` | `工作约束` |
| `noMemory` | `未配置静态记忆` | `未配置工作约束` |
| `editMemory` | `编辑静态记忆` | `编辑工作约束` |

## Files Changed Summary

| File | Change |
|------|--------|
| `internal/infra/store/db.go` | Add migration 39 |
| `internal/infra/model/worker.go` | Rename field + tags |
| `internal/infra/store/worker_store.go` | Update SQL + scan |
| `internal/domain/worker/manager.go` | Update CreateWorkerParams + assignment |
| `internal/domain/task/dispatcher.go` | Update field reference |
| `internal/ai/rules.go` | Update param + section header |
| `internal/ai/rules_test.go` | Update assertion strings |
| `internal/api/worker_handler.go` | Update request struct + JSON tag |
| `internal/mcp/tools.go` | Update tool parameters |
| `internal/mcp/tools_test.go` | Update test field names |
| `web/src/pages/worker-detail.tsx` | Update state vars + API field |
| `web/src/components/create-worker-sheet.tsx` | Update form field + API field |
| `web/src/locales/en.json` | Update copy |
| `web/src/locales/zh.json` | Update copy |
