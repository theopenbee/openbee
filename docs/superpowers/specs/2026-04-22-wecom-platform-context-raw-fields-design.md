# WeCom Platform Context: Raw Field Names Design

**Date:** 2026-04-22  
**Branch:** feat/platform-context-injection  
**Status:** Approved

## Problem

The current `ExtractContext` in `internal/platform/wecom/handler.go` renames and remaps fields before passing them to AI workers:

- `from.userid` is flattened into `userid` (loses nested structure)
- `chatid` is overridden with `from.userid` in single-chat scenarios (business logic leak)
- `msgtype` is not included at all

This makes the AI-facing context diverge from the actual WeCom wire format, requiring workers to know about the remapping.

## Goal

Pass WeCom platform context to AI workers using the **original field names and structure** from the WeCom message body, controlled by an explicit whitelist.

## Whitelist

Fields passed to AI:

| Field      | Type              | Notes                              |
|------------|-------------------|------------------------------------|
| `msgid`    | string            | Unique message ID                  |
| `aibotid`  | string            | AI bot identifier                  |
| `chatid`   | string            | Group chat ID; empty for single chat |
| `chattype` | string            | `"single"` or `"group"`            |
| `from`     | object `{userid}` | Sender info, preserves nesting     |
| `msgtype`  | string            | `"text"`, `"image"`, `"file"`, etc. |

## Design

### New Struct (`internal/platform/wecom/handler.go`)

Add `wecomContextFields` alongside existing body type definitions:

```go
type wecomContextFields struct {
    MsgID    string      `json:"msgid"`
    AiBotID  string      `json:"aibotid"`
    ChatID   string      `json:"chatid"`
    ChatType string      `json:"chattype"`
    From     messageFrom `json:"from"`
    MsgType  string      `json:"msgtype"`
}
```

Reuses the existing `messageFrom` struct (`{userid string}`).

### Updated `ExtractContext`

```go
func ExtractContext(raw string) string {
    var frame WsFrame
    if err := json.Unmarshal([]byte(raw), &frame); err != nil {
        return ""
    }
    var body messageBody
    if err := json.Unmarshal(frame.Body, &body); err != nil {
        return ""
    }
    ctx := wecomContextFields{
        MsgID:    body.MsgID,
        AiBotID:  body.AiBotID,
        ChatID:   body.ChatID,
        ChatType: body.ChatType,
        From:     body.From,
        MsgType:  body.MsgType,
    }
    b, _ := json.Marshal(map[string]any{"wecom": ctx})
    return string(b)
}
```

Key changes from current implementation:
- Removes the `chatID` computation that overrides single-chat `chatid` with `userid`
- Switches from `platform.BuildPlatformContext` (supports `map[string]string` only) to direct `json.Marshal`, consistent with the DingTalk pattern
- Adds `msgtype` to output
- Preserves `from` as a nested object instead of flattening

## Output Examples

**Group chat:**
```json
{
  "wecom": {
    "msgid": "4b4dff171a1892589c0df78ba9761d24",
    "aibotid": "aibGVm7KiVeSbYuuX-kZ00jsLZOXiU6lfyK",
    "chatid": "wrk6UuEAAAO6bKxkLsG__FZxibxyAt-g",
    "chattype": "group",
    "from": {"userid": "TengYongZhi"},
    "msgtype": "text"
  }
}
```

**Single chat:**
```json
{
  "wecom": {
    "msgid": "4b4dff171a1892589c0df78ba9761d24",
    "aibotid": "aibGVm7KiVeSbYuuX-kZ00jsLZOXiU6lfyK",
    "chatid": "",
    "chattype": "single",
    "from": {"userid": "TengYongZhi"},
    "msgtype": "text"
  }
}
```

## Test Changes

`internal/platform/wecom/handler_test.go` — update `TestExtractContext_ValidWeComRaw`:

- Assert `"from"` key exists in output JSON
- Assert `"userid"` exists nested inside `from`
- Assert `"msgtype"` field is present
- Assert single-chat `chatid` is empty string (not overridden with userid)
- Add group-chat test case verifying `chatid` is the group ID

## Scope

| File | Change |
|------|--------|
| `internal/platform/wecom/handler.go` | Add `wecomContextFields` struct; rewrite `ExtractContext` |
| `internal/platform/wecom/handler_test.go` | Update/extend `ExtractContext` tests |

No interface changes. No other platforms affected. `platform.BuildPlatformContext` is not removed — it remains available for other callers.
