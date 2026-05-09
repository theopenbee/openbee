# Skill Hint & Worker Persona via System Prompt

Date: 2026-05-08
Status: Draft

## Problem

`SkillHintPrefix` (the `[MANDATORY] You MUST invoke the openbee-bee/-worker skill immediately…` line) and `WorkerPersona` (role / identity / constraints) are currently injected into the **user message** at the start of a fresh session:

- `internal/domain/task/dispatcher.go:337-348` (worker)
- `internal/domain/bee/feeder.go:203-207` (bee)

Because they live in the user channel, the model treats them as one more user instruction among many. Empirically the skill hint is **not honoured 100% of the time** — agents occasionally produce a text reply before invoking the skill, breaking downstream contracts.

The hypothesis (validated by the Claude / pi CLI design) is that the same content delivered through the system-prompt channel carries higher priority and will be obeyed reliably.

## Goals

1. Move the skill hint **and** the full worker persona (role, identity, constraints) from the user prompt into the system prompt for engines that support it.
2. Keep call sites engine-agnostic: callers state *what* to inject; adapters decide *how*.
3. Preserve current resume semantics (no hint re-injection on resume) to minimise regression risk.
4. Leave codex and kimi behaviour unchanged — they continue to receive the content via user-prompt prefix exactly as today.

## Non-Goals

- Codex-specific workarounds (`AGENTS.md` injection, `-c` config overrides). Codex stays on the user-prompt-prefix path.
- Re-architecting the engine adapter interface beyond the one new field.
- Reworking the bee prompt structure beyond removing the `hint` argument from `buildPrompt`.
- Changing what the hint or persona say. Only the channel changes.

## Engine Capability Matrix

| Engine | System-prompt CLI flag | Strategy |
|---|---|---|
| claude | `--append-system-prompt <text>` | Native |
| pi | `--append-system-prompt <text>` | Native |
| codex | none | Adapter falls back to prepending to user prompt |
| kimi | none | Adapter falls back to prepending to user prompt |

The fallback for codex / kimi reproduces today's behaviour exactly (hint + persona prepended to the user message on fresh sessions only). They are not regressed by this change, just no longer the dispatcher's responsibility.

## Design

### 1. Adapter contract: add `RunOptions.SystemPrompt`

`internal/ai/contracts.go`:

```go
type RunOptions struct {
    SessionID    string
    Resume       bool
    APIKey       string
    ExtraEnv     []string
    ExtraArgs    []string
    SystemPrompt string // session-level system instructions (skill hint + persona)
}
```

Each adapter is responsible for routing `SystemPrompt` to the right channel:

| Adapter | Behaviour when `SystemPrompt != ""` |
|---|---|
| claude | Append `--append-system-prompt <SystemPrompt>` to argv |
| pi | Append `--append-system-prompt <SystemPrompt>` to argv |
| codex | Prepend `SystemPrompt + "\n\n"` to the user prompt (stdin / arg) |
| kimi | Prepend `SystemPrompt + "\n\n"` to the user prompt |

When `SystemPrompt == ""` every adapter behaves exactly as today.

### 2. New helper: `ai.BuildSystemPrompt`

`internal/ai/prompt.go`:

```go
// BuildSystemPrompt returns the full system-prompt body (skill hint plus
// worker persona block, when applicable). Returns "" for unknown roles.
func BuildSystemPrompt(role Role, w *model.Worker) string {
    hint := SkillHintPrefix(role)
    if hint == "" {
        return ""
    }
    if role == RoleWorker && w != nil {
        persona := WorkerPersona(w.Name, w.Description, w.Constraints)
        return hint + "\n<worker_persona>\n" + persona + "</worker_persona>"
    }
    return hint
}
```

The `<worker_persona>` wrapper is preserved so any existing parsing or operator habits keep working.

### 3. Caller changes

#### Worker dispatcher (`internal/domain/task/dispatcher.go`)

`executeWithHint` becomes:

```go
func (d *TaskDispatcher) executeWithHint(ctx context.Context, task DispatchTask, instruction, engineName string, worker *model.Worker) (model.WorkerExecution, error) {
    if d.workerLookup != nil && worker == nil {
        return model.WorkerExecution{}, fmt.Errorf("worker %q not found", task.WorkerID)
    }
    sysPrompt := ai.BuildSystemPrompt(ai.RoleWorker, worker)
    sessionID := uuid.New().String()
    d.upsertSessionContext(ctx, task, sessionID, engineName)
    return d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID, false, sysPrompt)
}
```

The resume branch in `resolveExecution` stays as today and passes `""` for `SystemPrompt`.

#### Worker manager (`internal/domain/worker/execution.go`)

`ExecuteWorker` and `launchRuntime` gain a `systemPrompt string` parameter that is plumbed straight into `ai.RunOptions.SystemPrompt`.

