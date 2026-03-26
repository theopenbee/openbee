# Design: `openbee claude` Subcommand

**Date:** 2026-03-25
**Status:** Approved

## Summary

Extract Claude Code download and provider/env configuration logic from the `config` command into a dedicated `openbee claude` subcommand with two sub-subcommands: `download` and `env`. The core logic is moved into a new `internal/claude` package shared by both the new command and the existing `config` command.

## Motivation

The `config` command's Step 1 bundles Claude executable setup (download + provider config) with platform/auth/advanced configuration. Users who only want to re-download Claude or switch providers must run the entire `config` wizard. A standalone `claude` subcommand enables targeted, repeatable operations without re-generating the full config file.

## Architecture

### File Changes

```
New:
  internal/claude/
    download.go       ← download logic extracted from config_claude.go
    provider.go       ← provider/env logic extracted from config_claude.go

New:
  cmd/openbee/claude.go   ← new subcommand entry point

Modified:
  cmd/openbee/config_claude.go  ← delegates to internal/claude package
```

### Call Graph

```
openbee claude download  →  cmd/openbee/claude.go  →  internal/claude.Download()
openbee claude env       →  cmd/openbee/claude.go  →  internal/claude.ConfigureProvider()

openbee config (Step 1)  →  cmd/openbee/config_claude.go
                              →  internal/claude.Download()
                              →  internal/claude.ConfigureProvider()
```

`internal/claude` contains only logic — no Cobra command definitions. The `cmd/` layer owns all CLI structure and interactive wrapping.

## `internal/claude` Package

### `download.go`

```go
// Download downloads Claude Code to stateDir/bin/claude.
// If already installed and force is false, returns the existing path without re-downloading.
// Returns the installed executable path on success.
func Download(stateDir string, force bool) (execPath string, err error)
```

Private helpers moved from `config_claude.go`:
- `fetchLatestClaudeVersion()`
- `detectPlatform()`, `mapArch()`, `isMusl()`, `isMuslWith()`
- `buildClaudeDownloadURL()`
- `isSupportedPlatform()`

### `provider.go`

```go
// ConfigureProvider runs an interactive survey to select a provider and API key,
// then writes the result to ~/.claude/settings.json (and ~/.claude.json where needed).
func ConfigureProvider() error
```

Exported constants (for use in cmd layer if needed):
```go
const (
    ProviderMoonshot   = "Moonshot (Kimi)"
    ProviderDeepSeek   = "DeepSeek"
    ProviderGLM        = "Zhipu (GLM)"
    ProviderMiniMax    = "MiniMax"
    ProviderAliyun     = "Alibaba Cloud (Qwen)"
    ProviderVolcengine = "Volcengine (Doubao)"
    ProviderTencent    = "Tencent Cloud"
    ProviderCustom     = "Custom provider"
)
```

Private helpers moved from `config_claude.go`:
- `moonshotEnv()`, `deepseekEnv()`, `glmEnv()`, `minimaxEnv()`, `aliyunEnv()`, `volcengineEnv()`, `tencentEnv()`, `customEnv()`
- `mergeClaudeSettings()`, `mergeClaudeJSON()`, `mergeJSONFile()`
- `promptAPIKey()`

## `cmd/openbee/claude.go`

### Command Tree

```
openbee claude              ← parent command, shows help when no subcommand given
  claude download           ← download Claude Code binary
  claude env                ← configure provider / env settings
```

### `openbee claude download`

Behavior:
1. Detect platform; exit with error if unsupported.
2. Check if `~/.openbee/bin/claude` already exists.
   - Exists and `--force` not set: print "already installed, use --force to re-download" and exit 0.
   - Missing or `--force` set: call `claude.Download(stateDir, force)`.
3. On success, print installed path.

Flags:
- `--force` — force re-download even if already installed.

### `openbee claude env`

Behavior:
1. Call `claude.ConfigureProvider()` directly (fully interactive, same survey UX as current `config` Step 1).
2. On success, print "Configuration written to ~/.claude/settings.json".

No flags (fully interactive).

### `openbee claude` (no subcommand)

Displays help listing both subcommands with descriptions.

## `cmd/openbee/config_claude.go` Changes

- `configureClaudeExecutable()`: keep PATH detection logic; replace `downloadClaude()` call with `claude.Download(openbeeStateDir(), false)`.
- `configureClaudeProvider()`: replace body with `return claude.ConfigureProvider()`.
- All extracted private helpers are removed from this file.

## Error Handling

- Unsupported platform in `download`: clear error message listing supported platforms, exit non-zero.
- Download failure: propagate error with context (network, checksum mismatch, etc.) — unchanged from current behavior.
- `ConfigureProvider` survey interrupt (Ctrl+C): return `ErrInterrupted` and let cmd layer handle gracefully — unchanged from current behavior.

## Testing

No new test surface is introduced by this refactor. The extracted functions have the same behavior as before. Existing tests (if any) that cover `config` Step 1 behavior continue to apply. The `internal/claude` package structure makes unit testing of download and provider logic easier in the future (functions can be tested without going through the Cobra command layer).
