# AI Bridge Design

Date: 2026-05-14

## Background

OpenBee currently lets business modules call `internal/ai` directly. The coupling is broad:

- `internal/domain/bee` depends on `ai.EngineAdapter`, lifecycle output, prompt prefix helpers, and run options.
- `internal/domain/worker` owns engine selection, workspace preparation, process handles, run options, engine args, and output handling.
- `internal/domain/task` builds worker session prefixes through `internal/ai`.
- `internal/tokenstat` holds engine adapters to collect token usage.
- `internal/app` creates engine adapters and passes them into multiple business modules.
- command/config/store code uses engine constants and parsing helpers from `internal/ai`.

As features grow, this shape makes AI engine details leak into business code. The bridge introduces one business-facing boundary between OpenBee domain logic and the AI engine implementation layer.

## Decisions

- Use one `bridge.Bridge` interface as the business-facing entry point.
- Put the bridge in `internal/bridge`.
- Business modules depend on bridge DTOs and the bridge interface, not on `internal/ai` types.
- Keep existing engine adapters under `internal/ai` for now. `internal/ai` becomes a downstream implementation dependency of bridge.
- Migrate all current non-engine direct imports of `internal/ai` as part of the first implementation phase.
- Treat "no business package imports `internal/ai`" as an acceptance criterion.

## Goals

- Make bridge the only business-side entry point for AI engine capabilities.
- Hide `ai.EngineAdapter`, `ai.RunOptions`, `ai.Output`, `ai.Process`, and similar implementation contracts from business code.
- Keep bee, worker, task, token usage, engine selection, engine validation, prompt prefix construction, and engine args behavior compatible with today.
- Allow future engine capability changes to be added behind bridge instead of scattered through business packages.

## Non-Goals

- Do not rewrite Claude/Codex/Pi/Kimi engine adapters.
- Do not rename or move `internal/ai` in this phase.
- Do not change task queueing, message processing, worker status, execution persistence, or platform behavior except where required to replace direct AI dependencies.
- Do not add new AI engine features as part of this refactor.

## Architecture

The target dependency direction is:

```text
business modules -> internal/bridge -> internal/ai -> engine-specific adapters
```

`internal/bridge` owns the translation between business requests and engine implementation requests. Business modules express intent such as running bee, running a worker, preparing a workspace, resolving engines, building session prefixes, and collecting token usage. Bridge translates those requests into the existing `internal/ai` adapter calls.

Allowed direct dependencies on `internal/ai` after migration:

- `internal/bridge`
- `internal/ai`
- engine implementation packages under `internal/ai/**`
- tests that are explicitly testing `internal/ai` or bridge internals

Business modules such as `bee`, `worker`, `task`, `command`, `tokenstat`, `api`, `infra/config`, `infra/store`, and `app` should not import `internal/ai`.

## Bridge Interface

Bridge exposes one coarse-grained interface:

```go
type Bridge interface {
	EnabledEngines() []string
	ValidateEngine(name string) error
	ResolveEngine(workerEngine string) (string, error)

	BuildBeeSessionPrefix() string
	BuildWorkerSessionPrefix(persona WorkerPersona) string

	PrepareBeeWorkspace(workDir string) error
	PrepareWorkerWorkspace(workDir string, engineName string) error

	RunBee(ctx context.Context, req BeeRunRequest) (RunHandle, error)
	RunWorker(ctx context.Context, req WorkerRunRequest) (RunHandle, error)

	CollectTokenUsage(ctx context.Context, sessionID, engineName string) ([]TokenUsage, error)
}
```

The interface is intentionally broad but still bounded. It contains AI engine boundary capabilities only. It must not absorb business workflows such as task scheduling, message status changes, worker status updates, execution persistence, platform reply delivery, or command parsing.

Bridge methods must keep business-level names. They should not expose lower-level engine adapter names or mirror `internal/ai` types directly.

## Bridge DTOs

Bridge defines its own request, result, lifecycle, process, persona, and token usage types. These types may map one-to-one to existing `internal/ai` fields internally, but business code must not see the `internal/ai` versions.

Core DTO shape:

```go
type BeeRunRequest struct {
	WorkDir   string
	Prompt    string
	SessionID string
	Resume    bool
	LogPath   string
}

type WorkerRunRequest struct {
	WorkerID         string
	WorkDir          string
	PermissionScopes []string
	WorkerEngine     string
	WorkerEngineArgs string
	Prompt           string
	SessionID        string
	Resume           bool
	LogPath          string
}

type RunHandle struct {
	Engine        string
	Process       ProcessHandle
	Events        <-chan LifecycleEvent
	ExtractResult func(logPath string) string
}

type ProcessHandle interface {
	PID() int
	Stop() error
}

type LifecycleEvent struct {
	Type    LifecycleEventType
	Content string
}
```

