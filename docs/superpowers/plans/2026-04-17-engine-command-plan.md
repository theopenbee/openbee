# `/engine` Command & Default Engine Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/engine` slash command that switches the default bee engine or a specific worker's engine at runtime, intercepted in the ingest layer before messages reach the AI.

**Architecture:** A new `bee_system_configs` table stores the persistent default engine. An `enginecfg` package holds a process-wide RWMutex-protected cache that is initialized from DB at startup and updated on `/engine` commands. The `msgingest.Gateway` detects command messages before DB write and delegates them to an `EngineCommandHandler`; a `DynamicAdapter` wraps all engine adapters and routes bee runs through the cache on each invocation.

**Tech Stack:** Go standard library (`sync`, `database/sql`), SQLite via existing store patterns, existing `platform.PlatformSenderAdapter` for replies.

---

## File Map

| Path | Action | Responsibility |
|------|--------|----------------|
| `internal/infra/store/db.go` | Modify | Add Migration v37 creating `bee_system_configs` |
| `internal/infra/model/system_config.go` | Create | `SystemConfig` struct + `SystemConfigKeyDefaultEngine` constant |
| `internal/infra/store/system_config_store.go` | Create | `Get` / `Set` for `bee_system_configs` |
| `internal/infra/store/system_config_store_test.go` | Create | Store integration tests |
| `internal/domain/enginecfg/default.go` | Create | `Init` / `Get` / `Set` — global RWMutex-protected engine name |
| `internal/domain/enginecfg/default_test.go` | Create | Concurrent access test |
| `internal/ai/dynamic.go` | Create | `DynamicAdapter` wrapping all engines, routes via `enginecfg.Get()` |
| `internal/ai/dynamic_test.go` | Create | DynamicAdapter routing test |
| `internal/domain/msgingest/command.go` | Create | `CommandHandler` interface |
| `internal/domain/command/engine.go` | Create | `EngineCommandHandler` — parses & executes `/engine` |
| `internal/domain/command/engine_test.go` | Create | Command parsing + execution tests |
| `internal/domain/msgingest/gateway.go` | Modify | Add `commandHandler` field + `WithCommandHandler` option; intercept in `onDebounce` |
| `internal/domain/msgingest/gateway_test.go` | Modify | Test that command messages skip DB write |
| `internal/domain/bee/feeder.go` | Modify | Replace 5× `f.cfg.EffectiveEngine()` → `enginecfg.Get()` |
| `internal/domain/task/dispatcher.go` | Modify | Replace `d.engineName` with `enginecfg.Get()` in `resolveWorkerEngine` + `ClearSession`; remove `engineName` field + `WithEngine` option |
| `internal/app/app.go` | Modify | Add `systemConfigStore` to `appStores`; init `enginecfg` from DB; build `DynamicAdapter`; inject `CommandHandler` into both gateways |

---

### Task 1: DB Migration + `SystemConfig` Model + `SystemConfigStore`

**Files:**
- Modify: `internal/infra/store/db.go`
- Create: `internal/infra/model/system_config.go`
- Create: `internal/infra/store/system_config_store.go`
- Create: `internal/infra/store/system_config_store_test.go`

- [ ] **Step 1: Add migration v37 to `db.go`**

Find the closing `}` of the migrations slice (after the `version: 36` block) and append:

```go
	{
		version: 37,
		name:    "create_bee_system_configs_table",
		sql: `
        CREATE TABLE IF NOT EXISTS bee_system_configs (
            key        TEXT PRIMARY KEY,
            value      TEXT NOT NULL,
            updated_at INTEGER NOT NULL
        );
    `,
	},
```

- [ ] **Step 2: Create the model**

Create `internal/infra/model/system_config.go`:

```go
package model

// SystemConfig is a global key/value system setting.
type SystemConfig struct {
	Key       string `db:"key"`
	Value     string `db:"value"`
	UpdatedAt int64  `db:"updated_at"`
}

// SystemConfigKeyDefaultEngine is the key for the default bee engine.
const SystemConfigKeyDefaultEngine = "default_engine"
```

- [ ] **Step 3: Write the failing store test**

Create `internal/infra/store/system_config_store_test.go`:

