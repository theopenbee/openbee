# Design: Strip Bot @Mention in Group Chat Commands

**Date:** 2026-04-20

## Problem

When a user sends a command to the bot via @mention in a group chat, the message content includes the `@BotName` prefix or suffix. This causes command parsing to fail because the command handlers check `fields[0]` for the command token (e.g., `/clear`, `/engine`).

Examples of broken inputs:
- `@机器人 /clear` → parsed as non-command (fields[0] = `@机器人`)
- `/clear 张三 @机器人` → `/clear` runs but with garbled arg `张三 @机器人`
- `@机器人 /engine codex` → not recognized as command

## Goals

- Allow `/clear`, `/clear <worker>`, `/engine <engine>`, `/engine <engine> <worker>` to work when prefixed, suffixed, or surrounded by `@BotName` tokens
- Exact case-sensitive match on configured bot name
- Do not alter the message content stored in the database (preserve original for audit and feeder processing)
- Each platform can independently configure its own bot name

## Non-Goals

- Stripping arbitrary @user mentions (only the bot's own name)
- Case-insensitive matching
- Runtime/dynamic bot name discovery (config-driven only)

## Architecture

### Configuration

Add `BotName string` to each platform config struct in `internal/infra/config/config.go`:

```go
type FeishuConfig struct {
    Enabled      bool   `yaml:"enabled"`
    AppID        string `yaml:"app_id"`
    AppSecret    string `yaml:"app_secret"`
    MaxMediaSize int    `yaml:"max_media_size"`
    BotName      string `yaml:"bot_name"` // NEW
}
// Same addition to DingTalkConfig, WeComConfig, TelegramConfig, WeixinConfig
```

### Gateway: BotNames Option

Add `botNames []string` field to `Gateway` struct and a new `WithBotNames` option in `internal/domain/msgingest/gateway.go`:

```go
type Gateway struct {
    // ... existing fields ...
    botNames []string
}

func WithBotNames(names []string) Option {
    return func(g *Gateway) {
        g.botNames = names
    }
}
```

Add a pure stripping function (no side effects, easily testable):

```go
// stripBotMentions removes any token matching "@<botName>" for each configured
// bot name. Only used for command matching; does not affect stored content.
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

In `onDebounce()`, strip before passing to command handler:

```go
cmdContent := stripBotMentions(content, g.botNames)
if g.commandHandler != nil {
    if g.commandHandler.HandleCommand(ctx, cmdContent, msgs[n-1]) {
        return
    }
}
```

### App Wiring

In `internal/app/app.go`, collect bot names from all platform configs and pass to gateway:

```go
var botNames []string
for _, n := range []string{
    cfg.Feishu.BotName,
    cfg.DingTalk.BotName,
    cfg.WeCom.BotName,
    cfg.Telegram.BotName,
    cfg.Weixin.BotName,
} {
    if n != "" {
        botNames = append(botNames, n)
    }
}

ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce,
    msgingest.WithCommandHandler(cmdChain),
    msgingest.WithBotNames(botNames))
```

## Data Flow

```
User sends in group: "@机器人 /clear 张三"
         │
Platform handler (feishu/dingtalk/wecom)
         │  InboundMessage.Content = "@机器人 /clear 张三" (unchanged)
         ▼
Gateway.onDebounce()
         │  cmdContent = stripBotMentions("@机器人 /clear 张三", ["机器人"])
         │            = "/clear 张三"
         ▼
CommandHandler.HandleCommand(ctx, "/clear 张三", msg)
         │  fields[0] = "/clear" ✓  fields[1] = "张三" ✓
         ▼
ClearHandler processes → clears worker "张三" session
```

## Error Handling

- If `BotName` is empty in config, `stripBotMentions` is a no-op — backward compatible
- If the entire content is `@机器人` (no command), stripping yields `""` → command handler returns false → message stored and processed normally

## Testing

Add unit tests in `internal/domain/msgingest/` for `stripBotMentions`:

| Input | BotNames | Expected Output |
|-------|----------|----------------|
| `@机器人 /clear` | `["机器人"]` | `/clear` |
| `/clear @机器人` | `["机器人"]` | `/clear` |
| `@机器人 /clear 张三` | `["机器人"]` | `/clear 张三` |
| `/clear 张三 @机器人` | `["机器人"]` | `/clear 张三` |
| `@机器人 /engine codex` | `["机器人"]` | `/engine codex` |
| `/clear 张三` | `["机器人"]` | `/clear 张三` (no-op) |
| `@机器人 /clear` | `[]` | `@机器人 /clear` (no-op) |
| `@机器人 /clear` | `["OpenBee"]` | `@机器人 /clear` (no match) |

## Files Changed

| File | Change |
|------|--------|
| `internal/infra/config/config.go` | Add `BotName string` to 5 platform config structs |
| `internal/domain/msgingest/gateway.go` | Add `botNames` field, `WithBotNames` option, `stripBotMentions` func, call in `onDebounce` |
| `internal/app/app.go` | Collect bot names, pass `WithBotNames` to ingest gateway |
| `internal/domain/msgingest/gateway_test.go` | Add `TestStripBotMentions` unit tests |
