# Platform-Aware Bot Mention Stripping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix cross-platform bot name prefix collision by making `msgingest.Gateway` apply only the bot name regex for the message's own platform, eliminating interference between platforms.

**Architecture:** Change `botNameREs` from `[]*regexp.Regexp` (flat, all-platform) to `map[string]*regexp.Regexp` (keyed by platform ID). The `Dispatch` method already receives `msg.Platform`; it now passes that string to `stripBotMention` to look up only the matching regex. `app.go` passes names as an explicit `map[string]string` instead of a flat slice.

**Tech Stack:** Go stdlib (`regexp`, `strings`), Go test (`testing`)

---

## File Map

| File | Action | What changes |
|---|---|---|
| `internal/domain/msgingest/gateway.go` | Modify | Field type, remove old helpers, add `WithPlatformBotNames`, rename strip func |
| `internal/domain/msgingest/strip_test.go` | Modify | Update test table to new signature; add prefix-collision regression case |
| `internal/domain/msgingest/gateway_test.go` | Modify | Replace `WithBotNames` calls with `WithPlatformBotNames` |
| `internal/app/app.go` | Modify | Replace flat `botNames` loop with per-platform map |

---

### Task 1: Refactor gateway.go and strip_test.go

**Files:**
- Modify: `internal/domain/msgingest/gateway.go`
- Modify: `internal/domain/msgingest/strip_test.go`

- [ ] **Step 1: Open strip_test.go and note the current call site**

Current call (line 108):
```go
got := stripBotMentions(tt.content, compileBotNameREs(tt.botNames))
```
The test table field is `botNames []string`. After the change the helper functions are gone; the test must build a `map[string]*regexp.Regexp` inline.

- [ ] **Step 2: Replace strip_test.go with the updated version**

Replace the entire file `internal/domain/msgingest/strip_test.go`:

```go
package msgingest

import (
	"regexp"
	"testing"
)

func buildREs(platform, name string) map[string]*regexp.Regexp {
	if name == "" {
		return nil
	}
	return map[string]*regexp.Regexp{
		platform: regexp.MustCompile(`\s*@` + regexp.QuoteMeta(name) + `\s*`),
	}
}

func TestStripBotMention(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		platform string
		botName  string
		want     string
	}{
		{
			name:     "prefix mention stripped",
			content:  "@机器人 /clear",
			platform: "test",
			botName:  "机器人",
			want:     "/clear",
		},
		{
			name:     "suffix mention stripped",
			content:  "/clear @机器人",
			platform: "test",
			botName:  "机器人",
			want:     "/clear",
		},
		{
			name:     "prefix mention with arg",
			content:  "@机器人 /clear 张三",
			platform: "test",
			botName:  "机器人",
			want:     "/clear 张三",
		},
		{
			name:     "suffix mention with arg",
			content:  "/clear 张三 @机器人",
			platform: "test",
			botName:  "机器人",
			want:     "/clear 张三",
		},
		{
			name:     "prefix mention engine command",
			content:  "@机器人 /engine codex",
			platform: "test",
			botName:  "机器人",
			want:     "/engine codex",
		},
		{
			name:     "no mention, no-op",
			content:  "/clear 张三",
			platform: "test",
			botName:  "机器人",
			want:     "/clear 张三",
		},
		{
			name:     "empty botName, no-op",
			content:  "@机器人 /clear",
			platform: "test",
			botName:  "",
			want:     "@机器人 /clear",
		},
		{
			name:     "unknown platform, no-op",
			content:  "@机器人 /clear",
			platform: "other",
			botName:  "机器人",
			want:     "@机器人 /clear",
		},
		{
			name:     "case sensitive no match",
			content:  "@机器人 /clear",
			platform: "test",
			botName:  "机器人Bot",
			want:     "@机器人 /clear",
		},
		{
			name:     "entire content is mention",
			content:  "@机器人",
			platform: "test",
			botName:  "机器人",
			want:     "",
		},
		{
			name:     "mention mid-sentence no word boundary",
			content:  "prefix@机器人suffix",
			platform: "test",
			botName:  "机器人",
			want:     "prefix suffix",
		},
		{
			name:     "mention on its own line",
			content:  "hello\n@机器人\nworld",
			platform: "test",
			botName:  "机器人",
			want:     "hello world",
		},
		{
			name:     "mention with leading newline",
			content:  "@机器人\nhello",
			platform: "test",
			botName:  "机器人",
			want:     "hello",
		},
		{
			name:     "mention with trailing newline",
			content:  "hello\n@机器人",
			platform: "test",
			botName:  "机器人",
			want:     "hello",
		},
		// Regression: DingTalk name is prefix of WeCom name; WeCom message must not be mangled.
		{
			name:     "prefix collision - wecom message unaffected by dingtalk name",
			content:  "@openbee本地测试 @someone hello",
			platform: "wecom",
			botName:  "openbee本地测试",
			want:     "@someone hello",
		},
		{
			name:     "prefix collision - dingtalk message unaffected by wecom name",
			content:  "@openbee hello",
			platform: "dingtalk",
			botName:  "openbee",
			want:     "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := buildREs(tt.platform, tt.botName)
			got := stripBotMention(tt.content, tt.platform, res)
			if got != tt.want {
				t.Errorf("stripBotMention(%q, %q) = %q, want %q", tt.content, tt.platform, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run to confirm compile failure**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/msgingest/... 2>&1 | head -20
```