```go
package store

import (
	"context"
	"testing"
)

func setupSystemConfigDB(t *testing.T) *SystemConfigStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewSystemConfigStore(db)
}

func TestSystemConfigStore_GetMissing(t *testing.T) {
	s := setupSystemConfigDB(t)
	_, found, err := s.Get(context.Background(), "missing_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Error("expected found=false for missing key")
	}
}

func TestSystemConfigStore_SetAndGet(t *testing.T) {
	s := setupSystemConfigDB(t)
	ctx := context.Background()

	if err := s.Set(ctx, "default_engine", "claude"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	cfg, found, err := s.Get(ctx, "default_engine")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true after Set")
	}
	if cfg.Value != "claude" {
		t.Errorf("expected claude, got %s", cfg.Value)
	}
}

func TestSystemConfigStore_SetOverwrites(t *testing.T) {
	s := setupSystemConfigDB(t)
	ctx := context.Background()

	_ = s.Set(ctx, "default_engine", "claude")
	_ = s.Set(ctx, "default_engine", "codex")

	cfg, _, _ := s.Get(ctx, "default_engine")
	if cfg.Value != "codex" {
		t.Errorf("expected codex after overwrite, got %s", cfg.Value)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/infra/store/... -run TestSystemConfigStore -v
```

Expected: compile error — `SystemConfigStore` undefined.

- [ ] **Step 5: Create `SystemConfigStore`**

Create `internal/infra/store/system_config_store.go`:

```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
)

// SystemConfigStore reads and writes global system configuration.
type SystemConfigStore struct {
	db *sql.DB
}

// NewSystemConfigStore constructs a SystemConfigStore.
func NewSystemConfigStore(db *sql.DB) *SystemConfigStore {
	return &SystemConfigStore{db: db}
}

// Get retrieves a system config by key. Returns (config, true, nil) if found,
// (zero, false, nil) if not found, or (zero, false, err) on error.
func (s *SystemConfigStore) Get(ctx context.Context, key string) (model.SystemConfig, bool, error) {
	var cfg model.SystemConfig
	err := s.db.QueryRowContext(ctx,
		`SELECT key, value, updated_at FROM bee_system_configs WHERE key = ?`, key,
	).Scan(&cfg.Key, &cfg.Value, &cfg.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.SystemConfig{}, false, nil
	}
	if err != nil {
		return model.SystemConfig{}, false, fmt.Errorf("get system config %q: %w", key, err)
	}
	return cfg, true, nil
}

// Set upserts a system config entry.
func (s *SystemConfigStore) Set(ctx context.Context, key, value string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bee_system_configs (key, value, updated_at) VALUES (?, ?, ?)
         ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, now,
	)
	if err != nil {
		return fmt.Errorf("set system config %q: %w", key, err)
	}
	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/infra/store/... -run TestSystemConfigStore -v
```

Expected: all three tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/infra/store/db.go \
        internal/infra/model/system_config.go \
        internal/infra/store/system_config_store.go \
        internal/infra/store/system_config_store_test.go
git commit -m "feat: add bee_system_configs table, model, and store"
```

---

### Task 2: `enginecfg` Package — Global Engine Cache

**Files:**
- Create: `internal/domain/enginecfg/default.go`
- Create: `internal/domain/enginecfg/default_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/domain/enginecfg/default_test.go`:

```go
package enginecfg_test

import (
	"sync"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

func TestInit(t *testing.T) {
	enginecfg.Init("claude")
	if got := enginecfg.Get(); got != "claude" {
		t.Errorf("Init: expected claude, got %s", got)
	}
}

func TestSet(t *testing.T) {
	enginecfg.Init("claude")
	enginecfg.Set("codex")
	if got := enginecfg.Get(); got != "codex" {
		t.Errorf("Set: expected codex, got %s", got)
	}
}

func TestConcurrentAccess(t *testing.T) {
	enginecfg.Init("claude")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); enginecfg.Set("codex") }()
		go func() { defer wg.Done(); _ = enginecfg.Get() }()
	}
	wg.Wait()
	// No race condition — test passes if race detector doesn't fire.
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/domain/enginecfg/... -v -race
```

Expected: compile error — package `enginecfg` not found.

- [ ] **Step 3: Create the package**

Create `internal/domain/enginecfg/default.go`:

```go
// Package enginecfg holds the process-wide default engine name.
// It is initialized once at startup and updated when the /engine command fires.
package enginecfg

import "sync"

var (
	mu  sync.RWMutex
	val string
)

// Init sets the initial engine name. Call once at app startup.
func Init(engine string) {
	mu.Lock()
	defer mu.Unlock()
	val = engine
}

// Get returns the current default engine name.
func Get() string {
	mu.RLock()
	defer mu.RUnlock()
	return val
}

