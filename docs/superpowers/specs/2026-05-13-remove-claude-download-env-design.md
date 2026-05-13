# Remove `openbee claude download` and `openbee claude env`

Date: 2026-05-13
Status: Approved (awaiting implementation plan)

## Goal

Remove openbee's "manage Claude binary + configure provider env" features. Treat `claude` as just another external executable the user is responsible for installing and configuring, aligned with how `codex` / `pi` / `kimi` are already treated.

## Motivation

The download and env-config commands embed openbee in the lifecycle of a third-party tool (Claude Code) — picking versions, mirroring binaries via CDN, hand-writing `~/.claude/settings.json`, baking in API-key flows for ~10 third-party providers. This is a maintenance burden for behavior the user can do themselves, and it leaves a large surface area (provider list, model names, env-var keys) that drifts out of date.

Dropping it returns openbee to a smaller, unix-style contract: openbee runs a `claude` executable; the user installs and configures it.

## User-Visible Behavior Changes (Breaking)

1. `openbee claude download` no longer exists. Users install Claude Code themselves (npm, manual binary, etc.).
2. `openbee claude env` no longer exists. Users edit `~/.claude/settings.json` themselves or export `ANTHROPIC_*` env vars.
3. `openbee config` Claude branch becomes minimal: `exec.LookPath("claude")`; if not found, prompt for a manual path. No auto-download. No provider configuration step.
4. The `openbee claude` parent command is removed entirely; `openbee claude ...` now reports "unknown command".
5. The legacy install location `~/.openbee/bin/claude` is no longer written. Existing binaries there are left alone (no active cleanup) — openbee just stops touching that path.

## Files Deleted (whole-file deletion)

| File | What it contains |
|---|---|
| `cmd/openbee/claude.go` | `claudeCmd`, `claudeDownloadCmd`, `claudeEnvCmd` and their `init()` |
| `cmd/openbee/config_claude.go` | `configureClaudeExecutable`, `configureClaudeProvider`, `promptClaudeManualPath` |
| `cmd/openbee/config_claude_test.go` | Contains `TestMain` — its body must be relocated (see "Files Modified") |
| `internal/ai/claude/download.go` | `Download` + platform detection + GitHub release lookup |
| `internal/ai/claude/download_test.go` | Tests for the above |
| `internal/ai/claude/provider.go` | `ConfigureProvider`, all per-provider env builders, `ErrInterrupted`, `HandleSurveyErr` |
| `internal/ai/claude/provider_test.go` | Tests for the above |

## Files Modified

### `cmd/openbee/config.go`

- Remove `"github.com/theopenbee/openbee/internal/ai/claude"` import.
- In `runConfig`, replace the Claude branch (currently `configureClaudeExecutable(&vals)` + `configureClaudeProvider(&vals)`) with a single call mirroring `configureCodexExecutable`:

  ```go
  configureEngineExecutable(
      "claude",
      i18n.M.Output.Config.ClaudeFound,
      i18n.M.Output.Config.ClaudeManualEntry,
      i18n.M.Prompt.ClaudePath,
      &vals.ClaudePath,
  )
  ```

  The existing i18n keys `ClaudeFound`, `ClaudeManualEntry`, and `ClaudePath` are reused for this purpose and therefore **retained** (see the i18n section for the precise keep/delete list).
- Replace `errors.Is(err, claude.ErrInterrupted)` with a local sentinel `errInterrupted` declared in this package.
- Inline the body of `claude.HandleSurveyErr` into the package-local `handleSurveyErr`:
  - On `terminal.InterruptErr`: print the cancellation message and return `errInterrupted`.
  - Otherwise pass the error through.
- This adds an import of `github.com/AlecAivazis/survey/v2/terminal` to `config.go` (already imported indirectly via survey).

### `cmd/openbee/main.go`

Remove the following lines (currently 89-90 and 103-105):
- `claudeDownloadCmd.Short = m.Cmd.ClaudeDownload.Short`
- `claudeEnvCmd.Short = m.Cmd.ClaudeEnv.Short`
- The three `claudeDownloadCmd.Flags().Lookup(...).Usage = ...` lines

### `cmd/openbee/config_test.go` (or another retained `_test.go` in this package)

Move the `TestMain` body from `config_claude_test.go` here so that the cmd/openbee test package continues to load i18n before any test runs. Without this, other tests in the package will panic on nil `i18n.M`.

### `internal/infra/i18n/messages.go`

**Remove** these fields (and their containing structs where the struct becomes empty after removal):

- `CmdMessages.ClaudeDownload`
- `CmdMessages.ClaudeEnv`
- `FlagMessages.ClaudeDownloadForce`, `ClaudeDownloadCDNURL`, `ClaudeDownloadCN`
- `PromptMessages.ClaudeNotFound` — the prompt that asked the user to pick between "enter path manually" and "auto-download" is gone
- `PromptMessages.OptionDownloadClaude` — option only used by the deleted prompt
- `PromptMessages.OptionEnterPathManually` — verified only used by the deleted prompt (no other engine references it)
- `OutputMessages.Claude` field and the `ClaudeOutput` struct
- `Messages.Provider` field and the entire `ProviderMessages` struct
- `ConfigOutput.ClaudeDownloadFailed`

**Retain** (still used by the new minimal Claude flow, mirroring codex):

- `PromptMessages.ClaudePath`
- `ConfigOutput.ClaudeFound`
- `ConfigOutput.ClaudeManualEntry`

### `internal/infra/i18n/locales/en.yaml` and `locales/zh.yaml`

Delete the YAML keys corresponding to every Go field removed above. Both files must stay in lock-step (matching key sets).

### `README.md` / `README.zh.md` / `install.sh` / `install.zh.sh`

Search for `openbee claude`, `claude download`, `claude env` and remove or rewrite each occurrence. Most likely these appear as install/onboarding instructions and should be replaced with a one-liner pointing the user at upstream Claude Code installation docs.

### `CHANGELOG.md`

Add an entry under the next unreleased version describing the removal as a breaking change. Per project convention, changelog content is written in English.

## Non-Goals (explicitly kept)

- `internal/ai/claude/adapter.go`, `invoker.go`, `token_usage.go` and their tests — the runtime path for executing Claude. Untouched.
- `cmd/openbee/upgrade.go` `--cdn-url` / `--cn` flags and `resolveCDNURL` — still used by `openbee upgrade` for self-update.
- The `internal/ai/claude` package itself — survives because adapter/invoker remain.
- Existing user-installed binaries at `~/.openbee/bin/claude` — left in place; openbee just stops creating new ones.

## Validation

- `go build ./...` succeeds.
- `go test ./...` succeeds (in particular, cmd/openbee tests still find i18n.M loaded).
- Manual smoke: `openbee config` with Claude engine selected runs through with no auto-download / no provider prompts, behaving identically to picking codex.
- `openbee claude` (without subcommand) returns an unknown-command error from cobra.
- A grep for `claude.Download`, `ConfigureProvider`, `claudeDownload`, `claudeEnv` across the repo returns zero results.

## Out of Scope

- Active deletion or migration of any user files (`~/.openbee/bin/claude`, `~/.claude/settings.json`).
- Replacement features (e.g. a documentation page on how to set provider env vars). May be added later; not part of this change.
