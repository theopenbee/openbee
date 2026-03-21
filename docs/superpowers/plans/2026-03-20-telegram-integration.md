# Telegram Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate Telegram as a first-class messaging platform in OpenBee, following the same `platform.Platform` interface pattern as Feishu, DingTalk, and WeCom.

**Architecture:** A new `internal/platform/telegram/` package implements `TelegramPlatform`, `TelegramReceiver` (long-polling via getUpdates), and `TelegramSender`. The platform is wired into `buildPlatforms()` in `app.go` and configured via the existing YAML config format. The config wizard (`cmd/openbee/config.go`) gains a Telegram option in its interactive flow.

**Tech Stack:** Go, `github.com/go-telegram-bot-api/telegram-bot-api/v5`, existing `media.Service`, `go.uber.org/zap`

**Spec:** `docs/telegram-channel-design.md`

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/platform/telegram/handler.go` | `TelegramPlatform` / `TelegramReceiver` / `TelegramSender` |
| Create | `internal/platform/telegram/handler_test.go` | Unit tests for pure helper functions |
| Modify | `internal/config/config.go` | Add `TelegramConfig`, extend `PlatformsConfig`, extend `applyDefaults` |
| Modify | `internal/config/config.yaml.tmpl` | Add `telegram:` section |
| Modify | `internal/app/app.go` | Wire Telegram into `buildPlatforms` |
| Modify | `cmd/openbee/config.go` | Add Telegram to wizard (`configValues`, `loadExistingConfig`, prompts) |

---

## Task 1: Add Go Dependency

**Files:**
- Modify: `go.mod`, `go.sum` (auto-updated by `go get`)

- [ ] **Step 1: Add the Telegram Bot API library**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go get github.com/go-telegram-bot-api/telegram-bot-api/v5
```

Expected: `go.mod` now contains `github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.x.x`

- [ ] **Step 2: Verify the module resolves**

```bash
go mod tidy
go build ./...
```

Expected: No errors. Build succeeds with existing code unchanged.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add telegram-bot-api/v5 dependency"
```

---

## Task 2: Add TelegramConfig to Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_bee_test.go`

The test file already exists and uses a temp-file pattern. We extend it.

- [ ] **Step 1: Write failing tests for TelegramConfig**

Add these test cases to `internal/config/config_bee_test.go`:

```go
func TestBeeConfig_TelegramDefaults(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Bee.Platforms.Telegram.MaxMediaSize != 50*1024*1024 {
		t.Errorf("Telegram.MaxMediaSize default: want %d got %d",
			50*1024*1024, cfg.Bee.Platforms.Telegram.MaxMediaSize)
	}
}

func TestBeeConfig_TelegramLoad(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
bee:
  platforms:
    telegram:
      enabled: true
      token: "test-token"
      max_media_size: 10485760
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tc := cfg.Bee.Platforms.Telegram
	if !tc.Enabled {
		t.Error("Telegram.Enabled: want true")
	}
	if tc.Token != "test-token" {
		t.Errorf("Telegram.Token: want test-token got %q", tc.Token)
	}
	if tc.MaxMediaSize != 10485760 {
		t.Errorf("Telegram.MaxMediaSize: want 10485760 got %d", tc.MaxMediaSize)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/config/... -run TestBeeConfig_Telegram -v
```

Expected: FAIL — `cfg.Bee.Platforms.Telegram` field does not exist yet.

- [ ] **Step 3: Implement TelegramConfig in config.go**

In `internal/config/config.go`, add the struct after `WeComConfig`:

```go
type TelegramConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	MaxMediaSize int    `yaml:"max_media_size"` // bytes; default 50MB
}
```

Add the field to `PlatformsConfig`:

```go
type PlatformsConfig struct {
	Feishu   FeishuConfig   `yaml:"feishu"`
	DingTalk DingTalkConfig `yaml:"dingtalk"`
	WeCom    WeComConfig    `yaml:"wecom"`
	Telegram TelegramConfig `yaml:"telegram"`
}
```

Add default in `applyDefaults`:

```go
if cfg.Bee.Platforms.Telegram.MaxMediaSize == 0 {
    cfg.Bee.Platforms.Telegram.MaxMediaSize = 50 * 1024 * 1024 // 50MB
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/config/... -run TestBeeConfig_Telegram -v
```

Expected: PASS both test cases.

