# Engine Timeout Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split `engine.timeout` into `engine.timeout.bee` and `engine.timeout.worker` to independently control bee and worker AI execution timeouts.

**Architecture:** Introduce a new `EngineTimeoutConfig` struct with `Bee` and `Worker` `time.Duration` fields, replacing the flat `EngineDefaultConfig.Timeout`. The bee feeder currently reads its execution timeout from `feeder.timeout`; this is migrated to `engine.timeout.bee`. Worker execution continues to use its timeout, now from `engine.timeout.worker`. The `FeederConfig.Timeout` field is removed as its role is fully absorbed.

**Tech Stack:** Go, YAML config, survey CLI prompts, go-i18n YAML locale files

---

## File Map

| Action   | File                                                           | Change Summary                                        |
|----------|----------------------------------------------------------------|-------------------------------------------------------|
| Modify   | `internal/infra/config/config.go`                             | Add `EngineTimeoutConfig`; update struct/methods/defaults |
| Modify   | `internal/infra/config/config.yaml.tmpl`                      | Replace `timeout: X` with `timeout.bee/worker`; remove `feeder.timeout` |
| Modify   | `internal/infra/i18n/messages.go`                             | Replace `EngineTimeout`/`FeederTimeout` with two new fields |
| Modify   | `internal/infra/i18n/locales/en.yaml`                         | Replace i18n keys                                     |
| Modify   | `internal/infra/i18n/locales/zh.yaml`                         | Replace i18n keys                                     |
| Modify   | `cmd/openbee/config.go`                                       | Update `configValues`; replace single timeout prompt with two |
| Modify   | `internal/domain/bee/feeder.go`                               | Use `f.cfg.Engine.Timeout.Bee` instead of `f.cfg.Feeder.Timeout` |
| Modify   | `internal/infra/config/config_bee_test.go`                    | Update default timeout assertion                      |
| Modify   | `internal/domain/bee/feeder_test.go`                          | Replace `cfg.Feeder.Timeout` with `cfg.Engine.Timeout.Bee` |

---

## Task 1: Add `EngineTimeoutConfig` and update config struct

**Files:**
- Modify: `internal/infra/config/config.go:59-135`
- Modify: `internal/infra/config/config.go:167-170` (FeederConfig)
- Modify: `internal/infra/config/config.go:261-286` (applyDefaults)

- [ ] **Step 1: Write the failing test**

In `internal/infra/config/config_bee_test.go`, add a test verifying the new split defaults:

```go
func TestBeeConfig_EngineTimeoutSplitDefaults(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
bee:
  name: "bee"
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Bee.Engine.Timeout.Bee != 5*time.Minute {
		t.Errorf("default engine.timeout.bee: want 5m got %v", cfg.Bee.Engine.Timeout.Bee)
	}
	if cfg.Bee.Engine.Timeout.Worker != 30*time.Minute {
		t.Errorf("default engine.timeout.worker: want 30m got %v", cfg.Bee.Engine.Timeout.Worker)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/infra/config/... -run TestBeeConfig_EngineTimeoutSplitDefaults -v
```

