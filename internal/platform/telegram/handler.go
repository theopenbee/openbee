package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
)

const PlatformID = "telegram"

var log = logger.With(zap.String("component", "telegram"))

// ─── Platform ─────────────────────────────────────────────────────────────────

type TelegramPlatform struct {
	receiver *TelegramReceiver
	sender   *TelegramSender
}

type SessionKeyMigrator interface {
	MigrateSessionKeyPrefix(ctx context.Context, from, to string) (int64, error)
}

func NewPlatform(cfg config.TelegramConfig, mediaSvc *media.Service, migrator SessionKeyMigrator) platform.Platform {
	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		panic(fmt.Sprintf("telegram: invalid token: %v", err))
	}
	var authStore *AuthStore
	if cfg.AuthCode != "" {
		authStore = newAuthStore()
	}

	setupBotCommands(bot)

	p := &TelegramPlatform{}
	p.receiver = &TelegramReceiver{
		cfg: cfg, mediaSvc: mediaSvc, bot: bot, authStore: authStore, migrator: migrator,
		unauthReplyLast: make(map[string]time.Time),
	}
	p.sender = &TelegramSender{cfg: cfg, bot: bot, migrator: migrator}
	return p
}

// setupBotCommands registers the bot's menu commands with Telegram.
func setupBotCommands(bot *tgbotapi.BotAPI) {
	commands := []tgbotapi.BotCommand{
		{Command: "clear", Description: "Clear conversation history"},
	}
	if err := bot.SetMyCommands(commands); err != nil {
		log.Error("failed to set bot commands", zap.Error(err))
		return
	}
	log.Info("bot menu commands registered", zap.Int("count", len(commands)))
}

func (p *TelegramPlatform) ID() string                                  { return PlatformID }
func (p *TelegramPlatform) Receiver() platform.PlatformReceiverAdapter  { return p.receiver }
func (p *TelegramPlatform) Sender() platform.PlatformSenderAdapter      { return p.sender }

// ─── Helpers ──────────────────────────────────────────────────────────────────

func buildSessionKey(chatID int64, threadID int, senderID int64) string {
	if threadID != 0 {
		return fmt.Sprintf("telegram:%d:%d:%d", chatID, threadID, senderID)
	}
	return fmt.Sprintf("telegram:%d:%d", chatID, senderID)
}

func sessionChatPrefix(chatID int64) string {
	return fmt.Sprintf("telegram:%d:", chatID)
}

func migrateChat(ctx context.Context, migrator SessionKeyMigrator, from, to int64) {
	if migrator == nil || from == to {
		return
	}
	n, err := migrator.MigrateSessionKeyPrefix(ctx, sessionChatPrefix(from), sessionChatPrefix(to))
	if err != nil {
		log.Error("migrate chat sessions failed", zap.Int64("from", from), zap.Int64("to", to), zap.Error(err))
		return
	}
	log.Info("migrated chat sessions", zap.Int64("from", from), zap.Int64("to", to), zap.Int64("tasks", n))
}

func buildPlatformMessageID(updateID int, messageID int) string {
	return fmt.Sprintf("%d:%d", updateID, messageID)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

type target struct {
	chatID    int64
	threadID  int
	replyToID int
}

func (t target) params() map[string]string {
	p := map[string]string{"chat_id": strconv.FormatInt(t.chatID, 10)}
	if t.threadID != 0 {
		p["message_thread_id"] = strconv.Itoa(t.threadID)
	}
	if t.replyToID != 0 {
		p["reply_to_message_id"] = strconv.Itoa(t.replyToID)
	}
	return p
}

func formValues(p map[string]string) url.Values {
	v := url.Values{}
	for k, val := range p {
		v.Set(k, val)
	}
	return v
}

type telegramRaw struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		MessageID       int  `json:"message_id"`
		MessageThreadID int  `json:"message_thread_id"`
		IsTopicMessage  bool `json:"is_topic_message"`
		Chat            struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

func parseRaw(raw string) (target, error) {
	var r telegramRaw
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return target{}, fmt.Errorf("parse telegram raw: %w", err)
	}
	t := target{chatID: r.Message.Chat.ID, replyToID: r.Message.MessageID}
	if r.Message.IsTopicMessage {
		t.threadID = r.Message.MessageThreadID
	}
	return t, nil
}

func parseSessionKey(key string) (target, error) {
	chat, err := platform.SessionChatID(key, PlatformID)
	if err != nil {
		return target{}, err
	}
	chatID, err := strconv.ParseInt(chat, 10, 64)
	if err != nil {
		return target{}, fmt.Errorf("parse telegram chat id %q: %w", chat, err)
	}
	t := target{chatID: chatID}
	if parts := strings.Split(key, ":"); len(parts) == 4 {
		if t.threadID, err = strconv.Atoi(parts[2]); err != nil {
			return target{}, fmt.Errorf("parse telegram thread id %q: %w", parts[2], err)
		}
	}
	return t, nil
}

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

