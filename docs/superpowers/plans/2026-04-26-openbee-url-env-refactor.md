# Remove OpenbeeURL from EngineConfig Parameter Chain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the explicit `openbeeURL` parameter from the engine construction chain and replace it with a single `os.Setenv("OPENBEE_URL", ...)` at server startup, so `OPENBEE_URL` and `OPENBEE_API_KEY` are handled symmetrically as pure env vars.

**Architecture:** The server process calls `os.Setenv("OPENBEE_URL", cfg.MCPBaseURL)` once before building engines. `BuildBaseEnv()` already snapshots `os.Environ()`, so all engine subprocesses inherit the URL naturally — no explicit threading needed. The `openbeeURL string` parameter is removed from `EngineConfig`, `NewAdapter`, `NewInvoker`, and `BuildBaseEnv` across all four engine packages.

**Tech Stack:** Go — `os.Setenv`, `os.Environ`

**Spec:** `docs/superpowers/specs/2026-04-26-openbee-url-env-refactor-design.md`

---

## File Map

| File | Action |
|------|--------|
| `internal/ai/process.go` | Modify — remove `openbeeURL string` param from `BuildBaseEnv` |
| `internal/ai/registry.go` | Modify — remove `OpenbeeURL string` from `EngineConfig` |
| `internal/app/app.go` | Modify — add `os.Setenv`, remove `OpenbeeURL` from `EngineConfig{}` |
| `internal/ai/claude/invoker.go` | Modify — remove `openbeeURL string` from `NewInvoker` |
| `internal/ai/claude/adapter.go` | Modify — remove `openbeeURL` from init and `NewAdapter` |
| `internal/ai/claude/invoker_test.go` | Modify — use `t.Setenv` in `TestNewInvoker` |
| `internal/ai/kimi/invoker.go` | Modify — remove `openbeeURL string` from `NewInvoker` |
| `internal/ai/kimi/adapter.go` | Modify — remove `openbeeURL` from init and `NewAdapter` |
| `internal/ai/codex/invoker.go` | Modify — remove `openbeeURL string` from `NewInvoker` |
| `internal/ai/codex/adapter.go` | Modify — remove `openbeeURL` from init and `NewAdapter` |
| `internal/ai/pi/invoker.go` | Modify — remove `openbeeURL string` from `NewInvoker` |
| `internal/ai/pi/adapter.go` | Modify — remove `openbeeURL` from init and `NewAdapter` |
| `internal/ai/pi/invoker_test.go` | Modify — use `t.Setenv` in two `NewInvoker` call sites |

---

## Task 1: Update `BuildBaseEnv` in `process.go`

**Files:**
- Modify: `internal/ai/process.go`

After this task the code will not compile yet — callers still pass the old argument. That is expected. Compilation is verified after Task 6.

- [ ] **Step 1: Open `internal/ai/process.go` and replace `BuildBaseEnv`**

Replace the entire function (lines 66–85):

```go
// BuildBaseEnv constructs the base environment for engine subprocesses.
// It prepends the current executable's directory to PATH.
// OPENBEE_URL is inherited from the server process environment (set via os.Setenv at startup).
func BuildBaseEnv() []string {
	sysEnv := os.Environ()
	env := make([]string, 0, len(sysEnv)+1)
	if exePath, err := os.Executable(); err == nil {
		oldPath := os.Getenv("PATH")
		for _, e := range sysEnv {
			if !strings.HasPrefix(e, "PATH=") {
				env = append(env, e)
			}
		}
		env = append(env, "PATH="+filepath.Dir(exePath)+string(os.PathListSeparator)+oldPath)
	} else {
		env = append(env, sysEnv...)
	}
	// Clip to length so concurrent append calls in Run() cannot share the backing array.
	return env[:len(env):len(env)]
}
```

Key changes: removed `openbeeURL string` parameter; removed `env = append(env, "OPENBEE_URL="+openbeeURL)` line; capacity hint changed from `+2` to `+1`; updated comment.

---

## Task 2: Update Claude engine package

**Files:**
- Modify: `internal/ai/claude/invoker.go`
- Modify: `internal/ai/claude/adapter.go`

- [ ] **Step 1: Update `NewInvoker` in `internal/ai/claude/invoker.go`**

Replace lines 20–25:

```go
// NewInvoker creates an Invoker. extraEnv entries are merged into the base environment at lowest priority.
// OPENBEE_URL is inherited from the server process environment.
func NewInvoker(binary string, extraEnv map[string]string) *Invoker {
	base := ai.BuildBaseEnv()
	return &Invoker{binary: binary, baseEnv: ai.AppendExtraEnv(base, extraEnv)}
}
```

