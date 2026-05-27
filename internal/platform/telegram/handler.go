package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	accountName string
	receiver    *TelegramReceiver
	sender      *TelegramSender
}

func NewPlatform(cfg config.TelegramConfig, mediaSvc *media.Service) platform.Platform {
	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		panic(fmt.Sprintf("telegram: invalid token: %v", err))
	}
	var authStore *AuthStore
	if cfg.AuthCode != "" {
		authStore = newAuthStore()
	}

	setupBotCommands(bot)

	p := &TelegramPlatform{accountName: cfg.Name}
	p.receiver = &TelegramReceiver{
		cfg: cfg, mediaSvc: mediaSvc, bot: bot, authStore: authStore,
		unauthReplyLast: make(map[string]time.Time),
	}
	p.sender = &TelegramSender{cfg: cfg, bot: bot}
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
func (p *TelegramPlatform) AccountName() string                         { return p.accountName }
func (p *TelegramPlatform) Receiver() platform.PlatformReceiverAdapter  { return p.receiver }
func (p *TelegramPlatform) Sender() platform.PlatformSenderAdapter      { return p.sender }

// ─── Helpers ──────────────────────────────────────────────────────────────────

func buildSessionKey(account string, chatID, senderID int64) string {
	return fmt.Sprintf("telegram:%s:%d:%d", account, chatID, senderID)
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

type telegramRaw struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		MessageID int `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

func parseRaw(raw string) (chatID int64, messageID int, err error) {
	var r telegramRaw
	if err = json.Unmarshal([]byte(raw), &r); err != nil {
		return 0, 0, fmt.Errorf("parse telegram raw: %w", err)
	}
	return r.Message.Chat.ID, r.Message.MessageID, nil
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

	// Rate-limit unauthorized reply: at most one per sender per 60s.
	unauthReplyMu   sync.Mutex
	unauthReplyLast map[string]time.Time
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

			// Handle auth if enabled.
			if r.authStore != nil && update.Message.From != nil {
				senderID := strconv.FormatInt(int64(update.Message.From.ID), 10)
				if r.handleAuth(bot, update.Message, senderID) {
					continue
				}
			}

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
	update tgbotapi.Update,
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

	rawBytes, err := json.Marshal(update)
	if err != nil {
		log.Error("marshal update", zap.Error(err))
		return nil
	}

	return &platform.InboundMessage{
		Platform:          PlatformID,
		AccountName:       r.cfg.Name,
		SenderID:          strconv.FormatInt(senderID, 10),
		SessionKey:        buildSessionKey(r.cfg.Name, m.Chat.ID, senderID),
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
		photo := tgbotapi.NewPhotoUpload(chatID, fileBytes)
		if replyToID != 0 {
			photo.ReplyToMessageID = replyToID
		}
		chattable = photo
	case ".mp4", ".mov", ".avi":
		video := tgbotapi.NewVideoUpload(chatID, fileBytes)
		if replyToID != 0 {
			video.ReplyToMessageID = replyToID
		}
		chattable = video
	case ".mp3", ".wav", ".flac", ".m4a", ".aac":
		audio := tgbotapi.NewAudioUpload(chatID, fileBytes)
		if replyToID != 0 {
			audio.ReplyToMessageID = replyToID
		}
		chattable = audio
	case ".ogg", ".opus":
		voice := tgbotapi.NewVoiceUpload(chatID, fileBytes)
		if replyToID != 0 {
			voice.ReplyToMessageID = replyToID
		}
		chattable = voice
	default:
		doc := tgbotapi.NewDocumentUpload(chatID, fileBytes)
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
var _ platform.Platform                = (*TelegramPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*TelegramReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*TelegramSender)(nil)
