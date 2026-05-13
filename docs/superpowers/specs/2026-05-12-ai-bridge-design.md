# AI Bridge — Decoupling Business Code from the AI Engine Subsystem

- Date: 2026-05-12
- Status: Design approved (sections §1–§4); pending written-spec review
- Scope: `internal/ai`, `internal/domain/{worker,bee,task,tokenstat,command}`, `internal/infra/{config,store}`, `internal/app`, `cmd/openbee`

---

## 1. Problem statement

Business code in this repo (worker / bee / task / tokenstat / command / store / config / cmd) directly imports `internal/ai` and operates against engine-shaped primitives — `ai.EngineAdapter`, `ai.RunOptions{SessionID,Resume,APIKey,ExtraEnv,ExtraArgs}`, `ai.RunResult{Process,Output chan,ExtractResult}`, `ai.Output{Type,Content}`, `ai.OutputDone/OutputError`, `ai.Process{PID,Stop}`, `ai.TokenUsage`, `ai.ResolveExtraArgs`, `ai.ValidateExtraArgs`, `ai.AllEngines`, engine-name constants — across 42 files. Three concrete consequences:

1. **Test surface bleed (痛点 A)** — `worker.Manager` / `bee.BeeProcess` / `tokenstat.Syncer` unit tests must stub `ai`-shaped types instead of business concepts.
2. **Engine-shape lock-in (痛点 B)** — any change to engine invocation shape (subprocess + `<-chan Output` + `ExtractResult` → in-process SDK, RPC, bidirectional stream) ripples into every business caller.
3. **Scattered assembly (痛点 C)** — token minting, env resolution, `engine_args` JSON merging, engine-selection fallback, log-path preparation are duplicated in `worker.Manager` (`launchRuntime`, `resolveEngine*`, `readSysConfigValue`) and `bee.BeeProcess.Run`.

## 2. Goals

- **G1.** Business code outside `internal/ai/bridge/`, `internal/ai/engine/*`, and the `cmd/openbee` composition root must not import `internal/ai`.
- **G2.** A change to engine invocation shape (channel → callback, subprocess → SDK, etc.) must be implementable inside `internal/ai/bridge/` without touching business packages.
- **G3.** Engine-invocation assembly (token, env, engine_args, engine selection, log path) lives in one place: `internal/ai/bridge`.

## 3. Non-goals

- No streaming / mid-run event delivery. Lifecycle is start → wait-for-terminal-outcome → stop. A future `Subscribe(handler)` extension is left as a clean addition that does not break §4's API.
- No changes to engine implementations under `internal/ai/engine/{claude,codex,kimi,pi}`.
- No changes to `enginecfg.Store`'s public behaviour. It is consumed inside the bridge via an `EngineSelector` adapter.
- `ai.RoleBee` / `ai.RoleWorker` in `internal/ai/contracts.go` stay where they are as internal types. The bridge does not introduce a public `Role` enum; role distinction is expressed by `RunWorker` vs `RunBee`.

## 4. Architecture

### 4.1 Package layout

```
internal/
  ai/                         // internal-only; business code may not import
    contracts.go              // EngineAdapter, RunOptions, RunResult, TokenUsage, …
    factory.go                // Factory, RegisterEngine, ResolveExtraArgs, ValidateExtraArgs
    doc.go                    // "internal-only; business code must import internal/ai/bridge"
    engine/{claude,codex,kimi,pi}/
    core/
    cliargs/
    bridge/                   // single business-facing facade
      doc.go
      facade.go               // Bridge interface, Config, New
      run.go                  // Handle, Outcome, Status, run-path implementation
      usage.go                // Usage, ErrSessionDataNotFound, CollectUsage
      names.go                // EngineXxx constants, AllEngines / EnabledEngines / IsEnabled
      validate.go             // ValidateEngine, ValidateEngineArgs
      deps.go                 // TokenIssuer, EnvResolver, EngineSelector, ArgsResolver, LogPathProvider
      adapters/               // concrete implementations of deps.go
        token.go env.go engine.go args.go logpath.go
```

### 4.2 Dependency direction (post-migration)

```
business (worker, bee, task, tokenstat, command, config, cmd, store, rpc)
    │ imports
    ▼
internal/ai/bridge ─── adapters/* ─── auth, env.Service, engineCfg, sysconfig.Store, executionStore
    │ imports
    ▼
internal/ai (EngineAdapter, Factory, ResolveExtraArgs, …)
    │ imports
    ▼
internal/ai/engine/{claude,codex,kimi,pi}
```