type TelegramReceiver struct {
	cfg       config.TelegramConfig
	mediaSvc  *media.Service
	bot       *tgbotapi.BotAPI
	authStore *AuthStore // nil when auth_code is empty (no auth required)
	migrator  SessionKeyMigrator

	// Rate-limit unauthorized reply: at most one per sender per 60s.
	unauthReplyMu   sync.Mutex
	unauthReplyLast map[string]time.Time
}

func (r *TelegramReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	bot := r.bot
	log.Info("Telegram bot started", zap.String("username", bot.Self.UserName))

	offset := 0
	for {
		if ctx.Err() != nil {
			return nil
		}

		updates, err := fetchUpdates(bot, offset, 30)
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
			offset = update.UpdateID + 1

			if update.Message == nil {
				continue
			}

			if r.handleMigration(ctx, update.Message) {
				continue
			}

			// Handle auth if enabled.
			if r.authStore != nil && update.Message.From != nil {
				senderID := strconv.FormatInt(int64(update.Message.From.ID), 10)
				if r.handleAuth(bot, update.Message, senderID) {
					continue
				}
			}

			go sendTyping(bot, target{chatID: update.Message.Chat.ID, threadID: update.threadID})

			msg := r.buildInboundMessage(ctx, bot, update)
			if msg == nil {
				continue
			}
			dispatch(*msg)
		}
	}
}

type inboundUpdate struct {
	tgbotapi.Update
	raw      json.RawMessage
	threadID int
}

func fetchUpdates(bot *tgbotapi.BotAPI, offset, timeout int) ([]inboundUpdate, error) {
	params := url.Values{"timeout": {strconv.Itoa(timeout)}}
	if offset != 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	resp, err := bot.MakeRequest("getUpdates", params)
	if err != nil {
		return nil, err
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(resp.Result, &raws); err != nil {
		return nil, fmt.Errorf("decode updates: %w", err)
	}
	updates := make([]inboundUpdate, 0, len(raws))
	for _, raw := range raws {
		u := inboundUpdate{raw: raw}
		if err := json.Unmarshal(raw, &u.Update); err != nil {
			log.Warn("decode update failed", zap.Error(err))
		}
		if t, err := parseRaw(string(raw)); err == nil {
			u.threadID = t.threadID
		}
		updates = append(updates, u)
	}
	return updates, nil
}

func sendTyping(bot *tgbotapi.BotAPI, t target) {
	params := t.params()
	params["action"] = tgbotapi.ChatTyping
	if _, err := bot.MakeRequest("sendChatAction", formValues(params)); err != nil {
		log.Warn("send typing action failed", zap.Error(err))
	}
}

func (r *TelegramReceiver) handleMigration(ctx context.Context, m *tgbotapi.Message) bool {
	switch {
	case m.MigrateToChatID != 0:
		migrateChat(ctx, r.migrator, m.Chat.ID, m.MigrateToChatID)
	case m.MigrateFromChatID != 0:
		migrateChat(ctx, r.migrator, m.MigrateFromChatID, m.Chat.ID)
	default:
		return false
	}
	return true
}

// handleAuth processes auth commands and blocks unauthorized users.
// Returns true if the message was consumed (either an auth command or unauthorized).
func (r *TelegramReceiver) handleAuth(bot *tgbotapi.BotAPI, m *tgbotapi.Message, senderID string) bool {
	parts := strings.Fields(strings.TrimSpace(m.Text))

	// Match "/auth" exactly (also handles Telegram's "/auth@botname" form).
	if len(parts) > 0 && (parts[0] == "/auth" || strings.HasPrefix(parts[0], "/auth@")) {
		if len(parts) < 2 {
			replyText(bot, m, "Usage: /auth <code>")
			return true
		}
		if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(r.cfg.AuthCode)) == 1 {
			r.authStore.Authorize(senderID)
			replyText(bot, m, "✅ Authorization successful.")
			log.Info("user authorized", zap.String("senderID", senderID))
		} else {
			replyText(bot, m, "❌ Invalid authorization code.")
			log.Warn("auth failed: invalid code", zap.String("senderID", senderID))
		}
		return true
	}

	if !r.authStore.IsAuthorized(senderID) {
		r.replyUnauthorized(bot, m, senderID)
		return true
	}

	return false
}

const unauthReplyCooldown = 60 * time.Second

