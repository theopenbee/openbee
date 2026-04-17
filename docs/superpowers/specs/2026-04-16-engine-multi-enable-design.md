# Engine Multi-Enable Configuration Design

**Date:** 2026-04-16  
**Branch:** feat/worker-engine-selection  
**Status:** Approved

## Background

Workers can now freely select their own engine (per-worker `Engine` field). The `openbee config` subcommand currently uses a single-select to pick one global default engine. This no longer fits: users need to enable multiple engines so workers can choose among them.

The new design mirrors how platforms are configured: multi-select to enable engines, then single-select to pick the default.

## Design

### 1. Config Structure Changes

**File:** `internal/infra/config/config.go`

Replace the flat `Claude`, `Codex`, `Pi`, `Kimi` fields on `BeeConfig` and the `Engine string` field with a structured `Engine` config and a nested `Engines` namespace.

```go
// EngineDefaultConfig holds the global engine default and shared timeout.
type EngineDefaultConfig struct {
    Default string        `yaml:"default"` // default engine name; must be one of the enabled engines
    Timeout time.Duration `yaml:"timeout"` // single shared timeout for all engines
}

// EngineItemConfig is the per-engine enable/path config (same structure for all engines).
type EngineItemConfig struct {
    Enabled bool   `yaml:"enabled"`
    Path    string `yaml:"path"`
}

// EnginesConfig groups all per-engine configs under the `engines:` YAML namespace.
type EnginesConfig struct {
    Claude EngineItemConfig `yaml:"claude"`
    Codex  EngineItemConfig `yaml:"codex"`
    Pi     EngineItemConfig `yaml:"pi"`
    Kimi   EngineItemConfig `yaml:"kimi"`
}

// IsEnabled returns whether the named engine is enabled.
func (e EnginesConfig) IsEnabled(name string) bool {
    switch name {
    case "claude":
        return e.Claude.Enabled
    case "codex":
        return e.Codex.Enabled
    case "pi":
        return e.Pi.Enabled
    case "kimi":
        return e.Kimi.Enabled
    }
    return false
}

// PathFor returns the executable path for the named engine.
func (e EnginesConfig) PathFor(name string) string {
    switch name {
    case "claude":
        return e.Claude.Path
    case "codex":
        return e.Codex.Path
    case "pi":
        return e.Pi.Path
    case "kimi":
        return e.Kimi.Path
    }
    return ""
}

type BeeConfig struct {
    MessageDebounce time.Duration   `yaml:"message_debounce"`
    Engine          EngineDefaultConfig `yaml:"engine"`   // was: Engine string + per-engine timeout fields
    Engines         EnginesConfig       `yaml:"engines"`  // was: Claude/Codex/Pi/Kimi flat fields
    Feeder          FeederConfig    `yaml:"feeder"`
    Platforms       PlatformsConfig `yaml:"platforms"`
    MCP             MCPConfig       `yaml:"mcp"`
    Media           MediaConfig     `yaml:"media"`
    MCPBaseURL string `yaml:"-"`
}
```

**Methods updated on `BeeConfig`:**

- `EffectiveEngine()` → reads `b.Engine.Default` (falls back to `"claude"`)
- `WorkerTimeout()` / `WorkerTimeoutFor()` → both return `b.Engine.Timeout` (single global timeout)
- `EngineConfigRawFor(name)` → reads `b.Engines.PathFor(name)`; note: Pi and Kimi no longer have per-engine `env` maps (removed along with per-engine timeout)

**`applyDefaults` updates:**

- Default path per engine: `claude`, `codex`, `pi`, `kimi`
- Default timeout: `b.Engine.Timeout` → `30m` if zero
- Default engine: `b.Engine.Default` → `"claude"` if empty

### 2. YAML Template Changes

**File:** `internal/infra/config/config.yaml.tmpl`

Replace the flat engine block with:

```yaml
bee:
  engine:
    default: {{.EngineDefault}}
    timeout: {{.EngineTimeout}}
  engines:
    claude:
      enabled: {{.ClaudeEnabled}}
      path: {{.ClaudePath}}
    codex:
      enabled: {{.CodexEnabled}}
      path: {{.CodexPath}}
    pi:
      enabled: {{.PiEnabled}}
      path: {{.PiPath}}
    kimi:
      enabled: {{.KimiEnabled}}
      path: {{.KimiPath}}
```

### 3. CLI Changes

**File:** `cmd/openbee/config.go`

**`configValues` struct:** Replace `Engine string`, `ClaudeTimeout`, `CodexTimeout`, `PiTimeout`, `KimiTimeout`, `PiEnv`, `KimiEnv` with:

```go
EngineDefault  string
EngineTimeout  string
ClaudeEnabled  bool
CodexEnabled   bool
PiEnabled      bool
KimiEnabled    bool
// ClaudePath, CodexPath, PiPath, KimiPath remain
```

**`loadExistingConfig`:** Map new config fields to new `configValues` fields.

**`runConfig` engine step (Step 1) — new flow:**

```
1. MultiSelect: "Which engines to enable?" (options: claude, codex, pi, kimi)
   - Pre-selected based on existing config `Engines.*.Enabled`
   - Validation: at least 1 must be selected

2. For each selected engine, prompt for path (reuse configureEngineExecutable pattern)
   - Claude also prompts for provider (reuse configureClaudeProvider)

3. Select: "Default engine?" (options: subset of selected engines)
   - Pre-selected: existing EngineDefault if still in selected set

4. Input: "Worker timeout?" (single prompt, default 30m)
```

All unselected engines have `Enabled = false`; their paths keep defaults (not prompted).

### 4. Runtime Changes

**File:** `internal/app/app.go`

`buildAllEngines` skips engines that are not enabled:

```go
func buildAllEngines(cfg config.BeeConfig) (map[string]ai.EngineAdapter, error) {
    result := make(map[string]ai.EngineAdapter)
    for _, name := range ai.AllEngines {
        if !cfg.Engines.IsEnabled(name) {
            continue
        }
        adapter, err := ai.New(name, ai.EngineConfig{
            OpenbeeURL: cfg.MCPBaseURL,
            Raw:        cfg.EngineConfigRawFor(name),
        })
        if err != nil {
            return nil, fmt.Errorf("init engine %q: %w", name, err)
        }
        result[name] = adapter
    }
    return result, nil
}
```

The existing `resolveEngine` in `worker/manager.go` already handles unknown engines gracefully (logs warning, falls back to default), so no changes needed there.

### 5. Validation Rules

- **CLI layer:** MultiSelect requires at least 1 engine selected; loop/reprompt if empty.
- **CLI layer:** Default engine SingleSelect is derived from selected engines only — guaranteed valid by construction.
- **App startup:** `EffectiveEngine()` falls back to `"claude"` if `Engine.Default` is empty; `resolveEngine` falls back gracefully if default isn't in the engines map.

### 6. Breaking Change

| Before | After |
|--------|-------|
| `bee.engine: claude` (string) | `bee.engine.default: claude` |
| `bee.claude.timeout: 30m` | `bee.engine.timeout: 30m` (shared) |
| `bee.claude.path: claude` | `bee.engines.claude.path: claude` |
| `bee.pi.env: {...}` | removed |

Users must re-run `openbee config` after upgrading. No automatic migration.

## Files Affected

| File | Change |
|------|--------|
| `internal/infra/config/config.go` | Replace `Engine string` + flat engine structs with `EngineDefaultConfig` + `EnginesConfig`; update all methods |
| `internal/infra/config/config.yaml.tmpl` | Replace flat engine block with `engine:` + `engines:` namespaces |
| `cmd/openbee/config.go` | Update `configValues`, `loadExistingConfig`, `runConfig` engine step |
| `internal/app/app.go` | Update `buildAllEngines` to skip disabled engines |
| `internal/infra/i18n/` | Add i18n strings for new engine multi-select prompts |
