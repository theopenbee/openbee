# Remove `claude download` / `claude env` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove `openbee claude download` / `openbee claude env` commands, the `openbee claude` parent command, and the auto-download + provider-config behavior inside `openbee config`. Treat `claude` like the other engines: `exec.LookPath` with a manual-path fallback.

**Architecture:** Pure deletion + small simplification refactor. After this change, the `cmd/openbee/` Claude branch mirrors codex/pi/kimi. The `internal/ai/claude/` package shrinks to just the runtime adapter/invoker/token-usage code. No new abstractions introduced.

**Tech Stack:** Go 1.x, cobra, AlecAivazis/survey/v2, gopkg.in/yaml.v3 (via i18n).

**Spec:** `docs/superpowers/specs/2026-05-13-remove-claude-download-env-design.md`

**Ordering rationale:** Each task ends with `go build ./... && go test ./...` passing. Task 2 is a single atomic commit because deleting `config_claude.go` removes symbols (`configureClaudeExecutable`, `configureClaudeProvider`) that `config.go` calls, and deleting `claude.go` removes symbols (`claudeCmd`, `claudeDownloadCmd`, `claudeEnvCmd`) that `main.go` references — these have to move together to keep the build green.

---

### Task 1: Preserve `TestMain` before deleting `config_claude_test.go`

The cmd/openbee test package relies on `TestMain` in `config_claude_test.go` to load i18n. Move it to its own file so it survives later deletions.

**Files:**
- Create: `cmd/openbee/testmain_test.go`

- [ ] **Step 1: Create the new TestMain file**

```go
package main

import (
	"os"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func TestMain(m *testing.M) {
	if err := i18n.Load("en"); err != nil {
		panic("i18n.Load failed: " + err.Error())
	}
	os.Exit(m.Run())
}
```

- [ ] **Step 2: Verify build & test still pass (TestMain is now defined twice — Go will error)**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./cmd/openbee/... -count=1`
Expected: FAIL with "TestMain redeclared in this block" — this is the proof that we actually have two definitions and need to remove the old one in the next step.

- [ ] **Step 3: Delete the old TestMain by emptying `config_claude_test.go`**

Replace the entire contents of `cmd/openbee/config_claude_test.go` with:

```go
package main
```

(We leave the file as a near-empty stub for this commit; it will be fully removed in Task 2.)

- [ ] **Step 4: Verify tests now pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./cmd/openbee/... -count=1`
Expected: PASS (all tests in the package run, including those that depend on `i18n.M`).

- [ ] **Step 5: Commit**

```bash
git add cmd/openbee/testmain_test.go cmd/openbee/config_claude_test.go
git commit -m "refactor(cmd): move TestMain to dedicated file"
```

---

### Task 2: Replace consumers and delete cmd-layer Claude files (atomic)

This single commit:
1. Rewrites the Claude branch in `cmd/openbee/config.go` to call `configureEngineExecutable("claude", ...)`.
2. Inlines the InterruptErr→cancellation→`errInterrupted` logic into `cmd/openbee/config.go` and removes the import of `internal/ai/claude`.
3. Deletes `cmd/openbee/claude.go` and `cmd/openbee/config_claude.go` (and the now-empty `config_claude_test.go`).
4. Removes the Claude-command lines from `cmd/openbee/main.go`.

**Files:**
- Modify: `cmd/openbee/config.go`
- Delete: `cmd/openbee/claude.go`
- Delete: `cmd/openbee/config_claude.go`
- Delete: `cmd/openbee/config_claude_test.go`
- Modify: `cmd/openbee/main.go`

- [ ] **Step 1: In `cmd/openbee/config.go`, replace the import block**

Old (around lines 1-23):

```go
import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/claude"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/skillinstall"
	"github.com/theopenbee/openbee/internal/infra/utils"
)
```

New:

```go
import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/skillinstall"
	"github.com/theopenbee/openbee/internal/infra/utils"
)
```

