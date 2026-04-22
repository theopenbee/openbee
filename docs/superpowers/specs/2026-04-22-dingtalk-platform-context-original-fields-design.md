# DingTalk Platform Context: Use Original Field Names and Structure

**Date:** 2026-04-22  
**Branch:** feat/platform-context-injection  
**Status:** Approved

## Problem

The current `ExtractContext` in `internal/platform/dingtalk/handler.go` translates DingTalk's original camelCase field names to snake_case and coerces types (e.g., `isAdmin: bool` → `"true"/"false": string`). This loses fidelity and makes it harder for the AI to correlate values with DingTalk API documentation. Additionally, `atUsers` (an array) and several other fields are absent entirely.

## Goal

Pass a whitelist-filtered subset of the raw DingTalk message to the AI, preserving original field names, original value types, and the original nested structure.

## Whitelist

The following 13 fields are included; all others are dropped:

```
conversationId, atUsers, chatbotCorpId, chatbotUserId, msgId,
senderNick, isAdmin, senderStaffId, senderCorpId, conversationType,
senderId, conversationTitle, msgtype
```

## Design

### Approach: Dynamic JSON Filter (Approach A)

Unmarshal raw into `map[string]any`, filter by whitelist, re-marshal wrapped in `{"dingtalk": ...}`.

**Why not Approach B (struct)?** A dedicated struct requires maintaining a parallel type alongside `BotCallbackDataModel`; it also needs a nested `AtUser` struct. Adding/removing whitelist fields requires struct changes.

**Why not Approach C (extend BuildPlatformContext)?** `BuildPlatformContext` is shared with Feishu and accepts `map[string]string`. Changing the signature requires updating all callers and their tests for no benefit specific to this feature.

### Implementation

**File:** `internal/platform/dingtalk/handler.go`

```go
var dingtalkContextWhitelist = map[string]bool{
    "conversationId":    true,
    "atUsers":           true,
    "chatbotCorpId":     true,
    "chatbotUserId":     true,
    "msgId":             true,
    "senderNick":        true,
    "isAdmin":           true,
    "senderStaffId":     true,
    "senderCorpId":      true,
    "conversationType":  true,
    "senderId":          true,
    "conversationTitle": true,
    "msgtype":           true,
}

func ExtractContext(raw string) string {
    var all map[string]any
    if err := json.Unmarshal([]byte(raw), &all); err != nil {
        return ""
    }
    filtered := make(map[string]any, len(dingtalkContextWhitelist))
    for k, v := range all {
        if dingtalkContextWhitelist[k] {
            filtered[k] = v
        }
    }
    b, _ := json.Marshal(map[string]any{"dingtalk": filtered})
    return string(b)
}
```

**Removed dependencies:** `strconv`, `chatbot.BotCallbackDataModel` (in ExtractContext), `platform.BuildPlatformContext` (in ExtractContext).

### Output Example

Given the raw DingTalk message from the task brief, the output is:

```json
{
  "dingtalk": {
    "conversationId": "cidsBVlnfigWBOLZQUwcG/QmA==",
    "atUsers": [
      {"dingtalkId": "$:LWCP_v1:$DOMz9+dvTpykALvQPInrVEmf6FB0mtYz", "staffId": ""},
      {"dingtalkId": "$:LWCP_v1:$RHJfrStLVEbA1L4sJJq0uA==", "staffId": "181440596621465183"},
      {"dingtalkId": "$:LWCP_v1:$TehUn2UU8Ia0ycIKXDaqp1SpLRFpEAaT", "staffId": ""}
    ],
    "chatbotCorpId": "dinga4232db39b38f741f2c783f7214b6d69",
    "chatbotUserId": "$:LWCP_v1:$TehUn2UU8Ia0ycIKXDaqp1SpLRFpEAaT",
    "msgId": "msg38bk6muVK1nPDBy8cm1YZg==",
    "senderNick": "滕勇志",
    "isAdmin": true,
    "senderStaffId": "060139276627947909",
    "senderCorpId": "dinga4232db39b38f741f2c783f7214b6d69",
    "conversationType": "2",
    "senderId": "$:LWCP_v1:$gx1Ryszf6ncHEdcZ5th2Qw==",
    "conversationTitle": "robobee测试群",
    "msgtype": "text"
  }
}
```

## Testing

**File:** `internal/platform/dingtalk/handler_test.go`

- `TestExtractContext_ValidDingTalkRaw`: Update fixture to include `atUsers`, `senderId`, `chatbotUserId`, `msgtype`. Update assertions to verify:
  - Original field names are present (e.g., `conversationId`, not `conversation_id`)
  - `isAdmin` is a JSON boolean (`true`/`false`), not a string
  - `atUsers` is a JSON array
  - Excluded fields (e.g., `sessionWebhook`, `createAt`) are absent
- `TestExtractContext_InvalidDingTalkRaw`: No change needed.

## Files Changed

| File | Change |
|------|--------|
| `internal/platform/dingtalk/handler.go` | Rewrite `ExtractContext`; add `dingtalkContextWhitelist`; remove `strconv` import |
| `internal/platform/dingtalk/handler_test.go` | Update fixture and assertions for `TestExtractContext_ValidDingTalkRaw` |

No other files are modified.