- [ ] **Step 2: Update `NewAdapter` and `init` in `internal/ai/claude/adapter.go`**

Replace lines 15–31:

```go
func init() {
	ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.ExtraEnv()), nil
	})
}

type claudeAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{
		invoker:   NewInvoker(binaryPath, extraEnv),
		collector: NewCollector(),
	}
}
```

---

## Task 3: Update Kimi engine package

**Files:**
- Modify: `internal/ai/kimi/invoker.go`
- Modify: `internal/ai/kimi/adapter.go`

- [ ] **Step 1: Update `NewInvoker` in `internal/ai/kimi/invoker.go`**

Replace lines 21–26:

```go
// NewInvoker creates an Invoker. extraEnv entries are merged into the base environment (e.g. MOONSHOT_API_KEY).
// OPENBEE_URL is inherited from the server process environment.
func NewInvoker(binary string, extraEnv map[string]string) *Invoker {
	base := ai.BuildBaseEnv()
	return &Invoker{binary: binary, baseEnv: ai.AppendExtraEnv(base, extraEnv)}
}
```

- [ ] **Step 2: Update `NewAdapter` and `init` in `internal/ai/kimi/adapter.go`**

Replace lines 9–25:

```go
func init() {
	ai.Register(ai.EngineKimi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineKimi), cfg.ExtraEnv()), nil
	})
}

type kimiAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &kimiAdapter{
		invoker:   NewInvoker(binaryPath, extraEnv),
		collector: NewCollector(),
	}
}
```

---

## Task 4: Update Codex engine package

**Files:**
- Modify: `internal/ai/codex/invoker.go`
- Modify: `internal/ai/codex/adapter.go`

- [ ] **Step 1: Update `NewInvoker` in `internal/ai/codex/invoker.go`**

Replace lines 26–31 (note: `store *SessionStore` parameter stays):

```go
// NewInvoker creates an Invoker. extraEnv entries are merged into the base environment at lowest priority.
// OPENBEE_URL is inherited from the server process environment.
func NewInvoker(binary string, store *SessionStore, extraEnv map[string]string) *Invoker {
	base := ai.BuildBaseEnv()
	return &Invoker{binary: binary, baseEnv: ai.AppendExtraEnv(base, extraEnv), store: store}
}
```

- [ ] **Step 2: Update `NewAdapter` and `init` in `internal/ai/codex/adapter.go`**

Replace lines 10–30:

```go
func init() {
	ai.Register(ai.EngineCodex, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineCodex), cfg.ExtraEnv())
	})
}

type codexAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init codex session store: %w", err)
	}
	return &codexAdapter{
		invoker:   NewInvoker(binaryPath, store, extraEnv),
		collector: NewCollector(),
	}, nil
}
```

---

## Task 5: Update Pi engine package

**Files:**
- Modify: `internal/ai/pi/invoker.go`
- Modify: `internal/ai/pi/adapter.go`

- [ ] **Step 1: Update `NewInvoker` in `internal/ai/pi/invoker.go`**

Replace lines 26–33 (note: returns `(*Invoker, error)` because of `MkdirAll`):

```go
func NewInvoker(binary string, extraEnv map[string]string) (*Invoker, error) {
	sessionDir := config.DefaultPiSessionsDir()
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir session dir: %w", err)
	}
	base := ai.BuildBaseEnv()
	return &Invoker{binary: binary, baseEnv: ai.AppendExtraEnv(base, extraEnv), sessionDir: sessionDir}, nil
}
```

- [ ] **Step 2: Update `NewAdapter` and `init` in `internal/ai/pi/adapter.go`**

Replace lines 9–26:

```go
func init() {
	ai.Register(ai.EnginePi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EnginePi), cfg.ExtraEnv())
	})
}

type piAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	inv, err := NewInvoker(binaryPath, extraEnv)
	if err != nil {
		return nil, err
	}
	return &piAdapter{invoker: inv, collector: NewCollector()}, nil
}
```

---

## Task 6: Update `EngineConfig` and `app.go`

**Files:**
- Modify: `internal/ai/registry.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Remove `OpenbeeURL` from `EngineConfig` in `internal/ai/registry.go`**

Replace lines 8–14:

```go
// EngineConfig holds the configuration passed to a Factory when constructing an engine.
type EngineConfig struct {
	// Raw holds engine-specific configuration (parsed from config.yaml).
	Raw map[string]any
}
```

- [ ] **Step 2: Update `buildAllEngines` in `internal/app/app.go`**

Replace lines 254–271:

```go
// buildAllEngines initializes engine adapters shared safely across concurrent workers.
func buildAllEngines(cfg config.BeeConfig) (map[string]ai.EngineAdapter, error) {
	os.Setenv("OPENBEE_URL", cfg.MCPBaseURL) //nolint:errcheck

	result := make(map[string]ai.EngineAdapter)
	for _, name := range ai.AllEngines() {
		if !cfg.Engines.IsEnabled(name) {
			continue
		}
		adapter, err := ai.New(name, ai.EngineConfig{
			Raw: cfg.EngineConfigRawFor(name),
		})
		if err != nil {
			return nil, fmt.Errorf("init engine %q: %w", name, err)
		}
		result[name] = adapter
	}
	return result, nil
}
```

Make sure `"os"` is present in the `app.go` import block (it likely already is).

- [ ] **Step 3: Verify the build compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go build ./internal/...
```

