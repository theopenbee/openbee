# Engine Multi-Enable Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single-engine selector in `openbee config` with a multi-select enable/disable system (mirroring platform config), and move all engine config under a structured `bee.engine` / `bee.engines` namespace.

**Architecture:** Add `EngineDefaultConfig` (default name + shared timeout) and `EnginesConfig` (per-engine enabled/path) structs to replace the flat `Engine string` and `Claude/Codex/Pi/Kimi` fields on `BeeConfig`. The CLI becomes MultiSelect → per-engine path config → SingleSelect default → timeout. Only enabled engines are initialized at app startup.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, `github.com/AlecAivazis/survey/v2`, existing i18n YAML locale files.

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/config/config.go` | Replace flat engine fields with `EngineDefaultConfig` + `EnginesConfig`; update helper methods |
| `internal/infra/config/config.yaml.tmpl` | Replace `bee.engine` string + flat engine blocks with `bee.engine:{default,timeout}` + `bee.engines:{claude,codex,pi,kimi}` |
| `internal/infra/i18n/messages.go` | Add `EngineTimeout`, `EngineDefault` prompt fields; remove per-engine timeout fields |
| `internal/infra/i18n/locales/en.yaml` | Add new i18n keys; remove old per-engine timeout keys |
| `internal/infra/i18n/locales/zh.yaml` | Same as en.yaml |
| `cmd/openbee/config.go` | Update `configValues`, `loadExistingConfig`, `runConfig` engine step (Step 1) |
| `cmd/openbee/config_claude.go` | Remove timeout prompt from `configureClaudeExecutable` |
| `cmd/openbee/config_codex.go` | Remove timeout prompt from `configureCodexExecutable` |
| `cmd/openbee/config_pi.go` | Remove timeout prompt from `configurePiExecutable` |
| `cmd/openbee/config_kimi.go` | Remove timeout prompt from `configureKimiExecutable` |
| `internal/app/app.go` | Update `buildAllEngines` to skip disabled engines |

---

### Task 1: Update Config Structs

**Files:**
- Modify: `internal/infra/config/config.go`

- [ ] **Step 1: Replace engine-related type definitions**

In `internal/infra/config/config.go`, replace the four separate engine config types and the flat `Engine string` field with the new structured types. The changes are:

1. Delete `ClaudeConfig`, `CodexConfig`, `PiConfig`, `KimiConfig` type definitions (lines 59–79).
2. Add `EngineDefaultConfig`, `EngineItemConfig`, `EnginesConfig` in their place.
3. Replace `Engine string`, `Claude ClaudeConfig`, `Codex CodexConfig`, `Pi PiConfig`, `Kimi KimiConfig` fields in `BeeConfig` with `Engine EngineDefaultConfig` and `Engines EnginesConfig`.

New type definitions to add (replace the deleted block):

```go
// EngineDefaultConfig holds the global engine default name and shared timeout.
type EngineDefaultConfig struct {
	Default string        `yaml:"default"`
	Timeout time.Duration `yaml:"timeout"`
}

// EngineItemConfig is the per-engine enable/path config.
type EngineItemConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// EnginesConfig groups all per-engine configs under the engines: YAML namespace.
type EnginesConfig struct {
	Claude EngineItemConfig `yaml:"claude"`
	Codex  EngineItemConfig `yaml:"codex"`
	Pi     EngineItemConfig `yaml:"pi"`
	Kimi   EngineItemConfig `yaml:"kimi"`
}

// IsEnabled reports whether the named engine is enabled.
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

