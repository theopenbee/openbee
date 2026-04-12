# Worker Permission Scopes Design

**Date:** 2026-04-09  
**Status:** Approved

## Overview

Workers currently cannot read system information (employees, departments, tasks), which prevents use cases like periodic status reporting and summarization. This design adds a `permission_scopes` field to the Worker model so that authorized workers can perform read-only operations via existing `openbee ctl` subcommands.

## Goals

- Workers with granted scopes can call read-only `openbee ctl` commands
- Scope is resource-level: `read:workers`, `read:departments`, `read:tasks`
- Scopes are set by administrators via `ctl worker create/update --scopes`
- No new CLI subcommands; existing subcommands gain worker-token support
- Bee token (admin) behavior is unchanged

## Non-Goals

- Write operations for workers (always forbidden)
- Fine-grained per-field access control
- Real-time scope revocation without token refresh

---

## Part 1: Data Layer

### Worker Model (`internal/infra/model/worker.go`)

Add field:

```go
type Worker struct {
    // ... existing fields ...
    PermissionScopes string `json:"permission_scopes" db:"permission_scopes"`
}
```

Stored as a comma-separated string (e.g., `"read:workers,read:tasks"`).

### Database Migration (`internal/infra/store/db.go`)

Add column to the `workers` table DDL:

```sql
permission_scopes TEXT NOT NULL DEFAULT ''
```

### Scope Constants (`internal/infra/auth/scopes.go`)

New file defining scope constants and the tool-to-scope mapping:

```go
const (
    ScopeReadWorkers     = "read:workers"
    ScopeReadDepartments = "read:departments"
    ScopeReadTasks       = "read:tasks"
)

// ToolScopeMap maps tool names to the scope required for worker-token callers.
// Tools in this map require the listed scope when called by a worker token.
// Tools absent from this map follow existing access rules (unchanged behavior).
var ToolScopeMap = map[string]string{
    utils.ListWorkers:     ScopeReadWorkers,
    utils.GetWorker:       ScopeReadWorkers,
    utils.GetWorkerStatus: ScopeReadWorkers,
    utils.ListDepartments: ScopeReadDepartments,
    utils.GetDepartment:   ScopeReadDepartments,
    utils.ListTasks:       ScopeReadTasks,
}
```

---

## Part 2: Auth Layer & Token

### MCPClaims Extension (`internal/infra/auth/token.go`)

Add `Scopes` field:

```go
type MCPClaims struct {
    Type     string   `json:"type"`
    WorkerID string   `json:"worker_id,omitempty"`
    Scopes   []string `json:"scopes,omitempty"`
    jwt.RegisteredClaims
}
```

### GenerateWorkerToken Signature Change

```go
func GenerateWorkerToken(secret, workerID string, scopes []string, ttl time.Duration) (string, error)
```

### Worker Manager (`internal/domain/worker/manager.go`)

Before generating the worker token, parse `worker.PermissionScopes` into `[]string`:

```go
scopes := parseScopes(worker.PermissionScopes) // splits on comma, trims whitespace
token, err := auth.GenerateWorkerToken(secret, worker.ID, scopes, ttl)
```

### Scope Check in Tool Dispatch (`internal/mcp/tools.go`)

Add `checkWorkerScope` helper called at the top of `beeCallTool`:

```go
func (s *MCPServer) checkWorkerScope(ctx context.Context, toolName string) error {
    tokenType, _ := ctx.Value(CtxWorkerIDKey).(string) // reuse existing ctx key pattern
    // Use the token type stored by JWTAuthMiddleware via gin ctx → request ctx
    // (accessed as CtxTokenTypeKey set alongside CtxWorkerIDKey)
    if tokenType == "" {
        // not a worker token (bee token has empty workerID): always allowed
        return nil
    }
    requiredScope, ok := auth.ToolScopeMap[toolName]
    if !ok {
        return nil // tool not in scope map: follow existing access rules
    }
    scopes, _ := ctx.Value(CtxScopesKey).([]string)
    for _, s := range scopes {
        if s == requiredScope {
            return nil
        }
    }
    return fmt.Errorf("permission denied: scope %s required", requiredScope)
}
```

