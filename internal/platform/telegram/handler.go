package telegram

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
)

const PlatformID = "telegram"

const (
	pollTimeout = time.Minute
	httpTimeout = 5 * time.Minute
)

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
	var authStore *AuthStore
	if cfg.AuthCode != "" {
		authStore = newAuthStore()
	}

	p := &TelegramPlatform{}
	p.receiver = &TelegramReceiver{
		cfg: cfg, mediaSvc: mediaSvc, authStore: authStore, migrator: migrator,
		unauthReplyLast: make(map[string]time.Time),
	}
	b, err := newBot(cfg.Token, p.receiver.handleUpdate)
	if err != nil {
		panic(fmt.Sprintf("telegram: invalid token: %v", err))
	}
	p.receiver.bot = b
	p.sender = &TelegramSender{cfg: cfg, bot: b, migrator: migrator}

	setupBotCommands(b)
	return p
}

func newBot(token string, handler bot.HandlerFunc, opts ...bot.Option) (*bot.Bot, error) {
	return bot.New(token, append([]bot.Option{
		bot.WithDefaultHandler(handler),
		bot.WithNotAsyncHandlers(),
		bot.WithHTTPClient(pollTimeout, &http.Client{Timeout: httpTimeout}),
		bot.WithErrorsHandler(func(err error) {
			log.Error("telegram bot error", zap.Error(err))
		}),
	}, opts...)...)
}

// setupBotCommands registers the bot's menu commands with Telegram.
func setupBotCommands(b *bot.Bot) {
	commands := []models.BotCommand{
		{Command: "clear", Description: "Clear conversation history"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: commands}); err != nil {
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

func buildPlatformMessageID(updateID int64, messageID int) string {
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

func (t target) replyParameters() *models.ReplyParameters {
	if t.replyToID == 0 {
		return nil
	}
	return &models.ReplyParameters{MessageID: t.replyToID}
}

func topicThreadID(m *models.Message) int {
	if m.IsTopicMessage {
		return m.MessageThreadID
	}
	return 0
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
	bot       *bot.Bot
	authStore *AuthStore // nil when auth_code is empty (no auth required)
	migrator  SessionKeyMigrator
	dispatch  func(platform.InboundMessage)

	// Rate-limit unauthorized reply: at most one per sender per 60s.
	unauthReplyMu   sync.Mutex
	unauthReplyLast map[string]time.Time
}

func (r *TelegramReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	r.dispatch = dispatch
	log.Info("Telegram bot started", zap.Int64("botID", r.bot.ID()))
	r.bot.Start(ctx)
	return nil
}

func (r *TelegramReceiver) handleUpdate(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil {
		return
	}

	if r.handleMigration(ctx, m) {
		return
	}

	// Handle auth if enabled.
	if r.authStore != nil && m.From != nil {
		senderID := strconv.FormatInt(m.From.ID, 10)
		if r.handleAuth(ctx, b, m, senderID) {
			return
		}
	}

	go sendTyping(ctx, b, target{chatID: m.Chat.ID, threadID: topicThreadID(m)})

	msg := r.buildInboundMessage(ctx, update)
	if msg == nil {
		return
	}
	r.dispatch(*msg)
}

func sendTyping(ctx context.Context, b *bot.Bot, t target) {
	if _, err := b.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID:          t.chatID,
		MessageThreadID: t.threadID,
		Action:          models.ChatActionTyping,
	}); err != nil {
		log.Warn("send typing action failed", zap.Error(err))
	}
}

func (r *TelegramReceiver) handleMigration(ctx context.Context, m *models.Message) bool {
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
func (r *TelegramReceiver) handleAuth(ctx context.Context, b *bot.Bot, m *models.Message, senderID string) bool {
	parts := strings.Fields(strings.TrimSpace(m.Text))

	// Match "/auth" exactly (also handles Telegram's "/auth@botname" form).
	if len(parts) > 0 && (parts[0] == "/auth" || strings.HasPrefix(parts[0], "/auth@")) {
		if len(parts) < 2 {
			replyText(ctx, b, m, "Usage: /auth <code>")
			return true
		}
		if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(r.cfg.AuthCode)) == 1 {
			r.authStore.Authorize(senderID)
			replyText(ctx, b, m, "✅ Authorization successful.")
			log.Info("user authorized", zap.String("senderID", senderID))
		} else {
			replyText(ctx, b, m, "❌ Invalid authorization code.")
			log.Warn("auth failed: invalid code", zap.String("senderID", senderID))
		}
		return true
	}

	if !r.authStore.IsAuthorized(senderID) {
		r.replyUnauthorized(ctx, b, m, senderID)
		return true
	}

	return false
}

const unauthReplyCooldown = 60 * time.Second

// replyUnauthorized sends an unauthorized hint at most once per sender per cooldown period.
func (r *TelegramReceiver) replyUnauthorized(ctx context.Context, b *bot.Bot, m *models.Message, senderID string) {
	r.unauthReplyMu.Lock()
	last := r.unauthReplyLast[senderID]
	now := time.Now()
	if now.Sub(last) < unauthReplyCooldown {
		r.unauthReplyMu.Unlock()
		return
	}
	r.unauthReplyLast[senderID] = now
	r.unauthReplyMu.Unlock()

	replyText(ctx, b, m, "🔒 Unauthorized. Please use /auth <code> to authenticate.")
}

func replyText(ctx context.Context, b *bot.Bot, m *models.Message, text string) {
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          m.Chat.ID,
		MessageThreadID: topicThreadID(m),
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: m.ID},
	}); err != nil {
		log.Warn("send auth reply failed", zap.Error(err))
	}
}