// Set updates the default engine name. Safe for concurrent use.
func Set(engine string) {
	mu.Lock()
	defer mu.Unlock()
	val = engine
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/domain/enginecfg/... -v -race
```

Expected: all three tests PASS with no race conditions.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/enginecfg/default.go internal/domain/enginecfg/default_test.go
git commit -m "feat: add enginecfg package for global engine cache"
```

---

### Task 3: `ai.DynamicAdapter` — Dynamic Engine Routing

**Files:**
- Create: `internal/ai/dynamic.go`
- Create: `internal/ai/dynamic_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ai/dynamic_test.go`:

```go
package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// stubEngine is a minimal EngineAdapter for testing.
type stubEngine struct {
	name    string
	prepared []string // workDirs seen
}

func (s *stubEngine) Prepare(workDir string, _ ai.PrepareOptions) error {
	s.prepared = append(s.prepared, workDir)
	return nil
}
func (s *stubEngine) Run(_ context.Context, _, _, _ string, _ ai.RunOptions, _ string) (ai.Process, <-chan ai.Output, error) {
	return nil, nil, errors.New(s.name + " run called")
}
func (s *stubEngine) ExtractResult(_ string) string { return s.name + "-result" }

func TestDynamicAdapter_PrepareCallsAll(t *testing.T) {
	a := &stubEngine{name: "a"}
	b := &stubEngine{name: "b"}
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": a, "b": b})
	if err := d.Prepare("/work", ai.PrepareOptions{}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(a.prepared) != 1 || len(b.prepared) != 1 {
		t.Errorf("expected each engine prepared once; a=%d b=%d", len(a.prepared), len(b.prepared))
	}
}

func TestDynamicAdapter_RunRoutesToCurrentEngine(t *testing.T) {
	enginecfg.Init("a")
	a := &stubEngine{name: "a"}
	b := &stubEngine{name: "b"}
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": a, "b": b})

	_, _, err := d.Run(context.Background(), "/w", "prompt", ai.RunOptions{}, "/log")
	if err == nil || err.Error() != "a run called" {
		t.Errorf("expected 'a run called', got %v", err)
	}

	enginecfg.Set("b")
	_, _, err = d.Run(context.Background(), "/w", "prompt", ai.RunOptions{}, "/log")
	if err == nil || err.Error() != "b run called" {
		t.Errorf("expected 'b run called', got %v", err)
	}
}

func TestDynamicAdapter_ExtractResultRoutesToCurrentEngine(t *testing.T) {
	enginecfg.Init("a")
	a := &stubEngine{name: "a"}
	b := &stubEngine{name: "b"}
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": a, "b": b})

	if got := d.ExtractResult("/log"); got != "a-result" {
		t.Errorf("expected a-result, got %s", got)
	}

	enginecfg.Set("b")
	if got := d.ExtractResult("/log"); got != "b-result" {
		t.Errorf("expected b-result, got %s", got)
	}
}

func TestDynamicAdapter_RunUnknownEngine(t *testing.T) {
	enginecfg.Init("missing")
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": &stubEngine{name: "a"}})
	_, _, err := d.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	if err == nil {
		t.Error("expected error for unknown engine")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ai/... -run TestDynamic -v
```

Expected: compile error — `ai.NewDynamicAdapter` undefined.

- [ ] **Step 3: Create `DynamicAdapter`**

Create `internal/ai/dynamic.go`:

```go
package ai

import (
	"context"
	"fmt"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// DynamicAdapter wraps multiple EngineAdapters and routes each Run/ExtractResult
// call to whichever engine enginecfg.Get() returns at call time.
type DynamicAdapter struct {
	engines map[string]EngineAdapter
}

// NewDynamicAdapter constructs a DynamicAdapter from a map of engine adapters.
func NewDynamicAdapter(engines map[string]EngineAdapter) *DynamicAdapter {
	return &DynamicAdapter{engines: engines}
}

// Prepare calls Prepare on every available engine adapter.
func (d *DynamicAdapter) Prepare(workDir string, opts PrepareOptions) error {
	for name, e := range d.engines {
		if err := e.Prepare(workDir, opts); err != nil {
			return fmt.Errorf("prepare engine %q: %w", name, err)
		}
	}
	return nil
}

// Run executes using the engine currently selected in enginecfg.
func (d *DynamicAdapter) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (Process, <-chan Output, error) {
	name := enginecfg.Get()
	e, ok := d.engines[name]
	if !ok {
		return nil, nil, fmt.Errorf("engine %q not available", name)
	}
	return e.Run(ctx, workDir, prompt, opts, logPath)
}

// ExtractResult extracts the result using the engine currently selected in enginecfg.
func (d *DynamicAdapter) ExtractResult(logPath string) string {
	name := enginecfg.Get()
	e, ok := d.engines[name]
	if !ok {
		return ""
	}
	return e.ExtractResult(logPath)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ai/... -run TestDynamic -v
```

Expected: all four tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/dynamic.go internal/ai/dynamic_test.go
git commit -m "feat: add ai.DynamicAdapter for runtime engine switching"
```

---

### Task 4: `CommandHandler` Interface + `EngineCommandHandler`

**Files:**
- Create: `internal/domain/msgingest/command.go`
- Create: `internal/domain/command/engine.go`
- Create: `internal/domain/command/engine_test.go`

- [ ] **Step 1: Create the `CommandHandler` interface**

Create `internal/domain/msgingest/command.go`:

```go
package msgingest

import (
	"context"

	"github.com/theopenbee/openbee/internal/platform"
)

// CommandHandler processes slash commands extracted from inbound messages.
// HandleCommand returns true if the message was a recognized command and
// was handled (the caller should skip normal message processing).
type CommandHandler interface {
	HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool
}
```

- [ ] **Step 2: Write the failing test for `EngineCommandHandler`**

Create `internal/domain/command/engine_test.go`:

```go
package command_test

import (
	"context"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// --- fakes ---

type fakeWorkerRepo struct {
	workers map[string]model.Worker // name → worker
	updated []model.Worker
}

func (f *fakeWorkerRepo) GetByName(name string) (model.Worker, error) {
	w, ok := f.workers[name]
	if !ok {
		return model.Worker{}, fmt.Errorf("worker not found")
	}
	return w, nil
}
func (f *fakeWorkerRepo) Update(w model.Worker) (model.Worker, error) {
	f.updated = append(f.updated, w)
	return w, nil
}

type fakeSysConfig struct {
	vals map[string]string
}

func (f *fakeSysConfig) Set(_ context.Context, key, value string) error {
	f.vals[key] = value
	return nil
}

type fakeSender struct {
	sent []string
}

func (f *fakeSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	f.sent = append(f.sent, msg.Content)
	return nil
}

func makeReplyTo() platform.InboundMessage {
	return platform.InboundMessage{
		Platform:   "feishu",
		SessionKey: "feishu:chat1:user1",
	}
}

func makeHandler(workers map[string]model.Worker) (*command.EngineCommandHandler, *fakeSender, *fakeSysConfig) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	h := command.NewEngineCommandHandler(repo, cfg, senders)
	return h, sender, cfg
}

// --- tests ---

func TestEngineCommand_NotACommand(t *testing.T) {
	h, sender, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "hello world", makeReplyTo())
	if handled {
		t.Error("should not handle non-command")
	}
	if len(sender.sent) != 0 {
		t.Error("should not send reply for non-command")
	}
}

