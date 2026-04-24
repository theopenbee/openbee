# Engine Extra Args Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow operators to configure extra CLI arguments (e.g. `--model`, `--effort`) for each AI engine at global, bee, and per-worker scopes, with smart merge (later scope overrides same-named keys from earlier scope).

**Architecture:** A new `EngineExtraArgsMap` type (`map[string]map[string]string`, engine→arg→value) is introduced in `internal/ai/extra_args.go`. System-level config is stored in two new `bee_system_configs` keys; workers get a new `engine_extra_args` DB column. At run time, `worker.Manager` and `bee.BeeProcess` load and merge the args, then pass the resolved `map[string]string` through `RunOptions.ExtraArgs` (renamed from the env-injection field) down to each engine invoker.

**Tech Stack:** Go, SQLite migrations, Cobra CLI, React/TypeScript, i18next

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/ai/extra_args.go` | **Create** | `EngineExtraArgsMap` type, `ParseEngineExtraArgs`, `MergeEngineExtraArgs`, `BuildExtraArgSlice` |
| `internal/ai/engine.go` | **Modify** | Add `ExtraArgs map[string]string` field to `RunOptions` |
| `internal/ai/claude/invoker.go` | **Modify** | Append `opts.ExtraArgs` to CLI args before `--print` |
| `internal/ai/codex/invoker.go` | **Modify** | Append `opts.ExtraArgs` to `buildArgs` result |
| `internal/ai/pi/invoker.go` | **Modify** | Append `opts.ExtraArgs` to CLI args |
| `internal/ai/kimi/invoker.go` | **Modify** | Append `opts.ExtraArgs` to `buildArgs` result |
| `internal/infra/model/system_config.go` | **Modify** | Add two new key constants |
| `internal/infra/model/worker.go` | **Modify** | Add `EngineExtraArgs string` field |
| `internal/infra/store/db.go` | **Modify** | Add migration 41: `ALTER TABLE bee_workers ADD COLUMN engine_extra_args` |
| `internal/infra/store/worker_store.go` | **Modify** | Include `engine_extra_args` in all column lists, scan, insert, update |
| `internal/domain/worker/manager.go` | **Modify** | Add `sysConfigStore` dep; load global extra args; merge with worker args; pass in `RunOptions` |
| `internal/domain/bee/bee_process.go` | **Modify** | Add `sysConfigStore` dep; load global + bee extra args; merge; pass in `RunOptions` |
| `internal/api/system_config_handler.go` | **Modify** | Accept the two new keys in `Get` and `Set` |
| `internal/api/worker_handler.go` | **Modify** | Accept `engine_extra_args` in create/update requests |
| `internal/app/app.go` | **Modify** | Wire `sysConfigStore` into `BeeProcess` and `worker.Manager` |
| `cmd/openbee/ctl_worker.go` | **Modify** | Add `--engine-extra-args` flag to `create` and `update` subcommands |
| `web/src/lib/types.ts` | **Modify** | Add `engine_extra_args` to `Worker` interface |
| `web/src/lib/api.ts` | **Modify** | Include `engine_extra_args` in create/update payloads |
| `web/src/components/create-worker-sheet.tsx` | **Modify** | Add engine extra args inputs in optional settings |
| `web/src/components/edit-worker-info-sheet.tsx` | **Modify** | Add engine extra args inputs |

---

## Task 1: Core types and utilities

**Files:**
- Create: `internal/ai/extra_args.go`
- Create: `internal/ai/extra_args_test.go`

- [ ] **Step 1.1: Write failing tests**

```go
// internal/ai/extra_args_test.go
package ai_test