`RunHandle.Engine` records the actual engine used for the run. Business code can continue storing engine names in execution records and session contexts without knowing which adapter handled the run.

## Data Flow

Bee run flow:

1. `bee.Feeder` asks bridge to prepare the bee workspace.
2. `bee.Feeder` builds the bee prompt using bridge's session prefix method.
3. `bee.Feeder` creates the execution and log path as it does today.
4. `bee.Feeder` calls `Bridge.RunBee`.
5. Bridge resolves the current default engine, injects the bee token, resolves bee env, merges global and bee engine args, calls the selected engine adapter, and maps lifecycle events.
6. `bee.Feeder` consumes bridge lifecycle events and stores execution result/status as it does today.

Worker run flow:

1. `worker.Manager` validates and resolves worker engine choices through bridge.
2. `worker.Manager` asks bridge to prepare worker workspaces.
3. `worker.Manager` creates execution records and log paths as it does today.
4. `worker.Manager` calls `Bridge.RunWorker`.
5. Bridge resolves the worker engine with default fallback, injects the worker token, resolves worker env, merges global and worker engine args, calls the selected engine adapter, and maps lifecycle events.
6. `worker.Manager` tracks the returned process handle and consumes bridge lifecycle events.

Task flow:

1. `task.Dispatcher` uses bridge to build worker session prefixes.
2. Session resume, task queueing, failure notifications, and context persistence stay in `task`.

Token usage flow:

1. `tokenstat.Syncer` collects pending session IDs from the database.
2. `tokenstat.Syncer` calls `Bridge.CollectTokenUsage`.
3. Bridge dispatches to the known engine adapter or performs the legacy fallback chain internally.
4. `tokenstat.Syncer` continues to write usage rows and tombstones.

## Error Handling

- Bridge returns business-readable errors such as unknown engine, engine unavailable, prepare failed, run failed, and collect usage failed.
- Bridge may wrap lower-level errors for context, but business packages should not need to compare against `internal/ai` error types.
- Unknown worker engine keeps today's fallback behavior: fall back to the default engine and log the mismatch. If the default is unavailable, return a clear error.
- `RunBee` and `RunWorker` expose the actual engine in `RunHandle.Engine` so execution records and session contexts remain accurate.
- Lifecycle events keep the current terminal model: done and error. Future streaming or progress events should be added to bridge's lifecycle model rather than exposing engine output directly.

## Migration Plan

1. Add `internal/bridge` with the `Bridge` interface, DTOs, and a service implementation that wraps current `internal/ai` adapters.
2. Move engine catalog behavior behind bridge: enabled engines, validation, default resolution, and engine constants needed by business modules.
3. Move prompt prefix construction behind bridge.
4. Move bee run and prepare calls from `internal/domain/bee` to bridge.
5. Move worker run, prepare, engine args, and process handle usage from `internal/domain/worker` to bridge.
6. Move token usage dispatch from `internal/tokenstat` to bridge.
7. Update app wiring so `BuildApp` creates bridge once and passes it to business modules.
8. Remove remaining direct imports of `internal/ai` from non-engine business and infrastructure packages.
9. Add a boundary test or script check to prevent future business imports of `internal/ai`.

## Testing

Bridge tests should cover:

- enabled engine ordering and validation
- worker engine resolution and fallback to default
- bee workspace preparation
- worker workspace preparation
- bee run request conversion, token injection, env injection, extra args injection, lifecycle mapping, and actual engine name
- worker run request conversion, token injection, permission scope handling, env injection, worker/global extra args merge, lifecycle mapping, and actual engine name
- token usage collection for known engines and legacy fallback behavior

Business package tests should be updated to fake `bridge.Bridge` instead of faking `ai.EngineAdapter`. Existing behavior coverage should remain for:

- bee session creation and resume
- worker session creation and resume
- engine switching
- worker-specific engine selection
- execution status updates
- process cancellation
- failure notifications
- token usage tombstones and sync rows

Boundary verification should assert that non-engine packages do not import `github.com/theopenbee/openbee/internal/ai`.

## Acceptance Criteria

- All non-engine business and infrastructure packages use `bridge.Bridge` for AI engine capabilities.
- No business package exposes or depends on `ai.EngineAdapter`, `ai.RunOptions`, `ai.Output`, `ai.Process`, or `ai.TokenUsage`.
- Existing bee and worker execution behavior remains compatible.
- Existing session context behavior remains compatible, including engine-scoped session IDs.
- Existing engine switching and worker-specific engine selection behavior remains compatible.
- Existing token usage sync behavior remains compatible, including legacy fallback and tombstones.
- A boundary check prevents reintroducing direct `internal/ai` imports outside bridge and engine implementation packages.

## OpenBee Team Decision

The approved design is a single `internal/bridge.Bridge` interface with independent bridge DTOs and an implementation that wraps the current `internal/ai` layer. The bridge is the mandatory boundary between business modules and AI engine implementations.
