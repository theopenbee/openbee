# Platform-Aware Bot Mention Stripping

**Date:** 2026-04-22
**Branch:** feat/platform-context-injection

---

## Problem

When multiple platforms are configured with bot names where one is a prefix of another, the current stripping logic produces incorrect output.

**Example configuration:**
- DingTalk `bot_name = "openbee"`
- WeCom `bot_name = "openbee本地测试"`

**Observed behavior:**
- WeCom message content: `@openbee本地测试 @咬人的皮卡丘 hello`
- Stored in DB: `本地测试 @咬人的皮卡丘 hello` (incorrect)

**Root cause:** `msgingest.Gateway` compiles all platform bot names into a single flat `[]*regexp.Regexp` list and applies every regex to every message, regardless of which platform the message came from. The DingTalk regex `\s*@openbee\s*` fires first on a WeCom message and consumes `@openbee` (with zero trailing spaces, since `本` is not whitespace), leaving `本地测试` orphaned. The WeCom regex then fails to match the already-mangled string.

---

## Design

### Core principle

Each platform's bot mention stripping must be isolated to messages from that platform only. Cross-platform regex interference is eliminated by keying the compiled regex map on platform ID.

### Changes

#### `internal/domain/msgingest/gateway.go`

Change `botNameREs` from a flat list to a per-platform map:

```go
// Before
botNameREs []*regexp.Regexp

// After
botNameREs map[string]*regexp.Regexp  // platform ID → compiled regex
```

Update the option constructor to accept a `map[string]string`:

```go
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

Update `stripBotMentions` to accept the platform string and look up only that platform's regex:

```go
func stripBotMention(content, platform string, res map[string]*regexp.Regexp) string {
    re, ok := res[platform]
    if !ok {
        return content
    }
    return strings.TrimSpace(re.ReplaceAllString(content, " "))
}
```

Update the call site in `Dispatch` (line 146):

```go
// Before
stripped := stripBotMentions(msg.Content, g.botNameREs)

// After
stripped := stripBotMention(msg.Content, msg.Platform, g.botNameREs)
```

Remove the old `compileBotNameREs` helper and `WithBotNames` option (replaced by `WithPlatformBotNames`).

#### `internal/app/app.go`

Replace the flat name collection loop with an explicit per-platform map:

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
ingest := msgingest.New(..., msgingest.WithBotNames(botNames))

// After
ingest := msgingest.New(..., msgingest.WithPlatformBotNames(map[string]string{
    "feishu":   cfg.Bee.Platforms.Feishu.BotName,
    "dingtalk": cfg.Bee.Platforms.DingTalk.BotName,
    "wecom":    cfg.Bee.Platforms.WeCom.BotName,
    "telegram": cfg.Bee.Platforms.Telegram.BotName,
    "weixin":   cfg.Bee.Platforms.Weixin.BotName,
}))
```

#### `internal/domain/msgingest/strip_test.go`

Update existing tests to use the new `stripBotMention(content, platform, map)` signature. Add a regression test covering the prefix-collision scenario:

- DingTalk bot name `"openbee"`, WeCom bot name `"openbee本地测试"`
- WeCom message `"@openbee本地测试 @someone hello"` → expected `"@someone hello"`
- DingTalk message `"@openbee hello"` → expected `"hello"`

---

## Affected Files

| File | Change |
|---|---|
| `internal/domain/msgingest/gateway.go` | Replace flat list with `map[string]*regexp.Regexp`; update option, strip function, and call site |
| `internal/domain/msgingest/strip_test.go` | Update test signatures; add prefix-collision regression test |
| `internal/app/app.go` | Replace flat loop with explicit per-platform map |

---

## Non-Goals

- No changes to platform handler code (stripping stays in msgingest layer)
- No changes to how bot names are configured in `config.yaml`
- No support for multiple bot names per platform