- [ ] **Step 2: Add the local `errInterrupted` sentinel near the top of `cmd/openbee/config.go` (right after the imports, before `var configTemplate = ...`)**

```go
// errInterrupted is returned when a survey prompt is cancelled (Ctrl+C).
// It is used as a sentinel so callers can suppress the error and return nil
// from the cobra RunE function.
var errInterrupted = errors.New("interrupted")
```

- [ ] **Step 3: Replace the body of `handleSurveyErr` at the bottom of `cmd/openbee/config.go`**

Old:

```go
func handleSurveyErr(err error) error {
	return claude.HandleSurveyErr(err)
}
```

New:

```go
func handleSurveyErr(err error) error {
	if errors.Is(err, terminal.InterruptErr) {
		fmt.Println(i18n.M.Prompt.Cancelled)
		return errInterrupted
	}
	return err
}
```

- [ ] **Step 4: Update the `configCmd` RunE to use the local sentinel**

In `cmd/openbee/config.go`, find:

```go
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Interactively generate a config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runConfig(cmd, args); errors.Is(err, claude.ErrInterrupted) {
			return nil
		} else {
			return err
		}
	},
}
```

Replace with:

```go
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Interactively generate a config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runConfig(cmd, args); errors.Is(err, errInterrupted) {
			return nil
		} else {
			return err
		}
	},
}
```

- [ ] **Step 5: Drop the provider-config call from `runConfig`**

In `cmd/openbee/config.go`, find:

```go
		case ai.EngineClaude:
			vals.ClaudeEnabled = true
			if err := configureClaudeExecutable(&vals); err != nil {
				return err
			}
			if err := configureClaudeProvider(&vals); err != nil {
				return err
			}
```

Replace with:

```go
		case ai.EngineClaude:
			vals.ClaudeEnabled = true
			if err := configureClaudeExecutable(&vals); err != nil {
				return err
			}
```

(`configureClaudeExecutable` still resolves to the function in `config_claude.go` at this point — we will replace its definition next.)

- [ ] **Step 6: Delete `cmd/openbee/claude.go`, `cmd/openbee/config_claude.go`, and the stub `cmd/openbee/config_claude_test.go`**

```bash
rm /Users/tengyongzhi/work/bot-workspaces/openbee/cmd/openbee/claude.go \
   /Users/tengyongzhi/work/bot-workspaces/openbee/cmd/openbee/config_claude.go \
   /Users/tengyongzhi/work/bot-workspaces/openbee/cmd/openbee/config_claude_test.go
```

- [ ] **Step 7: Re-add a minimal `configureClaudeExecutable` in `cmd/openbee/config.go`**

Append at the very bottom of `cmd/openbee/config.go`, after the existing `installBuiltinSkills` function:

```go
func configureClaudeExecutable(vals *configValues) error {
	return configureEngineExecutable(
		"claude",
		i18n.M.Output.Config.ClaudeFound,
		i18n.M.Output.Config.ClaudeManualEntry,
		i18n.M.Prompt.ClaudePath,
		&vals.ClaudePath,
	)
}
```

- [ ] **Step 8: Update `cmd/openbee/main.go`**

In `cmd/openbee/main.go`, remove lines 88-90 (currently):

```go
	claudeCmd.Short = m.Cmd.Claude.Short
	claudeDownloadCmd.Short = m.Cmd.ClaudeDownload.Short
	claudeEnvCmd.Short = m.Cmd.ClaudeEnv.Short
```

Remove lines 103-105 (currently):

```go
	claudeDownloadCmd.Flags().Lookup("force").Usage = m.Flag.ClaudeDownloadForce
	claudeDownloadCmd.Flags().Lookup("cdn-url").Usage = m.Flag.ClaudeDownloadCDNURL
	claudeDownloadCmd.Flags().Lookup("cn").Usage = m.Flag.ClaudeDownloadCN
```