// PathFor returns the executable path configured for the named engine.
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
```

New `BeeConfig` fields (replace old ones):

```go
type BeeConfig struct {
	MessageDebounce time.Duration      `yaml:"message_debounce"`
	Engine          EngineDefaultConfig `yaml:"engine"`
	Engines         EnginesConfig      `yaml:"engines"`
	Feeder          FeederConfig       `yaml:"feeder"`
	Platforms       PlatformsConfig    `yaml:"platforms"`
	MCP             MCPConfig          `yaml:"mcp"`
	Media           MediaConfig        `yaml:"media"`

	// Derived fields — not in YAML, computed by Load()
	MCPBaseURL string `yaml:"-"`
}
```

- [ ] **Step 2: Update helper methods on BeeConfig**

Replace `WorkerTimeout`, `WorkerTimeoutFor`, `EffectiveEngine`, `EngineConfigRaw`, `EngineConfigRawFor` with the new implementations:

```go
// WorkerTimeout returns the shared engine timeout.
func (b BeeConfig) WorkerTimeout() time.Duration {
	return b.Engine.Timeout
}

// WorkerTimeoutFor returns the shared engine timeout (same for all engines now).
func (b BeeConfig) WorkerTimeoutFor(_ string) time.Duration {
	return b.Engine.Timeout
}

// EffectiveEngine returns the configured default engine name, defaulting to "claude".
func (b BeeConfig) EffectiveEngine() string {
	if b.Engine.Default != "" {
		return b.Engine.Default
	}
	return "claude"
}

// EngineConfigRaw returns the raw config map for the default engine.
func (b BeeConfig) EngineConfigRaw() map[string]any {
	return b.EngineConfigRawFor(b.EffectiveEngine())
}

// EngineConfigRawFor returns the raw config map for the named engine.
func (b BeeConfig) EngineConfigRawFor(name string) map[string]any {
	path := b.Engines.PathFor(name)
	if path == "" {
		return nil
	}
	return map[string]any{"path": path}
}
```

- [ ] **Step 3: Update applyDefaults**

Replace the engine-related defaults in `applyDefaults` (currently touching `cfg.Bee.Claude.Path`, `cfg.Bee.Codex.Path`, `cfg.Bee.Codex.Timeout`, etc.):

```go
// Engine defaults
if cfg.Bee.Engine.Timeout == 0 {
    cfg.Bee.Engine.Timeout = 30 * time.Minute
}
if cfg.Bee.Engines.Claude.Path == "" {
    cfg.Bee.Engines.Claude.Path = "claude"
}
if cfg.Bee.Engines.Codex.Path == "" {
    cfg.Bee.Engines.Codex.Path = "codex"
}
if cfg.Bee.Engines.Pi.Path == "" {
    cfg.Bee.Engines.Pi.Path = "pi"
}
if cfg.Bee.Engines.Kimi.Path == "" {
    cfg.Bee.Engines.Kimi.Path = "kimi"
}
```

Remove the old per-engine timeout defaults (`cfg.Bee.Codex.Timeout`, `cfg.Bee.Pi.Timeout`, `cfg.Bee.Kimi.Timeout`).

- [ ] **Step 4: Verify the file compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./internal/infra/config/...
```

Expected: no errors. If there are errors about undefined fields (`b.Claude`, `b.Codex`, etc.), they will be fixed in later tasks.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/config/config.go
git commit -m "refactor(config): replace flat engine fields with EngineDefaultConfig + EnginesConfig"
```

---

### Task 2: Update YAML Template

**Files:**
- Modify: `internal/infra/config/config.yaml.tmpl`

- [ ] **Step 1: Replace the engine block in the template**

In `config.yaml.tmpl`, replace lines 19–43 (the `engine:` string and the four flat engine blocks) with:

```yaml
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

The full `bee:` section should now look like:

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
  mcp:
    token_secret: "{{.MCPTokenSecret}}"
    token_ttl: {{.MCPTokenTTL}}
  platforms:
    ...
```

Note: the old `PiEnv` / `KimiEnv` template blocks are removed entirely.

- [ ] **Step 2: Commit**

```bash
git add internal/infra/config/config.yaml.tmpl
git commit -m "refactor(config): update YAML template to engine.default/timeout + engines namespace"
```

---

### Task 3: Update i18n

**Files:**
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`

- [ ] **Step 1: Update PromptMessages in messages.go**

In `internal/infra/i18n/messages.go`, in the `PromptMessages` struct:

