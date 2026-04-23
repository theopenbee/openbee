# Remove platform_context Injection from Bee — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove `platform_context` from bee's message metadata so bee is truly platform-agnostic; worker injection is unchanged.

**Architecture:** Delete the `PlatformContext` field from `messageMeta` in `feeder.go` and remove the 3-line injection block in `buildPrompt()`. Update tests to enforce the new invariant that bee never emits `platform_context`, even when a platform extractor is registered.

**Tech Stack:** Go, `encoding/json`, internal `platform` package

---

## File Map

| Action | File |
|--------|------|
| Modify | `internal/domain/bee/feeder.go` — remove `PlatformContext` field (line 29) and injection block (lines 352-354) |
| Modify | `internal/domain/bee/feeder_internal_test.go` — delete `TestBuildPrompt_WithPlatformContext`; rename and strengthen `TestBuildPrompt_NoPlatformContext` |

---

### Task 1: Update Tests to Reflect New Invariant (TDD — Red Phase)

**Files:**
- Modify: `internal/domain/bee/feeder_internal_test.go`

- [ ] **Step 1: Delete `TestBuildPrompt_WithPlatformContext`**

Remove the entire function (lines 75–90). It asserts that `platform_context` appears in bee's output — which is now wrong behavior.

After removal, `feeder_internal_test.go` lines 75–101 should look like:

```go
func TestBuildPrompt_NeverHasPlatformContext(t *testing.T) {
	platform.RegisterExtractor("testplatform2", func(_ string) string {
		return `{"testplatform2":{"sender":{"open_id":"ou_abc"}}}`
	})
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "testplatform2", SessionKey: "testplatform2:oc_xyz:ou_abc", Content: "hello", Raw: "any-raw"},
	}
	got := buildPrompt(msgs, "")

	if strings.Contains(got, `"platform_context"`) {
		t.Errorf("platform_context must never appear in bee message_meta, got: %q", got)
	}
}
```

This replaces both old tests (`WithPlatformContext` deleted, `NoPlatformContext` replaced). The key difference from the old `NoPlatformContext` test: the platform **has a registered extractor** and **has non-empty Raw** — yet `platform_context` must still not appear.

- [ ] **Step 2: Run tests to confirm the new test fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/domain/bee/... -run TestBuildPrompt_NeverHasPlatformContext -v
```

Expected output: **FAIL** — the test should fail because `feeder.go` still injects `platform_context`.

```
--- FAIL: TestBuildPrompt_NeverHasPlatformContext (0.00s)
    feeder_internal_test.go:XX: platform_context must never appear in bee message_meta, got: ...
```

If the test passes at this point, stop and investigate — the injection logic may already be missing or `testplatform2` extractor isn't firing.

---

### Task 2: Remove platform_context from feeder.go (Green Phase)

**Files:**
- Modify: `internal/domain/bee/feeder.go:25-30` (struct), `feeder.go:352-354` (injection block)

- [ ] **Step 1: Remove `PlatformContext` field from `messageMeta` struct**

In `internal/domain/bee/feeder.go`, find the `messageMeta` struct (around line 25) and change it from:

```go
type messageMeta struct {
	From            string          `json:"from"`
	SessionKey      string          `json:"session_key"`
	MessageID       string          `json:"message_id"`
	PlatformContext json.RawMessage `json:"platform_context,omitempty"`
}
```

to:

```go
type messageMeta struct {
	From       string `json:"from"`
	SessionKey string `json:"session_key"`
	MessageID  string `json:"message_id"`
}
```

- [ ] **Step 2: Remove the injection block from `buildPrompt()`**

In `internal/domain/bee/feeder.go`, find the `buildPrompt` function (around line 343). Remove these 3 lines:

```go
		if ctx := platform.ExtractContext(m.Platform, m.Raw); ctx != "" {
			meta.PlatformContext = json.RawMessage(ctx)
		}
```

The surrounding code (before and after removal) should look like:

```go
		meta := messageMeta{
			From:       m.Platform,
			SessionKey: m.SessionKey,
			MessageID:  m.ID,
		}
		b, _ := json.Marshal(meta)
		fmt.Fprintf(&sb, "<message_meta>%s</message_meta>\n<message_content>\n%s\n</message_content>\n", b, m.Content)
```

- [ ] **Step 3: Check if `encoding/json` import is still needed**

After removing `PlatformContext json.RawMessage`, `json.RawMessage` is no longer referenced in `messageMeta`. However `json.Marshal` is still used in `buildPrompt()`, so the `"encoding/json"` import remains. Verify the file compiles:

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go build ./internal/domain/bee/...
```

Expected: no output (clean build). If there is a compile error about unused imports, remove `"encoding/json"` from the import block — but only if `json.Marshal` is truly gone (it shouldn't be).

- [ ] **Step 4: Run all bee tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/domain/bee/... -v
```

Expected: all tests **PASS**, including `TestBuildPrompt_NeverHasPlatformContext`.

Key assertions that must pass:
- `TestBuildPrompt_NoHint` — meta JSON contains `from`, `session_key`, `message_id` but no `platform_context`
- `TestBuildPrompt_NeverHasPlatformContext` — even with a registered extractor and non-empty Raw, no `platform_context` in output

- [ ] **Step 5: Run worker tests to confirm no regression**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/domain/task/... -v
```

Expected: all tests **PASS** — worker still injects `platform_context` unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_internal_test.go
git commit -m "feat(bee): remove platform_context from message metadata

Bee is platform-agnostic and does not need platform_context.
Worker injection in dispatcher.go is unchanged."
```

---

## Verification Checklist

After all tasks are complete:

- [ ] `go test ./internal/domain/bee/...` passes
- [ ] `go test ./internal/domain/task/...` passes
- [ ] Grep confirms no `platform_context` in bee output:
  ```bash
  grep -n "PlatformContext\|platform_context" internal/domain/bee/feeder.go
  # Expected: no matches
  ```
- [ ] Grep confirms worker still has it:
  ```bash
  grep -n "PlatformContext\|platform_context" internal/domain/task/dispatcher.go
  # Expected: matches on the taskMeta struct and injection lines
  ```
