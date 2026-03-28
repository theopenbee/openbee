# i18n Language Selection in `config` Wizard — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a bilingual language-selection prompt as the first step of the `openbee config` interactive wizard, immediately reloading i18n and saving the choice to `config.yaml`.

**Architecture:** A new `runLanguageStep(existingLang string) (string, error)` function is inserted at the top of `runConfig()`, after loading any existing config. It shows a hardcoded bilingual `survey.Select` prompt, calls `i18n.Load(lang)` to switch translations in-place, and returns the selected language code for storage in `configValues.Language`.

**Tech Stack:** Go, `github.com/AlecAivazis/survey/v2`, `github.com/theopenbee/openbee/internal/i18n`

---

## File Map

| File | Action | What changes |
|------|--------|--------------|
| `cmd/openbee/config.go` | Modify | Add `Language string` to `configValues`; populate it in `loadExistingConfig`; add `runLanguageStep()`; call it in `runConfig()` |
| `internal/config/config.yaml.tmpl` | Modify | Add `language: {{.Language}}` at the top (after the comment header) |

No other files need touching.

---

### Task 1: Add `Language` to `configValues` and `loadExistingConfig`

**Files:**
- Modify: `cmd/openbee/config.go:22-65` (struct) and `:88-131` (`loadExistingConfig`)

- [ ] **Step 1: Add `Language string` as the first field in `configValues`**

  Open `cmd/openbee/config.go`. The struct starts at line 22. Add `Language string` as the very first field:

  ```go
  type configValues struct {
  	Language string
  	ServerPort string
  	// ... rest of fields unchanged
  ```

- [ ] **Step 2: Populate `Language` in `loadExistingConfig`**

  In `loadExistingConfig`, the `return &configValues{...}` block starts around line 94. Add `Language: cfg.Language,` as the first field in the returned struct:

  ```go
  return &configValues{
  	Language:             cfg.Language,
  	ServerPort:           strconv.Itoa(cfg.Server.Port),
  	// ... rest of fields unchanged
  ```

- [ ] **Step 3: Build to verify no compile errors**

  ```bash
  go build ./cmd/openbee/...
  ```

  Expected: no output (clean build).

- [ ] **Step 4: Commit**

  ```bash
  git add cmd/openbee/config.go
  git commit -m "feat(i18n): add Language field to configValues"
  ```

---

### Task 2: Add `language` field to config template

**Files:**
- Modify: `internal/config/config.yaml.tmpl:1-4`

- [ ] **Step 1: Insert `language:` line after the header comments**

  The template currently starts with:

  ```yaml
  # OpenBee 配置文件
  # 由 openbee config 命令生成

  server:
  ```

  Change it to:

  ```yaml
  # OpenBee 配置文件
  # 由 openbee config 命令生成

  language: {{.Language}}

  server:
  ```

- [ ] **Step 2: Build to verify the template is still valid**

  ```bash
  go build ./cmd/openbee/...
  ```

  Expected: no output.

- [ ] **Step 3: Commit**

  ```bash
  git add internal/config/config.yaml.tmpl
  git commit -m "feat(i18n): write language field to config.yaml template"
  ```

---

### Task 3: Implement `runLanguageStep` and wire into `runConfig`

**Files:**
- Modify: `cmd/openbee/config.go` (new function + two lines in `runConfig`)

- [ ] **Step 1: Add `runLanguageStep` function**

  Add the following function anywhere in `cmd/openbee/config.go` (e.g., just before `promptPassword`):

  ```go
  // runLanguageStep shows a bilingual language-selection prompt and reloads i18n
  // with the chosen language. existingLang should be "" or a previously saved
  // language code ("en" or "zh"); it determines the default selection.
  func runLanguageStep(existingLang string) (string, error) {
  	defaultOpt := "English"
  	if existingLang == "zh" {
  		defaultOpt = "中文"
  	}

  	var selected string
  	if err := survey.AskOne(&survey.Select{
  		Message: "Select language / 选择语言",
  		Options: []string{"English", "中文"},
  		Default: defaultOpt,
  	}, &selected); err != nil {
  		return "", handleSurveyErr(err)
  	}

  	lang := "en"
  	if selected == "中文" {
  		lang = "zh"
  	}

  	if err := i18n.Load(lang); err != nil {
  		return "", fmt.Errorf("load i18n: %w", err)
  	}
  	return lang, nil
  }
  ```

