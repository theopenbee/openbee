# Design: Feishu @Mention Resolution at Ingest Time

**Date:** 2026-04-08  
**Status:** Approved  
**Scope:** `internal/platform/feishu/handler.go`

---

## Problem

In Feishu text messages, `@mentions` are stored as opaque keys (e.g. `@_user_1`) inside `message.content`. The `mentions` array on the same event provides the key-to-name mapping (e.g. `{"key": "@_user_1", "name": "Tom"}`). The current system stores content verbatim, so downstream consumers (the AI bee and any UI) see `@_user_1` instead of `@Tom`.

Post-type messages already resolve mentions correctly via the `at` element's `user_name` field during rich-text parsing. This design closes the gap for text-type messages and any other message types that may appear in the future.

---

## Goals

- Replace `@_user_N` mention keys with `@<display name>` before the message is stored or dispatched.
- Apply the replacement for all message types in a single location so new types are covered automatically.
- Preserve the original raw event in the `raw` column for auditability.

---

## Non-Goals

- Changing the database schema.
- Modifying post-message mention handling (already correct).
- Replacing mentions in the `raw` field (raw always reflects the original event).

---

## Design

### New helper function: `resolveMentions`

Added as a private function in `internal/platform/feishu/handler.go`:

```go
func resolveMentions(text string, mentions []*larkim.MentionEvent) string {
    for _, m := range mentions {
        if m.Key == nil || m.Name == nil {
            continue
        }
        text = strings.ReplaceAll(text, *m.Key, "@"+*m.Name)
    }
    return text
}
```

- Iterates over the `mentions` slice from the Feishu event.
- Replaces each `key` occurrence in `text` with `@name`.
- Skips entries where `Key` or `Name` is nil (defensive nil-check).
- Keys with no matching entry are left unchanged (information-preserving).

### Call site in `Start()`

After the `switch msgType` block and before `dispatch`, add one line:

```go
textContent = resolveMentions(textContent, msg.Mentions)
```

This single call site covers all message types—text, image, audio, video, post, etc.—without requiring per-type changes.

---

## Behavior Table

| Scenario | Input `content` | `mentions` | Result |
|---|---|---|---|
| Normal replacement | `"@_user_1 hello"` | `[{key: "@_user_1", name: "Tom"}]` | `"@Tom hello"` |
| Partial mapping | `"@_user_1 @_user_2"` | only `_user_1` mapped | `"@Tom @_user_2"` |
| Empty mentions | any text | `[]` | unchanged |
| Post message | no `@_user_N` keys | any | no-op (no matches) |
| Nil key or name | — | entry with nil field | entry skipped |

---

## Files Changed

| File | Change |
|---|---|
| `internal/platform/feishu/handler.go` | Add `resolveMentions` function; call it before `dispatch` |

No other files are modified.

---

## Testing

- Add unit tests for `resolveMentions` covering: normal replacement, partial mapping, empty mentions, nil key/name entries.
- Existing handler tests remain unaffected (they use mocked events without mentions).
