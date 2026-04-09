# Feishu @Mention Resolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Feishu mention keys (e.g. `@_user_1`) with display names (e.g. `@Tom`) in message content before dispatch, so downstream AI and UI consumers see human-readable names.

**Architecture:** Add a private `resolveMentions` function to `internal/platform/feishu/handler.go` that performs string replacement using the `Mentions` slice already present on the Feishu event message. Call it once after the `switch msgType` block and before `dispatch`, covering all current and future message types in a single location.

**Tech Stack:** Go, `github.com/larksuite/oapi-sdk-go/v3` (`larkim.MentionEvent`), standard `strings` package (already imported).

---

### Task 1: Add `resolveMentions` with tests

**Files:**
- Modify: `internal/platform/feishu/handler_test.go`
- Modify: `internal/platform/feishu/handler.go`

- [ ] **Step 1: Write the failing test in `handler_test.go`**

Append the following test after the existing `TestFeishuMediaMsgType` function:

```go
func TestResolveMentions(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name     string
		text     string
		mentions []*larkim.MentionEvent
		want     string
	}{
		{
			name: "single mention replaced",
			text: "@_user_1 hello",
			mentions: []*larkim.MentionEvent{
				{Key: strPtr("@_user_1"), Name: strPtr("Tom")},
			},
			want: "@Tom hello",
		},
		{
			name: "multiple mentions replaced",
			text: "@_user_1 and @_user_2",
			mentions: []*larkim.MentionEvent{
				{Key: strPtr("@_user_1"), Name: strPtr("Tom")},
				{Key: strPtr("@_user_2"), Name: strPtr("Alice")},
			},
			want: "@Tom and @Alice",
		},
		{
			name: "unknown key preserved",
			text: "@_user_1 @_user_2",
			mentions: []*larkim.MentionEvent{
				{Key: strPtr("@_user_1"), Name: strPtr("Tom")},
			},
			want: "@Tom @_user_2",
		},
		{
			name:     "empty mentions no change",
			text:     "@_user_1 hello",
			mentions: nil,
			want:     "@_user_1 hello",
		},
		{
			name: "nil key skipped",
			text: "@_user_1 hello",
			mentions: []*larkim.MentionEvent{
				{Key: nil, Name: strPtr("Tom")},
				{Key: strPtr("@_user_1"), Name: strPtr("Bob")},
			},
			want: "@Bob hello",
		},
		{
			name: "nil name skipped",
			text: "@_user_1 hello",
			mentions: []*larkim.MentionEvent{
				{Key: strPtr("@_user_1"), Name: nil},
			},
			want: "@_user_1 hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveMentions(tt.text, tt.mentions)
			if got != tt.want {
				t.Errorf("resolveMentions() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

Add the import for `larkim` to `handler_test.go`'s import block:

```go
import (
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/platform/feishu/... -run TestResolveMentions -v
```

Expected output: compile error — `resolveMentions` undefined.

- [ ] **Step 3: Implement `resolveMentions` in `handler.go`**

Append the following function at the end of `internal/platform/feishu/handler.go` (after the last closing brace):

```go
// resolveMentions replaces mention keys (e.g. "@_user_1") in text with
// "@<display name>" using the mentions slice from the Feishu event.
// Keys with no corresponding mention entry are left unchanged.
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

`strings` and `larkim` are already imported in `handler.go` — no import changes needed.

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/platform/feishu/... -run TestResolveMentions -v
```

Expected output:
```
--- PASS: TestResolveMentions/single_mention_replaced
--- PASS: TestResolveMentions/multiple_mentions_replaced
--- PASS: TestResolveMentions/unknown_key_preserved
--- PASS: TestResolveMentions/empty_mentions_no_change
--- PASS: TestResolveMentions/nil_key_skipped
--- PASS: TestResolveMentions/nil_name_skipped
PASS
```

- [ ] **Step 5: Run all feishu tests to confirm no regressions**

```bash
go test ./internal/platform/feishu/... -v
```

Expected: all existing tests still PASS.

---

### Task 2: Wire `resolveMentions` into the message dispatch path

**Files:**
- Modify: `internal/platform/feishu/handler.go` (lines ~104–108)

- [ ] **Step 1: Add the call site after the switch block**

In `internal/platform/feishu/handler.go`, locate this section (around line 104):

```go
			if textContent == "" {
				return nil
			}
```

Insert one line immediately before the `if textContent == ""` guard:

```go
			textContent = resolveMentions(textContent, msg.Mentions)

			if textContent == "" {
				return nil
			}
```

The full context after the change looks like:

```go
			var textContent string
			switch msgType {
			case "text":
				var content map[string]string
				if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
					return nil
				}
				textContent = content["text"]
			case "image", "file", "audio", "video", "media", "sticker":
				textContent = r.resolveMediaContent(ctx, messageID, msgType, contentJSON)
			case "post":
				textContent = r.resolvePostContent(ctx, messageID, contentJSON)
			default:
				log.Warn("skipping unsupported message type", zap.String("msgType", msgType))
				return nil
			}

			textContent = resolveMentions(textContent, msg.Mentions)

			if textContent == "" {
				return nil
			}
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run the full feishu test suite**

```bash
go test ./internal/platform/feishu/... -v
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/platform/feishu/handler.go internal/platform/feishu/handler_test.go
git commit -m "feat: resolve Feishu @mention keys to display names at ingest time"
```