Expected: compile error — `stripBotMentions undefined` or `stripBotMention undefined` and `compileBotNameREs undefined`.

- [ ] **Step 4: Update gateway.go**

In `internal/domain/msgingest/gateway.go`:

**4a.** Change the `botNameREs` field in `Gateway` struct (line 63):

```go
// Before
botNameREs     []*regexp.Regexp // compiled @mention patterns, built once from bot names

// After
botNameREs     map[string]*regexp.Regexp // platform → compiled @mention regex
```

**4b.** Replace `compileBotNameREs` and `WithBotNames` (lines 69–85) with `WithPlatformBotNames`:

```go
// WithPlatformBotNames sets a per-platform bot display name whose @mention is stripped
// from message content before command matching, debounce accumulation, and DB storage.
func WithPlatformBotNames(names map[string]string) Option {
	res := make(map[string]*regexp.Regexp, len(names))
	for platform, name := range names {
		if name != "" {
			res[platform] = regexp.MustCompile(`\s*@` + regexp.QuoteMeta(name) + `\s*`)
		}
	}
	return func(g *Gateway) { g.botNameREs = res }
}
```

**4c.** Replace `stripBotMentions` (lines 87–95) with `stripBotMention`:

```go
func stripBotMention(content, platform string, res map[string]*regexp.Regexp) string {
	re, ok := res[platform]
	if !ok {
		return content
	}
	return strings.TrimSpace(re.ReplaceAllString(content, " "))
}
```

**4d.** Update the call site in `Dispatch` (line 146):

```go
// Before
stripped := stripBotMentions(msg.Content, g.botNameREs)

// After
stripped := stripBotMention(msg.Content, msg.Platform, g.botNameREs)
```

- [ ] **Step 5: Run tests and verify pass**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/msgingest/... -run TestStripBotMention -v 2>&1
```

Expected: all `TestStripBotMention` subtests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/msgingest/gateway.go internal/domain/msgingest/strip_test.go
git commit -m "feat(msgingest): replace flat botNameREs with per-platform map

Each platform's @mention regex is now isolated to messages from that
platform. Eliminates cross-platform prefix collision (e.g. DingTalk
'openbee' regex mangling WeCom 'openbee本地测试' messages).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Update gateway_test.go

**Files:**
- Modify: `internal/domain/msgingest/gateway_test.go`

- [ ] **Step 1: Run existing gateway tests to see failures**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/msgingest/... -v 2>&1 | grep -E "FAIL|PASS|error"
```

Expected: compile errors on `WithBotNames` (removed in Task 1).

- [ ] **Step 2: Update TestGateway_BotMention_StrippedInEmitAndDB**

Find the test (around line 413). Change `WithBotNames` to `WithPlatformBotNames`. The `inbound` helper already uses `Platform: "test"`:

```go
// Before
g := msgingest.New(st, 100*time.Millisecond, noopHandler{},
    msgingest.WithBotNames([]string{"OpenBee"}),
)

// After
g := msgingest.New(st, 100*time.Millisecond, noopHandler{},
    msgingest.WithPlatformBotNames(map[string]string{"test": "OpenBee"}),
)
```

- [ ] **Step 3: Update TestGateway_BotMention_MergedMessagesStripped**

Find the test (around line 444). Change `WithBotNames` to `WithPlatformBotNames`:

```go
// Before
g := msgingest.New(st, 150*time.Millisecond, noopHandler{},
    msgingest.WithBotNames([]string{"Bot"}),
)

// After
g := msgingest.New(st, 150*time.Millisecond, noopHandler{},
    msgingest.WithPlatformBotNames(map[string]string{"test": "Bot"}),
)
```

- [ ] **Step 4: Run all msgingest tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/msgingest/... -v 2>&1
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/msgingest/gateway_test.go
git commit -m "test(msgingest): update gateway tests to use WithPlatformBotNames

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update app.go

**Files:**
- Modify: `internal/app/app.go` (lines 161–174)

- [ ] **Step 1: Replace the flat botNames loop with a per-platform map**

Find lines 161–174 in `internal/app/app.go`:

```go
// Before
var botNames []string
for _, n := range []string{
    cfg.Bee.Platforms.Feishu.BotName,
    cfg.Bee.Platforms.DingTalk.BotName,
    cfg.Bee.Platforms.WeCom.BotName,
    cfg.Bee.Platforms.Telegram.BotName,
    cfg.Bee.Platforms.Weixin.BotName,
} {
    if n != "" {
        botNames = append(botNames, n)
    }
}
ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce, cmdChain,
    msgingest.WithBotNames(botNames))
```

Replace with:

```go
ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce, cmdChain,
    msgingest.WithPlatformBotNames(map[string]string{
        "feishu":   cfg.Bee.Platforms.Feishu.BotName,
        "dingtalk": cfg.Bee.Platforms.DingTalk.BotName,
        "wecom":    cfg.Bee.Platforms.WeCom.BotName,
        "telegram": cfg.Bee.Platforms.Telegram.BotName,
        "weixin":   cfg.Bee.Platforms.Weixin.BotName,
    }))
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./... 2>&1
```

Expected: no output (clean build).

- [ ] **Step 3: Run full test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./... 2>&1 | tail -20
```

Expected: all packages PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): pass per-platform bot names to msgingest gateway

Replaces the flat name pool with an explicit platform→name map so each
platform's strip regex is isolated to its own messages.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```