Do not leave a blank gap of more than one line in either place.

- [ ] **Step 9: Verify build**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...`
Expected: PASS, no errors.

- [ ] **Step 10: Verify tests**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./... -count=1`
Expected: PASS for cmd/openbee, internal/* packages. (The internal/ai/claude package will still pass because its own files compile independently.)

- [ ] **Step 11: Manual smoke test of unknown-command**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go run ./cmd/openbee claude download 2>&1 | head -5`
Expected: cobra-style "Error: unknown command \"claude\" for \"openbee\"" (or similar). The exact text depends on cobra version; the key check is it errors out and there is no "Download Claude Code" handler.

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go run ./cmd/openbee claude env 2>&1 | head -5`
Expected: same — unknown-command error.

- [ ] **Step 12: Commit**

```bash
git add -A cmd/openbee/
git commit -m "refactor(cmd): remove claude download/env commands and inline config helpers"
```

---

### Task 3: Delete `internal/ai/claude/download.go` + `download_test.go`

After Task 2, no other code imports `claude.Download` or anything else from `download.go`.

**Files:**
- Delete: `internal/ai/claude/download.go`
- Delete: `internal/ai/claude/download_test.go`

- [ ] **Step 1: Verify no remaining references**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && grep -rn "claude\.Download\b\|buildClaudeDownloadURL\|fetchLatestClaudeVersion\|supportedPlatforms\|detectPlatform\b" --include="*.go" .`
Expected: only matches inside `internal/ai/claude/download.go` and `internal/ai/claude/download_test.go` themselves. If any other file matches, stop and investigate.

- [ ] **Step 2: Delete the files**

```bash
rm /Users/tengyongzhi/work/bot-workspaces/openbee/internal/ai/claude/download.go \
   /Users/tengyongzhi/work/bot-workspaces/openbee/internal/ai/claude/download_test.go
```

- [ ] **Step 3: Verify build & tests**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add -A internal/ai/claude/
git commit -m "refactor(claude): drop Claude Code binary downloader"
```

---

### Task 4: Delete `internal/ai/claude/provider.go` + `provider_test.go`

After Task 2, no other code imports `claude.ConfigureProvider`, `claude.HandleSurveyErr`, or `claude.ErrInterrupted`.

**Files:**
- Delete: `internal/ai/claude/provider.go`
- Delete: `internal/ai/claude/provider_test.go`

- [ ] **Step 1: Verify no remaining references**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && grep -rn "claude\.ConfigureProvider\|claude\.HandleSurveyErr\|claude\.ErrInterrupted\|ProviderKimiCode\|ProviderMoonshot" --include="*.go" .`
Expected: only matches inside `internal/ai/claude/provider.go` and `internal/ai/claude/provider_test.go` themselves.

- [ ] **Step 2: Delete the files**

```bash
rm /Users/tengyongzhi/work/bot-workspaces/openbee/internal/ai/claude/provider.go \
   /Users/tengyongzhi/work/bot-workspaces/openbee/internal/ai/claude/provider_test.go
```

- [ ] **Step 3: Verify build & tests**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add -A internal/ai/claude/
git commit -m "refactor(claude): drop interactive provider configuration"
```

---

### Task 5: Clean up i18n (messages.go, en.yaml, zh.yaml together)

After Task 2, the following i18n keys have zero Go consumers and can be removed. Do this in a single commit to keep `messages.go` and the two YAML locale files synchronized.

**Files:**
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`

**Keys to remove (across all three files):**

| Where | Key |
|---|---|
| CmdMessages | `Claude` / `claude` |
| CmdMessages | `ClaudeDownload` / `claude_download` |
| CmdMessages | `ClaudeEnv` / `claude_env` |
| FlagMessages | `ClaudeDownloadForce` / `claude_download_force` |
| FlagMessages | `ClaudeDownloadCDNURL` / `claude_download_cdn_url` |
| FlagMessages | `ClaudeDownloadCN` / `claude_download_cn` |
| PromptMessages | `ClaudeNotFound` / `claude_not_found` |
| PromptMessages | `OptionEnterPathManually` / `option_enter_path_manually` |
| PromptMessages | `OptionDownloadClaude` / `option_download_claude` |
| OutputMessages | `Claude` field + entire `ClaudeOutput` struct (`output.claude.*` block) |
| Messages | `Provider` field + entire `ProviderMessages` struct (top-level `provider:` block) |
| ConfigOutput | `ClaudeDownloadFailed` / `claude_download_failed` |

**Keys to retain** (still used by the new Claude branch in `config.go`):
- `PromptMessages.ClaudePath` / `prompt.claude_path`
- `ConfigOutput.ClaudeFound` / `output.config.claude_found`
- `ConfigOutput.ClaudeManualEntry` / `output.config.claude_manual_entry`

- [ ] **Step 1: Edit `internal/infra/i18n/messages.go`**

**In `CmdMessages` struct (around line 21-41):** delete the three lines:

```go
	Claude         CmdEntry `yaml:"claude"`
	ClaudeDownload CmdEntry `yaml:"claude_download"`
	ClaudeEnv      CmdEntry `yaml:"claude_env"`
```

**In `PromptMessages` struct (around line 54-56 + 114-115):** delete:

```go
	// Claude setup
	ClaudeNotFound string `yaml:"claude_not_found"`
```

Keep `ClaudePath string` — used by new flow.

Then delete:

```go
	OptionEnterPathManually string `yaml:"option_enter_path_manually"`
	OptionDownloadClaude    string `yaml:"option_download_claude"`
```

**In `FlagMessages` struct (around line 134-136):** delete:

```go
	ClaudeDownloadForce    string `yaml:"claude_download_force"`
	ClaudeDownloadCDNURL   string `yaml:"claude_download_cdn_url"`
	ClaudeDownloadCN       string `yaml:"claude_download_cn"`
```

**Remove entire `ProviderMessages` struct (lines 139-158)** and remove its field from `Messages` (line 9):

```go
	Provider ProviderMessages `yaml:"provider"`
```

**In `OutputMessages` struct (around line 170-180):** delete the `Claude` field:

```go
	Claude  ClaudeOutput  `yaml:"claude"`
```

**Remove entire `ClaudeOutput` struct (lines 261-267)**.

**In `ConfigOutput` struct (around line 247):** delete:

```go
	ClaudeDownloadFailed    string `yaml:"claude_download_failed"`   // contains %v
```

Keep `ClaudeFound` and `ClaudeManualEntry`.

- [ ] **Step 2: Edit `internal/infra/i18n/locales/en.yaml`**

Delete the YAML keys/blocks corresponding to every Go field removed above. Exact line removals (current line numbers, but adjust if your editor reflows):

- Lines 21-26 (the three `claude` / `claude_download` / `claude_env` cmd entries — six lines total including key+short)
- Line 50: `claude_not_found: ...`
- Lines 100-101: `option_enter_path_manually:` and `option_download_claude:`
- Lines 115-117: the three `claude_download_*` flag descriptions
- Line 168: `claude_download_failed: ...` (keep 167 `claude_found:` and 169 `claude_manual_entry:`)
- Lines 180-184: the entire `claude:` block under `output:` (5 keys: `already_installed`, `use_force`, `installed_at`, `using_cdn`)
- Lines 197-214: the entire top-level `provider:` block

- [ ] **Step 3: Edit `internal/infra/i18n/locales/zh.yaml`**

Apply the exact same key/block deletions as Step 2 to the Chinese locale. Use `grep -n claude /Users/tengyongzhi/work/bot-workspaces/openbee/internal/infra/i18n/locales/zh.yaml` and `grep -n option_enter_path_manually /Users/tengyongzhi/work/bot-workspaces/openbee/internal/infra/i18n/locales/zh.yaml` to confirm the keys are gone after editing. Also remove the entire top-level `provider:` block.

- [ ] **Step 4: Verify build & tests**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./... && go test ./... -count=1`
Expected: PASS. (i18n's `yaml.Unmarshal` is non-strict by default; extra YAML keys would be silently ignored, but we delete them anyway for hygiene.)

- [ ] **Step 5: Verify no orphan i18n references**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && grep -rn "ClaudeDownload\|ClaudeEnv\|i18n\.M\.Provider\|i18n\.M\.Output\.Claude\.\|ClaudeDownloadFailed\|OptionDownloadClaude\|OptionEnterPathManually\|ClaudeNotFound" --include="*.go" .`
Expected: no matches.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/i18n/
git commit -m "refactor(i18n): drop messages for removed claude commands"
```

---

### Task 6: Update CHANGELOG and verify docs/install scripts

**Files:**
- Modify: `CHANGELOG.md`
- Verify (likely no changes needed): `README.md`, `README.zh.md`, `install.sh`, `install.zh.sh`

- [ ] **Step 1: Confirm READMEs and install scripts do not document the removed commands**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && grep -n "openbee claude\|claude download\|claude env" README.md README.zh.md install.sh install.zh.sh`
Expected output: no matches (a prior scan confirmed only `Claude Code` engine-description lines exist, which we keep). If anything else turns up, remove or rewrite the line to refer the user to upstream Claude installation instead.

- [ ] **Step 2: Add a CHANGELOG entry**

Open `CHANGELOG.md` and add the following to the top of the unreleased / next-version section (per project convention, English content):

```markdown
### Removed

- `openbee claude download` and `openbee claude env` subcommands, along with the
  `openbee claude` parent command, have been removed. Install Claude Code via
  upstream channels (e.g. `npm i -g @anthropic-ai/claude-code`) and configure
  provider environment variables directly in `~/.claude/settings.json` or via
  shell env. The `openbee config` flow no longer offers automatic Claude
  download or provider setup; selecting Claude only prompts for the executable
  path, matching the codex/pi/kimi flows.
```

If there is no unreleased section, follow the existing pattern in the file (look at how prior versions are structured) and add a new entry above the most recent version.

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog entry for removing claude download/env commands"
```

---

### Task 7: Final end-to-end verification

**Files:** none modified (verification only).

- [ ] **Step 1: Full build**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...`
Expected: PASS.

- [ ] **Step 2: Full test suite**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 3: `go vet`**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go vet ./...`
Expected: clean.

- [ ] **Step 4: Orphan-reference scan**

Run:

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && \
  grep -rn "claudeDownload\|claudeEnv\|claude\.Download\b\|claude\.ConfigureProvider\|claude\.HandleSurveyErr\|claude\.ErrInterrupted\|ProviderKimiCode\|ProviderMoonshot\|ConfigureProvider\|HandleSurveyErr" \
    --include="*.go" \
    --include="*.yaml" \
    --include="*.md" \
    --include="*.sh" .
```

Expected: no matches in any file (only the design/plan docs may legitimately mention these names in English prose — if so, that's fine and intentional).

- [ ] **Step 5: Manual CLI smoke test**

Run:

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go run ./cmd/openbee --help 2>&1 | grep -i claude
```

Expected: only matches in the `OpenBee core service` description (if any) — no `claude` subcommand listed.

Run:

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go run ./cmd/openbee claude 2>&1 | head -3
```

Expected: error from cobra about unknown command `claude`.

- [ ] **Step 6: Verify final git log**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && git log --oneline main..HEAD`
Expected: exactly the commits from Tasks 1, 2, 3, 4, 5, 6 (six commits beyond `main`).

- [ ] **Step 7: Report completion**

Send a final summary message to the user listing:
- Files deleted (count and paths)
- Files modified (count and paths)
- Confirmation that all verification steps passed
- The six commit SHAs
