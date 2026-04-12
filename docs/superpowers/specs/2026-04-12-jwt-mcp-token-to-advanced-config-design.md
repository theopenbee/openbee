# Design: Migrate JWT Secret and MCP Token Secret to Advanced Config

**Date:** 2026-04-12
**Branch:** dev

---

## Background

The `openbee config` interactive wizard currently handles JWT Secret in Step 3 (Web Authentication) and MCP Token Secret as a standalone block after Step 4 (Advanced Config). Both are security-sensitive secrets that most users never need to manually manage — they should be auto-generated silently by default, and only exposed to users who opt into advanced configuration.

---

## Goal

Move the JWT Secret prompt and MCP Token Secret prompt into the Advanced Config section (Step 4), so that:

- Ordinary users who skip advanced config never see these prompts
- Power users who enter advanced config can inspect and regenerate them
- Silent auto-generation remains the fallback for users who skip advanced config

---

## Scope

- **In scope:** `cmd/openbee/config.go` — `runConfig` function
- **Out of scope:** `AuthAccessTokenTTL`, `AuthRefreshTokenTTL` (remain as defaults, not exposed in wizard)

---

## Current Flow

```
Step 1: Engine config
Step 2: Platform config
Step 3: Web Authentication
  - Username
  - Password
  - JWT Secret (prompt if existing; auto-generate if new)   ← MOVE
Step 4: Advanced Config (gated by confirm)
  - Server port, host, debug
  - DB path
  - Feeder timeout, max concurrent bee, message debounce
  - FFprobe path, FFmpeg path
[standalone] MCP Token Secret                               ← MOVE
[standalone] Confirm write
```

## Target Flow

```
Step 1: Engine config
Step 2: Platform config
Step 3: Web Authentication
  - Username
  - Password
  (JWT Secret removed from here)
Step 4: Advanced Config (gated by confirm)
  → User selects Yes:
      - Server port, host, debug
      - DB path
      - Feeder timeout, max concurrent bee, message debounce
      - FFprobe path, FFmpeg path
      - JWT Secret (prompt if existing; auto-generate if new)   ← NEW
      - MCP Token Secret (prompt if existing; auto-generate)    ← MOVED
  → User selects No (skip):
      - JWT Secret: silently auto-generate if empty, keep if existing
      - MCP Token Secret: silently auto-generate if empty, keep if existing
Step 5: Confirm write
```

---

## Behavior Specification

### JWT Secret

| Scenario | Old behavior | New behavior |
|---|---|---|
| First run, no existing config | Auto-generate, print confirmation | Same, but silent (no print) when advanced is skipped |
| Existing config, user enters advanced | Prompt "regenerate?" in Step 3 | Prompt "regenerate?" inside advanced block |
| Existing config, user skips advanced | N/A (always prompted) | Keep existing value silently |
| First run, user enters advanced | Auto-generate, print confirmation | Auto-generate, print confirmation (same) |

### MCP Token Secret

| Scenario | Old behavior | New behavior |
|---|---|---|
| First run, no existing config | Auto-generate, print with value | Same, but only when advanced is entered |
| Existing config | Prompt "regenerate?" | Same, but inside advanced block |
| First run, user skips advanced | Auto-generate (always ran) | Auto-generate silently (no print) |
| Existing config, user skips advanced | Prompt "regenerate?" (always ran) | Keep existing value silently |

---

## Code Changes (`cmd/openbee/config.go`)

### 1. Remove JWT Secret block from Step 3 (lines ~447–466)

Delete the following from the Web Authentication section:

```go
if vals.AuthJWTSecret != "" {
    // ... regenerate confirm prompt
} else {
    // ... auto-generate and print
}
```

### 2. Add JWT Secret + MCP Token Secret inside advanced block

Append to the end of `if customAdvanced { ... }`:

```go
// JWT Secret
if vals.AuthJWTSecret != "" {
    // prompt: regenerate?
} else {
    // auto-generate, print
}

// MCP Token Secret
if vals.MCPTokenSecret != "" {
    // prompt: regenerate?
} else {
    // auto-generate, print
}
```

### 3. Add silent fallback after advanced block

After the `if customAdvanced { ... }` block (for users who skip):

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

### 4. Remove standalone MCP Token Secret block (lines ~559–574)

The original standalone MCP token block (outside the advanced `if`) is deleted, replaced by the logic in steps 2 and 3 above.

---

## Non-Goals

- No changes to config template (`config.yaml` output structure unchanged)
- No changes to i18n keys (existing prompts reused)
- No changes to `AuthAccessTokenTTL` / `AuthRefreshTokenTTL` (still defaults only)

---

## Testing

Manual test matrix:

1. **Fresh run, skip advanced** → JWT Secret and MCP Token Secret silently generated, no prompts shown
2. **Fresh run, enter advanced** → Both secrets generated with printed confirmation inside advanced section
3. **Existing config with secrets, skip advanced** → Existing secrets preserved, no prompts
4. **Existing config with secrets, enter advanced** → Prompts to regenerate both secrets appear
5. **Existing config without secrets, enter advanced** → Secrets auto-generated with confirmation prints
