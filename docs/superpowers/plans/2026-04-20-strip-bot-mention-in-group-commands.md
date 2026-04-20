# Strip Bot @Mention in Group Chat Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow slash commands (`/clear`, `/engine`, etc.) to work in group chats when the user prefixes or suffixes the command with the bot's @mention (e.g., `@机器人 /clear 张三`).

**Architecture:** Each platform config gets a `BotName` field. At startup, the app collects all bot names and passes them to the ingest gateway via a new `WithBotNames` option. Inside the gateway, before passing content to the command handler, a `stripBotMentions` helper removes any `@<BotName>` tokens from the content. The stored message content in the DB is unchanged — stripping only affects command matching.

**Tech Stack:** Go, `strings` stdlib, existing `msgingest.Option` pattern, yaml config tags.

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/config/config.go` | Add `BotName string` to 5 platform config structs |
| `internal/domain/msgingest/gateway.go` | Add `botNames` field, `WithBotNames` option, `stripBotMentions` func, call in `onDebounce` |
| `internal/domain/msgingest/strip_test.go` | New file: unit tests for `stripBotMentions` (package msgingest internal test) |
| `internal/app/app.go` | Collect bot names from platform configs, pass `WithBotNames` to ingest gateway |

---

### Task 1: Add `BotName` to platform config structs

**Files:**
- Modify: `internal/infra/config/config.go:165-200`

- [ ] **Step 1: Add `BotName` field to all five platform config structs**

Open `internal/infra/config/config.go`. Find the five config structs (lines 165–200) and add `BotName string` to each:

```go
type FeishuConfig struct {
	Enabled      bool   `yaml:"enabled"`
	AppID        string `yaml:"app_id"`
	AppSecret    string `yaml:"app_secret"`
	MaxMediaSize int    `yaml:"max_media_size"` // maximum media download size in bytes; default 100 MB
	BotName      string `yaml:"bot_name"`       // bot display name used to strip @mention in group commands
}

type DingTalkConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	BotName      string `yaml:"bot_name"` // bot display name used to strip @mention in group commands
}

type WeComConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BotID        string `yaml:"bot_id"`
	Secret       string `yaml:"secret"`
	WebSocketURL string `yaml:"websocket_url"`
	BotName      string `yaml:"bot_name"` // bot display name used to strip @mention in group commands
}

type TelegramConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	MaxMediaSize int    `yaml:"max_media_size"` // bytes; default 50MB
	AuthCode     string `yaml:"auth_code"`      // passcode for user authorization; empty = no auth required
	BotName      string `yaml:"bot_name"`       // bot display name used to strip @mention in group commands
}

type WeixinConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	BaseURL      string `yaml:"base_url"`
	CDNBaseURL   string `yaml:"cdn_base_url"`
	RouteTag     int    `yaml:"route_tag"`
	UserID       string `yaml:"user_id"`
	MaxMediaSize int    `yaml:"max_media_size"` // bytes; default 100MB
	BotName      string `yaml:"bot_name"`       // bot display name used to strip @mention in group commands
}
```

- [ ] **Step 2: Verify the project builds with no errors**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/config/config.go
git commit -m "feat(config): add BotName field to platform config structs"
```

---

### Task 2: Add `stripBotMentions` and `WithBotNames` to gateway

**Files:**
- Modify: `internal/domain/msgingest/gateway.go:46-65` (struct + options)
- Modify: `internal/domain/msgingest/gateway.go:195-199` (onDebounce command dispatch)
- Create: `internal/domain/msgingest/strip_test.go`

- [ ] **Step 1: Write failing tests for `stripBotMentions`**

Create `internal/domain/msgingest/strip_test.go`:

```go
package msgingest

import (
	"testing"
)

func TestStripBotMentions(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		botNames []string
		want     string
	}{
		{
			name:     "prefix mention stripped",
			content:  "@机器人 /clear",
			botNames: []string{"机器人"},
			want:     "/clear",
		},
		{
			name:     "suffix mention stripped",
			content:  "/clear @机器人",
			botNames: []string{"机器人"},
			want:     "/clear",
		},
		{
			name:     "prefix mention with arg",
			content:  "@机器人 /clear 张三",
			botNames: []string{"机器人"},
			want:     "/clear 张三",
		},
		{
			name:     "suffix mention with arg",
			content:  "/clear 张三 @机器人",
			botNames: []string{"机器人"},
			want:     "/clear 张三",
		},
		{
			name:     "middle mention with args",
			content:  "@机器人 /engine codex",
			botNames: []string{"机器人"},
			want:     "/engine codex",
		},
		{
			name:     "no mention, no-op",
			content:  "/clear 张三",
			botNames: []string{"机器人"},
			want:     "/clear 张三",
		},
		{
			name:     "empty botNames, no-op",
			content:  "@机器人 /clear",
			botNames: []string{},
			want:     "@机器人 /clear",
		},
		{
			name:     "nil botNames, no-op",
			content:  "@机器人 /clear",
			botNames: nil,
			want:     "@机器人 /clear",
		},
		{
			name:     "case sensitive, no match",
			content:  "@机器人 /clear",
			botNames: []string{"机器人Bot"},
			want:     "@机器人 /clear",
		},
		{
			name:     "multiple bot names, matches first",
			content:  "@OpenBee /engine codex",
			botNames: []string{"机器人", "OpenBee"},
			want:     "/engine codex",
		},
		{
			name:     "entire content is just mention",
			content:  "@机器人",
			botNames: []string{"机器人"},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripBotMentions(tt.content, tt.botNames)
			if got != tt.want {
				t.Errorf("stripBotMentions(%q, %v) = %q, want %q", tt.content, tt.botNames, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail (function not yet defined)**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/msgingest/ -run TestStripBotMentions -v
```