Only `internal/ai/bridge`, `internal/ai/engine/*`, and `cmd/openbee` may import `internal/ai`. Enforcement starts as a doc.go convention; phase 3 may add a `depguard` rule.

### 4.3 Composition root (`internal/app/app.go`)

```go
factory, _ := buildEngineFactory(cfg.Bee)
engines    := factory.Enabled()
engineCfg  := enginecfg.NewStore(defaultEngine)

br := bridge.New(bridge.Config{
    Engines: engines,
    Deps: bridge.Deps{
        TokenIssuer:     adapters.NewTokenIssuer(cfg.Bee.RPC.TokenSecret, cfg.Bee.RPC.TokenTTL),
        EnvResolver:     adapters.NewEnvResolver(envSvc),
        EngineSelector:  adapters.NewEngineSelector(engines, engineCfg),
        ArgsResolver:    adapters.NewArgsResolver(s.systemConfigStore),
        LogPathProvider: adapters.NewLogPathProvider(s.execStore),
    },
})

mgr         := worker.NewManager(workerBaseDir, bc, s.workerStore, s.execStore, br)
feeder      := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore, br, …)
tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore, br)
```

## 5. Bridge facade API

```go
package bridge

// ─── Engine constants ───
const (
    EngineClaude = "claude"
    EngineCodex  = "codex"
    EnginePi     = "pi"
    EngineKimi   = "kimi"
)

// ─── Run terminal state ───
type Status int
const (
    StatusCompleted Status = iota + 1
    StatusFailed
    StatusAbandoned   // process exited without a Done/Error signal
)
type Outcome struct {
    Status Status
    Result string
}

// ─── Lifecycle handle ───
type Handle interface {
    PID() int
    EngineName() string
    Stop() error
    Wait(ctx context.Context) (Outcome, error) // blocks until terminal
}

// ─── Request types ───
type WorkerRunRequest struct {
    WorkerID         string
    PermissionScopes []string
    ExecutionID      string
    StartedAt        time.Time
    EngineHint       string         // worker.Engine; "" = use default
    EngineArgs       string         // raw worker.EngineArgs; bridge merges global
    WorkDir          string
    Prompt           string
    SessionID        string
    Resume           bool
    Timeout          time.Duration  // 0 = no timeout
}
type BeeRunRequest struct {
    WorkDir   string
    Prompt    string
    SessionID string
    Resume    bool
    LogPath   string
}

// ─── Usage ───
type Usage struct {
    Model               string
    InputTokens         int64
    OutputTokens        int64
    CacheCreationTokens int64
    CacheReadTokens     int64
}
var ErrSessionDataNotFound = errors.New("bridge: session data not found")

// ─── Facade ───
type Bridge interface {
    // Run
    RunWorker(ctx context.Context, req WorkerRunRequest) (Handle, error)
    RunBee(ctx context.Context, req BeeRunRequest)       (Handle, error)

    // Names / validation
    AllEngines()                    []string
    EnabledEngines()                []string
    IsEnabled(name string)          bool
    ValidateEngine(name string)     error
    ValidateEngineArgs(line string) error

    // Engine selection (callers that must record engine name up front)
    ResolveEngineForWorker(workerID, hint string) string
    ResolveEngineForBee()                         string

    // Usage
    CollectUsage(ctx context.Context, engineName, sessionID string) ([]Usage, error)
}
```

### 5.1 API rationale

- **`Handle.Wait` swallows the `Output` channel loop.** Bridge translates `ai.OutputDone → StatusCompleted`, `ai.OutputError → StatusFailed`, and "channel closed with no terminal" → `StatusAbandoned`. Business code never writes `for range Output { switch ev.Type }` again.
- **`EngineHint` + `ResolveEngineForWorker` coexist.** `worker.Manager.ExecuteWorker` resolves engine first, creates an execution row carrying engine name, then runs. Bridge exposes the resolution rule directly (`ResolveEngineForWorker`) so the row can be persisted up front; `RunWorker` then re-resolves from `EngineHint` internally using the same code path so the two paths cannot drift.
- **Error split.** `RunXxx` returns `error` only for startup failure (log path, token, env, immediate `engine.Run` error). All runtime failures surface as `Outcome{Status: Failed | Abandoned}`. `Wait` returns `error` only when its `ctx` is cancelled (returns `ctx.Err()`).
- **`BeeRunRequest.LogPath` is explicit.** Bee currently has no execution-row owner of log paths. Promoting it to bridge later is additive.