import (
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestParseEngineExtraArgs(t *testing.T) {
	raw := map[string]string{
		"claude": "--model claude-sonnet-4-5 --effort high",
		"codex":  "--model o3",
	}
	got, err := ai.ParseEngineExtraArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["claude"]["model"] != "claude-sonnet-4-5" {
		t.Errorf("claude model: got %q", got["claude"]["model"])
	}
	if got["claude"]["effort"] != "high" {
		t.Errorf("claude effort: got %q", got["claude"]["effort"])
	}
	if got["codex"]["model"] != "o3" {
		t.Errorf("codex model: got %q", got["codex"]["model"])
	}
}

func TestParseEngineExtraArgs_BooleanFlag(t *testing.T) {
	raw := map[string]string{"claude": "--verbose"}
	got, err := ai.ParseEngineExtraArgs(raw)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := got["claude"]["verbose"]
	if !ok {
		t.Fatal("verbose key missing")
	}
	if v != "" {
		t.Errorf("verbose value should be empty, got %q", v)
	}
}

func TestMergeEngineExtraArgs(t *testing.T) {
	base := ai.EngineExtraArgsMap{
		"claude": {"model": "sonnet", "effort": "high"},
	}
	override := ai.EngineExtraArgsMap{
		"claude": {"model": "opus"},
		"codex":  {"model": "o3"},
	}
	got := ai.MergeEngineExtraArgs(base, override)
	if got["claude"]["model"] != "opus" {
		t.Errorf("expected opus, got %q", got["claude"]["model"])
	}
	if got["claude"]["effort"] != "high" {
		t.Errorf("effort should be inherited: got %q", got["claude"]["effort"])
	}
	if got["codex"]["model"] != "o3" {
		t.Errorf("codex model: got %q", got["codex"]["model"])
	}
}

func TestBuildExtraArgSlice(t *testing.T) {
	args := map[string]string{"model": "claude-sonnet-4-5", "verbose": ""}
	slice := ai.BuildExtraArgSlice(args)
	// order is non-deterministic; check presence
	found := map[string]bool{}
	for i := 0; i < len(slice); i++ {
		found[slice[i]] = true
	}
	if !found["--model"] {
		t.Error("missing --model")
	}
	if !found["claude-sonnet-4-5"] {
		t.Error("missing model value")
	}
	if !found["--verbose"] {
		t.Error("missing --verbose")
	}
}
```

- [ ] **Step 1.2: Run tests to confirm they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/ai/... -run TestParseEngineExtraArgs -v 2>&1 | head -20
```

Expected: compile error (type not found)

- [ ] **Step 1.3: Implement `internal/ai/extra_args.go`**

```go
package ai

import "strings"

// EngineExtraArgsMap maps engine name -> (arg key -> arg value).
// Boolean flags use an empty string as value.
type EngineExtraArgsMap map[string]map[string]string

// ParseEngineExtraArgs parses raw CLI strings per engine into a structured map.
// Each value is a whitespace-separated sequence of --key [value] tokens.
func ParseEngineExtraArgs(raw map[string]string) (EngineExtraArgsMap, error) {
	result := make(EngineExtraArgsMap, len(raw))
	for engine, s := range raw {
		result[engine] = parseArgString(s)
	}
	return result, nil
}

// parseArgString converts "--key value --flag" into {"key":"value","flag":""}.
func parseArgString(s string) map[string]string {
	tokens := strings.Fields(s)
	m := make(map[string]string)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if !strings.HasPrefix(tok, "--") {
			continue
		}
		key := strings.TrimPrefix(tok, "--")
		if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "--") {
			m[key] = tokens[i+1]
			i++
		} else {
			m[key] = ""
		}
	}
	return m
}

// MergeEngineExtraArgs merges two maps; override wins on conflicting keys.
// Engines present only in base or only in override are preserved as-is.
func MergeEngineExtraArgs(base, override EngineExtraArgsMap) EngineExtraArgsMap {
	result := make(EngineExtraArgsMap, len(base))
	for engine, args := range base {
		cp := make(map[string]string, len(args))
		for k, v := range args {
			cp[k] = v
		}
		result[engine] = cp
	}
	for engine, overrideArgs := range override {
		if result[engine] == nil {
			result[engine] = make(map[string]string, len(overrideArgs))
		}
		for k, v := range overrideArgs {
			result[engine][k] = v
		}
	}
	return result
}

// BuildExtraArgSlice converts a single engine's arg map into a CLI arg slice.
func BuildExtraArgSlice(args map[string]string) []string {
	out := make([]string, 0, len(args)*2)
	for k, v := range args {
		out = append(out, "--"+k)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
```

- [ ] **Step 1.4: Run tests to confirm they pass**

```bash
go test ./internal/ai/... -run "TestParseEngineExtraArgs|TestMergeEngineExtraArgs|TestBuildExtraArgSlice" -v
```

Expected: all PASS

- [ ] **Step 1.5: Commit**

```bash
git add internal/ai/extra_args.go internal/ai/extra_args_test.go
git commit -m "feat(ai): add EngineExtraArgsMap type with parse/merge/build utilities"
```

---

## Task 2: Add `ExtraArgs` to `RunOptions`

**Files:**
- Modify: `internal/ai/engine.go:27-32`

- [ ] **Step 2.1: Add `ExtraArgs` field to `RunOptions`**

In `internal/ai/engine.go`, update `RunOptions`:

```go
// RunOptions controls session behaviour for an engine invocation.
type RunOptions struct {
	SessionID string
	Resume    bool
	APIKey    string
	ExtraEnv  []string            // additional KEY=VALUE env vars to inject
	ExtraArgs map[string]string   // additional CLI flags to pass (arg -> value; "" value = boolean flag)
}
```

- [ ] **Step 2.2: Build to confirm no compile errors**

```bash
go build ./...
```

Expected: success (ExtraArgs is additive; existing code passes nil implicitly)

- [ ] **Step 2.3: Commit**

```bash
git add internal/ai/engine.go
git commit -m "feat(ai): add ExtraArgs field to RunOptions"
```

---

## Task 3: Wire `ExtraArgs` into engine invokers

**Files:**
- Modify: `internal/ai/claude/invoker.go:85-98`
- Modify: `internal/ai/codex/invoker.go:49-58`
- Modify: `internal/ai/pi/invoker.go` (args construction)
- Modify: `internal/ai/kimi/invoker.go:28-35`

- [ ] **Step 3.1: Claude invoker — append extra args before `--print`**

In `internal/ai/claude/invoker.go`, update the `Run` method args block:

```go
// Replace the args assembly block (lines ~86-99):
args := []string{
    "--dangerously-skip-permissions",
    "--verbose",
    "--output-format", "stream-json",
}
if opts.SessionID != "" {
    if opts.Resume {
        args = append(args, "--resume", opts.SessionID)
    } else {
        args = append(args, "--session-id", opts.SessionID)
    }
}
args = append(args, ai.BuildExtraArgSlice(opts.ExtraArgs)...)
args = append(args, "--print")
```

- [ ] **Step 3.2: Codex invoker — append extra args**

In `internal/ai/codex/invoker.go`, update `buildArgs`:

```go
func buildArgs(threadID string, resume bool, prompt string, extraArgs map[string]string) []string {
	var base []string
	if resume && threadID != "" {
		base = []string{"exec", "resume", threadID, "--json", "--dangerously-bypass-approvals-and-sandbox"}
		if prompt != "" {
			base = append(base, prompt)
		}
	} else {
		base = []string{"exec", "-", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	}
	return append(base, ai.BuildExtraArgSlice(extraArgs)...)
}
```

Update the `Run` method call site:

```go
args := buildArgs(threadID, resume, prompt, opts.ExtraArgs)
```

- [ ] **Step 3.3: Pi invoker — append extra args**

Find the args construction in `internal/ai/pi/invoker.go` (the `buildArgs` or inline args slice). Append `ai.BuildExtraArgSlice(opts.ExtraArgs)...` to the args slice before the command is started. The exact location depends on the current code — look for the `exec.CommandContext` call and the args assembled before it.

- [ ] **Step 3.4: Kimi invoker — append extra args**

In `internal/ai/kimi/invoker.go`, update `buildArgs`:

```go
func buildArgs(sessionID string, extraArgs map[string]string) []string {
	base := []string{
		"--session=" + sessionID,
		"--yolo",
		"--output-format=stream-json",
		"--print",
	}
	return append(base, ai.BuildExtraArgSlice(extraArgs)...)
}
```

Update the `Run` method call site to pass `opts.ExtraArgs`.

- [ ] **Step 3.5: Build**

```bash
go build ./...
```

Expected: success

- [ ] **Step 3.6: Commit**

```bash
git add internal/ai/claude/invoker.go internal/ai/codex/invoker.go internal/ai/pi/invoker.go internal/ai/kimi/invoker.go
git commit -m "feat(ai): pass RunOptions.ExtraArgs as CLI flags in all engine invokers"
```

---

## Task 4: DB migration and model update

**Files:**
- Modify: `internal/infra/store/db.go` (add migration 41)
- Modify: `internal/infra/model/worker.go`
- Modify: `internal/infra/model/system_config.go`

- [ ] **Step 4.1: Add migration 41 to `db.go`**

In `internal/infra/store/db.go`, append to the `migrations` slice after the last entry (version 40):

```go
{
    version: 41,
    name:    "add_engine_extra_args_to_workers",
    sql:     `ALTER TABLE bee_workers ADD COLUMN engine_extra_args TEXT NOT NULL DEFAULT '{}'`,
},
```

- [ ] **Step 4.2: Update `Worker` model**

In `internal/infra/model/worker.go`, add the new field:

```go
type Worker struct {
    ID               string       `json:"id" db:"id"`
    Name             string       `json:"name" db:"name"`
    Description      string       `json:"description" db:"description"`
    Constraints      string       `json:"constraints" db:"constraints"`
    WorkDir          string       `json:"work_dir" db:"work_dir"`
    Engine           string       `json:"engine" db:"engine"`
    EngineExtraArgs  string       `json:"engine_extra_args" db:"engine_extra_args"`
    Status           WorkerStatus `json:"status" db:"status"`
    PermissionScopes string       `json:"permission_scopes" db:"permission_scopes"`
    CreatedAt        int64        `json:"created_at" db:"created_at"`
    UpdatedAt        int64        `json:"updated_at" db:"updated_at"`
}
```

- [ ] **Step 4.3: Add system config key constants**

In `internal/infra/model/system_config.go`, add:

```go
const (
    SystemConfigKeyDefaultEngine        = "default_engine"
    SystemConfigKeyEngineExtraArgsGlobal = "engine_extra_args_global"
    SystemConfigKeyEngineExtraArgsBee    = "engine_extra_args_bee"
)
```

- [ ] **Step 4.4: Build**

```bash
go build ./...
```

Expected: success (new field is additive)

- [ ] **Step 4.5: Commit**

```bash
git add internal/infra/store/db.go internal/infra/model/worker.go internal/infra/model/system_config.go
git commit -m "feat(db): add engine_extra_args column to bee_workers (migration 41)"
```

---

## Task 5: Update `WorkerStore` to include `engine_extra_args`

**Files:**
- Modify: `internal/infra/store/worker_store.go`

- [ ] **Step 5.1: Update column constants and scan function**

In `internal/infra/store/worker_store.go`:

```go
const (
    workerColumns        = `id, name, description, constraints, work_dir, engine, engine_extra_args, status, permission_scopes, created_at, updated_at`
    workerColumnsAliased = `w.id, w.name, w.description, w.constraints, w.work_dir, w.engine, w.engine_extra_args, w.status, w.permission_scopes, w.created_at, w.updated_at`
)

func scanWorker(scanner interface{ Scan(...any) error }) (model.Worker, error) {
    var w model.Worker
    err := scanner.Scan(
        &w.ID, &w.Name, &w.Description, &w.Constraints,
        &w.WorkDir, &w.Engine, &w.EngineExtraArgs, &w.Status, &w.PermissionScopes, &w.CreatedAt, &w.UpdatedAt,
    )
    if err != nil {
        return model.Worker{}, err
    }
    return w, nil
}
```

- [ ] **Step 5.2: Update `Create` to include `engine_extra_args`**

```go
func (s *WorkerStore) Create(w model.Worker) (model.Worker, error) {
    if w.ID == "" {
        w.ID = uuid.New().String()
    }
    if w.EngineExtraArgs == "" {
        w.EngineExtraArgs = "{}"
    }
    w.Status = model.WorkerStatusIdle
    w.CreatedAt = time.Now().UnixMilli()
    w.UpdatedAt = w.CreatedAt

    _, err := s.db.Exec(
        `INSERT INTO bee_workers (id, name, description, constraints, work_dir, engine, engine_extra_args, status, permission_scopes, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        w.ID, w.Name, w.Description, w.Constraints, w.WorkDir, w.Engine, w.EngineExtraArgs,
        w.Status, w.PermissionScopes, w.CreatedAt, w.UpdatedAt,
    )
    if err != nil {
        return model.Worker{}, fmt.Errorf("insert worker: %w", err)
    }
    return w, nil
}
```

- [ ] **Step 5.3: Update `Update` to include `engine_extra_args`**

```go
func (s *WorkerStore) Update(w model.Worker) (model.Worker, error) {
    w.UpdatedAt = time.Now().UnixMilli()
    _, err := s.db.Exec(
        `UPDATE bee_workers SET name=?, description=?, constraints=?, work_dir=?, engine=?, engine_extra_args=?, status=?, permission_scopes=?, updated_at=?
         WHERE id=?`,
        w.Name, w.Description, w.Constraints, w.WorkDir, w.Engine, w.EngineExtraArgs,
        w.Status, w.PermissionScopes, w.UpdatedAt, w.ID,
    )
    if err != nil {
        return model.Worker{}, fmt.Errorf("update worker: %w", err)
    }
    return w, nil
}
```

- [ ] **Step 5.4: Build and run store tests**

```bash
go build ./...
go test ./internal/infra/store/... -v -run TestWorker
```

Expected: all pass (existing tests do not check EngineExtraArgs, but should still pass)

- [ ] **Step 5.5: Commit**

```bash
git add internal/infra/store/worker_store.go
git commit -m "feat(store): include engine_extra_args in WorkerStore queries"
```

---

## Task 6: Wire extra args into `worker.Manager`

**Files:**
- Modify: `internal/domain/worker/manager.go`

- [ ] **Step 6.1: Add `sysConfigStore` interface and field**

At the top of `internal/domain/worker/manager.go`, add interface and field:

```go
// systemConfigReader is satisfied by *store.SystemConfigStore.
type systemConfigReader interface {
    Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
}
```

Add `sysConfigStore systemConfigReader` to the `Manager` struct, and update `NewManager`:

```go
func NewManager(
    workerBaseDir string,
    bc config.BeeConfig,
    ws *store.WorkerStore,
    es *store.ExecutionStore,
    engines map[string]ai.EngineAdapter,
    engineCfg *enginecfg.Store,
    envService *env.Service,
    sysConfigStore systemConfigReader,
) *Manager {
    // ... existing body ...
    return &Manager{
        // ... existing fields ...
        sysConfigStore: sysConfigStore,
    }
}
```

- [ ] **Step 6.2: Add helper to load and parse global extra args**

Add method to `Manager`:

```go
func (m *Manager) loadGlobalExtraArgs(ctx context.Context) ai.EngineExtraArgsMap {
    cfg, found, err := m.sysConfigStore.Get(ctx, model.SystemConfigKeyEngineExtraArgsGlobal)
    if err != nil || !found || cfg.Value == "" || cfg.Value == "{}" {
        return nil
    }
    var raw map[string]string
    if json.Unmarshal([]byte(cfg.Value), &raw) != nil {
        return nil
    }
    parsed, err := ai.ParseEngineExtraArgs(raw)
    if err != nil {
        return nil
    }
    return parsed
}
```

Add `"context"` and `"encoding/json"` to imports if not already present.

- [ ] **Step 6.3: Resolve worker's extra args and merge in `launchRuntime`**

Add a helper to resolve the effective extra args for the worker's engine:

```go
func (m *Manager) resolveExtraArgs(ctx context.Context, worker model.Worker, engineName string) map[string]string {
    globalMap := m.loadGlobalExtraArgs(ctx)

    var workerMap ai.EngineExtraArgsMap
    if worker.EngineExtraArgs != "" && worker.EngineExtraArgs != "{}" {
        var raw map[string]string
        if json.Unmarshal([]byte(worker.EngineExtraArgs), &raw) == nil {
            workerMap, _ = ai.ParseEngineExtraArgs(raw)
        }
    }

    merged := ai.MergeEngineExtraArgs(globalMap, workerMap)
    return merged[engineName]
}
```

Update `launchRuntime` to determine the engine name and pass extra args:

```go
func (m *Manager) launchRuntime(ctx context.Context, exec model.WorkerExecution, worker model.Worker, engineName string, engine ai.EngineAdapter, timeout time.Duration, prompt string, resume bool) error {
    // ... existing code for logPath, token, execCtx, extraEnv ...

    extraArgs := m.resolveExtraArgs(ctx, worker, engineName)

    runRes, err := engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
        SessionID: exec.SessionID,
        Resume:    resume,
        APIKey:    token,
        ExtraEnv:  extraEnv,
        ExtraArgs: extraArgs,
    }, logPath)
    // ... rest unchanged ...
}
```

Update the `ExecuteWorker` call to `launchRuntime` to pass `ctx`, `engineName`:

```go
// In ExecuteWorker, after resolving engine:
engineName := worker.Engine
if engineName == "" {
    engineName = m.engineCfg.Get()
}
// ...
if err := m.launchRuntime(ctx, exec, worker, engineName, engine, timeout, triggerInput, resume); err != nil {
```

- [ ] **Step 6.4: Build**

```bash
go build ./...
```

Expected: compile errors about `NewManager` call sites — fix in `internal/app/app.go` in Task 8.

- [ ] **Step 6.5: Commit (after Task 8 build passes)**

---

## Task 7: Wire extra args into `bee.BeeProcess`

**Files:**
- Modify: `internal/domain/bee/bee_process.go`

- [ ] **Step 7.1: Add `sysConfigStore` interface and fields to `BeeProcess`**

```go
// systemConfigReader is satisfied by *store.SystemConfigStore.
type systemConfigReader interface {
    Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
}
```

Add to `BeeProcess`:

```go
type BeeProcess struct {
    engine         ai.EngineAdapter
    tokenSecret    string
    tokenTTL       time.Duration
    envService     *env.Service
    sysConfigStore systemConfigReader
    engineName     string
}

func NewBeeProcess(cfg config.BeeConfig, engineName string, engine ai.EngineAdapter, envSvc *env.Service, sysStore systemConfigReader) *BeeProcess {
    return &BeeProcess{
        engine:         engine,
        tokenSecret:    cfg.MCP.TokenSecret,
        tokenTTL:       cfg.MCP.TokenTTL,
        envService:     envSvc,
        sysConfigStore: sysStore,
        engineName:     engineName,
    }
}
```

- [ ] **Step 7.2: Load and merge global + bee extra args in `Run`**

```go
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.RunResult, error) {
    token, err := auth.GenerateBeeToken(p.tokenSecret, p.tokenTTL)
    if err != nil {
        return ai.RunResult{}, fmt.Errorf("generate bee token: %w", err)
    }

    extraEnv, err := p.envService.ResolveBeeEnv(defaultBeeID)
    if err != nil {
        return ai.RunResult{}, fmt.Errorf("resolve bee env: %w", err)
    }
    opts.ExtraEnv = extraEnv
    opts.APIKey = token
    opts.ExtraArgs = p.resolveExtraArgs(ctx)
    return p.engine.Run(ctx, workDir, prompt, opts, logPath)
}