Expected: compile error — `undefined: stripBotMentions`.

- [ ] **Step 3: Add `botNames` field, `WithBotNames` option, and `stripBotMentions` to gateway.go**

In `internal/domain/msgingest/gateway.go`:

**3a.** Add `botNames []string` to the `Gateway` struct (after the `commandHandler` field at line 54):

```go
// Gateway receives raw platform messages, deduplicates, debounces, and emits IngestedMessages.
type Gateway struct {
	msgStore       MessageStore
	debounce       time.Duration
	sessions       map[string]*debounceState
	seen           map[string]struct{} // in-memory dedup set keyed by platform_msg_id
	seenPrev       map[string]struct{} // previous generation, checked on lookup only
	mu             sync.Mutex
	out            chan IngestedMessage
	commandHandler CommandHandler // optional; intercepts slash commands before DB write
	botNames       []string       // @mention tokens to strip before command matching
}
```

**3b.** Add `WithBotNames` option after `WithCommandHandler` (around line 65):

```go
// WithBotNames sets the bot display names whose @mentions are stripped from message
// content before command matching. Does not affect stored message content.
func WithBotNames(names []string) Option {
	return func(g *Gateway) { g.botNames = names }
}
```

**3c.** Add `stripBotMentions` as a package-level function (after `WithBotNames`):

```go
// stripBotMentions removes any token equal to "@<name>" for each configured bot name.
// Used only for command matching; never mutates stored message content.
func stripBotMentions(content string, botNames []string) string {
	if len(botNames) == 0 {
		return content
	}
	mentions := make(map[string]struct{}, len(botNames))
	for _, name := range botNames {
		mentions["@"+name] = struct{}{}
	}
	fields := strings.Fields(content)
	out := fields[:0]
	for _, f := range fields {
		if _, skip := mentions[f]; !skip {
			out = append(out, f)
		}
	}
	return strings.Join(out, " ")
}
```

**3d.** Add `"strings"` to the import block at the top of `gateway.go` (it's not imported yet):

```go
import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"go.uber.org/zap"
)
```

- [ ] **Step 4: Run tests to confirm `stripBotMentions` passes**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/msgingest/ -run TestStripBotMentions -v
```

Expected: all 11 subtests PASS.

- [ ] **Step 5: Update `onDebounce` to strip bot mentions before command matching**

In `internal/domain/msgingest/gateway.go`, replace lines 195–199:

```go
	if g.commandHandler != nil {
		if g.commandHandler.HandleCommand(context.Background(), content, msgs[n-1]) {
			return
		}
	}
```

with:

```go
	if g.commandHandler != nil {
		cmdContent := stripBotMentions(content, g.botNames)
		if g.commandHandler.HandleCommand(context.Background(), cmdContent, msgs[n-1]) {
			return
		}
	}
```

- [ ] **Step 6: Run all msgingest tests to confirm no regressions**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/msgingest/ -v
```

Expected: all existing tests plus `TestStripBotMentions` PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/msgingest/gateway.go internal/domain/msgingest/strip_test.go
git commit -m "feat(msgingest): strip bot @mention tokens before command matching"
```

---

### Task 3: Wire bot names from config into the ingest gateway

**Files:**
- Modify: `internal/app/app.go:160`

- [ ] **Step 1: Collect bot names and pass `WithBotNames` to ingest gateway**

In `internal/app/app.go`, replace line 160:

```go
ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce, msgingest.WithCommandHandler(cmdChain))
```

with:

```go
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
ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce,
    msgingest.WithCommandHandler(cmdChain),
    msgingest.WithBotNames(botNames))
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run full test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./...
```

Expected: all tests PASS, no failures.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire platform bot names into ingest gateway for @mention stripping"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** Config field (Task 1) ✓, `stripBotMentions` + `WithBotNames` (Task 2) ✓, app wiring (Task 3) ✓, tests (Task 2) ✓
- [x] **Placeholder scan:** No TBDs. All code blocks are complete.
- [x] **Type consistency:** `stripBotMentions(content string, botNames []string) string` used consistently in test and implementation. `WithBotNames(names []string) Option` matches the `Option func(*Gateway)` pattern.
- [x] **Backward compatibility:** `BotName` defaults to empty string → `WithBotNames(nil/empty)` → `stripBotMentions` is a no-op. No behavior change without config.