func (r *TelegramReceiver) buildInboundMessage(ctx context.Context, update *models.Update) *platform.InboundMessage {
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
		content = r.downloadLargestPhoto(ctx, m.Photo)
	case m.Document != nil:
		content = r.downloadFile(ctx, m.Document.FileID, m.Document.FileName, "document")
	case m.Audio != nil:
		content = r.downloadFile(ctx, m.Audio.FileID, "", "audio")
	case m.Voice != nil:
		content = r.downloadFile(ctx, m.Voice.FileID, "voice.ogg", "audio")
	case m.Video != nil:
		content = r.downloadFile(ctx, m.Video.FileID, "video.mp4", "video")
	case m.Sticker != nil:
		content = r.downloadFile(ctx, m.Sticker.FileID, "sticker.webp", "sticker")
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
		Platform:          PlatformID,
		SenderID:          strconv.FormatInt(senderID, 10),
		SessionKey:        buildSessionKey(m.Chat.ID, topicThreadID(m), senderID),
		Content:           content,
		RawContent:        content,
		Raw:               string(rawBytes),
		PlatformMessageID: buildPlatformMessageID(update.ID, m.ID),
		MessageTime:       int64(m.Date) * 1000,
	}
}

func (r *TelegramReceiver) downloadLargestPhoto(ctx context.Context, photos []models.PhotoSize) string {
	if len(photos) == 0 {
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	largest := photos[len(photos)-1]
	return r.downloadFile(ctx, largest.FileID, "photo.jpg", "image")
}

func (r *TelegramReceiver) downloadFile(ctx context.Context, fileID, fileName, mediaType string) string {
	f, err := r.bot.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		log.Error("get file info failed", zap.String("fileID", fileID), zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	url := r.bot.FileDownloadLink(f)

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
	bot      *bot.Bot
	migrator SessionKeyMigrator
}

func (s *TelegramSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	t, err := sendTarget(msg)
	if err != nil {
		return err
	}
	err = s.deliver(ctx, t, msg)
	var migrateErr *bot.MigrateError
	if !errors.As(err, &migrateErr) {
		return err
	}
	to := int64(migrateErr.MigrateToChatID)
	migrateChat(ctx, s.migrator, t.chatID, to)
	return s.deliver(ctx, target{chatID: to}, msg)
}

func (s *TelegramSender) deliver(ctx context.Context, t target, msg platform.OutboundMessage) error {
	if msg.MediaPath != "" {
		return s.sendMedia(ctx, t, msg.MediaPath)
	}
	return s.sendText(ctx, t, msg.Content)
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

func (s *TelegramSender) sendText(ctx context.Context, t target, text string) error {
	if _, err := s.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          t.chatID,
		MessageThreadID: t.threadID,
		Text:            escapeHTML(text),
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: t.replyParameters(),
	}); err != nil {
		return fmt.Errorf("send text: %w", err)
	}
	return nil
}

func (s *TelegramSender) sendMedia(ctx context.Context, t target, mediaPath string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("read media file: %w", err)
	}
	maxSize := s.cfg.MaxMediaSize
	if len(data) > maxSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", len(data), maxSize)
	}

	fileName := platform.SanitizeFileName(filepath.Base(mediaPath))
	file := &models.InputFileUpload{Filename: fileName, Data: bytes.NewReader(data)}
	reply := t.replyParameters()

	switch strings.ToLower(filepath.Ext(mediaPath)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		_, err = s.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID: t.chatID, MessageThreadID: t.threadID, Photo: file, ReplyParameters: reply,
		})
	case ".mp4", ".mov", ".avi":
		_, err = s.bot.SendVideo(ctx, &bot.SendVideoParams{
			ChatID: t.chatID, MessageThreadID: t.threadID, Video: file, ReplyParameters: reply,
		})
	case ".mp3", ".wav", ".flac", ".m4a", ".aac":
		_, err = s.bot.SendAudio(ctx, &bot.SendAudioParams{
			ChatID: t.chatID, MessageThreadID: t.threadID, Audio: file, ReplyParameters: reply,
		})
	case ".ogg", ".opus":
		_, err = s.bot.SendVoice(ctx, &bot.SendVoiceParams{
			ChatID: t.chatID, MessageThreadID: t.threadID, Voice: file, ReplyParameters: reply,
		})
	default:
		_, err = s.bot.SendDocument(ctx, &bot.SendDocumentParams{
			ChatID: t.chatID, MessageThreadID: t.threadID, Document: file, ReplyParameters: reply,
		})
	}
	if err != nil {
		return fmt.Errorf("send media: %w", err)
	}
	return nil
}

// Interface compliance guards.
var _ platform.Platform                = (*TelegramPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*TelegramReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*TelegramSender)(nil)