- [ ] **Step 5: Run full config test suite to catch regressions**

```bash
go test ./internal/config/... -v
```

Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_bee_test.go
git commit -m "feat(config): add TelegramConfig to PlatformsConfig"
```

---

## Task 3: Implement Telegram Platform Handler

**Files:**
- Create: `internal/platform/telegram/handler.go`
- Create: `internal/platform/telegram/handler_test.go`

This task implements the full platform, receiver, and sender. Tests cover pure helper functions (no live API calls needed — the pattern mirrors `feishu/handler_test.go`).

- [ ] **Step 1: Write failing tests for pure helper functions**

Create `internal/platform/telegram/handler_test.go`:

```go
package telegram

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTelegramPlatformID(t *testing.T) {
	// TelegramPlatform.ID() is a pure method with no dependencies; safe to call on zero value.
	p := &TelegramPlatform{}
	if p.ID() != "telegram" {
		t.Errorf("ID() = %q, want %q", p.ID(), "telegram")
	}
}

func TestBuildSessionKey(t *testing.T) {
	tests := []struct {
		name     string
		chatID   int64
		senderID int64
		want     string
	}{
		{"private chat", 123, 123, "telegram:123:123"},
		{"group chat",   -100456, 789, "telegram:-100456:789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSessionKey(tt.chatID, tt.senderID)
			if got != tt.want {
				t.Errorf("buildSessionKey(%d, %d) = %q, want %q",
					tt.chatID, tt.senderID, got, tt.want)
			}
		})
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"<b>bold</b>", "&lt;b&gt;bold&lt;/b&gt;"},
		{"a & b", "a &amp; b"},
		{"price: 5 > 3", "price: 5 &gt; 3"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeHTML(tt.input)
			if got != tt.want {
				t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRaw(t *testing.T) {
	raw := `{"update_id":100,"message":{"message_id":42,"chat":{"id":-9876},"date":1700000000}}`
	chatID, msgID, err := parseRaw(raw)
	if err != nil {
		t.Fatalf("parseRaw error: %v", err)
	}
	if chatID != -9876 {
		t.Errorf("chatID = %d, want -9876", chatID)
	}
	if msgID != 42 {
		t.Errorf("msgID = %d, want 42", msgID)
	}
}

func TestParseRaw_InvalidJSON(t *testing.T) {
	_, _, err := parseRaw("not json")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestBuildPlatformMessageID(t *testing.T) {
	got := buildPlatformMessageID(100, 42)
	if got != "100:42" {
		t.Errorf("buildPlatformMessageID(100, 42) = %q, want %q", got, "100:42")
	}
}

func TestMediaTypeFromTelegram(t *testing.T) {
	tests := []struct {
		msgType string
		want    string
	}{
		{"photo",    "image"},
		{"video",    "video"},
		{"audio",    "audio"},
		{"voice",    "audio"},
		{"document", "document"},
		{"sticker",  "sticker"},
		{"unknown",  "document"},
	}
	for _, tt := range tests {
		t.Run(tt.msgType, func(t *testing.T) {
			got := mediaTypeFromTelegram(tt.msgType)
			if got != tt.want {
				t.Errorf("mediaTypeFromTelegram(%q) = %q, want %q",
					tt.msgType, got, tt.want)
			}
		})
	}
}

// Ensure package compiles and interface compliance.
var _ interface{ ID() string } = (*TelegramPlatform)(nil)
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/platform/telegram/... -v
```

Expected: FAIL — package `telegram` does not exist yet.

- [ ] **Step 3: Implement handler.go**

Create `internal/platform/telegram/handler.go`:

```go
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/media"
	"github.com/theopenbee/openbee/internal/platform"
)

var log = logger.With(zap.String("component", "telegram"))

// ─── Platform ─────────────────────────────────────────────────────────────────

// TelegramPlatform implements platform.Platform for Telegram.
type TelegramPlatform struct {
	receiver *TelegramReceiver
	sender   *TelegramSender
}

// NewPlatform constructs a TelegramPlatform from configuration.
// The bot client is created once here and shared between receiver and sender
// to avoid repeated auth round-trips on every Send() call.
func NewPlatform(cfg config.TelegramConfig, mediaSvc *media.Service) platform.Platform {
	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		// Panic here is intentional: a bad token means the platform is misconfigured
		// and should not start silently. The error will surface at startup.
		panic(fmt.Sprintf("telegram: invalid token: %v", err))
	}
	p := &TelegramPlatform{}
	p.receiver = &TelegramReceiver{cfg: cfg, mediaSvc: mediaSvc, bot: bot}
	p.sender = &TelegramSender{cfg: cfg, bot: bot}
	return p
}

func (p *TelegramPlatform) ID() string                                 { return "telegram" }
func (p *TelegramPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *TelegramPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// ─── Helpers ──────────────────────────────────────────────────────────────────

func buildSessionKey(chatID, senderID int64) string {
	return fmt.Sprintf("telegram:%d:%d", chatID, senderID)
}

func buildPlatformMessageID(updateID int, messageID int) string {
	return fmt.Sprintf("%d:%d", updateID, messageID)
}

// escapeHTML escapes <, >, & for Telegram HTML parse mode.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// telegramRaw is the minimal structure stored in InboundMessage.Raw,
// used by TelegramSender to route replies.
type telegramRaw struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		MessageID int `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

// parseRaw deserialises an InboundMessage.Raw string and returns chatID and messageID.
func parseRaw(raw string) (chatID int64, messageID int, err error) {
	var r telegramRaw
	if err = json.Unmarshal([]byte(raw), &r); err != nil {
		return 0, 0, fmt.Errorf("parse telegram raw: %w", err)
	}
	return r.Message.Chat.ID, r.Message.MessageID, nil
}

// mediaTypeFromTelegram maps a Telegram message kind to the media type string
// used by media.Service placeholder building.
func mediaTypeFromTelegram(msgType string) string {
	switch msgType {
	case "photo":
		return "image"
	case "sticker":
		return "sticker"
	case "video":
		return "video"
	case "audio", "voice":
		return "audio"
	default:
		return "document"
	}
}

// ─── Receiver ─────────────────────────────────────────────────────────────────

// TelegramReceiver connects to Telegram via long-polling and dispatches inbound messages.
type TelegramReceiver struct {
	cfg      config.TelegramConfig
	mediaSvc *media.Service
	bot      *tgbotapi.BotAPI
}

func (r *TelegramReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	bot := r.bot
	log.Info("Telegram bot started", zap.String("username", bot.Self.UserName))

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	for {
		if ctx.Err() != nil {
			return nil
		}

		updates, err := bot.GetUpdates(u)
		if err != nil {
			log.Error("getUpdates error, retrying in 5s", zap.Error(err))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				continue
			}
		}

		for _, update := range updates {
			u.Offset = update.UpdateID + 1

			if update.Message == nil {
				continue
			}

			// Send typing action as acknowledgement (best-effort, non-blocking).
			go func(chatID int64) {
				action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
				if _, err := bot.Send(action); err != nil {
					log.Warn("send typing action failed", zap.Error(err))
				}
			}(update.Message.Chat.ID)

			msg := r.buildInboundMessage(ctx, bot, update)
			if msg == nil {
				continue
			}
			dispatch(*msg)
		}
	}
}

func (r *TelegramReceiver) buildInboundMessage(
	ctx context.Context,
	bot *tgbotapi.BotAPI,
	update tgbotapi.Update,
) *platform.InboundMessage {
	m := update.Message
	senderID := int64(0)
	if m.From != nil {
		senderID = m.From.ID
	}
	if senderID == 0 {
		log.Warn("skipping message with no sender")
		return nil
	}

	var content string
	switch {
	case m.Text != "":
		content = m.Text
	case m.Photo != nil:
		content = r.downloadLargestPhoto(ctx, bot, m.Photo)
	case m.Document != nil:
		content = r.downloadFile(ctx, bot, m.Document.FileID, m.Document.FileName, "document")
	case m.Audio != nil:
		content = r.downloadFile(ctx, bot, m.Audio.FileID, m.Audio.FileName, "audio")
	case m.Voice != nil:
		content = r.downloadFile(ctx, bot, m.Voice.FileID, "voice.ogg", "audio")
	case m.Video != nil:
		content = r.downloadFile(ctx, bot, m.Video.FileID, "video.mp4", "video")
	case m.Sticker != nil:
		content = r.downloadFile(ctx, bot, m.Sticker.FileID, "sticker.webp", "sticker")
	default:
		log.Warn("skipping unsupported message type")
		return nil
	}

	if content == "" {
		return nil
	}

	rawBytes, err := json.Marshal(update)
	if err != nil {
		log.Error("marshal update", zap.Error(err))
		return nil
	}

	return &platform.InboundMessage{
		Platform:          "telegram",
		SenderID:          strconv.FormatInt(senderID, 10),
		SessionKey:        buildSessionKey(m.Chat.ID, senderID),
		Content:           content,
		RawContent:        content,
		Raw:               string(rawBytes),
		PlatformMessageID: buildPlatformMessageID(update.UpdateID, m.MessageID),
		MessageTime:       int64(m.Date) * 1000,
	}
}

func (r *TelegramReceiver) downloadLargestPhoto(
	ctx context.Context,
	bot *tgbotapi.BotAPI,
	photos []tgbotapi.PhotoSize,
) string {
	if len(photos) == 0 {
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	// Telegram sends multiple resolutions; last element is the largest.
	largest := photos[len(photos)-1]
	return r.downloadFile(ctx, bot, largest.FileID, "photo.jpg", "image")
}

func (r *TelegramReceiver) downloadFile(
	ctx context.Context,
	bot *tgbotapi.BotAPI,
	fileID, fileName, mediaType string,
) string {
	fc, err := bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		log.Error("get file info failed", zap.String("fileID", fileID), zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	url := fc.Link(r.cfg.Token)

	dlCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		log.Error("build download request failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error("download file failed", zap.String("fileID", fileID), zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}
	defer resp.Body.Close()

	maxSize := r.cfg.MaxMediaSize
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxSize)+1))
	if err != nil {
		log.Error("read file body failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}
	if len(data) > maxSize {
		log.Warn("file too large, skipping download",
			zap.String("fileID", fileID), zap.Int("maxSize", maxSize))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	ext := filepath.Ext(fileName)
	if ext == "" {
		mime := r.mediaSvc.DetectMIME(data, fileName)
		ext = r.mediaSvc.ExtensionFromMIME(mime)
	}

	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Error("save inbound media failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	return r.mediaSvc.BuildPlaceholder(mediaType, path, fileName)
}

// ─── Sender ───────────────────────────────────────────────────────────────────

// TelegramSender sends messages via the Telegram Bot API.
type TelegramSender struct {
	cfg config.TelegramConfig
	bot *tgbotapi.BotAPI
}

func (s *TelegramSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	bot := s.bot
	chatID, replyToID, err := parseRaw(msg.ReplyTo.Raw)
	if err != nil {
		return fmt.Errorf("parse reply context: %w", err)
	}

	if msg.MediaPath != "" {
		return s.sendMedia(bot, chatID, replyToID, msg.MediaPath)
	}
	return s.sendText(bot, chatID, replyToID, msg.Content)
}

func (s *TelegramSender) sendText(bot *tgbotapi.BotAPI, chatID int64, replyToID int, text string) error {
	outMsg := tgbotapi.NewMessage(chatID, escapeHTML(text))
	outMsg.ParseMode = tgbotapi.ModeHTML
	if replyToID != 0 {
		outMsg.ReplyToMessageID = replyToID
	}
	_, err := bot.Send(outMsg)
	if err != nil {
		return fmt.Errorf("send text: %w", err)
	}
	return nil
}

func (s *TelegramSender) sendMedia(bot *tgbotapi.BotAPI, chatID int64, replyToID int, mediaPath string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("read media file: %w", err)
	}
	maxSize := s.cfg.MaxMediaSize
	if len(data) > maxSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", len(data), maxSize)
	}

	fileName := platform.SanitizeFileName(filepath.Base(mediaPath))
	ext := strings.ToLower(filepath.Ext(mediaPath))
	fileBytes := tgbotapi.FileBytes{Name: fileName, Bytes: data}

	var chattable tgbotapi.Chattable
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		photo := tgbotapi.NewPhoto(chatID, fileBytes)
		if replyToID != 0 {
			photo.ReplyToMessageID = replyToID
		}
		chattable = photo
	case ".mp4", ".mov", ".avi":
		video := tgbotapi.NewVideo(chatID, fileBytes)
		if replyToID != 0 {
			video.ReplyToMessageID = replyToID
		}
		chattable = video
	case ".mp3", ".wav", ".flac", ".m4a", ".aac":
		audio := tgbotapi.NewAudio(chatID, fileBytes)
		if replyToID != 0 {
			audio.ReplyToMessageID = replyToID
		}
		chattable = audio
	case ".ogg", ".opus":
		voice := tgbotapi.NewVoice(chatID, fileBytes)
		if replyToID != 0 {
			voice.ReplyToMessageID = replyToID
		}
		chattable = voice
	default:
		doc := tgbotapi.NewDocument(chatID, fileBytes)
		if replyToID != 0 {
			doc.ReplyToMessageID = replyToID
		}
		chattable = doc
	}

	if _, err := bot.Send(chattable); err != nil {
		return fmt.Errorf("send media: %w", err)
	}
	return nil
}

// Interface compliance guards.
var _ platform.Platform               = (*TelegramPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*TelegramReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*TelegramSender)(nil)
```

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/platform/telegram/... -v
```

Expected: All tests PASS.

- [ ] **Step 5: Verify build**

```bash
go build ./internal/platform/telegram/...
```

Expected: No errors.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/telegram/
git commit -m "feat(telegram): add TelegramPlatform receiver and sender"
```

---

## Task 4: Wire Telegram into app.go

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add the telegram import and extend buildPlatforms**

In `internal/app/app.go`, add to the import block:

```go
"github.com/theopenbee/openbee/internal/platform/telegram"
```

Modify the `buildPlatforms` signature and body (find the existing function around line 215):

```go
func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig, wc config.WeComConfig, tc config.TelegramConfig, mc config.MediaConfig) []platform.Platform {
	mediaSvc := media.NewService()
	var result []platform.Platform
	if fc.Enabled {
		result = append(result, feishu.NewPlatform(fc, mediaSvc))
	}
	if dc.Enabled {
		result = append(result, dingtalk.NewPlatform(dc, mc, mediaSvc))
	}
	if wc.Enabled {
		result = append(result, wecom.NewPlatform(wc, mediaSvc))
	}
	if tc.Enabled {
		result = append(result, telegram.NewPlatform(tc, mediaSvc))
	}
	return result
}
```

Update the call site (around line 117) to pass the Telegram config:

```go
platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom, cfg.Bee.Platforms.Telegram, cfg.Bee.Media)
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire Telegram platform into buildPlatforms"
```

---

## Task 5: Update Config Template

**Files:**
- Modify: `internal/config/config.yaml.tmpl`

- [ ] **Step 1: Add telegram section to the template**

Open `internal/config/config.yaml.tmpl`. After the `wecom:` block, add:

```yaml
    telegram:
      enabled: {{.TelegramEnabled}}
      token: "{{.TelegramToken}}"
      # max_media_size: 52428800  # 50MB default
```

The full `platforms:` section should now look like:

```yaml
  platforms:
    feishu:
      enabled: {{.FeishuEnabled}}
      app_id: "{{.FeishuAppID}}"
      app_secret: "{{.FeishuAppSecret}}"
      # max_media_size: 104857600  # 100MB，最大媒体下载大小（字节）
    dingtalk:
      enabled: {{.DingtalkEnabled}}
      client_id: "{{.DingtalkClientID}}"
      client_secret: "{{.DingtalkClientSecret}}"
    wecom:
      enabled: {{.WecomEnabled}}
      bot_id: "{{.WecomBotID}}"
      secret: "{{.WecomSecret}}"
      # websocket_url: wss://openws.work.weixin.qq.com  # optional, this is the default
    telegram:
      enabled: {{.TelegramEnabled}}
      token: "{{.TelegramToken}}"
      # max_media_size: 52428800  # 50MB default
```

- [ ] **Step 2: Build to confirm template compiles**

```bash
go build ./...
```

Expected: No errors (template is embedded at build time; compile verifies the embed).

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.yaml.tmpl
git commit -m "feat(config): add telegram section to config template"
```

---

## Task 6: Update Config Wizard

**Files:**
- Modify: `cmd/openbee/config.go`

- [ ] **Step 1: Add Telegram fields to configValues struct**

In `cmd/openbee/config.go`, extend the `configValues` struct:

```go
type configValues struct {
	// ... existing fields ...
	WecomEnabled bool
	WecomBotID   string
	WecomSecret  string

	TelegramEnabled bool   // new
	TelegramToken   string // new

	// ... rest ...
}
```

- [ ] **Step 2: Populate Telegram fields in loadExistingConfig**

In `loadExistingConfig`, add inside the returned struct literal:

```go
TelegramEnabled: cfg.Bee.Platforms.Telegram.Enabled,
TelegramToken:   cfg.Bee.Platforms.Telegram.Token,
```

- [ ] **Step 3: Add Telegram default reset in runConfig**

In `runConfig`, find where platform flags are reset (around line 225):

```go
// Reset platform flags — they'll be re-enabled based on selection
vals.FeishuEnabled = false
vals.DingtalkEnabled = false
vals.WecomEnabled = false
vals.TelegramEnabled = false  // add this line
```

- [ ] **Step 4: Add Telegram to the defaultPlatforms slice**

Find the block that builds `defaultPlatforms` (around line 204) and add:

```go
if vals.TelegramEnabled {
    defaultPlatforms = append(defaultPlatforms, "Telegram")
}
```

- [ ] **Step 5: Add "Telegram" to the MultiSelect options**

Find the `survey.MultiSelect` call and add `"Telegram"` to the Options slice:

```go
if err := survey.AskOne(&survey.MultiSelect{
    Message: "Which platforms to enable?",
    Options: []string{"Feishu", "DingTalk", "WeCom", "Telegram"},
    Default: defaultPlatforms,
}, &selectedPlatforms); err != nil {
    return handleSurveyErr(err)
}
```

- [ ] **Step 6: Add the Telegram case to the selection loop**

In the `for _, p := range selectedPlatforms` switch, add:

```go
case "Telegram":
    vals.TelegramEnabled = true
    if err := survey.AskOne(&survey.Password{
        Message: "Telegram Bot Token:",
        Help:    "Get a token from @BotFather on Telegram",
    }, &vals.TelegramToken, survey.WithValidator(survey.Required)); err != nil {
        return handleSurveyErr(err)
    }
```

Note: `survey.Password` hides the token on input, matching the security guidance from the design doc.

- [ ] **Step 7: Build to verify no compile errors**

```bash
go build ./cmd/openbee/...
```

Expected: No errors.

- [ ] **Step 8: Commit**

```bash
git add cmd/openbee/config.go
git commit -m "feat(cli): add Telegram to config wizard"
```

---

## Task 7: Final Verification

- [ ] **Step 1: Run the full test suite**

```bash
go test ./... 2>&1
```

Expected: All tests PASS. No new failures.

- [ ] **Step 2: Run go vet**

```bash
go vet ./...
```

Expected: No issues reported.

- [ ] **Step 3: Smoke-test config wizard output**

Create a temporary test config by running the template manually to confirm the telegram section renders:

```bash
go run ./cmd/openbee config -o /tmp/test-config.yaml
# Select Telegram, enter a fake token like "test:token123"
cat /tmp/test-config.yaml | grep -A3 "telegram:"
```

Expected output:
```yaml
    telegram:
      enabled: true
      token: "test:token123"
```

- [ ] **Step 4: Final commit (if any cleanup needed)**

```bash
git add -p  # review any remaining changes
git commit -m "chore: final cleanup for telegram integration"
```

---

## Acceptance Criteria

| # | Criterion | How to Verify |
|---|-----------|--------------|
| 1 | `TelegramConfig` loads correctly from YAML | `go test ./internal/config/... -v` passes |
| 2 | `TelegramPlatform.ID()` returns `"telegram"` | `go test ./internal/platform/telegram/... -v` passes |
| 3 | `buildSessionKey` formats correctly for DM and group | Unit test in Task 3 |
| 4 | `escapeHTML` correctly escapes `<`, `>`, `&` | Unit test in Task 3 |
| 5 | `parseRaw` extracts chatID and messageID from JSON | Unit test in Task 3 |
| 6 | `go build ./...` succeeds with no errors | Task 7, Step 1 |
| 7 | `go vet ./...` reports no issues | Task 7, Step 2 |
| 8 | Config wizard offers Telegram and writes valid config | Task 7, Step 3 |
| 9 | No existing tests broken | `go test ./...` full pass |

---

## Dependencies Between Tasks

```
Task 1 (dependency) → Task 2 (config) → Task 3 (handler)
                                       → Task 4 (app wiring)  ← requires Task 3
                                       → Task 5 (template)
                                       → Task 6 (wizard)
All → Task 7 (final verification)
```

Tasks 3, 4, 5, 6 can be done in any order after Task 2, but Task 4 requires Task 3 to compile.