1. Change `EngineSelect` comment to reflect multi-select.
2. Remove: `ClaudeTimeout`, `CodexTimeout`, `PiTimeout`, `KimiTimeout` fields.
3. Add: `EngineDefault` and `EngineTimeout` fields.

The engine-related section of `PromptMessages` should become:

```go
// Engine selection
EngineSelect       string `yaml:"engine_select"`
EngineDefault      string `yaml:"engine_default"`
EngineTimeout      string `yaml:"engine_timeout"`
OptionEngineClaude string `yaml:"option_engine_claude"`
OptionEngineCodex  string `yaml:"option_engine_codex"`
OptionEnginePi     string `yaml:"option_engine_pi"`
OptionEngineKimi   string `yaml:"option_engine_kimi"`
// Claude setup
ClaudeNotFound string `yaml:"claude_not_found"`
ClaudePath     string `yaml:"claude_path"`
// Codex setup
CodexPath string `yaml:"codex_path"`
// Pi setup
PiPath string `yaml:"pi_path"`
// Kimi setup
KimiPath string `yaml:"kimi_path"`
```

- [ ] **Step 2: Update en.yaml**

In `internal/infra/i18n/locales/en.yaml`, under `prompt:`:

Remove:
```yaml
  claude_timeout: "Claude timeout:"
  codex_timeout: "Codex timeout:"
  pi_timeout: "Pi timeout:"
  kimi_timeout: "Kimi timeout:"
```

Add (after `engine_select`):
```yaml
  engine_default: "Default engine:"
  engine_timeout: "Worker timeout:"
```

- [ ] **Step 3: Update zh.yaml**

In `internal/infra/i18n/locales/zh.yaml`, under `prompt:`:

Remove:
```yaml
  claude_timeout: "Claude 超时时间："
  codex_timeout: "Codex 超时时间："
  pi_timeout: "Pi 超时时间："
  kimi_timeout: "Kimi 超时时间："
```

Add (after `engine_select`):
```yaml
  engine_default: "默认引擎："
  engine_timeout: "Worker 超时时间："
```

- [ ] **Step 4: Verify i18n compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./internal/infra/i18n/...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/en.yaml internal/infra/i18n/locales/zh.yaml
git commit -m "refactor(i18n): add engine_default/engine_timeout, remove per-engine timeout prompts"
```

---

### Task 4: Update configureEngineExecutable signature and per-engine configure files

**Files:**
- Modify: `cmd/openbee/config.go` (signature only — full flow update is Task 5)
- Modify: `cmd/openbee/config_claude.go`
- Modify: `cmd/openbee/config_codex.go`
- Modify: `cmd/openbee/config_pi.go`
- Modify: `cmd/openbee/config_kimi.go`

The shared helper `configureEngineExecutable` in `config.go` currently requires a `timeoutMsg` and `timeoutDst` parameter. Update its signature first, then update all callers. This must happen before Task 5 to keep intermediate builds valid.

- [ ] **Step 1: Update configureEngineExecutable signature in config.go**

Replace the current `configureEngineExecutable` function (around line 696) with the no-timeout version:

```go
func configureEngineExecutable(binaryName, foundMsg, manualMsg, pathMsg string, pathDst *string) error {
	if found, err := exec.LookPath(binaryName); err == nil {
		fmt.Printf(foundMsg+"\n", found)
		*pathDst = found
	} else {
		fmt.Println(manualMsg)
		if err := survey.AskOne(&survey.Input{
			Message: pathMsg,
			Default: *pathDst,
		}, pathDst, survey.WithValidator(executablePathValidator)); err != nil {
			return handleSurveyErr(err)
		}
	}
	return nil
}
```

- [ ] **Step 2: Update config_claude.go — remove timeout prompt**

Replace the entire `configureClaudeExecutable` function with a version that only handles path detection (no timeout):

```go
// configureClaudeExecutable handles claude path detection:
// 1. Auto-detect claude in PATH
// 2. If not found: manual input or download
func configureClaudeExecutable(vals *configValues) error {
	if claudePath, err := exec.LookPath("claude"); err == nil {
		fmt.Printf(i18n.M.Output.Config.ClaudeFound+"\n", claudePath)
		vals.ClaudePath = claudePath
	} else {
		var method string
		if err := survey.AskOne(&survey.Select{
			Message: i18n.M.Prompt.ClaudeNotFound,
			Options: []string{i18n.M.Prompt.OptionEnterPathManually, i18n.M.Prompt.OptionDownloadClaude},
		}, &method); err != nil {
			return handleSurveyErr(err)
		}

		switch method {
		case i18n.M.Prompt.OptionEnterPathManually:
			if err := promptClaudeManualPath(vals); err != nil {
				return err
			}
		case i18n.M.Prompt.OptionDownloadClaude:
			path, err := claude.Download(openbeeStateDir(), false, "")
			if err != nil {
				fmt.Printf(i18n.M.Output.Config.ClaudeDownloadFailed+"\n", err)
				fmt.Println(i18n.M.Output.Config.ClaudeManualEntry)
				if err := promptClaudeManualPath(vals); err != nil {
					return err
				}
			} else {
				vals.ClaudePath = path
			}
		}
	}
	return nil
}
```

- [ ] **Step 3: Update config_codex.go — remove timeout arg**

```go
package main