Expected: no errors (the web embed error about `dist` is pre-existing and unrelated — ignore it; if you see only that error the build is clean for our changes).

---

## Task 7: Update invoker tests

**Files:**
- Modify: `internal/ai/claude/invoker_test.go`
- Modify: `internal/ai/pi/invoker_test.go`

- [ ] **Step 1: Update `TestNewInvoker` in `internal/ai/claude/invoker_test.go`**

Replace lines 24–40:

```go
func TestNewInvoker(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	inv := NewInvoker("/usr/bin/claude", nil)
	if inv.binary != "/usr/bin/claude" {
		t.Errorf("binary: want /usr/bin/claude, got %s", inv.binary)
	}
	wantURL := "OPENBEE_URL=http://localhost:8080"
	var found bool
	for _, e := range inv.baseEnv {
		if e == wantURL {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("baseEnv missing %s", wantURL)
	}
}
```

Also update the other `NewInvoker` calls in this file that pass `""` as the second argument — remove the second argument:
- Line 43: `NewInvoker("echo", "", nil)` → `NewInvoker("echo", nil)`
- Line 77: `NewInvoker("echo", "", nil)` → `NewInvoker("echo", nil)`
- Line 102: `NewInvoker("sleep", "", nil)` → `NewInvoker("sleep", nil)`
- Line 122: `NewInvoker("echo", "", nil)` → `NewInvoker("echo", nil)`
- Line 153: `NewInvoker(os.Args[0], "", nil)` → `NewInvoker(os.Args[0], nil)`

- [ ] **Step 2: Update pi invoker tests in `internal/ai/pi/invoker_test.go`**

Find the two `NewInvoker` call sites that pass `"http://localhost:8080"` (lines ~79 and ~92). For each test function, add `t.Setenv` and remove the URL argument:

```go
func TestResolveSessionPath_UsesUUID(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	sessionID := "4d0ce91b-0856-44e2-b0d7-7765d824bba3"
	inv, err := NewInvoker("true", nil)
	if err != nil {
		t.Fatalf("NewInvoker: %v", err)
	}
	got := inv.sessionFilePath(sessionID)
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".openbee", ".pi", "sessions", sessionID+".jsonl")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInvoker_Run_ExitsCleanly(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	inv, err := NewInvoker("true", nil)
	if err != nil {
		t.Fatalf("NewInvoker: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "pi.log")

	_, ch, err := inv.Run(context.Background(), t.TempDir(), "hello",
		ai.RunOptions{SessionID: "4d0ce91b-0856-44e2-b0d7-7765d824bba3"}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for range ch {
	}
}
```

---

## Task 8: Run tests and commit

**Files:** None — verification only

- [ ] **Step 1: Run the full internal/ai test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/ai/... -v 2>&1 | tail -40
```

Expected: all tests PASS. The `TestNewInvoker` test should pass because `t.Setenv("OPENBEE_URL", "http://localhost:8080")` sets the env var before `BuildBaseEnv()` snapshots `os.Environ()`, so the URL appears in `baseEnv`.

- [ ] **Step 2: Commit all changes**

```bash
git add \
  internal/ai/process.go \
  internal/ai/registry.go \
  internal/app/app.go \
  internal/ai/claude/invoker.go \
  internal/ai/claude/adapter.go \
  internal/ai/claude/invoker_test.go \
  internal/ai/kimi/invoker.go \
  internal/ai/kimi/adapter.go \
  internal/ai/codex/invoker.go \
  internal/ai/codex/adapter.go \
  internal/ai/pi/invoker.go \
  internal/ai/pi/adapter.go \
  internal/ai/pi/invoker_test.go

git commit -m "refactor(ai): remove OpenbeeURL from EngineConfig parameter chain

OPENBEE_URL is now injected via os.Setenv at server startup and inherited
by subprocesses through os.Environ(), matching the optional env-var semantics
of OPENBEE_API_KEY. Removes the explicit openbeeURL parameter from EngineConfig,
NewAdapter, NewInvoker, and BuildBaseEnv across all four engine packages."
```
