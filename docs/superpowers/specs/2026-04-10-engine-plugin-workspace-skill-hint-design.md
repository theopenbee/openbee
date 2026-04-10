# Engine Plugin: Workspace Simplification & Skill Hint Prefix

**Date:** 2026-04-10
**Branch:** feat/engine-plugin

## Overview

Two related changes for the codex/pi engine plugin path:

1. **Workspace simplification** — stop writing `.openbee.md` for codex/pi engines; AGENTS.md contains only persona.
2. **Skill hint prefix** — on the first message of a new session, prepend `use openbee-<role> skill.` so the agent loads the correct skill without relying on a static file instruction.

These two changes are intentionally coupled: removing the `.openbee.md` file-based rule injection is safe only because the skill hint prefix takes over the responsibility of bootstrapping the agent on session start.

## Background

Previously, codex/pi engines used a two-file approach:
- `AGENTS.md`: persona + `LoadInstruction` (telling the agent to read `.openbee.md`)
- `.openbee.md`: full system rules including the "invoke the Skill tool" directive

This design required the agent to read a separate file on every task. The new approach removes the indirection:
- `AGENTS.md`: persona only (identity, name, description, memory constraints)
- No `.openbee.md`
- Session-start message includes `use openbee-<role> skill.` as a plain-language directive

Claude is **not affected** — it has its own workspace implementation using `CLAUDE.md` + `@import`.

---

## Change 1: Workspace Simplification

### Files Changed
- `internal/ai/workspace.go`
- `internal/ai/rules.go` (new worker persona helper; existing `BeeRules`/`WorkerRules` are **kept** — still used by Claude engine)

### Design

`SetupWorkspace()` for codex/pi engines (called from codex and pi adapters):

**Bee role:**
```
AGENTS.md content:
You are B, an AI assistant.
```
(uses existing `BeePersona` constant as-is)

**Worker role:**
```
AGENTS.md content:
You are a Worker in an AI team.
Name: <name>
Description: <description>

## Memory Constraints
<memory>
```

New helper function added to `rules.go`:
```go
func WorkerPersona(name, description, memory string) string {
    s := "You are a Worker in an AI team.\n"
    if name != "" {
        s += fmt.Sprintf("Name: %s\n", name)
    }
    if description != "" {
        s += fmt.Sprintf("Description: %s\n", description)
    }
    if memory != "" {
        s += fmt.Sprintf("\n## Memory Constraints\n%s\n", memory)
    }
    return s
}
```

This is distinct from `WorkerRules()` (which contains the full rules directive) — `WorkerRules()` remains unchanged and is still used by the Claude engine.

The `LoadInstruction` and `.openbee.md` write are removed entirely from `SetupWorkspace()`. The `writeSystemRules()` call is dropped.

### Idempotency

`createAgentsMD` uses `CreateFileOnce` — existing workspaces with old AGENTS.md are not overwritten. Old `.openbee.md` files already on disk are left in place but are inert (no LoadInstruction points to them).

---

## Change 2: Skill Hint Prefix

### Files Changed
- `internal/ai/rules.go` (new `SkillHintPrefix` function)
- `internal/domain/bee/feeder.go` (`buildPrompt` signature change)
- `internal/domain/task/dispatcher.go` (`resolveExecution` prefix injection)

### New Helper

Added to `internal/ai/rules.go`:

```go
func SkillHintPrefix(role Role) string {
    switch role {
    case RoleBee:
        return "use openbee-bee skill."
    case RoleWorker:
        return "use openbee-worker skill."
    default:
        return ""
    }
}
```

### Bee Side (feeder.go)

`buildPrompt` gains a `skillHint string` parameter:

```go
func buildPrompt(msgs []store.ClaimedMessage, skillHint string) string {
    var sb strings.Builder
    if skillHint != "" {
        sb.WriteString(skillHint)
        sb.WriteByte('\n')
    }
    // existing message wrapping logic unchanged
}
```

Call site in `processBeeGroup` (resume is already determined at line 159):

```go
hint := ""
if !resume {
    hint = ai.SkillHintPrefix(ai.RoleBee)
}
prompt := buildPrompt(msgs, hint)
```

Result for a new bee session:
```
use openbee-bee skill.
<message_meta>{"from":"feishu",...}</message_meta>
<message_content>
user message
</message_content>
```

### Worker Side (dispatcher.go)

Prefix logic is injected inside `resolveExecution()`, which already contains the session resume decision — no extra DB calls needed:

```go
func (d *TaskDispatcher) resolveExecution(ctx context.Context, task DispatchTask, instruction string) (model.WorkerExecution, error) {
    hint := ai.SkillHintPrefix(ai.RoleWorker)

    if task.TaskType != model.TaskTypeImmediate {
        // Non-immediate tasks have no session key — always fresh, always add hint
        return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
    }
    sessionID, err := d.sessionStore.GetSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, d.engineName)
    if err != nil {
        log.Error("get session context", zap.Error(err))
    }
    if sessionID == "" {
        // New session — add hint
        return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
    }
    // Resuming existing session — no hint
    exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID)
    if err == nil {
        return exec, nil
    }
    // Resume failed, falling back to fresh — add hint
    log.Error("resume error, falling back to fresh", zap.Error(err))
    if clearErr := d.sessionStore.ClearSessionContexts(ctx, task.SessionKey); clearErr != nil {
        log.Error("clear stale session contexts", zap.String("sessionKey", task.SessionKey), zap.Error(clearErr))
    }
    return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
}
```

---

## How "New Session" Is Determined

| Context | Mechanism |
|---------|-----------|
| Bee | `resume` variable computed from `GetSessionContextForEngine()` returning `""` |
| Worker (immediate task) | `sessionID == ""` check inside `resolveExecution()` |
| Worker (non-immediate task) | Always new — no session key |
| Worker (resume fallback) | After clearing stale context, re-executes as new |

---

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Claude engine | Unaffected — has independent workspace setup via `claude/adapter.go` |
| Existing workspace with old AGENTS.md | `CreateFileOnce` skips write; old content unchanged until workspace is recreated |
| Old `.openbee.md` on disk | Stays on disk but is inert — no LoadInstruction references it |
| Resume session (bee or worker) | No prefix added; agent already has skill loaded in context |
| Worker resume fails → fresh retry | Fresh execution gets prefix |
| Execution log stores prompt | Hint is included in stored prompt; acceptable for audit purposes |

---

## Affected Files Summary

| File | Change |
|------|--------|
| `internal/ai/workspace.go` | Remove `.openbee.md` write; AGENTS.md persona-only |
| `internal/ai/rules.go` | Add `SkillHintPrefix(role Role) string`; add `WorkerPersona(name, description, memory string) string` |
| `internal/domain/bee/feeder.go` | `buildPrompt` gains `skillHint` param; call site passes hint on new session |
| `internal/domain/task/dispatcher.go` | `resolveExecution` prepends hint on new/fresh sessions |