The middleware sets `CtxScopesKey` from `claims.Scopes` alongside the existing `CtxKeyWorkerID`.

---

## Part 3: CLI Layer

### ctl worker create / update (`cmd/openbee/ctl_worker.go`)

New `--scopes` flag (comma-separated):

```
openbee ctl worker create --name 天天 --scopes read:workers,read:tasks
openbee ctl worker update <id> --scopes read:workers,read:departments,read:tasks
```

The flag value is passed through to the `create_worker` / `update_worker` tool as `permission_scopes`.

### ctl client token priority (`internal/ctlclient/client.go`)

`NewClient` already reads `OPENBEE_API_KEY` first. No change needed — workers set this env var at launch with their worker token, so ctl commands automatically use the worker token in worker contexts and the bee token in admin contexts.

---

## Part 4: Security Boundary

| Caller | Token type | Write tools (create/update/delete) | Scoped read tools | Other worker tools |
|--------|-----------|-------------------------------------|-------------------|--------------------|
| Admin | bee | ✅ allowed | ✅ allowed | ✅ allowed |
| Worker (no scopes) | worker | ✅ unchanged* | ❌ denied | ✅ unchanged* |
| Worker (read:workers) | worker | ✅ unchanged* | ✅ list/get workers | ✅ unchanged* |
| Worker (read:*) | worker | ✅ unchanged* | ✅ all read tools | ✅ unchanged* |

*Existing behavior for tools not in ToolScopeMap is unchanged. Current code allows workers to call all tools via RequireBeeOrWorker — this design does not regress that. Future hardening of write-tool access for workers is out of scope.

- Scope changes take effect on the next worker token refresh (worker restart or token TTL expiry)
- Workers can never call write operations regardless of scopes
- `send_message`, `create_task`, `save_memory`, etc. remain worker-callable without scope (they were already accessible via worker token — unchanged behavior)

> **Note on existing worker-callable tools:** Tools like `send_message`, `create_task`, `save_memory`, `get_memory`, `delete_memory`, `clear_session`, `get_system_overview`, `list_bee_executions`, `list_session_contexts`, `clear_worker_session` are currently callable by workers and remain so — they are not added to `ToolScopeMap` and are not blocked by the new check. Only the new read-org tools (workers/departments/tasks listing) require explicit scope grants.

---

## Part 5: SKILL.md Update

Add a "Read-Only Query Commands" section to `internal/infra/skillinstall/skills/openbee-worker/SKILL.md` under the existing `openbee ctl CLI Reference` section:

```markdown
### Read-Only Query Commands (Requires Permission Scope)

The following commands are available only if the administrator has granted the corresponding
permission scope to this worker. The worker token in OPENBEE_API_KEY is used automatically.

**Requires `read:workers` scope:**
```bash
openbee ctl worker list                       # List all workers
openbee ctl worker list --department <id>     # Filter by department
openbee ctl worker get <id>                   # Get worker details
openbee ctl worker status <id>                # Get worker current status
```

**Requires `read:departments` scope:**
```bash
openbee ctl department list                   # List all departments (tree)
openbee ctl department get <id|name>          # Get department details
```

**Requires `read:tasks` scope:**
```bash
openbee ctl task list --worker-id <id>        # List tasks for a worker
openbee ctl task list --status pending        # Filter tasks by status
```
```

---

## Change Summary

| File | Change |
|------|--------|
| `internal/infra/model/worker.go` | Add `PermissionScopes string` field |
| `internal/infra/store/db.go` | Add `permission_scopes` column to workers DDL |
| `internal/infra/auth/scopes.go` | New file: scope constants + ToolScopeMap |
| `internal/infra/auth/token.go` | Add `Scopes []string` to MCPClaims; update GenerateWorkerToken signature |
| `internal/domain/worker/manager.go` | Parse scopes from worker model before token generation |
| `internal/mcp/auth.go` | Set CtxScopesKey from claims.Scopes in JWTAuthMiddleware |
| `internal/mcp/tools.go` | Add checkWorkerScope; call at top of beeCallTool dispatch |
| `cmd/openbee/ctl_worker.go` | Add --scopes flag to create/update commands |
| `internal/infra/skillinstall/skills/openbee-worker/SKILL.md` | Add read-only commands section |