## 6. Internal dependency interfaces

Bridge does not hold business stores. It depends on five narrow ports, each with 1–2 methods, defined in `deps.go` and implemented by `adapters/`.

```go
type TokenIssuer interface {
    WorkerToken(workerID string, scopes []string) (string, error)
    BeeToken() (string, error)
}
type EnvResolver interface {
    WorkerEnv(workerID string) ([]string, error)
    BeeEnv()                  ([]string, error)
}
type EngineSelector interface {
    ForWorker(hint string) string  // hint = worker.Engine, "" = default
    ForBee()               string
}
type ArgsResolver interface {
    ForWorker(ctx context.Context, workerEngineArgs, engineName string) string
    ForBee(ctx context.Context, engineName string)                     string
}
type LogPathProvider interface {
    PrepareForWorker(executionID string, startedAt time.Time) (string, error)
}
```

Adapter implementations are 1-line wrappers around existing code:

| Port method | Implementation |
|---|---|
| `TokenIssuer.WorkerToken` | `auth.GenerateWorkerToken(secret, id, scopes, ttl)` |
| `TokenIssuer.BeeToken` | `auth.GenerateBeeToken(secret, ttl)` |
| `EnvResolver.WorkerEnv` | `envSvc.ResolveWorkerEnv(id)` |
| `EnvResolver.BeeEnv` | `envSvc.ResolveBeeEnv(defaultBeeID)` |
| `EngineSelector.ForWorker(hint)` | `if engines[hint] != nil { return hint }; return engineCfg.Get()` |
| `EngineSelector.ForBee` | `engineCfg.Get()` |
| `ArgsResolver.ForWorker` | `ai.ResolveExtraArgs(en, readSysConfig(global), workerEngineArgs)` |
| `ArgsResolver.ForBee` | `ai.ResolveExtraArgs(en, readSysConfig(global), readSysConfig(bee))` |
| `LogPathProvider.PrepareForWorker` | `execStore.PrepareLogPath(execID, startedAt)` |

## 7. Lifecycle invariants

Bridge implementations must preserve, with covering unit tests, six invariants inherited from `worker.Manager.monitorExecution`:

1. **Single terminal outcome.** A given `Handle` produces at most one `Outcome`. Repeated `Wait` calls return the same value (`sync.Once`).
2. **Abandoned fallback.** If the underlying `Output` channel closes without `OutputDone` or `OutputError`, `Outcome.Status = StatusAbandoned`. Empty `Result` is replaced by the placeholder `"process exited without completion signal"`.
3. **Single `ExtractResult` call per terminal branch.** Done / Error / Abandoned branches each call `ExtractResult()` exactly once.
4. **Idempotent `Stop`.** Multiple `Stop` calls do not error. `Wait` still returns an `Outcome` after `Stop`.
5. **Context cancellation.** `Wait`'s `ctx` cancellation returns `ctx.Err()`. The engine's own context (with `Timeout`) is owned and cancelled by the bridge; business `ctx` cancellation does not directly kill the engine process (matches today's behaviour where `monitorExecution` cancels via `defer cancel()` only after the channel loop ends).
6. **PID immediately observable.** `Handle.PID()` is readable as soon as `RunXxx` returns nil error, matching today's synchronous `executionStore.UpdatePID(exec.ID, runRes.Process.PID())`.

## 8. Migration plan (three phases)

### Phase 1 — Build the bridge package; business untouched

- Create `internal/ai/bridge/` with full facade, types, deps interfaces, and `adapters/` implementations.
- Construct the bridge in `internal/app/app.go`; keep a reference but do not pass it to business objects yet.
- Add bridge-package unit tests covering all six invariants, adapters, `ResolveEngineForWorker` fallback, and `CollectUsage` `ErrSessionDataNotFound` translation.
- **Exit:** `go build ./... && go test ./internal/ai/bridge/...` green.
- **Risk:** low. Rollback = delete `internal/ai/bridge/`.

### Phase 2 — Migrate the worker path

- `worker.Manager`: drop `engines map[string]ai.EngineAdapter`, `engineCfg`, `envService`, `sysConfigStore`, `tokenSecret`, `tokenTTL`; add `bridge bridge.Bridge`.
- `worker/execution.go`: `launchRuntime` → `bridge.RunWorker`; `monitorExecution` → `go func() { outcome, err := handle.Wait(ctx); … }`; `activeProcesses map[string]ai.Process` → `map[string]bridge.Handle`.
- `worker.Manager`: `ValidateEngine` / `ValidateEngineArgs` / `EnabledEngines` / `resolveEngine` delegate to bridge; remove `resolveEngineArgs` and `readSysConfigValue`.
- `internal/app/app.go`: `buildWorkerManager` signature changes from `(engines, engineCfg, envSvc, …)` to `(bridge, …)`.
- `worker/manager_test.go`: replace engine-map stubs with a `fakeBridge` covering only `RunWorker` / `EnabledEngines` / `ValidateEngineArgs`.
- **Exit:** `grep "theopenbee/openbee/internal/ai\"" internal/domain/worker/` produces no hits; full test suite green.
- **Risk:** medium. `activeProcesses` rewrite and `Wait` error paths need careful coverage.

### Phase 3 — Migrate bee, tokenstat, and remaining `internal/ai` imports

- bee: delete `bee.BeeProcess`; `bee.Feeder` holds a `bridge.Bridge` and uses `RunBee` → `Wait` → persist.
- tokenstat: `Syncer` constructor becomes `(db, store, bridge)`; iteration uses `bridge.CollectUsage`.
- `task` / `command` / `config` / `cmd` / `rpc` tests: replace remaining `internal/ai` imports with `internal/ai/bridge` (usually `TokenUsage` → `Usage` or engine-name constants).
- `internal/infra/store/session_store.go`, `internal/infra/store/db.go`, `internal/infra/config/config.go`: switch `ai.TokenUsage` → `bridge.Usage`; bridge package internally translates.
- Clean-up:
  - Remove `ai.Factory.Dynamic` if it has no remaining consumers (worker / bee now drive selection themselves through the bridge).
  - Add `internal/ai/doc.go` noting "internal-only; business code must import internal/ai/bridge".
  - Repo self-check: `grep -r "theopenbee/openbee/internal/ai\"" --include='*.go'` should match only `internal/ai/bridge`, `internal/ai/bridge/adapters`, `internal/ai/engine/*`, and the `internal/app` bridge-construction point.
  - Optional: add a `depguard` lint rule prohibiting `internal/ai` imports outside the allow-list.
- Split into two PRs (bee+tokenstat; remaining import renames) for easier revert.
- **Exit:** grep clean; full test suite green; end-to-end run of one worker task and one bee task succeed.
- **Risk:** medium.

## 9. Testing strategy

- **Bridge unit tests** (`internal/ai/bridge/*_test.go`): a `fakeEngine` implementing `ai.EngineAdapter` drives all six lifecycle invariants, both run paths, and `CollectUsage`. Each adapter has its own unit test against the wrapped collaborator.
- **Business unit tests**: a `fakeBridge` implementing only the `Bridge` methods used by the test under study. No more stubbing of `ai.EngineAdapter` outside the bridge package.
- **Integration tests**: the existing `manager_test.go`, `feeder_test.go`, `syncer_test.go` happy paths must continue to pass.
- **End-to-end**: at the end of phases 2 and 3, run one real worker task and one real bee task locally before merging.

## 10. Rollback

- Each phase is a separate commit / PR.
- Phase 1 revert: delete the bridge directory.
- Phase 2 revert: revert the worker patch; the bridge stays unused until phase 3 is re-attempted.
- Phase 3 revert: handled by the two-PR split (bee+tokenstat, and the rename sweep).

## 11. Open questions / explicit defaults pending review

- **Engine-name constants in bridge** — defaulted to "yes, re-export `EngineClaude/Codex/Pi/Kimi`" so business code only needs to update import paths. Boss to confirm or override.
- **Package path** — defaulted to `internal/ai/bridge` (subdirectory of `internal/ai`) rather than `internal/aibridge` (top-level peer). Subdirectory placement matches "bridge is the front for the AI subsystem" semantics and makes the eventual lint rule easier to express.
- **Test files counted in migration scope** — defaulted to "yes; tests are part of business surface and must not import `internal/ai`".
- **`internal/ai` internal-only marker** — defaulted to a `doc.go` comment in phase 3; a `depguard` rule is optional follow-up.