Expected: FAIL — `cfg.Bee.Engine.Timeout.Bee` undefined (field doesn't exist yet)

- [ ] **Step 3: Add `EngineTimeoutConfig` struct and update `EngineDefaultConfig`**

In `internal/infra/config/config.go`, replace:

```go
// EngineDefaultConfig holds the global engine default name and shared timeout.
type EngineDefaultConfig struct {
	Default string        `yaml:"default"`
	Timeout time.Duration `yaml:"timeout"`
}
```

with:

```go
// EngineTimeoutConfig holds separate timeout durations for the bee and worker roles.
type EngineTimeoutConfig struct {
	Bee    time.Duration `yaml:"bee"`
	Worker time.Duration `yaml:"worker"`
}

// EngineDefaultConfig holds the global engine default name and per-role timeouts.
type EngineDefaultConfig struct {
	Default string             `yaml:"default"`
	Timeout EngineTimeoutConfig `yaml:"timeout"`
}
```

- [ ] **Step 4: Remove `Timeout` from `FeederConfig`**

In `internal/infra/config/config.go`, replace:

```go
type FeederConfig struct {
	Timeout          time.Duration `yaml:"timeout"`
	MaxConcurrentBee int           `yaml:"max_concurrent_bee"`
}
```

with:

```go
type FeederConfig struct {
	MaxConcurrentBee int `yaml:"max_concurrent_bee"`
}
```

- [ ] **Step 5: Update `WorkerTimeout`, `WorkerTimeoutFor`, and add `BeeTimeout`**

In `internal/infra/config/config.go`, replace the three methods:

```go
// WorkerTimeout returns the shared engine timeout.
func (b BeeConfig) WorkerTimeout() time.Duration {
	return b.Engine.Timeout
}

// WorkerTimeoutFor returns the shared engine timeout (same for all engines now).
func (b BeeConfig) WorkerTimeoutFor(_ string) time.Duration {
	return b.Engine.Timeout
}
```

with:

```go
// BeeTimeout returns the bee engine execution timeout.
func (b BeeConfig) BeeTimeout() time.Duration {
	return b.Engine.Timeout.Bee
}

// WorkerTimeout returns the worker engine execution timeout.
func (b BeeConfig) WorkerTimeout() time.Duration {
	return b.Engine.Timeout.Worker
}

// WorkerTimeoutFor returns the worker engine execution timeout.
func (b BeeConfig) WorkerTimeoutFor(_ string) time.Duration {
	return b.Engine.Timeout.Worker
}
```

- [ ] **Step 6: Update `applyDefaults`**

In `internal/infra/config/config.go`, replace:

```go
	if cfg.Bee.Feeder.Timeout == 0 {
		cfg.Bee.Feeder.Timeout = 5 * time.Minute
	}
	...
	if cfg.Bee.Engine.Timeout == 0 {
		cfg.Bee.Engine.Timeout = 30 * time.Minute
	}
```

with:

```go
	if cfg.Bee.Engine.Timeout.Bee == 0 {
		cfg.Bee.Engine.Timeout.Bee = 5 * time.Minute
	}
	if cfg.Bee.Engine.Timeout.Worker == 0 {
		cfg.Bee.Engine.Timeout.Worker = 30 * time.Minute
	}
```

(Remove the `cfg.Bee.Feeder.Timeout == 0` block entirely.)

- [ ] **Step 7: Run the new test to verify it passes**

```bash
go test ./internal/infra/config/... -run TestBeeConfig_EngineTimeoutSplitDefaults -v
```

Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/infra/config/config.go internal/infra/config/config_bee_test.go
git commit -m "feat(config): split engine.timeout into timeout.bee and timeout.worker"
```

---

## Task 2: Update the existing feeder-timeout test

**Files:**
- Modify: `internal/infra/config/config_bee_test.go:55-72`

The existing `TestBeeConfig_FeederTimeoutDefault` (line 52) checks `cfg.Bee.Feeder.Timeout` which no longer exists. It must be updated.

- [ ] **Step 1: Update the existing default-timeout test**

In `internal/infra/config/config_bee_test.go`, replace the existing test that checks `cfg.Bee.Feeder.Timeout`:

```go
// Before (find this test by looking for "default timeout: want 5m"):
if cfg.Bee.Feeder.Timeout != 5*time.Minute {
    t.Errorf("default timeout: want 5m got %v", cfg.Bee.Feeder.Timeout)
}
```

Replace that assertion with a check that `FeederConfig` no longer has `Timeout` (the field is gone, so simply delete the assertion — the compilation itself verifies removal):

```go
// The Feeder.Timeout field has been removed; bee execution timeout is now cfg.Bee.Engine.Timeout.Bee
// No assertion needed here — covered by TestBeeConfig_EngineTimeoutSplitDefaults.
```

i.e. delete the `if cfg.Bee.Feeder.Timeout != ...` block from that test function.

- [ ] **Step 2: Run all config tests**

```bash
go test ./internal/infra/config/... -v
```

Expected: all PASS, no compilation errors

- [ ] **Step 3: Commit**

```bash
git add internal/infra/config/config_bee_test.go
git commit -m "test(config): remove stale feeder.timeout assertion"
```

---

## Task 3: Update feeder to use `engine.timeout.bee`

**Files:**
- Modify: `internal/domain/bee/feeder.go:201`
- Modify: `internal/domain/bee/feeder_test.go` (multiple lines)

- [ ] **Step 1: Write a failing test that verifies the new field path**

In `internal/domain/bee/feeder_test.go`, find all occurrences of `cfg.Feeder.Timeout` and note that after the change they must become `cfg.Engine.Timeout.Bee`. The test already sets this for timeout behavior verification. Simply running the existing tests after changing `feeder.go` will catch any mismatch.

First, update `feeder.go` line 201:

```go
// Before:
beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Feeder.Timeout)