func TestEngineCommand_SwitchBeeEngine(t *testing.T) {
	enginecfg.Init("claude")
	h, sender, cfg := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if enginecfg.Get() != "codex" {
		t.Errorf("expected enginecfg=codex, got %s", enginecfg.Get())
	}
	if cfg.vals["default_engine"] != "codex" {
		t.Errorf("expected DB updated to codex, got %s", cfg.vals["default_engine"])
	}
	if len(sender.sent) != 1 {
		t.Fatal("expected one reply")
	}
	if sender.sent[0] != "已将默认 engine 切换为 codex" {
		t.Errorf("unexpected reply: %s", sender.sent[0])
	}
}

func TestEngineCommand_SwitchWorkerEngine(t *testing.T) {
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	h, sender, _ := makeHandler(workers)
	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 || sender.sent[0] != `已将 Worker "alice" 的 engine 切换为 codex` {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_InvalidEngine(t *testing.T) {
	h, sender, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "/engine xyz", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 {
		t.Fatal("expected one reply")
	}
	want := "未知的 engine: xyz，支持的 engine：claude / codex / pi / kimi"
	if sender.sent[0] != want {
		t.Errorf("unexpected reply:\ngot  %s\nwant %s", sender.sent[0], want)
	}
}

func TestEngineCommand_WorkerNotFound(t *testing.T) {
	h, sender, _ := makeHandler(map[string]model.Worker{})
	handled := h.HandleCommand(context.Background(), "/engine claude nobody", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := `Worker "nobody" 不存在`
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_NoArgs(t *testing.T) {
	h, sender, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "/engine", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定 worker 的 engine"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}
```

- [ ] **Step 3: Add missing import in test file**

Add `"fmt"` to the imports in `engine_test.go` (the `fakeWorkerRepo.GetByName` method uses `fmt.Errorf`):

```go
import (
	"context"
	"fmt"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)
```

- [ ] **Step 4: Run test to verify it fails**

```bash
go test ./internal/domain/command/... -v
```

Expected: compile error — package `command` not found.

- [ ] **Step 5: Create `EngineCommandHandler`**

Create `internal/domain/command/engine.go`:

```go
package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// WorkerRepository is the subset of WorkerStore needed by EngineCommandHandler.
type WorkerRepository interface {
	GetByName(name string) (model.Worker, error)
	Update(w model.Worker) (model.Worker, error)
}

// SystemConfigWriter is the subset of SystemConfigStore needed by EngineCommandHandler.
type SystemConfigWriter interface {
	Set(ctx context.Context, key, value string) error
}

// EngineCommandHandler handles the /engine slash command.
type EngineCommandHandler struct {
	workers  WorkerRepository
	sysCfg   SystemConfigWriter
	senders  map[string]platform.PlatformSenderAdapter
}

// NewEngineCommandHandler constructs an EngineCommandHandler.
func NewEngineCommandHandler(
	workers WorkerRepository,
	sysCfg SystemConfigWriter,
	senders map[string]platform.PlatformSenderAdapter,
) *EngineCommandHandler {
	return &EngineCommandHandler{workers: workers, sysCfg: sysCfg, senders: senders}
}

const usageMsg = "用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定 worker 的 engine"

// HandleCommand implements msgingest.CommandHandler.
// Returns true if content is a /engine command (whether or not it succeeded).
func (h *EngineCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != "/engine" {
		return false
	}

	switch len(fields) {
	case 1:
		h.reply(ctx, replyTo, usageMsg)
	case 2:
		h.handleBeeEngine(ctx, replyTo, fields[1])
	case 3:
		h.handleWorkerEngine(ctx, replyTo, fields[1], fields[2])
	default:
		h.reply(ctx, replyTo, usageMsg)
	}
	return true
}

func (h *EngineCommandHandler) handleBeeEngine(ctx context.Context, replyTo platform.InboundMessage, engineName string) {
	if err := ai.ValidateEngine(engineName); err != nil {
		h.reply(ctx, replyTo, fmt.Sprintf("未知的 engine: %s，支持的 engine：%s",
			engineName, strings.Join(ai.AllEngines, " / ")))
		return
	}
	if err := h.sysCfg.Set(ctx, model.SystemConfigKeyDefaultEngine, engineName); err != nil {
		h.reply(ctx, replyTo, "切换失败，请稍后重试")
		return
	}
	enginecfg.Set(engineName)
	h.reply(ctx, replyTo, fmt.Sprintf("已将默认 engine 切换为 %s", engineName))
}

func (h *EngineCommandHandler) handleWorkerEngine(ctx context.Context, replyTo platform.InboundMessage, engineName, workerName string) {
	if err := ai.ValidateEngine(engineName); err != nil {
		h.reply(ctx, replyTo, fmt.Sprintf("未知的 engine: %s，支持的 engine：%s",
			engineName, strings.Join(ai.AllEngines, " / ")))
		return
	}
	w, err := h.workers.GetByName(workerName)
	if err != nil {
		h.reply(ctx, replyTo, fmt.Sprintf("Worker %q 不存在", workerName))
		return
	}
	w.Engine = engineName
	if _, err := h.workers.Update(w); err != nil {
		h.reply(ctx, replyTo, "切换失败，请稍后重试")
		return
	}
	h.reply(ctx, replyTo, fmt.Sprintf("已将 Worker %q 的 engine 切换为 %s", workerName, engineName))
}

func (h *EngineCommandHandler) reply(ctx context.Context, replyTo platform.InboundMessage, text string) {
	sender, ok := h.senders[replyTo.Platform]
	if !ok {
		return
	}
	_ = sender.Send(ctx, platform.OutboundMessage{
		Content:    text,
		ReplyTo:    replyTo,
		SourceType: "system",
	})
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/domain/command/... -v
```

Expected: all six tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/msgingest/command.go \
        internal/domain/command/engine.go \
        internal/domain/command/engine_test.go
git commit -m "feat: add CommandHandler interface and EngineCommandHandler"
```

---

### Task 5: Gateway Command Interception

**Files:**
- Modify: `internal/domain/msgingest/gateway.go`
- Modify: `internal/domain/msgingest/gateway_test.go`

- [ ] **Step 1: Write failing test for command interception**

Add this test to `internal/domain/msgingest/gateway_test.go` (after existing tests):

```go
// mockCommandHandler records whether HandleCommand was called and what to return.
type mockCommandHandler struct {
	handled  bool
	contents []string
}

func (m *mockCommandHandler) HandleCommand(_ context.Context, content string, _ platform.InboundMessage) bool {
	m.contents = append(m.contents, content)
	return m.handled
}

func TestGateway_CommandHandlerInterceptsBeforeDB(t *testing.T) {
	store := newMock()
	handler := &mockCommandHandler{handled: true}
	g := msgingest.New(store, 0, msgingest.WithCommandHandler(handler))

	g.Dispatch(platform.InboundMessage{
		Platform:   "feishu",
		SessionKey: "feishu:c1:u1",
		Content:    "/engine claude",
	})

	// Wait for debounce to fire (debounce=0 but runs in goroutine).
	time.Sleep(20 * time.Millisecond)

	if len(store.batches) != 0 {
		t.Errorf("expected 0 DB writes when command handled, got %d", len(store.batches))
	}
	if len(handler.contents) != 1 || handler.contents[0] != "/engine claude" {
		t.Errorf("expected handler called with '/engine claude', got %v", handler.contents)
	}
}

func TestGateway_CommandHandlerPassesThroughNonCommands(t *testing.T) {
	store := newMock()
	handler := &mockCommandHandler{handled: false}
	g := msgingest.New(store, 0, msgingest.WithCommandHandler(handler))

	g.Dispatch(platform.InboundMessage{
		Platform:   "feishu",
		SessionKey: "feishu:c1:u1",
		Content:    "hello",
	})
	time.Sleep(20 * time.Millisecond)

	if len(store.batches) != 1 {
		t.Errorf("expected 1 DB write for non-command, got %d", len(store.batches))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/domain/msgingest/... -run TestGateway_Command -v
```

Expected: compile error — `msgingest.WithCommandHandler` undefined.

- [ ] **Step 3: Add `commandHandler` field and `WithCommandHandler` option to Gateway**

In `internal/domain/msgingest/gateway.go`, modify the `Gateway` struct and `New` function:

```go
// Gateway receives raw platform messages, deduplicates, debounces, and emits IngestedMessages.
type Gateway struct {
	msgStore       MessageStore
	debounce       time.Duration
	sessions       map[string]*debounceState
	seen           map[string]struct{}
	mu             sync.Mutex
	out            chan IngestedMessage
	commandHandler CommandHandler // optional; intercepts slash commands before DB write
}

// Option configures a Gateway.
type Option func(*Gateway)

// WithCommandHandler sets an optional slash-command handler.
// When set, each debounced message is offered to the handler before DB write.
// If the handler returns true, the message is consumed and not stored.
func WithCommandHandler(h CommandHandler) Option {
	return func(g *Gateway) { g.commandHandler = h }
}

// New constructs a Gateway.
func New(msgStore MessageStore, debounce time.Duration, opts ...Option) *Gateway {
	g := &Gateway{
		msgStore: msgStore,
		debounce: debounce,
		sessions: make(map[string]*debounceState),
		seen:     make(map[string]struct{}),
		out:      make(chan IngestedMessage, 64),
	}
	for _, o := range opts {
		o(g)
	}
	return g
}
```

- [ ] **Step 4: Add command interception in `onDebounce`**

In `onDebounce`, add the check **before** the `g.msgStore.CreateBatch` call. The existing code in `onDebounce` after the mutex unlock looks like:

```go
// ADD THIS BLOCK before "inserted, err := g.msgStore.CreateBatch(...)"
if g.commandHandler != nil {
    if g.commandHandler.HandleCommand(context.Background(), content, msgs[n-1]) {
        return
    }
}
```

Full `onDebounce` after the mutex unlock section should read:

```go
	if g.commandHandler != nil {
		if g.commandHandler.HandleCommand(context.Background(), content, msgs[n-1]) {
			return
		}
	}

	inserted, err := g.msgStore.CreateBatch(context.Background(), batch)
```

- [ ] **Step 5: Run all gateway tests**

```bash
go test ./internal/domain/msgingest/... -v
```

Expected: all existing tests PASS + two new command tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/msgingest/gateway.go internal/domain/msgingest/gateway_test.go
git commit -m "feat: intercept /engine commands in msgingest.Gateway before DB write"
```

---

### Task 6: Feeder + Dispatcher — Dynamic Engine

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/task/dispatcher.go`

- [ ] **Step 1: Update `feeder.go` — replace `EffectiveEngine()` with `enginecfg.Get()`**

In `internal/domain/bee/feeder.go`, add the import:

```go
"github.com/theopenbee/openbee/internal/domain/enginecfg"
```

Then replace all 5 occurrences of `f.cfg.EffectiveEngine()` with `enginecfg.Get()`:

- Line ~151: `GetSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, f.cfg.EffectiveEngine())`
  → `GetSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, enginecfg.Get())`

- Line ~163: `UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID, f.cfg.EffectiveEngine())`
  → `UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID, enginecfg.Get())`

- Line ~210: `DeleteSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, f.cfg.EffectiveEngine())`
  → `DeleteSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, enginecfg.Get())`

- Line ~245: `GetSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, f.cfg.EffectiveEngine())`
  → `GetSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, enginecfg.Get())`

- Line ~253: `UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID, f.cfg.EffectiveEngine())`
  → `UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID, enginecfg.Get())`

- [ ] **Step 2: Update `dispatcher.go` — remove `engineName` field, use `enginecfg.Get()`**

In `internal/domain/task/dispatcher.go`:

1. Add import: `"github.com/theopenbee/openbee/internal/domain/enginecfg"`

2. Remove the `engineName string` field from the `TaskDispatcher` struct.

3. Remove the `WithEngine` option function entirely:
   ```go
   // DELETE this function:
   func WithEngine(name string) Option {
       return func(d *TaskDispatcher) { d.engineName = name }
   }
   ```

4. In `ClearSession` (line ~192), replace `d.engineName` with `enginecfg.Get()`:
   ```go
   if err := d.sessionStore.ClearSessionContexts(context.Background(), enginecfg.Get()); err != nil {
   ```

5. In `resolveWorkerEngine` (line ~320), replace the fallback:
   ```go
   func (d *TaskDispatcher) resolveWorkerEngine(workerID string) (string, *model.Worker) {
       if d.workerLookup != nil {
           if w, err := d.workerLookup.GetByID(workerID); err == nil {
               engine := w.Engine
               if engine == "" {
                   engine = enginecfg.Get()
               }
               return engine, &w
           }
       }
       return enginecfg.Get(), nil
   }
   ```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/domain/bee/... ./internal/domain/task/...
```

Expected: successful compile (no output).

- [ ] **Step 4: Run existing tests**

```bash
go test ./internal/domain/bee/... ./internal/domain/task/... -v
```

Expected: all existing tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/task/dispatcher.go
git commit -m "feat: feeder and dispatcher read engine from enginecfg cache"
```

---

### Task 7: App Wiring

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add `systemConfigStore` to `appStores`**

In `internal/app/app.go`, update the `appStores` struct:

```go
type appStores struct {
	workerStore       *store.WorkerStore
	envConfigStore    *store.EnvConfigStore
	systemConfigStore *store.SystemConfigStore  // ADD THIS LINE
	execStore         *store.ExecutionStore
	msgStore          *store.MessageStore
	taskStore         *store.TaskStore
	sessionStore      *store.SessionStore
	outboundMsgStore  *store.OutboundMessageStore
	memoryStore       *store.MemoryStore
	departmentStore   *store.DepartmentStore
	statsStore        *store.StatsStore
}
```

- [ ] **Step 2: Populate `systemConfigStore` in `buildStores`**

In the `buildStores` function, add:

```go
return db, appStores{
    // ... existing fields ...
    systemConfigStore: store.NewSystemConfigStore(db),
}, nil
```

- [ ] **Step 3: Add required imports to `app.go`**

Ensure these imports are present in `internal/app/app.go`:

```go
"github.com/theopenbee/openbee/internal/domain/command"
"github.com/theopenbee/openbee/internal/domain/enginecfg"
```

- [ ] **Step 4: Initialize `enginecfg` from DB at startup**

In the `Build` function, after `buildStores` and `buildAllEngines` succeed, add:

```go
// Initialize the default engine cache from DB, falling back to config.
if dbCfg, found, err := s.systemConfigStore.Get(context.Background(), model.SystemConfigKeyDefaultEngine); err == nil && found {
    enginecfg.Init(dbCfg.Value)
} else {
    enginecfg.Init(cfg.Bee.EffectiveEngine())
}
```

Also add the `model` import if not already present:

```go
"github.com/theopenbee/openbee/internal/infra/model"
```

- [ ] **Step 5: Replace fixed engine with `DynamicAdapter` in `buildBee`**

Change `buildBee` to accept the full engines map instead of a single adapter:

```go
func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task.DispatchTask,
	failureNotifier bee.FailureNotifier, engines map[string]ai.EngineAdapter, envSvc *env.Service) (*bee.Feeder, *task.Scheduler) {
	dynamic := ai.NewDynamicAdapter(engines)
	beeProcess := bee.NewBeeProcess(cfg, dynamic, envSvc)
	feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore, beeProcess, config.DefaultBeeWorkDir(), cfg,
		bee.WithFailureNotifier(failureNotifier),
		bee.WithWorkerDispatch(s.workerStore))
	sched := task.NewScheduler(s.taskStore, dispatchCh, bee.PollInterval)
	return feeder, sched
}
```

Update the `buildBee` call site in `Build`:

```go
feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engines, envSvc)
```

(Remove the `engines[cfg.Bee.EffectiveEngine()]` lookup.)

- [ ] **Step 6: Remove `WithEngine` from `buildPipeline` and its call**

Update `buildPipeline`:

```go
func buildPipeline(
	debounce time.Duration,
	s appStores,
	mgr *worker.Manager,
	dispatchCh chan task.DispatchTask,
	failureNotifier task.FailureNotifier,
) (*msgingest.Gateway, *task.TaskDispatcher) {
	ingest := msgingest.New(s.msgStore, debounce)
	disp := task.New(mgr, s.taskStore, s.sessionStore, s.execStore, dispatchCh,
		task.WithFailureNotifier(failureNotifier),
		task.WithWorkerLookup(s.workerStore),
	)
	return ingest, disp
}
```

Update the call site (remove `cfg.Bee.EffectiveEngine()` arg):

```go
ingest, disp := buildPipeline(cfg.Bee.MessageDebounce, s, mgr, dispatchCh, failureNotifier)
```

- [ ] **Step 7: Build the `EngineCommandHandler` and inject into both gateways**

In `Build`, after `sendersByPlatform` is fully populated (after the platform loop), add:

```go
engineCmdHandler := command.NewEngineCommandHandler(s.workerStore, s.systemConfigStore, sendersByPlatform)
```

Then update the `ingest` gateway construction inside `buildPipeline` — pass the handler via `WithCommandHandler`. Since `buildPipeline` now needs the handler, update its signature:

```go
func buildPipeline(
	debounce time.Duration,
	s appStores,
	mgr *worker.Manager,
	dispatchCh chan task.DispatchTask,
	failureNotifier task.FailureNotifier,
	cmdHandler msgingest.CommandHandler,
) (*msgingest.Gateway, *task.TaskDispatcher) {
	ingest := msgingest.New(s.msgStore, debounce, msgingest.WithCommandHandler(cmdHandler))
	disp := task.New(mgr, s.taskStore, s.sessionStore, s.execStore, dispatchCh,
		task.WithFailureNotifier(failureNotifier),
		task.WithWorkerLookup(s.workerStore),
	)
	return ingest, disp
}
```

Update call site:

```go
ingest, disp := buildPipeline(cfg.Bee.MessageDebounce, s, mgr, dispatchCh, failureNotifier, engineCmdHandler)
```

Also inject into `localIngest`:

```go
localIngest := msgingest.New(s.msgStore, 100*time.Millisecond, msgingest.WithCommandHandler(engineCmdHandler))
```

Note: `sendersByPlatform` must be fully populated **before** building `engineCmdHandler`. Verify the ordering in `Build`: local sender and platform senders are added to the map; only then create the handler.

- [ ] **Step 8: Verify full build**

```bash
go build ./...
```

Expected: successful compile (no output).

- [ ] **Step 9: Run all tests**

```bash
go test ./... 2>&1 | tail -30
```

Expected: all packages PASS (or SKIP for packages requiring external tools).

- [ ] **Step 10: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire /engine command handler and dynamic engine into app"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Covered by task |
|---|---|
| New `bee_system_configs` table | Task 1 (migration) |
| `model.SystemConfig` + `SystemConfigKeyDefaultEngine` | Task 1 |
| `SystemConfigStore.Get` / `.Set` | Task 1 |
| `enginecfg.Init` / `Get` / `Set` | Task 2 |
| `ai.DynamicAdapter` routes via `enginecfg.Get()` | Task 3 |
| `CommandHandler` interface | Task 4 |
| `EngineCommandHandler` with all reply cases | Task 4 |
| Gateway intercepts before DB write | Task 5 |
| `localIngest` also gets handler | Task 7, Step 7 |
| `feeder.go` uses `enginecfg.Get()` | Task 6 |
| `dispatcher.go` fallback uses `enginecfg.Get()` | Task 6 |
| `WithEngine` option removed | Task 6 |
| Startup loads engine from DB, falls back to config | Task 7, Step 4 |
| Reply messages in zh-CN | Task 4, Step 5 |

**No placeholders found.** All code steps contain complete, compilable code.

**Type consistency:**
- `CommandHandler.HandleCommand(ctx, content, replyTo)` → same signature used in Gateway and test mocks.
- `WorkerRepository.GetByName` / `Update` → matches `store.WorkerStore` public methods.
- `SystemConfigWriter.Set(ctx, key, value)` → matches `SystemConfigStore.Set`.
- `ai.DynamicAdapter` implements `ai.EngineAdapter` (same `Prepare`/`Run`/`ExtractResult` signatures).
- `enginecfg.Get()` returns `string` everywhere it replaces `f.cfg.EffectiveEngine()` and `d.engineName`.