func (p *BeeProcess) resolveExtraArgs(ctx context.Context) map[string]string {
    globalMap := p.loadExtraArgs(ctx, model.SystemConfigKeyEngineExtraArgsGlobal)
    beeMap := p.loadExtraArgs(ctx, model.SystemConfigKeyEngineExtraArgsBee)
    merged := ai.MergeEngineExtraArgs(globalMap, beeMap)
    return merged[p.engineName]
}

func (p *BeeProcess) loadExtraArgs(ctx context.Context, key string) ai.EngineExtraArgsMap {
    cfg, found, err := p.sysConfigStore.Get(ctx, key)
    if err != nil || !found || cfg.Value == "" || cfg.Value == "{}" {
        return nil
    }
    var raw map[string]string
    if json.Unmarshal([]byte(cfg.Value), &raw) != nil {
        return nil
    }
    parsed, _ := ai.ParseEngineExtraArgs(raw)
    return parsed
}
```

Add imports: `"context"`, `"encoding/json"`, `"github.com/theopenbee/openbee/internal/infra/model"`.

- [ ] **Step 7.3: Build (compile errors expected until Task 8)**

```bash
go build ./... 2>&1 | grep -v "^#"
```

---

## Task 8: Wire new dependencies in `app.go`

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 8.1: Pass `sysConfigStore` to `worker.NewManager`**

Find the `buildWorkerManager` or equivalent call in `app.go`. Add `sysConfigStore` as the last argument:

```go
workerMgr := worker.NewManager(
    workerBaseDir,
    cfg.Bee,
    workerStore,
    executionStore,
    engines,
    engineCfg,
    envService,
    sysConfigStore,   // new
)
```

- [ ] **Step 8.2: Pass `engineName` and `sysConfigStore` to `bee.NewBeeProcess`**

Find where `bee.NewBeeProcess` is called. The current signature is `NewBeeProcess(cfg, engine, envSvc)`. The engine is obtained from the dynamic adapter. Determine the engine name:

```go
beeEngineName := engineCfg.Get()  // current default engine
beeProcess := bee.NewBeeProcess(cfg.Bee, beeEngineName, dynamicEngineAdapter, envService, sysConfigStore)
```

Note: `BeeProcess.engineName` needs to stay in sync when the default engine changes. See step 8.3.

- [ ] **Step 8.3: Keep bee engine name in sync**

The bee uses `ai.NewDynamicAdapter` which switches engine at runtime. To keep `engineName` in sync, expose a setter on `BeeProcess` or pass a function:

Add to `BeeProcess`:

```go
func (p *BeeProcess) SetEngineName(name string) {
    p.engineName = name
}
```

In `app.go`, when the default engine changes (in the `engineCfg` change path), call `beeProcess.SetEngineName(newEngine)`. Look for where `engineCfg.Set` is called (in `system_config_handler.go`) and thread this through. The simplest approach: store the engine name as a pointer or have `BeeProcess` read from `engineCfg` directly:

Alternative (simpler): Instead of storing `engineName` as a string, pass `engineCfg *enginecfg.Store` to `BeeProcess`:

```go
type BeeProcess struct {
    engine         ai.EngineAdapter
    tokenSecret    string
    tokenTTL       time.Duration
    envService     *env.Service
    sysConfigStore systemConfigReader
    engineCfg      *enginecfg.Store
}

