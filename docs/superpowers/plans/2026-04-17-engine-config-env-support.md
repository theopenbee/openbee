# Engine config.yaml env Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow operators to set static env vars per engine in `config.yaml`; these are injected into the engine process at the lowest priority (overridden by DB-backed global/dept/worker/bee env scopes).

**Architecture:** `EngineItemConfig` gains an `Env` field; `EngineConfigRawFor` passes it into `cfg.Raw["env"]`; claude and codex adapters/invokers are updated to read that key (mirroring the existing pi/kimi pattern). Pi and kimi require only the config-side change since their adapters already read `cfg.Raw["env"]`.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, existing `ai.BuildBaseEnv` / `ai.BuildRunEnv` helpers.

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/config/config.go` | Add `Env` to `EngineItemConfig`; update `EngineConfigRawFor` |
| `internal/infra/config/config_bee_test.go` | Add tests for `Env` field loading and `EngineConfigRawFor` output |
| `internal/ai/claude/invoker.go` | `NewInvoker` accepts `extraEnv map[string]string`, merges into `baseEnv` |
| `internal/ai/claude/adapter.go` | `init` reads `cfg.Raw["env"]`; `NewAdapter` accepts `extraEnv` |
| `internal/ai/claude/adapter_test.go` | Update `newTestAdapter` helper; add env-in-baseEnv test |
| `internal/ai/codex/invoker.go` | `NewInvoker` accepts `extraEnv map[string]string`, merges into `baseEnv` |
| `internal/ai/codex/adapter.go` | `init` reads `cfg.Raw["env"]`; `NewAdapter` accepts `extraEnv` |
| `internal/ai/codex/adapter_test.go` | Update `NewAdapter` call; add env-in-baseEnv test |

Pi and kimi: **no changes** to adapter or invoker.

---

## Task 1: Config — add `Env` field and update `EngineConfigRawFor`

**Files:**
- Modify: `internal/infra/config/config.go:72-75` (EngineItemConfig), `config.go:147-153` (EngineConfigRawFor)
- Test: `internal/infra/config/config_bee_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/infra/config/config_bee_test.go`:

```go
func TestEngineItemConfig_EnvField(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
bee:
  engines:
    claude:
      path: claude
      env:
        ANTHROPIC_BASE_URL: https://proxy.example.com
        CUSTOM_KEY: "value with spaces"
    pi:
      path: pi
      env:
        PI_VAR: pi_value
    kimi:
      path: kimi
    codex:
      path: codex
`)
	f.Close()
	defer os.Remove(f.Name())

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Bee.Engines.Claude.Env["ANTHROPIC_BASE_URL"]; got != "https://proxy.example.com" {
		t.Errorf("Claude.Env[ANTHROPIC_BASE_URL]: want https://proxy.example.com got %q", got)
	}
	if got := cfg.Bee.Engines.Claude.Env["CUSTOM_KEY"]; got != "value with spaces" {
		t.Errorf("Claude.Env[CUSTOM_KEY]: want 'value with spaces' got %q", got)
	}
	if got := cfg.Bee.Engines.Pi.Env["PI_VAR"]; got != "pi_value" {
		t.Errorf("Pi.Env[PI_VAR]: want pi_value got %q", got)
	}
	if cfg.Bee.Engines.Kimi.Env != nil {
		t.Errorf("Kimi.Env should be nil when not set, got %v", cfg.Bee.Engines.Kimi.Env)
	}
}

func TestEngineConfigRawFor_IncludesEnv(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
bee:
  engines:
    claude:
      path: my-claude
      env:
        MY_KEY: my_value
    codex:
      path: codex
`)
	f.Close()
	defer os.Remove(f.Name())

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	raw := cfg.Bee.EngineConfigRawFor("claude")
	if raw == nil {
		t.Fatal("EngineConfigRawFor(claude): want non-nil")
	}
	if got, _ := raw["path"].(string); got != "my-claude" {
		t.Errorf("raw[path]: want my-claude got %q", got)
	}
	env, _ := raw["env"].(map[string]string)
	if env["MY_KEY"] != "my_value" {
		t.Errorf("raw[env][MY_KEY]: want my_value got %q", env["MY_KEY"])
	}

	// Engine with no env — raw["env"] must still be present (nil is fine for type assertion)
	rawCodex := cfg.Bee.EngineConfigRawFor("codex")
	if rawCodex == nil {
		t.Fatal("EngineConfigRawFor(codex): want non-nil")
	}
	envCodex, _ := rawCodex["env"].(map[string]string)
	if len(envCodex) != 0 {
		t.Errorf("codex raw[env] should be empty, got %v", envCodex)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/infra/config/... -run "TestEngineItemConfig_EnvField|TestEngineConfigRawFor_IncludesEnv" -v
```

Expected: FAIL — `cfg.Bee.Engines.Claude.Env` is nil (field doesn't exist yet), `raw["env"]` is missing.

- [ ] **Step 3: Add `Env` field to `EngineItemConfig`**

In `internal/infra/config/config.go`, change:

```go
// EngineItemConfig is the per-engine enable/path config.
type EngineItemConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}
```

to:

```go
// EngineItemConfig is the per-engine enable/path config.
type EngineItemConfig struct {
	Enabled bool              `yaml:"enabled"`
	Path    string            `yaml:"path"`
	Env     map[string]string `yaml:"env"`
}
```

- [ ] **Step 4: Update `EngineConfigRawFor` to include env**

In `internal/infra/config/config.go`, change:

```go
// EngineConfigRawFor returns the raw config map for the named engine.
func (b BeeConfig) EngineConfigRawFor(name string) map[string]any {
	path := b.Engines.PathFor(name)
	if path == "" {
		return nil
	}
	return map[string]any{"path": path}
}
```

to:

```go
// EngineConfigRawFor returns the raw config map for the named engine.
func (b BeeConfig) EngineConfigRawFor(name string) map[string]any {
	item := b.Engines.itemFor(name)
	if item.Path == "" {
		return nil
	}
	return map[string]any{
		"path": item.Path,
		"env":  item.Env,
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/infra/config/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/config/config.go internal/infra/config/config_bee_test.go
git commit -m "feat(config): add Env field to EngineItemConfig and populate EngineConfigRawFor"
```

---

## Task 2: Claude — add extraEnv support to invoker and adapter

**Files:**
- Modify: `internal/ai/claude/invoker.go:21-23`
- Modify: `internal/ai/claude/adapter.go:14-21` (init), `adapter.go:27-29` (NewAdapter)
- Test: `internal/ai/claude/adapter_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/ai/claude/adapter_test.go`:

```go
func TestClaudeAdapter_ExtraEnvInBaseEnv(t *testing.T) {
	a := claude.NewAdapter("echo", "http://localhost:9999", map[string]string{
		"MY_CUSTOM_VAR": "hello",
		"ANOTHER_KEY":   "world",
	})
	// Access baseEnv indirectly: run a command that echoes env and check output.
	// Since we cannot inspect baseEnv directly, we verify NewAdapter doesn't panic
	// and the adapter satisfies the interface.
	var _ ai.EngineAdapter = a
}
```

This test will fail to compile because `NewAdapter` doesn't accept a third argument yet.

- [ ] **Step 2: Run test to verify it fails (compile error)**

```bash
go test ./internal/ai/claude/... -run TestClaudeAdapter_ExtraEnvInBaseEnv -v
```

Expected: compile error — `too many arguments in call to claude.NewAdapter`.

- [ ] **Step 3: Update `claude/invoker.go` — add extraEnv parameter**

Change `NewInvoker` from:

```go
// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
func NewInvoker(binary, openbeeURL string) *Invoker {
	return &Invoker{binary: binary, baseEnv: ai.BuildBaseEnv(openbeeURL)}
}
```

to:

```go
// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
// extraEnv entries are merged into the base environment at lowest priority.
func NewInvoker(binary, openbeeURL string, extraEnv map[string]string) *Invoker {
	base := ai.BuildBaseEnv(openbeeURL)
	for k, v := range extraEnv {
		if v != "" {
			base = append(base, k+"="+v)
		}
	}
	return &Invoker{binary: binary, baseEnv: base}
}
```

- [ ] **Step 4: Update `claude/adapter.go` — read env from Raw, pass to NewInvoker**

Change `init` and `NewAdapter`:

```go
func init() {
	ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = ai.EngineClaude
		}
		extraEnv, _ := cfg.Raw["env"].(map[string]string)
		return NewAdapter(path, cfg.OpenbeeURL, extraEnv), nil
	})
}

type claudeAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{invoker: NewInvoker(binaryPath, openbeeURL, extraEnv)}
}
```

Note: `Prepare` and `Run` methods are unchanged.

- [ ] **Step 5: Update existing test helper in `adapter_test.go`**

Change `newTestAdapter`:

```go
func newTestAdapter(t *testing.T) ai.EngineAdapter {
	t.Helper()
	return claude.NewAdapter("echo", "http://localhost:9999", nil)
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/ai/claude/... -v
```

Expected: all PASS including the new `TestClaudeAdapter_ExtraEnvInBaseEnv`.

- [ ] **Step 7: Commit**

```bash
git add internal/ai/claude/invoker.go internal/ai/claude/adapter.go internal/ai/claude/adapter_test.go
git commit -m "feat(claude): add extraEnv support to invoker and adapter"
```

---

## Task 3: Codex — add extraEnv support to invoker and adapter

**Files:**
- Modify: `internal/ai/codex/invoker.go:27-29`
- Modify: `internal/ai/codex/adapter.go:14-24` (init), `adapter.go:28-34` (NewAdapter)
- Test: `internal/ai/codex/adapter_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/ai/codex/adapter_test.go`:

```go
func TestAdapter_ExtraEnvInBaseEnv(t *testing.T) {
	a, err := codex.NewAdapter("echo", "http://localhost:9999", map[string]string{
		"CODEX_CUSTOM": "value",
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	var _ ai.EngineAdapter = a
}
```

- [ ] **Step 2: Run test to verify it fails (compile error)**

```bash
go test ./internal/ai/codex/... -run TestAdapter_ExtraEnvInBaseEnv -v
```

Expected: compile error — `too many arguments in call to codex.NewAdapter`.

- [ ] **Step 3: Update `codex/invoker.go` — add extraEnv parameter**

Change `NewInvoker` from:

```go
// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
func NewInvoker(binary, openbeeURL string, store *SessionStore) *Invoker {
	return &Invoker{binary: binary, baseEnv: ai.BuildBaseEnv(openbeeURL), store: store}
}
```

to:

```go
// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
// extraEnv entries are merged into the base environment at lowest priority.
func NewInvoker(binary, openbeeURL string, store *SessionStore, extraEnv map[string]string) *Invoker {
	base := ai.BuildBaseEnv(openbeeURL)
	for k, v := range extraEnv {
		if v != "" {
			base = append(base, k+"="+v)
		}
	}
	return &Invoker{binary: binary, baseEnv: base, store: store}
}
```

- [ ] **Step 4: Update `codex/adapter.go` — read env from Raw, pass to NewAdapter/NewInvoker**

Change `init` and `NewAdapter`:

```go
func init() {
	ai.Register(ai.EngineCodex, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = ai.EngineCodex
		}
		extraEnv, _ := cfg.Raw["env"].(map[string]string)
		return NewAdapter(path, cfg.OpenbeeURL, extraEnv)
	})
}

type codexAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init codex session store: %w", err)
	}
	return &codexAdapter{invoker: NewInvoker(binaryPath, openbeeURL, store, extraEnv)}, nil
}
```

- [ ] **Step 5: Update existing test calls in `adapter_test.go`**

Change all `codex.NewAdapter("echo", "http://localhost:9999")` calls to `codex.NewAdapter("echo", "http://localhost:9999", nil)`:

```go
func TestAdapter_Prepare_NoOp(t *testing.T) {
	dir := t.TempDir()
	a, err := codex.NewAdapter("echo", "http://localhost:9999", nil)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	// ... rest unchanged
}

func TestAdapter_Prepare_BothRoles(t *testing.T) {
	a, err := codex.NewAdapter("echo", "http://localhost:9999", nil)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	// ... rest unchanged
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/ai/codex/... -v
```

Expected: all PASS including `TestAdapter_ExtraEnvInBaseEnv`.

- [ ] **Step 7: Commit**

```bash
git add internal/ai/codex/invoker.go internal/ai/codex/adapter.go internal/ai/codex/adapter_test.go
git commit -m "feat(codex): add extraEnv support to invoker and adapter"
```

---

## Task 4: Final verification — all engines compile and tests pass

- [ ] **Step 1: Run full test suite for all touched packages**

```bash
go test ./internal/infra/config/... ./internal/ai/claude/... ./internal/ai/codex/... ./internal/ai/pi/... ./internal/ai/kimi/... -v 2>&1 | tail -20
```

Expected: all packages report `ok`.

- [ ] **Step 2: Build the whole module to catch any missed call sites**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit (if any fixups were needed)**

If steps 1–2 required any additional fixes, commit them:

```bash
git add -p
git commit -m "fix: address compilation issues after engine extraEnv refactor"
```

If no fixes were needed, skip this step.