// replyUnauthorized sends an unauthorized hint at most once per sender per cooldown period.
func (r *TelegramReceiver) replyUnauthorized(bot *tgbotapi.BotAPI, m *tgbotapi.Message, senderID string) {
	r.unauthReplyMu.Lock()
	last := r.unauthReplyLast[senderID]
	now := time.Now()
	if now.Sub(last) < unauthReplyCooldown {
		r.unauthReplyMu.Unlock()
		return
	}
	r.unauthReplyLast[senderID] = now
	r.unauthReplyMu.Unlock()

	replyText(bot, m, "🔒 Unauthorized. Please use /auth <code> to authenticate.")
}

func replyText(bot *tgbotapi.BotAPI, m *tgbotapi.Message, text string) {
	reply := tgbotapi.NewMessage(m.Chat.ID, text)
	reply.ReplyToMessageID = m.MessageID
	if _, err := bot.Send(reply); err != nil {
		log.Warn("send auth reply failed", zap.Error(err))
	}
}

func (r *TelegramReceiver) buildInboundMessage(
	ctx context.Context,
	bot *tgbotapi.BotAPI,
	update inboundUpdate,
) *platform.InboundMessage {
	m := update.Message
	senderID := int64(0)
	if m.From != nil {
		senderID = int64(m.From.ID)
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
		content = r.downloadLargestPhoto(ctx, bot, *m.Photo)
	case m.Document != nil:
		content = r.downloadFile(ctx, bot, m.Document.FileID, m.Document.FileName, "document")
	case m.Audio != nil:
		content = r.downloadFile(ctx, bot, m.Audio.FileID, "", "audio")
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

	return &platform.InboundMessage{
		Platform:          PlatformID,
		SenderID:          strconv.FormatInt(senderID, 10),
		SessionKey:        buildSessionKey(m.Chat.ID, update.threadID, senderID),
		Content:           content,
		RawContent:        content,
		Raw:               string(update.raw),
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

type TelegramSender struct {
	cfg      config.TelegramConfig
	bot      *tgbotapi.BotAPI
	migrator SessionKeyMigrator
}

func (s *TelegramSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	t, err := sendTarget(msg)
	if err != nil {
		return err
	}
	err = s.deliver(t, msg)
	var apiErr tgbotapi.Error
	if !errors.As(err, &apiErr) || apiErr.MigrateToChatID == 0 {
		return err
	}
	migrateChat(ctx, s.migrator, t.chatID, apiErr.MigrateToChatID)
	return s.deliver(target{chatID: apiErr.MigrateToChatID}, msg)
}

func (s *TelegramSender) deliver(t target, msg platform.OutboundMessage) error {
	if msg.MediaPath != "" {
		return s.sendMedia(t, msg.MediaPath)
	}
	return s.sendText(t, msg.Content)
}

func sendTarget(msg platform.OutboundMessage) (target, error) {
	if !msg.Proactive {
		t, err := parseRaw(msg.ReplyTo.Raw)
		if err != nil {
			return target{}, fmt.Errorf("parse reply context: %w", err)
		}
		return t, nil
	}
	return parseSessionKey(msg.SessionKey)
}

func (s *TelegramSender) sendText(t target, text string) error {
	params := t.params()
	params["text"] = escapeHTML(text)
	params["parse_mode"] = tgbotapi.ModeHTML
	if _, err := s.bot.MakeRequest("sendMessage", formValues(params)); err != nil {
		return fmt.Errorf("send text: %w", err)
	}
	return nil
}

func (s *TelegramSender) sendMedia(t target, mediaPath string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("read media file: %w", err)
	}
	maxSize := s.cfg.MaxMediaSize
	if len(data) > maxSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", len(data), maxSize)
	}

	fileName := platform.SanitizeFileName(filepath.Base(mediaPath))
	method, field := mediaUpload(filepath.Ext(mediaPath))
	file := tgbotapi.FileBytes{Name: fileName, Bytes: data}
	if _, err := s.bot.UploadFile(method, t.params(), field, file); err != nil {
		return fmt.Errorf("send media: %w", err)
	}
	return nil
}

func mediaUpload(ext string) (method, field string) {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return "sendPhoto", "photo"
	case ".mp4", ".mov", ".avi":
		return "sendVideo", "video"
	case ".mp3", ".wav", ".flac", ".m4a", ".aac":
		return "sendAudio", "audio"
	case ".ogg", ".opus":
		return "sendVoice", "voice"
	default:
		return "sendDocument", "document"
	}
}

// Interface compliance guards.
var _ platform.Platform                = (*TelegramPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*TelegramReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*TelegramSender)(nil)