func NewBeeProcess(cfg config.BeeConfig, engine ai.EngineAdapter, envSvc *env.Service, sysStore systemConfigReader, engineCfg *enginecfg.Store) *BeeProcess {
    return &BeeProcess{
        engine:         engine,
        tokenSecret:    cfg.MCP.TokenSecret,
        tokenTTL:       cfg.MCP.TokenTTL,
        envService:     envSvc,
        sysConfigStore: sysStore,
        engineCfg:      engineCfg,
    }
}
```

Then in `resolveExtraArgs`, use `p.engineCfg.Get()` instead of `p.engineName`.

Update `app.go`:

```go
beeProcess := bee.NewBeeProcess(cfg.Bee, dynamicEngineAdapter, envService, sysConfigStore, engineCfg)
```

- [ ] **Step 8.4: Build**

```bash
go build ./...
```

Expected: success

- [ ] **Step 8.5: Commit tasks 6, 7, 8 together**

```bash
git add internal/domain/worker/manager.go internal/domain/bee/bee_process.go internal/app/app.go
git commit -m "feat: load and merge engine extra args at runtime for bee and workers"
```

---

## Task 9: API — system config handler for new keys

**Files:**
- Modify: `internal/api/system_config_handler.go`

- [ ] **Step 9.1: Expand `Get` to return all three config keys**

```go
func (h *SystemConfigHandler) Get(c *gin.Context) {
    ctx := c.Request.Context()
    keys := []string{
        model.SystemConfigKeyDefaultEngine,
        model.SystemConfigKeyEngineExtraArgsGlobal,
        model.SystemConfigKeyEngineExtraArgsBee,
    }
    result := make(map[string]string, len(keys))
    for _, key := range keys {
        cfg, found, err := h.store.Get(ctx, key)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        if found {
            result[key] = cfg.Value
        } else {
            result[key] = ""
        }
    }
    c.JSON(http.StatusOK, result)
}
```

- [ ] **Step 9.2: Expand `Set` to allow the two new keys**

```go
func (h *SystemConfigHandler) Set(c *gin.Context) {
    key := c.Param("key")
    var req setSystemConfigRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    switch key {
    case model.SystemConfigKeyDefaultEngine:
        if err := h.validator.ValidateEngine(req.Value); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }
        if err := h.store.Set(c.Request.Context(), key, req.Value); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        h.engineCfg.Set(req.Value)

    case model.SystemConfigKeyEngineExtraArgsGlobal, model.SystemConfigKeyEngineExtraArgsBee:
        // value is a JSON object mapping engine -> raw CLI string; validate it parses
        if req.Value != "" && req.Value != "{}" {
            var raw map[string]string
            if err := json.Unmarshal([]byte(req.Value), &raw); err != nil {
                c.JSON(http.StatusBadRequest, gin.H{"error": "value must be a JSON object mapping engine to CLI args string"})
                return
            }
            if _, err := ai.ParseEngineExtraArgs(raw); err != nil {
                c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
                return
            }
        }
        if err := h.store.Set(c.Request.Context(), key, req.Value); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

    default:
        c.JSON(http.StatusBadRequest, gin.H{"error": "unknown config key"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"ok": true})
}
```

Add imports `"encoding/json"` and `ai "github.com/theopenbee/openbee/internal/ai"`.

- [ ] **Step 9.3: Build**

```bash
go build ./...
```

Expected: success

- [ ] **Step 9.4: Commit**

```bash
git add internal/api/system_config_handler.go
git commit -m "feat(api): support engine_extra_args_global and engine_extra_args_bee system config keys"
```

---

## Task 10: API — worker handler for `engine_extra_args`

**Files:**
- Modify: `internal/api/worker_handler.go`
- Modify: `internal/domain/worker/manager.go`

- [ ] **Step 10.1: Add `EngineExtraArgs` to `createWorkerRequest` and `CreateWorkerParams`**

In `internal/api/worker_handler.go`:

```go
type createWorkerRequest struct {
    Name             string            `json:"name" binding:"required"`
    Engine           string            `json:"engine"`
    Description      string            `json:"description"`
    Constraints      string            `json:"constraints"`
    WorkDir          string            `json:"work_dir"`
    PermissionScopes string            `json:"permission_scopes"`
    EngineExtraArgs  map[string]string `json:"engine_extra_args"` // engine -> raw CLI string
}
```

In `Create` handler, marshal and pass through:

```go
var engineExtraArgsJSON string
if len(req.EngineExtraArgs) > 0 {
    b, _ := json.Marshal(req.EngineExtraArgs)
    engineExtraArgsJSON = string(b)
} else {
    engineExtraArgsJSON = "{}"
}

