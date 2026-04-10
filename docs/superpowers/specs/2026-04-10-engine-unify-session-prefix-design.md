# Design: Unify Engine Session Prefix Mechanism (Remove File Generation)

**Date:** 2026-04-10
**Branch:** feat/engine-plugin

## Background

The system has three engine adapters: Claude Code, Codex, and PI Agent. Previously, engines injected system rules into the AI via generated config files:

- Claude engine: wrote `CLAUDE.md` (with `@.openbee.md` import) and `.openbee.md` (full system rules)
- Codex/PI engines: wrote `AGENTS.md` (persona info; `.openbee.md` generation was already removed)

A newer mechanism was introduced: on new sessions, prepend a skill hint (and worker persona) directly to the prompt. This is already implemented for all three engines via the Feeder (Bee) and TaskDispatcher (Worker). The file-based approach is now redundant and creates inconsistency.

## Goal

Remove all config file generation from all three engine adapters. All engines rely solely on the "prepend prefix on new session" mechanism for injecting system rules and persona.

Additionally, clean up residual Claude-specific files from existing workspaces.

## Decisions

| Question | Decision |
|----------|----------|
| Existing `.openbee.md` / `CLAUDE.md` files in old workspaces | Claude's `Prepare` hook actively cleans them up (delete `.openbee.md`, remove `@.openbee.md` line from `CLAUDE.md`) |
| Scope of file generation removal | All three engines: Claude removes CLAUDE.md/.openbee.md generation; Codex/PI remove AGENTS.md generation |
| `SetupWorkspace` interface method | Renamed to `Prepare`, signature simplified to `Prepare(workDir string) error` (no `role` or `WorkspaceOptions`) |
| workDir creation responsibility | Moved to Manager (calls `os.MkdirAll` before `engine.Prepare`), not the engine's concern |
| Existing AGENTS.md in Codex/PI workspaces | Left as-is (harmless, self-deprecating over time) |
| `BeeRules()` / `WorkerRules()` in rules.go | Deleted (only used by file generation logic) |
| `WorkerPersona()` / `SkillHintPrefix()` in rules.go | Kept (used by TaskDispatcher and Feeder for prompt injection) |

## Architecture

### EngineAdapter Interface Change

```go
// Before
type EngineAdapter interface {
    SetupWorkspace(workDir string, role Role, opts WorkspaceOptions) error
    Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (Process, <-chan Output, error)
    ExtractResult(logPath string) string
}

// After
type EngineAdapter interface {
    Prepare(workDir string) error  // engine-specific initialization hook; no-op for most engines
    Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (Process, <-chan Output, error)
    ExtractResult(logPath string) string
}
```

### Manager: workDir Creation

`Manager` calls `os.MkdirAll(workDir, 0755)` before calling `engine.Prepare(workDir)`. This is the Manager's responsibility as the execution context orchestrator.

### Claude Engine (`internal/ai/claude/`)

`Prepare(workDir string) error` performs cleanup only:

1. Delete `.openbee.md` if it exists (silent if absent)
2. Remove the `@.openbee.md` line from `CLAUDE.md` if it exists (silent if file absent or line absent)

No file creation. No rule injection. No persona writing.

`claudemd.go` cleanup:
- Remove `writeCLAUDEMD()`, `EnsureSystemRules()`, and related file-writing helpers
- Keep only helpers needed for the cleanup logic (e.g., line removal from file)
- If the file becomes trivially small, merge remaining helpers into `adapter.go` and delete `claudemd.go`

### Codex Engine (`internal/ai/codex/`)

`Prepare(workDir string) error` returns `nil` immediately (no-op).

Remove AGENTS.md writing logic from `adapter.go`. Delete or simplify `internal/ai/workspace.go` if it was shared only by Codex/PI for AGENTS.md generation.

### PI Agent Engine (`internal/ai/pi/`)

`Prepare(workDir string) error` returns `nil` immediately (no-op).

Same as Codex: remove AGENTS.md writing logic.

### rules.go (`internal/ai/rules.go`)

Remove:
- `BeeRules() string`
- `WorkerRules(name, description, memory string) string`
- `BeePersona` constant (only used by file generation code being deleted)

Keep:
- `WorkerPersona(name, description, memory string) string`
- `SkillHintPrefix(role Role) string`

### Existing Mechanism: Unchanged

The prompt-prefix injection mechanism is already in place and remains unchanged:

- **Bee** (Feeder, `internal/domain/bee/feeder.go`): prepends `use openbee-bee skill.\n` on new sessions
- **Worker** (TaskDispatcher, `internal/domain/task/dispatcher.go`): prepends skill hint + `<worker_persona>` block on new sessions via `workerSkillHint()`

## File Change Summary

| File | Change |
|------|--------|
| `internal/ai/engine.go` | Rename `SetupWorkspace` → `Prepare`; remove `role`, `WorkspaceOptions` params |
| `internal/ai/claude/adapter.go` | `Prepare`: delete `.openbee.md`, remove `@.openbee.md` line from `CLAUDE.md` |
| `internal/ai/claude/claudemd.go` | Remove write functions; keep/merge cleanup helpers; possibly delete file |
| `internal/ai/codex/adapter.go` | `Prepare`: return nil; remove AGENTS.md write |
| `internal/ai/pi/adapter.go` | `Prepare`: return nil; remove AGENTS.md write |
| `internal/ai/workspace.go` | Delete entirely — the file only contained the shared `SetupWorkspace` / `createAgentsMD` functions, which are fully removed |
| `internal/ai/rules.go` | Remove `BeeRules()`, `WorkerRules()` |
| `internal/ai/manager.go` | Add `os.MkdirAll` before `engine.Prepare`; update call site |

## Testing

- Verify Claude engine: on a workspace with existing `.openbee.md` and `CLAUDE.md` (with `@.openbee.md`), calling `Prepare` removes the file and the line
- Verify Claude engine: on a workspace without these files, `Prepare` returns nil without error
- Verify Codex/PI: `Prepare` returns nil
- Integration: new session with Claude engine receives skill hint via prompt prefix (not via CLAUDE.md)
- Integration: resumed session with Claude engine does not receive prefix