import "github.com/theopenbee/openbee/internal/infra/i18n"

func configureCodexExecutable(vals *configValues) error {
	return configureEngineExecutable(
		"codex",
		i18n.M.Output.Config.CodexFound,
		i18n.M.Output.Config.CodexManualEntry,
		i18n.M.Prompt.CodexPath,
		&vals.CodexPath,
	)
}
```

- [ ] **Step 4: Update config_pi.go — remove timeout arg**

```go
package main

import "github.com/theopenbee/openbee/internal/infra/i18n"

func configurePiExecutable(vals *configValues) error {
	return configureEngineExecutable(
		"pi",
		i18n.M.Output.Config.PiFound,
		i18n.M.Output.Config.PiManualEntry,
		i18n.M.Prompt.PiPath,
		&vals.PiPath,
	)
}
```

- [ ] **Step 5: Update config_kimi.go — remove timeout arg**

```go
package main

import "github.com/theopenbee/openbee/internal/infra/i18n"

func configureKimiExecutable(vals *configValues) error {
	return configureEngineExecutable(
		"kimi",
		i18n.M.Output.Config.KimiFound,
		i18n.M.Output.Config.KimiManualEntry,
		i18n.M.Prompt.KimiPath,
		&vals.KimiPath,
	)
}
```

- [ ] **Step 6: Commit**

```bash
git add cmd/openbee/config.go cmd/openbee/config_claude.go cmd/openbee/config_codex.go cmd/openbee/config_pi.go cmd/openbee/config_kimi.go
git commit -m "refactor(config): remove per-engine timeout prompts from engine configure functions"
```

---

### Task 5: Update config.go — configValues, loadExistingConfig, configureEngineExecutable, runConfig

**Files:**
- Modify: `cmd/openbee/config.go`

This is the largest change. The engine step in `runConfig` becomes a multi-select flow.

- [ ] **Step 1: Update configValues struct**

In `cmd/openbee/config.go`, replace the engine-related fields in `configValues`:

Old fields to remove:
```go
Engine        string
ClaudeTimeout string
CodexTimeout  string
PiPath        string
PiTimeout     string
PiEnv         map[string]string
KimiPath      string
KimiTimeout   string
KimiEnv       map[string]string
```

New fields to add (keep `ClaudePath`, `CodexPath`; add enabled flags and new engine fields):

```go
EngineDefault string
EngineTimeout string
ClaudeEnabled bool
CodexEnabled  bool
PiEnabled     bool
KimiEnabled   bool
ClaudePath    string
CodexPath     string
PiPath        string
KimiPath      string
```

- [ ] **Step 2: Update runConfig default values**

In `runConfig`, replace the engine-related default assignments:

Old:
```go
Engine:                 "claude",
ClaudePath:             "claude",
ClaudeTimeout:          "30m",
CodexPath:              "codex",
CodexTimeout:           "30m",
PiPath:                 "pi",
PiTimeout:              "30m",
KimiPath:               "kimi",
KimiTimeout:            "30m",
```

New:
```go
EngineDefault: "claude",
EngineTimeout: "30m",
ClaudeEnabled: true,
ClaudePath:    "claude",
CodexPath:     "codex",
PiPath:        "pi",
KimiPath:      "kimi",
```

- [ ] **Step 3: Update loadExistingConfig**

Replace the engine-related field mappings in `loadExistingConfig`:

Old:
```go
Engine:               cfg.Bee.Engine,
ClaudePath:           cfg.Bee.Claude.Path,
ClaudeTimeout:        cfg.Bee.Claude.Timeout.String(),
CodexPath:            cfg.Bee.Codex.Path,
CodexTimeout:         cfg.Bee.Codex.Timeout.String(),
PiPath:               cfg.Bee.Pi.Path,
PiTimeout:            cfg.Bee.Pi.Timeout.String(),
PiEnv:                cfg.Bee.Pi.Env,
KimiPath:             cfg.Bee.Kimi.Path,
KimiTimeout:          cfg.Bee.Kimi.Timeout.String(),
KimiEnv:              cfg.Bee.Kimi.Env,
```

New:
```go
EngineDefault: cfg.Bee.Engine.Default,
EngineTimeout: cfg.Bee.Engine.Timeout.String(),
ClaudeEnabled: cfg.Bee.Engines.Claude.Enabled,
CodexEnabled:  cfg.Bee.Engines.Codex.Enabled,
PiEnabled:     cfg.Bee.Engines.Pi.Enabled,
KimiEnabled:   cfg.Bee.Engines.Kimi.Enabled,
ClaudePath:    cfg.Bee.Engines.Claude.Path,
CodexPath:     cfg.Bee.Engines.Codex.Path,
PiPath:        cfg.Bee.Engines.Pi.Path,
KimiPath:      cfg.Bee.Engines.Kimi.Path,
```

- [ ] **Step 4: Replace the engine step in runConfig (Step 1)**

Replace the entire current engine step (from `// Step 1 — Engine config` through the closing `}`of the switch statement, lines ~222–272) with the new multi-select flow:

```go
// Step 1 — Engine config
fmt.Println(i18n.M.Output.Config.SectionEngine)

// Build default selections from existing config
var defaultEngines []string
if vals.ClaudeEnabled {
    defaultEngines = append(defaultEngines, i18n.M.Prompt.OptionEngineClaude)
}
if vals.CodexEnabled {
    defaultEngines = append(defaultEngines, i18n.M.Prompt.OptionEngineCodex)
}
if vals.PiEnabled {
    defaultEngines = append(defaultEngines, i18n.M.Prompt.OptionEnginePi)
}
if vals.KimiEnabled {
    defaultEngines = append(defaultEngines, i18n.M.Prompt.OptionEngineKimi)
}
if len(defaultEngines) == 0 {
    defaultEngines = []string{i18n.M.Prompt.OptionEngineClaude}
}

var selectedEngines []string
if err := survey.AskOne(&survey.MultiSelect{
    Message: i18n.M.Prompt.EngineSelect,
    Options: []string{
        i18n.M.Prompt.OptionEngineClaude,
        i18n.M.Prompt.OptionEngineCodex,
        i18n.M.Prompt.OptionEnginePi,
        i18n.M.Prompt.OptionEngineKimi,
    },
    Default: defaultEngines,
}, &selectedEngines, survey.WithValidator(func(ans any) error {
    if v, ok := ans.([]survey.OptionAnswer); ok && len(v) == 0 {
        return errors.New("select at least one engine")
    }
    return nil
})); err != nil {
    return handleSurveyErr(err)
}

// Reset all engine flags; re-enable based on selection
vals.ClaudeEnabled = false
vals.CodexEnabled = false
vals.PiEnabled = false
vals.KimiEnabled = false

for _, e := range selectedEngines {
    switch e {
    case i18n.M.Prompt.OptionEngineClaude:
        vals.ClaudeEnabled = true
        if err := configureClaudeExecutable(&vals); err != nil {
            return err
        }
        if err := configureClaudeProvider(&vals); err != nil {
            return err
        }
    case i18n.M.Prompt.OptionEngineCodex:
        vals.CodexEnabled = true
        if err := configureCodexExecutable(&vals); err != nil {
            return err
        }
    case i18n.M.Prompt.OptionEnginePi:
        vals.PiEnabled = true
        if err := configurePiExecutable(&vals); err != nil {
            return err
        }
    case i18n.M.Prompt.OptionEngineKimi:
        vals.KimiEnabled = true
        if err := configureKimiExecutable(&vals); err != nil {
            return err
        }
    }
}

// Select default engine from enabled ones
defaultEngineOpt := ""
for _, e := range selectedEngines {
    switch e {
    case i18n.M.Prompt.OptionEngineClaude:
        if vals.EngineDefault == "claude" {
            defaultEngineOpt = i18n.M.Prompt.OptionEngineClaude
        }
    case i18n.M.Prompt.OptionEngineCodex:
        if vals.EngineDefault == "codex" {
            defaultEngineOpt = i18n.M.Prompt.OptionEngineCodex
        }
    case i18n.M.Prompt.OptionEnginePi:
        if vals.EngineDefault == "pi" {
            defaultEngineOpt = i18n.M.Prompt.OptionEnginePi
        }
    case i18n.M.Prompt.OptionEngineKimi:
        if vals.EngineDefault == "kimi" {
            defaultEngineOpt = i18n.M.Prompt.OptionEngineKimi
        }
    }
}
if defaultEngineOpt == "" {
    defaultEngineOpt = selectedEngines[0]
}

var selectedDefault string
if len(selectedEngines) == 1 {
    selectedDefault = selectedEngines[0]
} else {
    if err := survey.AskOne(&survey.Select{
        Message: i18n.M.Prompt.EngineDefault,
        Options: selectedEngines,
        Default: defaultEngineOpt,
    }, &selectedDefault); err != nil {
        return handleSurveyErr(err)
    }
}

switch selectedDefault {
case i18n.M.Prompt.OptionEngineClaude:
    vals.EngineDefault = "claude"
case i18n.M.Prompt.OptionEngineCodex:
    vals.EngineDefault = "codex"
case i18n.M.Prompt.OptionEnginePi:
    vals.EngineDefault = "pi"
case i18n.M.Prompt.OptionEngineKimi:
    vals.EngineDefault = "kimi"
}

// Single global timeout
if err := survey.AskOne(&survey.Input{
    Message: i18n.M.Prompt.EngineTimeout,
    Default: vals.EngineTimeout,
}, &vals.EngineTimeout); err != nil {
    return handleSurveyErr(err)
}
```

- [ ] **Step 5: Build to check for compile errors**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./cmd/openbee/...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add cmd/openbee/config.go
git commit -m "feat(config): engine multi-select with default + global timeout"
```

---

### Task 6: Update app.go — skip disabled engines

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Update buildAllEngines to skip disabled engines**

Replace the current `buildAllEngines` function body:

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

- [ ] **Step 2: Build the full project**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./...
```

Expected: no errors. Fix any remaining references to removed fields (e.g., `cfg.Bee.Claude`, `cfg.Bee.Engine` as string).

- [ ] **Step 3: Run tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./...
```

Expected: all pass. If any test references old config fields (`BeeConfig.Claude`, `BeeConfig.Engine` as string, per-engine timeouts), update them to use the new struct paths.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): only initialize enabled engines at startup"
```

---

### Task 7: Push branch

- [ ] **Step 1: Push to remote**

```bash
git push origin feat/worker-engine-selection
```

Expected: branch pushed successfully.