- [ ] **Step 2: Call `runLanguageStep` in `runConfig`**

  In `runConfig`, after the `if existing := loadExistingConfig(...)` block (around line 154) and before the `// Step 1 — Claude config` comment, insert:

  ```go
  	// Language selection — always shown first, before all other prompts
  	lang, err := runLanguageStep(vals.Language)
  	if err != nil {
  		return err
  	}
  	vals.Language = lang
  ```

- [ ] **Step 3: Build to verify no compile errors**

  ```bash
  go build ./cmd/openbee/...
  ```

  Expected: no output.

- [ ] **Step 4: Commit**

  ```bash
  git add cmd/openbee/config.go
  git commit -m "feat(i18n): add runLanguageStep to config wizard"
  ```

---

### Task 4: Smoke-test the full flow manually

This feature is interactive (uses `survey` TTY prompts), so automated unit tests cannot cover `runLanguageStep` itself. Verify manually:

- [ ] **Step 1: Run config wizard with no existing config**

  ```bash
  go run ./cmd/openbee config -o /tmp/test-config.yaml
  ```

  Expected first prompt:
  ```
  ? Select language / 选择语言  [Use arrows to move, type to filter]
  > English
    中文
  ```
  Default highlight should be `English`.

- [ ] **Step 2: Select 中文 and verify subsequent prompts switch to Chinese**

  Arrow-down to `中文`, press Enter. The next prompt should be in Chinese (e.g., `Claude 可执行文件路径:` instead of `Claude executable path:`). Press Ctrl+C to exit early.

- [ ] **Step 3: Verify language is written to config.yaml**

  Run the wizard again, select `中文`, and complete it (or at least reach the write step and confirm). Then:

  ```bash
  head -5 /tmp/test-config.yaml
  ```

  Expected:
  ```yaml
  # OpenBee 配置文件
  # 由 openbee config 命令生成

  language: zh
  ```

- [ ] **Step 4: Run config wizard with existing config that has `language: zh`**

  ```bash
  go run ./cmd/openbee config -o /tmp/test-config.yaml
  ```

  Expected first prompt: default highlight should now be `中文` (not `English`).

- [ ] **Step 5: Verify Ctrl+C at language prompt exits cleanly**

  ```bash
  go run ./cmd/openbee config -o /tmp/test-config.yaml
  ```

  Press Ctrl+C immediately at the language prompt. Expected: clean exit with no stack trace, exit code 0 (same behavior as Ctrl+C on any other config prompt).

- [ ] **Step 6: Commit smoke-test confirmation**

  ```bash
  git add -p   # no code changes expected; skip if nothing to stage
  git commit --allow-empty -m "chore: manual smoke test passed for i18n language selection"
  ```

  (Use `--allow-empty` only if no files changed; otherwise skip this step.)

---

## Done

All requirements from the spec are covered:

| Requirement | Task |
|-------------|------|
| Language prompt is always first in config wizard | Task 3 |
| Existing `language` value used as default | Task 1 (populate) + Task 3 (pass to step) |
| Bilingual prompt text (`Select language / 选择语言`) | Task 3 |
| Subsequent prompts use selected language | Task 3 (`i18n.Load` inside `runLanguageStep`) |
| Selected language written to `config.yaml` | Task 1 (struct field) + Task 2 (template) + Task 3 (set `vals.Language`) |
| No system language auto-detection | n/a (not implemented by design) |