w, err := h.manager.CreateWorker(worker.CreateWorkerParams{
    Name:             req.Name,
    Engine:           req.Engine,
    Description:      req.Description,
    Constraints:      req.Constraints,
    WorkDir:          req.WorkDir,
    PermissionScopes: req.PermissionScopes,
    EngineExtraArgs:  engineExtraArgsJSON,
})
```

Add `"encoding/json"` to imports.

- [ ] **Step 10.2: Update `CreateWorkerParams` and `CreateWorker`**

In `internal/domain/worker/manager.go`:

```go
type CreateWorkerParams struct {
    Name             string
    Description      string
    Constraints      string
    WorkDir          string
    PermissionScopes string
    Engine           string
    EngineExtraArgs  string // JSON: map[engine]rawCLIString
}
```

In `CreateWorker`, set `EngineExtraArgs`:

```go
workerModel := model.Worker{
    ID:               id,
    Name:             p.Name,
    Description:      p.Description,
    Constraints:      p.Constraints,
    WorkDir:          p.WorkDir,
    Engine:           p.Engine,
    EngineExtraArgs:  p.EngineExtraArgs,
    PermissionScopes: p.PermissionScopes,
}
```

- [ ] **Step 10.3: Add `EngineExtraArgs` to `UpdateWorkerParams`**

In `manager.go`:

```go
type UpdateWorkerParams struct {
    Name             *string            `json:"name"`
    Description      *string            `json:"description"`
    Constraints      *string            `json:"constraints"`
    PermissionScopes *string            `json:"permission_scopes"`
    Engine           *string            `json:"engine"`
    EngineExtraArgs  map[string]string  `json:"engine_extra_args"` // engine -> raw CLI string; nil = no change
}
```

Update `HasChanges`:

```go
func (p UpdateWorkerParams) HasChanges() bool {
    return p.Name != nil || p.Description != nil || p.Constraints != nil ||
        p.PermissionScopes != nil || p.Engine != nil || p.EngineExtraArgs != nil
}
```

Update `ApplyTo` to merge (patch-style — only the engines explicitly listed are updated):

```go
func (p UpdateWorkerParams) ApplyTo(w *model.Worker) {
    // ... existing fields ...
    if p.EngineExtraArgs != nil {
        // Load existing map
        existing := make(map[string]string)
        if w.EngineExtraArgs != "" && w.EngineExtraArgs != "{}" {
            json.Unmarshal([]byte(w.EngineExtraArgs), &existing)
        }
        // Patch: apply provided engines; empty value clears that engine
        for engine, args := range p.EngineExtraArgs {
            if args == "" {
                delete(existing, engine)
            } else {
                existing[engine] = args
            }
        }
        b, _ := json.Marshal(existing)
        w.EngineExtraArgs = string(b)
    }
}
```

Add `"encoding/json"` import to `manager.go`.

- [ ] **Step 10.4: Build**

```bash
go build ./...
```

Expected: success

- [ ] **Step 10.5: Commit**

```bash
git add internal/api/worker_handler.go internal/domain/worker/manager.go
git commit -m "feat(api): accept engine_extra_args in worker create and update endpoints"
```

---

## Task 11: CLI — `--engine-extra-args` flag

**Files:**
- Modify: `cmd/openbee/ctl_worker.go`

- [ ] **Step 11.1: Add `--engine-extra-args` to `create`**

Add variable and flag:

```go
var workerCreateEngineExtraArgs []string  // each entry: "engine=--flag value"
```

In `init()`:

```go
ctlWorkerCreateCmd.Flags().StringArrayVar(&workerCreateEngineExtraArgs, "engine-extra-args", nil,
    `Extra CLI args per engine, e.g. "claude=--model claude-sonnet-4-5 --effort high" (repeatable)`)
