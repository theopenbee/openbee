# Worker Persona Injection on New Session

**Date:** 2026-04-10
**Branch:** feat/engine-plugin

## Overview

When a Worker agent starts a new session (no prior session context), the dispatcher already prepends `use openbee-worker skill.` to the instruction. This change extends that hint to also include the worker's name, description, and memory — injected via a `<worker_persona>` XML block — so the agent has full identity context on first boot without relying on a static file.

## Background

The existing skill hint prefix (`use openbee-worker skill.`) bootstraps the agent's behavior rules on new sessions. However, name/description/memory are currently only written to `AGENTS.md` (for codex/pi engines). On resume sessions this is fine — the agent already has context. But on a fresh session the agent may not have read its own identity yet before the first instruction arrives.

This change injects persona inline alongside the skill hint, ensuring identity is always present on session start regardless of whether the agent reads `AGENTS.md` first.

## Design

### Format

New sessions use the following prompt structure:

```
use openbee-worker skill.
<worker_persona>
You are a Worker in an AI team.
Name: <name>
Description: <description>

## Memory Constraints
<memory>
</worker_persona>
<task_meta>{"message_id":"...","task_id":"..."}</task_meta>
<task_content>
actual instruction
</task_content>
```

Resume sessions are unchanged — no prefix injected.

Empty fields (name/description/memory) are omitted by `WorkerPersona()`, which already handles this gracefully. The `<worker_persona>` block is still injected even if all fields are empty (in that case it contains only `You are a Worker in an AI team.\n`).

### Interface

A new narrow interface is defined in `internal/domain/task/dispatcher.go`:

```go
// WorkerLookup fetches worker metadata for persona injection on new sessions.
type WorkerLookup interface {
    GetByID(id string) (model.Worker, error)
}
```

`store.WorkerStore` satisfies this interface without modification.

### TaskDispatcher Changes

New field and option in `internal/domain/task/dispatcher.go`:

```go
type TaskDispatcher struct {
    // ... existing fields unchanged ...
    workerLookup WorkerLookup // optional; if nil, only skill hint is injected
}

func WithWorkerLookup(lookup WorkerLookup) Option {
    return func(d *TaskDispatcher) { d.workerLookup = lookup }
}
```

New helper method:

```go
func (d *TaskDispatcher) workerSkillHint(workerID string) (string, error) {
    hint := ai.SkillHintPrefix(ai.RoleWorker)
    if d.workerLookup == nil {
        return hint, nil
    }
    w, err := d.workerLookup.GetByID(workerID)
    if err != nil {
        return "", fmt.Errorf("lookup worker for persona hint: %w", err)
    }
    persona := ai.WorkerPersona(w.Name, w.Description, w.Memory)
    return hint + "\n<worker_persona>\n" + persona + "</worker_persona>", nil
}
```

`resolveExecution` calls `workerSkillHint` on all new/fresh execution paths:

```go
func (d *TaskDispatcher) resolveExecution(ctx context.Context, task DispatchTask, instruction string) (model.WorkerExecution, error) {
    if task.TaskType != model.TaskTypeImmediate {
        hint, err := d.workerSkillHint(task.WorkerID)
        if err != nil {
            return model.WorkerExecution{}, err
        }
        return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
    }
    sessionID, err := d.sessionStore.GetSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, d.engineName)
    if err != nil {
        log.Error("get session context", zap.Error(err))
    }
    if sessionID == "" {
        hint, err := d.workerSkillHint(task.WorkerID)
        if err != nil {
            return model.WorkerExecution{}, err
        }
        return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
    }
    exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID)
    if err == nil {
        return exec, nil
    }
    log.Error("resume error, falling back to fresh", zap.Error(err))
    if clearErr := d.sessionStore.ClearSessionContexts(ctx, task.SessionKey); clearErr != nil {
        log.Error("clear stale session contexts", zap.String("sessionKey", task.SessionKey), zap.Error(clearErr))
    }
    hint, err := d.workerSkillHint(task.WorkerID)
    if err != nil {
        return model.WorkerExecution{}, err
    }
    return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
}
```

### Wiring

In `internal/app/app.go`, pass `WithWorkerLookup` when constructing the dispatcher:

```go
task.New(..., task.WithWorkerLookup(workerStore))
```

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| `workerLookup` is nil | Only `use openbee-worker skill.` injected; no DB call |
| name/description/memory all empty | `<worker_persona>` block contains only the base role line |
| `GetByID` returns error (worker deleted etc.) | Hard fail: `resolveExecution` returns error; task fails and user is notified |
| Resume session | No prefix injected; existing behavior unchanged |
| Resume fails → fallback to fresh | Fresh path executes; full persona hint injected |

## Affected Files

| File | Change |
|------|--------|
| `internal/domain/task/dispatcher.go` | Add `WorkerLookup` interface, `workerLookup` field, `WithWorkerLookup` option, `workerSkillHint` method; update `resolveExecution` |
| `internal/app/app.go` | Pass `WithWorkerLookup(workerStore)` when constructing dispatcher |

`internal/ai/rules.go` — `WorkerPersona()` and `SkillHintPrefix()` reused as-is, no changes.