```go
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string, resume bool, systemPrompt string) (model.WorkerExecution, error)
```

The `task.ExecutionManager` interface is updated to match.

#### Bee feeder (`internal/domain/bee/feeder.go`)

```go
sysPrompt := ""
if !resume {
    sysPrompt = ai.BuildSystemPrompt(ai.RoleBee, nil)
}
prompt := buildPrompt(msgs, "")
runRes, err := f.runner.Run(beeCtx, f.workDir, prompt, ai.RunOptions{
    SessionID:    sessionID,
    Resume:       resume,
    SystemPrompt: sysPrompt,
}, logPath)
```

`buildPrompt` keeps its second parameter for now (passing `""`) to minimise the diff; a follow-up can drop it once nothing else depends on the signature.

### 4. Resume policy: only fresh sessions carry `SystemPrompt`

This is a deliberate, conservative choice:

- Fresh session — `SystemPrompt = BuildSystemPrompt(...)` (hint + persona)
- Resume — `SystemPrompt = ""` (no injection)

Why:

- Skill / persona context is established on the first turn and persists in the session memory; later turns inherit it.
- Matches current behaviour exactly, so the diff is minimal and the regression surface is small.
- If reality shows resumes occasionally lose the skill, flipping the policy to "always inject" is a one-line change in dispatcher / feeder.

## Per-Engine Implementation Notes

### claude (`internal/ai/claude/invoker.go`)

In `Run`, before assembling `args`:

```go
if opts.SystemPrompt != "" {
    args = append(args, "--append-system-prompt", opts.SystemPrompt)
}
```

The flag must be added before `--print` to match existing argv ordering. `opts.ExtraArgs` continue to append after, so user-supplied engine args can still override or extend.

### pi (`internal/ai/pi/invoker.go`)

In `buildArgs`:

```go
if systemPrompt != "" {
    base = append(base, "--append-system-prompt", systemPrompt)
}
```

`buildArgs` gains a `systemPrompt string` parameter; `Run` passes `opts.SystemPrompt` through.

### codex (`internal/ai/codex/invoker.go`)

In `Run`, before passing `prompt` to `cmd.Stdin` / `buildArgs`:

```go
if opts.SystemPrompt != "" {
    prompt = opts.SystemPrompt + "\n\n" + prompt
}
```

No resume guard needed at the adapter layer — the caller (`dispatcher` / `feeder`) only sets `SystemPrompt` on fresh sessions, so the adapter can treat it as the single source of truth.

### kimi (`internal/ai/kimi/invoker.go`)

Equivalent prepend in kimi's `Run`. Exact line depends on how kimi assembles its prompt; the rule is identical to codex.

## Testing

Update existing tests; no new test files required.

- `internal/ai/prompt_test.go` — add coverage for `BuildSystemPrompt` (worker with / without persona, bee, unknown role).
- `internal/ai/claude/invoker_test.go` — assert `--append-system-prompt <body>` is present in argv when `SystemPrompt` is set, and absent when it is empty.
- `internal/ai/pi/invoker_test.go` — same assertion against pi's argv.
- `internal/ai/codex/invoker_test.go` — assert the prompt sent on stdin starts with `SystemPrompt` when set, and is unchanged when `SystemPrompt` is empty.
- `internal/ai/kimi/invoker_test.go` — same assertion as codex.
- `internal/domain/task/dispatcher_test.go` — the existing assertions that `instruction` starts with `SkillHintPrefix(...)` flip:
  - Fresh path: `instruction` no longer starts with the hint; the hint now appears in the new `SystemPrompt` parameter.
  - Resume path: unchanged (no hint).
- `internal/domain/bee/feeder_test.go` — same shape: assert the prompt no longer carries the hint and that `RunOptions.SystemPrompt` does on fresh runs.

## Migration / Rollout

This is an internal refactor with no external API surface. There are no on-disk schemas to migrate, no config keys to deprecate, and no user-visible behaviour change beyond improved skill-invocation reliability on claude / pi.

A single PR ships:
1. Contract change (`RunOptions.SystemPrompt`) and `BuildSystemPrompt` helper.
2. Adapter implementations (claude, pi, codex, kimi).
3. Caller updates (dispatcher, manager, feeder) and interface bumps.
4. Test updates.

No feature flag — the change is uniformly safer than the current behaviour, and the codex / kimi paths preserve current semantics by construction.

## Risks & Open Questions

- **Cache invalidation on claude.** `--append-system-prompt` content participates in claude's prompt cache. Persona changes (worker rename / constraint edit) will invalidate the cached prefix for that worker's next session, which is correct behaviour but worth noting.
- **Persona length.** Long worker constraints inflate the system prompt on every fresh session. This is no worse than today (same content, different channel) but if persona grows we may want a length guard later.
- **Codex / kimi parity.** These engines do not benefit from the system-prompt boost. Acceptable per the brief; revisit if compliance issues are observed there.