// After:
beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Engine.Timeout.Bee)
```

- [ ] **Step 2: Update all `cfg.Feeder.Timeout` references in `feeder_test.go`**

There are 6 occurrences of `cfg.Feeder.Timeout = 5 * time.Second` in `feeder_test.go`. Replace every one:

```go
// Before:
cfg.Feeder.Timeout = 5 * time.Second

// After:
cfg.Engine.Timeout.Bee = 5 * time.Second
```

Run this to find all occurrences first:
```bash
grep -n "Feeder.Timeout" internal/domain/bee/feeder_test.go
```

- [ ] **Step 3: Run feeder tests**

```bash
go test ./internal/domain/bee/... -v -timeout 60s
```

Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_test.go
git commit -m "feat(bee): feeder uses engine.timeout.bee instead of feeder.timeout"
```

---

## Task 4: Update YAML config template

**Files:**
- Modify: `internal/infra/config/config.yaml.tmpl:18-21,61-63`

- [ ] **Step 1: Update the template**

In `internal/infra/config/config.yaml.tmpl`, replace:

```yaml
bee:
  engine:
    default: {{.EngineDefault}}
    timeout: {{.EngineTimeout}}
```

with:

```yaml
bee:
  engine:
    default: {{.EngineDefault}}
    timeout:
      bee: {{.EngineTimeoutBee}}
      worker: {{.EngineTimeoutWorker}}
```

Also remove the `feeder.timeout` line. Replace:

```yaml
  feeder:
    timeout: {{.FeederTimeout}}
    max_concurrent_bee: {{.FeederMaxConcurrentBee}}
```

with:

```yaml
  feeder:
    max_concurrent_bee: {{.FeederMaxConcurrentBee}}
```

- [ ] **Step 2: Build to confirm template compiles**

```bash
go build ./...
```

Expected: FAIL — `configValues` still has the old `EngineTimeout`/`FeederTimeout` fields referenced in the template. This is expected; Task 5 fixes cmd/config.go.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/config/config.yaml.tmpl
git commit -m "feat(config): update YAML template for engine.timeout.bee/worker"
```

---

## Task 5: Update i18n keys

**Files:**
- Modify: `internal/infra/i18n/messages.go:44-48,95`
- Modify: `internal/infra/i18n/locales/en.yaml:44`
- Modify: `internal/infra/i18n/locales/zh.yaml:44`

- [ ] **Step 1: Update `messages.go`**

In `internal/infra/i18n/messages.go`, inside `PromptMessages`, replace:

```go
	EngineTimeout      string `yaml:"engine_timeout"`
```

with:

```go
	EngineTimeoutBee    string `yaml:"engine_timeout_bee"`
	EngineTimeoutWorker string `yaml:"engine_timeout_worker"`
```

Also remove the `FeederTimeout` field:

```go
	// Before (find in PromptMessages or a nearby struct):
	FeederTimeout       string `yaml:"feeder_timeout"`
```

Delete that line.

- [ ] **Step 2: Update `en.yaml`**

In `internal/infra/i18n/locales/en.yaml`, replace:

```yaml
  engine_timeout: "Worker timeout:"
```

with:

```yaml
  engine_timeout_bee: "Bee timeout:"
  engine_timeout_worker: "Worker timeout:"
```

Also remove the `feeder_timeout` line (find it and delete it).

- [ ] **Step 3: Update `zh.yaml`**

In `internal/infra/i18n/locales/zh.yaml`, replace:

```yaml
  engine_timeout: "Worker 超时时间："