```

In `ctlWorkerCreateCmd.RunE`, parse and add to payload:

```go
if len(workerCreateEngineExtraArgs) > 0 {
    parsed := parseEngineExtraArgsFlag(workerCreateEngineExtraArgs)
    a["engine_extra_args"] = parsed
}
```

- [ ] **Step 11.2: Add `--engine-extra-args` to `update`**

```go
var workerUpdateEngineExtraArgs []string
```

In `init()`:

```go
ctlWorkerUpdateCmd.Flags().StringArrayVar(&workerUpdateEngineExtraArgs, "engine-extra-args", nil,
    `Extra CLI args per engine, e.g. "claude=--model claude-opus-4-7" (repeatable); pass "claude=" to clear`)
```

In `ctlWorkerUpdateCmd.RunE`:

```go
if cmd.Flags().Changed("engine-extra-args") {
    a["engine_extra_args"] = parseEngineExtraArgsFlag(workerUpdateEngineExtraArgs)
}
```

- [ ] **Step 11.3: Add `parseEngineExtraArgsFlag` helper**

```go
// parseEngineExtraArgsFlag converts ["claude=--model sonnet", "codex=--model o3"]
// into map[string]string{"claude": "--model sonnet", "codex": "--model o3"}.
func parseEngineExtraArgsFlag(entries []string) map[string]string {
    result := make(map[string]string, len(entries))
    for _, entry := range entries {
        idx := strings.Index(entry, "=")
        if idx < 0 {
            continue
        }
        engine := entry[:idx]
        args := entry[idx+1:]
        result[engine] = args
    }
    return result
}
```

Add `"strings"` to imports if not already present.

- [ ] **Step 11.4: Build**

```bash
go build ./cmd/openbee/...
```

Expected: success

- [ ] **Step 11.5: Smoke-test CLI (manual)**

```bash
./openbee ctl worker create --help 2>&1 | grep engine-extra-args
./openbee ctl worker update --help 2>&1 | grep engine-extra-args
```

Expected: flag listed in both help outputs

- [ ] **Step 11.6: Commit**

```bash
git add cmd/openbee/ctl_worker.go
git commit -m "feat(cli): add --engine-extra-args flag to worker create and update"
```

---

## Task 12: Web — types and API client

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`

