# DingTalk Platform Context Original Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current translated/typed-down DingTalk platform context with a whitelist-filtered subset of the raw message that preserves original camelCase field names, original value types (bool, array), and original nested structure.

**Architecture:** `ExtractContext` unmarshals raw JSON into `map[string]any`, filters by a package-level whitelist constant, and re-marshals into `{"dingtalk": <filtered>}`. No changes to `BuildPlatformContext`, Feishu, or any other caller.

**Tech Stack:** Go 1.21+, `encoding/json`, `github.com/stretchr/testify/assert`

**Spec:** `docs/superpowers/specs/2026-04-22-dingtalk-platform-context-original-fields-design.md`

---

### Task 1: Update ExtractContext — TDD

**Files:**
- Modify: `internal/platform/dingtalk/handler_test.go:132-148`
- Modify: `internal/platform/dingtalk/handler.go:33-48`

#### Step 1: Write failing tests

Replace the existing `TestExtractContext_ValidDingTalkRaw` test (lines 132-141 in `handler_test.go`) with two more precise tests:

```go
func TestExtractContext_ValidDingTalkRaw(t *testing.T) {
	raw := `{
		"conversationId": "conv1",
		"atUsers": [
			{"dingtalkId": "dt001", "staffId": "s001"},
			{"dingtalkId": "dt002", "staffId": ""}
		],
		"chatbotCorpId": "botcorp1",
		"chatbotUserId": "botuser1",
		"msgId": "msg1",
		"senderNick": "Alice",
		"isAdmin": true,
		"senderStaffId": "emp001",
		"senderCorpId": "corp1",
		"conversationType": "2",
		"senderId": "sender1",
		"conversationTitle": "Test Group",
		"msgtype": "text",
		"sessionWebhook": "https://should-be-excluded.example.com",
		"createAt": 1700000000000
	}`

	got := ExtractContext(raw)
	assert.NotEmpty(t, got)

	var wrapper map[string]any
	assert.NoError(t, json.Unmarshal([]byte(got), &wrapper))

	ctx, ok := wrapper["dingtalk"].(map[string]any)
	assert.True(t, ok, "expected dingtalk key with object value")

	// Whitelist fields present with original names
	assert.Equal(t, "conv1", ctx["conversationId"])
	assert.Equal(t, "botcorp1", ctx["chatbotCorpId"])
	assert.Equal(t, "botuser1", ctx["chatbotUserId"])
	assert.Equal(t, "msg1", ctx["msgId"])
	assert.Equal(t, "Alice", ctx["senderNick"])
	assert.Equal(t, "emp001", ctx["senderStaffId"])
	assert.Equal(t, "corp1", ctx["senderCorpId"])
	assert.Equal(t, "2", ctx["conversationType"])
	assert.Equal(t, "sender1", ctx["senderId"])
	assert.Equal(t, "Test Group", ctx["conversationTitle"])
	assert.Equal(t, "text", ctx["msgtype"])

	// isAdmin is bool, not string
	isAdmin, ok := ctx["isAdmin"].(bool)
	assert.True(t, ok, "isAdmin should be bool")
	assert.True(t, isAdmin)

	// atUsers is array
	atUsers, ok := ctx["atUsers"].([]any)
	assert.True(t, ok, "atUsers should be array")
	assert.Len(t, atUsers, 2)
	first := atUsers[0].(map[string]any)
	assert.Equal(t, "dt001", first["dingtalkId"])
	assert.Equal(t, "s001", first["staffId"])

	// Excluded fields must not appear
	assert.NotContains(t, ctx, "sessionWebhook")
	assert.NotContains(t, ctx, "createAt")
}

func TestExtractContext_InvalidDingTalkRaw(t *testing.T) {
	got := ExtractContext("not-json")
	assert.Empty(t, got)
}
```

- [ ] **Step 1: Replace the two existing ExtractContext tests with the code above**

  In `handler_test.go`, replace lines 132-148 with the two test functions above.
  Also ensure the import block includes `"encoding/json"` and `"github.com/stretchr/testify/assert"` (both already present).
  Remove the `"strings"` import if it is no longer used after the replacement.

- [ ] **Step 2: Run tests to verify they fail**

  ```bash
  go test ./internal/platform/dingtalk/... -run TestExtractContext -v
  ```

  Expected: `TestExtractContext_ValidDingTalkRaw` FAIL — current output has snake_case keys and `isAdmin` as string, does not include `atUsers`.

- [ ] **Step 3: Rewrite ExtractContext in handler.go**

  Replace lines 33-48 of `internal/platform/dingtalk/handler.go` with:

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

- [ ] **Step 4: Clean up unused imports in handler.go**

  Remove `"strconv"` from the import block (it is only used by the old ExtractContext).
  Verify `"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"` is still used elsewhere in the file (it is — `BotCallbackDataModel` appears in other functions). Keep it.
  `"github.com/theopenbee/openbee/internal/platform"` is still used by `platform.RegisterExtractor` and others. Keep it.

- [ ] **Step 5: Run tests to verify they pass**

  ```bash
  go test ./internal/platform/dingtalk/... -run TestExtractContext -v
  ```

  Expected:
  ```
  --- PASS: TestExtractContext_ValidDingTalkRaw
  --- PASS: TestExtractContext_InvalidDingTalkRaw
  PASS
  ```

- [ ] **Step 6: Run full package tests**

  ```bash
  go test ./internal/platform/dingtalk/... -v
  ```

  Expected: all tests PASS.

- [ ] **Step 7: Run full build to catch import issues**

  ```bash
  go build ./...
  ```

  Expected: no errors.

- [ ] **Step 8: Commit**

  ```bash
  git add internal/platform/dingtalk/handler.go internal/platform/dingtalk/handler_test.go
  git commit -m "feat(dingtalk): use original field names and structure in platform context"
  ```
