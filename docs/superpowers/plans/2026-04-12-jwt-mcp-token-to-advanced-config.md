# JWT & MCP Token Secret → Advanced Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move JWT Secret and MCP Token Secret prompts from their current locations into the Advanced Config section of the `openbee config` wizard, so ordinary users never see these prompts unless they opt into advanced configuration.

**Architecture:** All changes are confined to `cmd/openbee/config.go` in the `runConfig` function. The JWT Secret block is cut from Step 3 (Auth) and pasted into the `if customAdvanced` block. The standalone MCP Token Secret block (currently after Step 4) is also moved into the same `if customAdvanced` block. A silent fallback is added after the advanced block to auto-generate both secrets when the user skips advanced config.

**Tech Stack:** Go, github.com/AlecAivazis/survey/v2, existing i18n keys (no new keys needed)

---

## File Map

| File | Change |
|---|---|
| `cmd/openbee/config.go` | Only file modified — 4 edits within `runConfig` |

---

### Task 1: Remove JWT Secret block from Step 3 (Auth section)

**Files:**
- Modify: `cmd/openbee/config.go:447-466`

The JWT Secret handling currently sits right after the password prompts in Step 3. Delete it entirely — it will be re-added inside the advanced block in Task 2.

- [ ] **Step 1: Delete lines 447–466 from `cmd/openbee/config.go`**

Locate and remove this exact block (between the password section and the `// Step 4 — Advanced config` comment):

```go
	if vals.AuthJWTSecret != "" {
		var regenerate bool
		if err := survey.AskOne(&survey.Confirm{
			Message: i18n.M.Prompt.JWTRegenConfirm,
			Default: false,
		}, &regenerate); err != nil {
			return handleSurveyErr(err)
		}
		if regenerate {
			b := make([]byte, 32)
			rand.Read(b)
			vals.AuthJWTSecret = hex.EncodeToString(b)
			fmt.Println(i18n.M.Output.Config.JWTRegenerated)
		}
	} else {
		b := make([]byte, 32)
		rand.Read(b)
		vals.AuthJWTSecret = hex.EncodeToString(b)
		fmt.Println(i18n.M.Output.Config.JWTGenerated)
	}
```

After deletion, the code immediately after the password section should jump directly to:

```go
	// Step 4 — Advanced config
	fmt.Println(i18n.M.Output.Config.SectionAdvanced)
```

- [ ] **Step 2: Verify the file compiles**

```bash
go build ./cmd/openbee/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/openbee/config.go
git commit -m "refactor: remove JWT secret prompt from auth step"
```

---

### Task 2: Add JWT Secret + MCP Token Secret inside advanced block

**Files:**
- Modify: `cmd/openbee/config.go` — inside `if customAdvanced { ... }` block

Append both secret-handling blocks to the end of the `if customAdvanced` block, after the existing FFmpeg prompt (currently ending at line ~556).

- [ ] **Step 1: Append JWT Secret handling inside `if customAdvanced`**

Find the closing `}` of the `if customAdvanced` block (the one that ends right after the FFmpegPath prompt). Insert the JWT Secret block immediately before that closing brace:

```go
		// JWT Secret
		if vals.AuthJWTSecret != "" {
			var regenerate bool
			if err := survey.AskOne(&survey.Confirm{
				Message: i18n.M.Prompt.JWTRegenConfirm,
				Default: false,
			}, &regenerate); err != nil {
				return handleSurveyErr(err)
			}
			if regenerate {
				b := make([]byte, 32)
				rand.Read(b)
				vals.AuthJWTSecret = hex.EncodeToString(b)
				fmt.Println(i18n.M.Output.Config.JWTRegenerated)
			}
		} else {
			b := make([]byte, 32)
			rand.Read(b)
			vals.AuthJWTSecret = hex.EncodeToString(b)
			fmt.Println(i18n.M.Output.Config.JWTGenerated)
		}
```

- [ ] **Step 2: Append MCP Token Secret handling inside `if customAdvanced` (after JWT block)**

Immediately after the JWT Secret block (still before the closing `}` of `if customAdvanced`), add:

```go
		// MCP Token Secret
		if vals.MCPTokenSecret != "" {
			var regenerate bool
			if err := survey.AskOne(&survey.Confirm{
				Message: i18n.M.Prompt.MCPTokenRegenConfirm,
				Default: false,
			}, &regenerate); err != nil {
				return handleSurveyErr(err)
			}
			if regenerate {
				vals.MCPTokenSecret = config.GenerateRandomSecret()
				fmt.Println(i18n.M.Output.Config.MCPTokenSecretRegenerated)
			}
		} else {
			vals.MCPTokenSecret = config.GenerateRandomSecret()
			fmt.Printf(i18n.M.Output.Config.MCPTokenSecretGenerated+"\n", vals.MCPTokenSecret)
		}
```