- [ ] **Step 12.1: Add `engine_extra_args` to `Worker` interface**

In `web/src/lib/types.ts`, update `Worker`:

```ts
export interface Worker {
  id: string
  name: string
  description: string
  constraints: string
  work_dir: string
  engine: Engine
  engine_extra_args?: Record<string, string>  // engine -> raw CLI string
  permission_scopes?: string
  status: WorkerStatus
  departments?: DepartmentBrief[]
  created_at: number
  updated_at: number
}
```

Also add the two new system config key constants:

```ts
export const SYSTEM_CONFIG_KEY_ENGINE_EXTRA_ARGS_GLOBAL = "engine_extra_args_global"
export const SYSTEM_CONFIG_KEY_ENGINE_EXTRA_ARGS_BEE    = "engine_extra_args_bee"
```

- [ ] **Step 12.2: Update `api.workers.create` to include `engine_extra_args`**

In `web/src/lib/api.ts`, update the `create` method signature:

```ts
create: (data: {
  name: string
  engine: Engine
  description: string
  constraints?: string
  work_dir?: string
  permission_scopes?: string
  engine_extra_args?: Record<string, string>
}) => fetchAPI<Worker>("/workers", { method: "POST", body: JSON.stringify(data) }),
```

The `update` method already accepts `Partial<Worker>`, so it will pick up `engine_extra_args` automatically.

- [ ] **Step 12.3: TypeScript compile check**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run build 2>&1 | tail -20
```

Expected: no new type errors

- [ ] **Step 12.4: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts
git commit -m "feat(web): add engine_extra_args to Worker type and API client"
```

---

## Task 13: Web — Create Worker Sheet

**Files:**
- Modify: `web/src/components/create-worker-sheet.tsx`

- [ ] **Step 13.1: Add `engineExtraArgs` state and initialize from `initialValues`**

Inside `CreateWorkerSheet`, add state:

```ts
const [engineExtraArgs, setEngineExtraArgs] = useState<Record<string, string>>({})
```

In the `useEffect` that runs when `open` changes, reset it:

```ts
setEngineExtraArgs(iv?.engine_extra_args ?? {})
```

Also add `engine_extra_args` to `WorkerInitialValues` and `workerToInitialValues`:

```ts
export interface WorkerInitialValues {
  // ... existing fields ...
  engine_extra_args: Record<string, string>
}

export function workerToInitialValues(worker: Worker): WorkerInitialValues {
  return {
    // ... existing fields ...
    engine_extra_args: worker.engine_extra_args ?? {},
  }
}
```

- [ ] **Step 13.2: Include `engine_extra_args` in `handleSubmit`**

```ts
const worker = await createWorker.mutateAsync({
  name: name.trim(),
  engine,
  description,
  constraints: constraints || undefined,
  work_dir: workDir || undefined,
  permission_scopes: serializeScopes(selectedScopes) || undefined,
  engine_extra_args: Object.keys(engineExtraArgs).length > 0 ? engineExtraArgs : undefined,
})
```

- [ ] **Step 13.3: Add engine extra args UI inside the optional settings section**

Inside the optional settings collapsible `<div className="px-6 pb-5 space-y-5">`, after the constraints field, add:

```tsx
{enabledEngines.length > 0 && (
  <div className="space-y-2">
    <Label>{t("workers.form.engineExtraArgs")}</Label>
    <div className="space-y-2">
      {enabledEngines.map((eng) => (
        <div key={eng} className="space-y-1">
          <span className="text-xs font-medium text-muted-foreground capitalize">{eng}</span>
          <Input
            value={engineExtraArgs[eng] ?? ""}
            onChange={(e) => setEngineExtraArgs((prev) => ({
              ...prev,
              [eng]: e.target.value,
            }))}
            placeholder={t("workers.form.engineExtraArgsPlaceholder")}
            className="font-mono text-xs"
          />
        </div>
      ))}
    </div>
    <p className="text-xs text-muted-foreground">{t("workers.form.engineExtraArgsHelper")}</p>
  </div>
)}
```

- [ ] **Step 13.4: Add i18n translation keys**

Find the i18n JSON files (typically `web/src/i18n/en.json` and `web/src/i18n/zh.json`). Add:

```json
"engineExtraArgs": "Engine Extra Args",
"engineExtraArgsPlaceholder": "--model claude-sonnet-4-5 --effort high",
"engineExtraArgsHelper": "Extra CLI flags passed to the engine for this worker."
```

(Add Chinese translations in zh.json as appropriate.)

- [ ] **Step 13.5: TypeScript compile check**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run build 2>&1 | tail -20
```

Expected: success

- [ ] **Step 13.6: Commit**

```bash
git add web/src/components/create-worker-sheet.tsx web/src/i18n/
git commit -m "feat(web): add engine extra args inputs to create worker sheet"
```

---

## Task 14: Web — Edit Worker Info Sheet

**Files:**
- Modify: `web/src/components/edit-worker-info-sheet.tsx`

- [ ] **Step 14.1: Add `engineExtraArgs` state**

```ts
const [engineExtraArgs, setEngineExtraArgs] = useState<Record<string, string>>({})
```

In `useEffect`:

```ts
setEngineExtraArgs(worker.engine_extra_args ?? {})
```

- [ ] **Step 14.2: Include in change detection and `handleSubmit`**

```ts
const workerChanged =
  description !== (worker.description ?? "") ||
  engine !== pickDefaultEngine(worker.engine, enabledEngines) ||
  JSON.stringify(engineExtraArgs) !== JSON.stringify(worker.engine_extra_args ?? {})
```

In the `updateWorker.mutateAsync` call:

```ts
ops.push(updateWorker.mutateAsync({
  id: worker.id,
  data: { description, engine, engine_extra_args: engineExtraArgs },
}))
```

- [ ] **Step 14.3: Add engine extra args UI**

After the engine select section, add the same per-engine inputs as in the create sheet:

```tsx
{enabledEngines.length > 0 && (
  <div className="space-y-2">
    <Label>{t("workers.form.engineExtraArgs")}</Label>
    <div className="space-y-2">
      {enabledEngines.map((eng) => (
        <div key={eng} className="space-y-1">
          <span className="text-xs font-medium text-muted-foreground capitalize">{eng}</span>
          <Input
            value={engineExtraArgs[eng] ?? ""}
            onChange={(e) => setEngineExtraArgs((prev) => ({
              ...prev,
              [eng]: e.target.value,
            }))}
            placeholder={t("workers.form.engineExtraArgsPlaceholder")}
            className="font-mono text-xs"
          />
        </div>
      ))}
    </div>
    <p className="text-xs text-muted-foreground">{t("workers.form.engineExtraArgsHelper")}</p>
  </div>
)}
```

- [ ] **Step 14.4: TypeScript compile and build**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run build 2>&1 | tail -20
```

Expected: success

- [ ] **Step 14.5: Commit**

```bash
git add web/src/components/edit-worker-info-sheet.tsx
git commit -m "feat(web): add engine extra args inputs to edit worker info sheet"
```

---

## Task 15: Final integration check

- [ ] **Step 15.1: Full Go build and tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./...
go test ./internal/ai/... ./internal/infra/store/... ./internal/domain/... -v 2>&1 | tail -40
```

Expected: all pass

- [ ] **Step 15.2: Web build**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run build 2>&1 | tail -20
```

Expected: success

- [ ] **Step 15.3: Final commit if any loose ends**

```bash
git status
```

If any unstaged changes remain, stage and commit them with an appropriate message.
