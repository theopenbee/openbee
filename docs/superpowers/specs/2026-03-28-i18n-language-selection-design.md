# Design: i18n Language Selection in `config` Subcommand

**Date:** 2026-03-28
**Branch:** feat/i18n
**Status:** Approved

---

## Overview

Add a language selection step at the start of the `config` interactive wizard. The user chooses between English and Chinese (中文) before any other prompts. The selected language is immediately applied to all subsequent prompts and saved to `config.yaml`.

---

## Requirements

1. When running `openbee config`, the first prompt is always a language selection.
2. If `config.yaml` already exists and has a `language` field, use that as the default selection; otherwise default to English.
3. The language selection prompt itself is bilingual (`Select language / 选择语言`) since the user's language is unknown at that point.
4. After the user selects a language, all subsequent interactive prompts use that language.
5. The selected language is written to `config.yaml` as the `language` field.
6. No system language auto-detection — the user always makes an explicit choice.

---

## Architecture

```
runConfig()
  └─ 1. loadExistingConfig(path)         ← reads existing config.yaml (incl. language field)
  └─ 2. runLanguageStep(existingLang)    ← NEW: bilingual survey.Select prompt
         │  returns "en" or "zh"
         └─ i18n.Load(lang)              ← reloads translations immediately
  └─ 3. Step 1: Claude config            ← unchanged, uses i18n.M.Prompt.*
  └─ 4. Step 2: Platform config          ← unchanged
  └─ 5. Step 3: Auth/Advanced config     ← unchanged
  └─ 6. Step 4: Write config.yaml        ← vals.Language is set, written via template
```

`lang.go`'s `detectLang()` is not modified. It initializes i18n at startup from any pre-existing `config.yaml`. `runLanguageStep` overwrites that with the user's explicit selection mid-command.

---

## Implementation Details

### 1. `configValues` struct — add `Language` field

**File:** `cmd/openbee/config.go`

```go
type configValues struct {
    Language string  // "en" or "zh"
    // ... existing fields unchanged
}
```

### 2. `loadExistingConfig()` — populate `Language` from existing config

**File:** `cmd/openbee/config.go`

```go
return &configValues{
    Language: cfg.Language,  // add this line
    // ... existing fields unchanged
}
```

### 3. `runLanguageStep()` — new function

**File:** `cmd/openbee/config.go`

```go
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

### 4. `runConfig()` — call `runLanguageStep` after loading existing config

**File:** `cmd/openbee/config.go`

```go
func runConfig(cmd *cobra.Command, args []string) error {
    vals := configValues{ /* defaults */ }

    if existing := loadExistingConfig(configOutputPath); existing != nil {
        fmt.Printf(i18n.M.Output.Config.FoundExisting+"\n", configOutputPath)
        vals = *existing
    }

    // Language selection — always shown, before all other prompts
    lang, err := runLanguageStep(vals.Language)
    if err != nil {
        return err
    }
    vals.Language = lang

    // Step 1 — Claude config (unchanged)
    // ...
}
```

### 5. `config.yaml.tmpl` — add `language` field at top

**File:** `internal/config/config.yaml.tmpl`

```yaml
# OpenBee 配置文件
language: {{.Language}}

server:
  port: {{.ServerPort}}
  ...
```

---

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Fresh install, no config.yaml | Default selection: English |
| Existing config with `language: zh` | Default selection: 中文 |
| Existing config with `language: en` | Default selection: English |
| Existing config with no `language` field | Default selection: English (`cfg.Language` is empty string) |
| User presses Ctrl+C at language prompt | `handleSurveyErr` returns `claude.ErrInterrupted`, caught in cobra RunE, exits cleanly |

---

## Files Changed

| File | Change |
|------|--------|
| `cmd/openbee/config.go` | Add `Language` to `configValues`; populate in `loadExistingConfig`; add `runLanguageStep()`; call it in `runConfig()` |
| `internal/config/config.yaml.tmpl` | Add `language: {{.Language}}` at top |

No other files require modification. The i18n YAML translation files, `lang.go`, and `messages.go` are unchanged.

---

## Out of Scope

- System language auto-detection (removed from requirements)
- Adding new languages beyond `zh` and `en`
- Web UI language switcher (already exists separately)