- [ ] **Step 3: Verify the file compiles**

```bash
go build ./cmd/openbee/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/config.go
git commit -m "feat: add JWT and MCP token secret prompts inside advanced config block"
```

---

### Task 3: Remove standalone MCP Token Secret block and add silent fallback

**Files:**
- Modify: `cmd/openbee/config.go:559-574` (standalone MCP block, now after `if customAdvanced`)

After Task 2, the old standalone MCP block still exists after the `if customAdvanced` closing brace. Replace it with a silent fallback that handles both secrets for users who skip advanced config.

- [ ] **Step 1: Replace standalone MCP block with silent fallback**

Locate the block that currently reads (immediately after `if customAdvanced { ... }`):

```go
	if vals.MCPTokenSecret != "" {
		var regenerate bool
		if err := survey.AskOne(&survey.Confirm{
			Message: i18n.M.Prompt.MCPTokenRegenConfirm,
			Default: false,
		}, &regenerate); err != nil {
			return handleSurveyErr(err)
		}
		if regenerate {
			vals.MCPTokenSecret = config.GenerateRandomSecret()
			fmt.Println(i18n.M.Output.Config.MCPTokenSecretRegenerated)
		}
	} else {
		vals.MCPTokenSecret = config.GenerateRandomSecret()
		fmt.Printf(i18n.M.Output.Config.MCPTokenSecretGenerated+"\n", vals.MCPTokenSecret)
	}
```

Replace it entirely with:

```go
	if !customAdvanced {
		if vals.AuthJWTSecret == "" {
			b := make([]byte, 32)
			rand.Read(b)
			vals.AuthJWTSecret = hex.EncodeToString(b)
		}
		if vals.MCPTokenSecret == "" {
			vals.MCPTokenSecret = config.GenerateRandomSecret()
		}
	}
```

- [ ] **Step 2: Verify the file compiles**

```bash
go build ./cmd/openbee/...
```

Expected: no errors.

- [ ] **Step 3: Run existing tests**

```bash
go test ./cmd/openbee/... ./internal/infra/config/...
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/config.go
git commit -m "feat: migrate JWT and MCP token secrets to advanced config with silent fallback"
```

---

### Task 4: Manual verification

No automated tests exist for the interactive survey wizard (it requires a TTY). Verify all 5 scenarios manually.

- [ ] **Scenario 1 — Fresh run, skip advanced**

```bash
go run ./cmd/openbee config -o /tmp/test1.yaml
```

Walk through: select any engine, any platform, set username+password, choose **No** at advanced config prompt.

Expected:
- No JWT prompt shown
- No MCP token prompt shown
- `/tmp/test1.yaml` contains `jwt_secret` and `token_secret` fields with generated values

- [ ] **Scenario 2 — Fresh run, enter advanced**

```bash
go run ./cmd/openbee config -o /tmp/test2.yaml
```

Walk through: select any engine, any platform, set username+password, choose **Yes** at advanced config prompt.

Expected:
- JWT Secret auto-generated, confirmation line printed
- MCP Token Secret auto-generated, printed with value
- `/tmp/test2.yaml` contains both secrets

- [ ] **Scenario 3 — Existing config with secrets, skip advanced**

```bash
go run ./cmd/openbee config -o /tmp/test1.yaml
```

(Use the file created in Scenario 1 — it already has secrets.)

Walk through: choose **No** at advanced config.

Expected:
- No JWT prompt
- No MCP token prompt
- Existing secrets preserved in output file (same values as input)

- [ ] **Scenario 4 — Existing config with secrets, enter advanced, choose NOT to regenerate**

```bash
go run ./cmd/openbee config -o /tmp/test1.yaml
```

Walk through: choose **Yes** at advanced, answer **No** to both regenerate prompts.

Expected:
- Both "regenerate?" prompts appear
- Secrets unchanged in output file

- [ ] **Scenario 5 — Existing config with secrets, enter advanced, choose to regenerate both**

```bash
go run ./cmd/openbee config -o /tmp/test1.yaml
```

Walk through: choose **Yes** at advanced, answer **Yes** to both regenerate prompts.

Expected:
- "JWT secret regenerated" message printed
- "MCP token secret regenerated" message printed
- Output file contains new (different) secret values

- [ ] **Step 6: Commit if any fixes were needed from manual testing**

```bash
git add cmd/openbee/config.go
git commit -m "fix: correct JWT/MCP token secret migration edge cases"
```