```

with:

```yaml
  engine_timeout_bee: "Bee 超时时间："
  engine_timeout_worker: "Worker 超时时间："
```

Also remove the `feeder_timeout` line.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/en.yaml internal/infra/i18n/locales/zh.yaml
git commit -m "feat(i18n): replace engine_timeout/feeder_timeout with engine_timeout_bee/worker"
```

---

## Task 6: Update `cmd/openbee/config.go`

**Files:**
- Modify: `cmd/openbee/config.go:43-90` (configValues struct)
- Modify: `cmd/openbee/config.go:118-172` (loadExistingConfig)
- Modify: `cmd/openbee/config.go:175-196` (runConfig defaults)
- Modify: `cmd/openbee/config.go:342-348` (EngineTimeout prompt)
- Modify: `cmd/openbee/config.go:589-594` (FeederTimeout prompt)

- [ ] **Step 1: Update `configValues` struct**

In `cmd/openbee/config.go`, replace:

```go
	EngineDefault string
	EngineTimeout string
```

with:

```go
	EngineDefault       string
	EngineTimeoutBee    string
	EngineTimeoutWorker string
```

Also remove `FeederTimeout string` from the struct.

- [ ] **Step 2: Update `loadExistingConfig`**

Replace:

```go
		EngineTimeout: cfg.Bee.Engine.Timeout.String(),
```

with:

```go
		EngineTimeoutBee:    cfg.Bee.Engine.Timeout.Bee.String(),
		EngineTimeoutWorker: cfg.Bee.Engine.Timeout.Worker.String(),
```

Remove the line:

```go
		FeederTimeout:        cfg.Bee.Feeder.Timeout.String(),
```

- [ ] **Step 3: Update `runConfig` defaults**

Replace:

```go
		EngineDefault: "claude",
		EngineTimeout: "30m",
```

with:

```go
		EngineDefault:       "claude",
		EngineTimeoutBee:    "5m",
		EngineTimeoutWorker: "30m",
```

Remove the line:

```go
		FeederTimeout:          "5m",
```

- [ ] **Step 4: Replace the single timeout prompt with two prompts**

Replace the existing `EngineTimeout` prompt block (around line 342):

```go
	// Single global timeout
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.EngineTimeout,
		Default: vals.EngineTimeout,
	}, &vals.EngineTimeout); err != nil {
		return handleSurveyErr(err)
	}
```

with:

```go
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.EngineTimeoutBee,
		Default: vals.EngineTimeoutBee,
	}, &vals.EngineTimeoutBee); err != nil {
		return handleSurveyErr(err)
	}

	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.EngineTimeoutWorker,
		Default: vals.EngineTimeoutWorker,
	}, &vals.EngineTimeoutWorker); err != nil {
		return handleSurveyErr(err)
	}
```

- [ ] **Step 5: Remove the `FeederTimeout` prompt**

Find and delete the entire survey block for `FeederTimeout` (around line 589):

```go
		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.FeederTimeout,
			Default: vals.FeederTimeout,
		}, &vals.FeederTimeout); err != nil {
			return handleSurveyErr(err)
		}
```

- [ ] **Step 6: Build to confirm everything compiles**

```bash
go build ./...
```

Expected: PASS — all references to old fields are gone

- [ ] **Step 7: Commit**

```bash
git add cmd/openbee/config.go
git commit -m "feat(cmd): update config wizard for engine.timeout.bee/worker prompts"
```

---

## Task 7: Full test run and verification

- [ ] **Step 1: Run all tests**

```bash
go test ./... -timeout 120s
```

Expected: all PASS

- [ ] **Step 2: Verify build is clean**

```bash
go build ./...
go vet ./...
```

Expected: no errors or warnings

- [ ] **Step 3: Spot-check YAML generation**

Run the config command in dry-run / print mode to verify the generated YAML looks correct:

```bash
go run ./cmd/openbee config --help
```

(Or manually trace that `configTemplate` uses `EngineTimeoutBee` and `EngineTimeoutWorker`.)

- [ ] **Step 4: Commit if any clean-up needed, otherwise tag complete**

```bash
git log --oneline -8
```

Verify all task commits are present with clean messages.
