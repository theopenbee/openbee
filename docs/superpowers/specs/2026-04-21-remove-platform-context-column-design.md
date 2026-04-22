# Design: Remove `platform_context` Column, Extract from `raw` On Demand

**Date:** 2026-04-21  
**Branch:** feat/platform-context-injection  
**Status:** Approved

---

## Problem

The `bee_platform_messages` table has a `platform_context` column (added in migration 41) that stores a curated subset of fields from the platform event (e.g., `open_id`, `chat_id`). These fields are already present in the `raw` column, which stores the full Webhook payload. The `platform_context` column is therefore redundant storage.

## Goal

Remove the `platform_context` DB column. Extract platform-specific context from `raw` on demand at the feeder (bee) and dispatcher (worker) layers, using platform-registered extractor functions.

---

## Architecture

### 1. Database

- **Revert migration 41**: Delete the migration 41 entry (`add_platform_context_to_platform_messages`) from `db.go`. No new migration is added because the branch is still in development and has not been merged.

### 2. Message Store (`internal/infra/store/message_store.go`)

- **`ClaimBatch` SQL**: Replace `platform_context` with `raw` in the SELECT:
  ```sql
  SELECT id, session_key, platform, content, raw
  FROM bee_platform_messages m WHERE ...
  ```

- **`ClaimedMessage` struct**: Remove `PlatformContext string`, add `Raw string`:
  ```go
  type ClaimedMessage struct {
      ID         string
      SessionKey string
      Platform   string
      Content    string
      Raw        string
  }
  ```

- **`CreateBatch`**: Remove `platform_context` from the INSERT column list and its corresponding argument.

### 3. Platform Package (`internal/platform`)

Add an extractor registry to `internal/platform/context.go`:

```go
var extractors = map[string]func(string) string{}

func RegisterExtractor(name string, fn func(string) string) {
    extractors[name] = fn
}

// ExtractContext returns platform-native fields as a JSON string.
// Returns "" if no extractor is registered for the platform or raw is empty.
func ExtractContext(platformName, raw string) string {
    if fn, ok := extractors[platformName]; ok {
        return fn(raw)
    }
    return ""
}
```

`BuildPlatformContext` (the JSON encoding helper) remains in `context.go` unchanged.

### 4. Platform Handlers

Each platform handler:

- Renames its private `buildXxxContext` function to an exported `ExtractContext(raw string) string`.
- The new function unmarshals the raw event JSON and calls `BuildPlatformContext` with the same fields as before.
- Removes the code that sets `InboundMessage.PlatformContext`.

Affected files:
- `internal/platform/feishu/handler.go` — `buildFeishuContext` → `ExtractContext`
- `internal/platform/dingtalk/handler.go` — `buildDingTalkContext` → `ExtractContext`
- `internal/platform/wecom/handler.go` — `buildWeComContext` → `ExtractContext`

### 5. `InboundMessage` (`internal/platform/interfaces.go`)

Remove the `PlatformContext string` field. It was only used to carry the pre-extracted context from the handler to the store; with the store no longer persisting it, the field has no purpose.

### 6. Server Startup (`internal/app/app.go`)

In `buildPlatforms`, register each platform's extractor alongside its adapter creation:

```go
platform.RegisterExtractor("feishu", feishu.ExtractContext)
platform.RegisterExtractor("dingtalk", dingtalk.ExtractContext)
platform.RegisterExtractor("wecom", wecom.ExtractContext)
```

Telegram and Weixin do not have structured context extraction today; they are not registered, and `platform.ExtractContext` returns `""` for unregistered platforms (safe no-op).

### 7. Feeder (`internal/domain/bee/feeder.go`)

Replace the `m.PlatformContext` reference with an on-demand extraction:

```go
if ctx := platform.ExtractContext(m.Platform, m.Raw); ctx != "" {
    meta.PlatformContext = json.RawMessage(ctx)
}
```

Add `internal/platform` to the import list.

### 8. Dispatcher (`internal/domain/task/dispatcher.go`)

Replace the `t.ReplyTo.PlatformContext` reference:

```go
if ctx := platform.ExtractContext(t.ReplyTo.Platform, t.ReplyTo.Raw); ctx != "" {
    meta.PlatformContext = json.RawMessage(ctx)
}
```

Add `internal/platform` to the import list.

---

## Data Flow (After Change)

```
Platform event arrives
  → Handler parses event, builds InboundMessage{Raw: "<full payload>"}
  → Store saves raw to bee_platform_messages (no platform_context column)

ClaimBatch runs
  → Selects id, session_key, platform, content, raw
  → Returns ClaimedMessage{Raw: "<full payload>"}

Feeder/Dispatcher processes ClaimedMessage
  → Calls platform.ExtractContext(m.Platform, m.Raw)
  → Registered extractor parses raw, returns curated JSON
  → Injects into message_meta / task_meta as platform_context
```

---

## Trade-offs

| Factor | Impact |
|---|---|
| Storage | Removes redundant `platform_context` column |
| ClaimBatch payload | Slightly larger per message (`raw` vs curated fields), but `raw` is already in DB |
| Extraction timing | Moved from ingest time to claim time; happens every ClaimBatch call |
| Coupling | Feeder/dispatcher gain a dependency on `internal/platform` (acceptable) |
| Extensibility | Adding a new platform only requires calling `RegisterExtractor`; feeder/dispatcher unchanged |

---

## Files Changed

| File | Change |
|---|---|
| `internal/infra/store/db.go` | Remove migration 41 entry |
| `internal/infra/store/message_store.go` | ClaimBatch SQL, ClaimedMessage struct, CreateBatch args |
| `internal/platform/context.go` | Add `RegisterExtractor` + `ExtractContext` |
| `internal/platform/interfaces.go` | Remove `PlatformContext` from `InboundMessage` |
| `internal/platform/feishu/handler.go` | Export `ExtractContext`, remove `InboundMessage.PlatformContext` assignment |
| `internal/platform/dingtalk/handler.go` | Same |
| `internal/platform/wecom/handler.go` | Same |
| `internal/domain/bee/feeder.go` | Use `platform.ExtractContext` |
| `internal/domain/task/dispatcher.go` | Use `platform.ExtractContext` |
| `internal/app/app.go` | Register feishu/dingtalk/wecom extractors in `buildPlatforms` |
