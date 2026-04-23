# WeCom Platform Context Raw Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the remapped/flattened WeCom platform context with the original WeCom wire-format field names and structure, controlled by an explicit whitelist.

**Architecture:** Add a `wecomContextFields` typed struct in `handler.go` mirroring the WeCom wire format, then rewrite `ExtractContext` to populate it directly from the parsed `messageBody` — removing the `chatID` override logic and switching from `platform.BuildPlatformContext` to direct `json.Marshal` (consistent with the DingTalk pattern).

**Tech Stack:** Go, `encoding/json`, existing `messageBody` / `messageFrom` types in `internal/platform/wecom/handler.go`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/platform/wecom/handler.go` | Modify | Add `wecomContextFields` struct; rewrite `ExtractContext` |
| `internal/platform/wecom/handler_test.go` | Modify | Update/extend `ExtractContext` tests |

---

### Task 1: Update tests to reflect new contract

**Files:**
- Modify: `internal/platform/wecom/handler_test.go:435-453`

- [ ] **Step 1: Replace `TestExtractContext_ValidWeComRaw` with two focused tests**

Open `internal/platform/wecom/handler_test.go` and replace the existing `TestExtractContext_ValidWeComRaw` and `TestExtractContext_InvalidWeComRaw` with the following four tests:

```go
func TestExtractContext_SingleChat(t *testing.T) {
	body := `{"msgid":"msg1","aibotid":"bot1","chatid":"","chattype":"single","from":{"userid":"user1"},"msgtype":"text","create_time":1700000000}`
	frame := `{"cmd":"aibot_callback","headers":{"req_id":"req1"},"body":` + body + `}`
	got := ExtractContext(frame)
	if got == "" {
		t.Fatal("expected non-empty context")
	}
	// from must be a nested object, not flattened
	if !strings.Contains(got, `"from"`) {
		t.Errorf("expected 'from' key in context, got: %q", got)
	}
	if !strings.Contains(got, `"userid":"user1"`) {
		t.Errorf("expected userid inside from, got: %q", got)
	}
	// single chat: chatid must be empty string, NOT overridden with userid
	if !strings.Contains(got, `"chatid":""`) {
		t.Errorf("expected empty chatid for single chat, got: %q", got)
	}
	// msgtype must be present
	if !strings.Contains(got, `"msgtype":"text"`) {
		t.Errorf("expected msgtype in context, got: %q", got)
	}
	// userid must NOT appear as a top-level key (old flattened field)
	if strings.Contains(got, `"userid":"user1","`) || strings.HasPrefix(got, `{"wecom":{"userid"`) {
		t.Errorf("userid should not be a top-level context field, got: %q", got)
	}
}

func TestExtractContext_GroupChat(t *testing.T) {
	body := `{"msgid":"msg2","aibotid":"bot1","chatid":"group1","chattype":"group","from":{"userid":"user1"},"msgtype":"text","create_time":1700000000}`
	frame := `{"cmd":"aibot_callback","headers":{"req_id":"req1"},"body":` + body + `}`
	got := ExtractContext(frame)
	if got == "" {
		t.Fatal("expected non-empty context")
	}
	// group chat: chatid must be the group ID
	if !strings.Contains(got, `"chatid":"group1"`) {
		t.Errorf("expected group chatid, got: %q", got)
	}
	if !strings.Contains(got, `"chattype":"group"`) {
		t.Errorf("expected chattype group, got: %q", got)
	}
}

func TestExtractContext_InvalidRaw(t *testing.T) {
	got := ExtractContext("not-json")
	if got != "" {
		t.Errorf("expected empty string for invalid raw, got %q", got)
	}
}

func TestExtractContext_InvalidBody(t *testing.T) {
	// Valid WsFrame but body is not a messageBody
	frame := `{"cmd":"aibot_callback","headers":{"req_id":"req1"},"body":"not-an-object"}`
	got := ExtractContext(frame)
	if got != "" {
		t.Errorf("expected empty string for invalid body, got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/platform/wecom/... -run "TestExtractContext" -v
```

Expected: `TestExtractContext_SingleChat` and `TestExtractContext_GroupChat` FAIL (wrong output from current implementation). `TestExtractContext_InvalidRaw` may pass. `TestExtractContext_InvalidBody` may fail.

- [ ] **Step 3: Commit the failing tests**

```bash
git add internal/platform/wecom/handler_test.go
git commit -m "test(wecom): update ExtractContext tests for raw field contract"
```

---

### Task 2: Implement `wecomContextFields` and rewrite `ExtractContext`

**Files:**
- Modify: `internal/platform/wecom/handler.go:29-49` (ExtractContext and body types section)

- [ ] **Step 1: Add `wecomContextFields` struct**

In `internal/platform/wecom/handler.go`, add the new struct after the `quoteContent` type (around line 128), before the outbound body types section:

```go
// wecomContextFields is the whitelist of fields passed to AI workers as platform context.
// Field names match the WeCom wire format exactly.
type wecomContextFields struct {
	MsgID    string      `json:"msgid"`
	AiBotID  string      `json:"aibotid"`
	ChatID   string      `json:"chatid"`
	ChatType string      `json:"chattype"`
	From     messageFrom `json:"from"`
	MsgType  string      `json:"msgtype"`
}
```

- [ ] **Step 2: Rewrite `ExtractContext`**

Replace the existing `ExtractContext` function (lines 29-49) with:

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

- [ ] **Step 3: Run tests to verify they pass**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/platform/wecom/... -run "TestExtractContext" -v
```

Expected: All four `TestExtractContext_*` tests PASS.

- [ ] **Step 4: Run the full wecom package tests**

```bash
go test ./internal/platform/wecom/... -v
```

Expected: All tests PASS. No compilation errors.

- [ ] **Step 5: Run the full test suite**

```bash
go test ./... 2>&1 | tail -20
```

Expected: All packages PASS (or pre-existing failures only — no new failures introduced by this change).

- [ ] **Step 6: Commit the implementation**

```bash
git add internal/platform/wecom/handler.go
git commit -m "feat(wecom): pass raw field names in platform context

Use wecomContextFields typed struct to preserve original WeCom wire
format field names (msgid, aibotid, chatid, chattype, from, msgtype).
Removes chatid override logic for single chats and adds msgtype.
Consistent with DingTalk's typed struct pattern."
```
