package weixin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/media"
	"github.com/theopenbee/openbee/internal/platform"
)

var log = logger.With(zap.String("component", "weixin"))

var _ platform.Platform = (*WeixinPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*WeixinReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*WeixinSender)(nil)

// WeixinPlatform implements platform.Platform for WeChat intelligent agent.
type WeixinPlatform struct {
	cfg      config.WeixinConfig
	media    *media.Service
	receiver *WeixinReceiver
	sender   *WeixinSender
	client   *WeixinAPIClient
}

// NewPlatform creates a new WeChat platform instance.
func NewPlatform(cfg config.WeixinConfig, mediaSvc *media.Service) platform.Platform {
	client := NewAPIClient(cfg.BaseURL, cfg.CdnBaseURL, cfg.Token)
	p := &WeixinPlatform{
		cfg:    cfg,
		media:  mediaSvc,
		client: client,
	}
	p.receiver = &WeixinReceiver{cfg: cfg, media: mediaSvc, client: client, session: newSessionManager()}
	p.sender = &WeixinSender{cfg: cfg, media: mediaSvc, client: client}
	return p
}

func (p *WeixinPlatform) ID() string                                 { return "weixin" }
func (p *WeixinPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *WeixinPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// --- Receiver ---

// WeixinReceiver implements long-polling message reception.
type WeixinReceiver struct {
	cfg     config.WeixinConfig
	media   *media.Service
	client  *WeixinAPIClient
	session *sessionManager
}

// Start begins the long-polling loop and dispatches messages.
func (r *WeixinReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	log.Info("weixin receiver starting")

	syncBuf := loadSyncBuf()
	consecutiveFailures := 0

	for {
		if ctx.Err() != nil {
			return nil
		}

		if r.session.isPaused() {
			remaining := r.session.remainingPause()
			log.Warn("session paused", zap.Duration("remaining", remaining))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(30 * time.Second):
				continue
			}
		}

		resp, err := r.client.GetUpdates(ctx, syncBuf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			consecutiveFailures++
			if consecutiveFailures >= 3 {
				log.Error("3 consecutive failures, backing off 30s", zap.Error(err))
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(30 * time.Second):
				}
			}
			continue
		}

		// Check for session timeout
		if resp.ErrCode == -14 {
			log.Warn("session timeout (errcode -14), pausing for 1 hour. Consider re-running 'openbee config' to re-login.")
			r.session.pause()
			consecutiveFailures = 0
			continue
		}

		consecutiveFailures = 0

		// Persist sync cursor
		if resp.GetUpdatesBuf != "" {
			syncBuf = resp.GetUpdatesBuf
			saveSyncBuf(syncBuf)
		}

		// Process messages
		for _, msg := range resp.Msgs {
			if msg.MessageType != MessageTypeUser || msg.MessageState != MessageStateFinish {
				continue
			}
			inbound := r.buildInboundMessage(ctx, msg)
			if inbound == nil {
				continue
			}

			// Send typing indicator in background
			go r.sendTypingIndicator(ctx, msg)

			dispatch(*inbound)
		}
	}
}

func (r *WeixinReceiver) buildInboundMessage(ctx context.Context, msg WeixinMessage) *platform.InboundMessage {
	if msg.FromUserID == "" {
		log.Warn("skipping message with no sender")
		return nil
	}

	var textParts []string
	var contentParts []string

	for _, item := range msg.ItemList {
		switch item.Type {
		case MessageItemTypeText:
			if item.TextItem != nil {
				textParts = append(textParts, item.TextItem.Text)
				contentParts = append(contentParts, item.TextItem.Text)
			}

		case MessageItemTypeImage:
			if item.ImageItem != nil {
				placeholder := r.downloadMedia(ctx, item.ImageItem.CDNMedia, "image", "")
				if placeholder != "" {
					contentParts = append(contentParts, placeholder)
				}
			}

		case MessageItemTypeVoice:
			if item.VoiceItem != nil {
				placeholder := r.downloadMedia(ctx, item.VoiceItem.CDNMedia, "audio", "")
				if placeholder != "" {
					contentParts = append(contentParts, placeholder)
				}
				if item.VoiceItem.Text != "" {
					textParts = append(textParts, "[语音转文字] "+item.VoiceItem.Text)
					contentParts = append(contentParts, "[语音转文字] "+item.VoiceItem.Text)
				}
			}

		case MessageItemTypeFile:
			if item.FileItem != nil {
				placeholder := r.downloadMedia(ctx, item.FileItem.CDNMedia, "document", item.FileItem.FileName)
				if placeholder != "" {
					contentParts = append(contentParts, placeholder)
				}
			}

		case MessageItemTypeVideo:
			if item.VideoItem != nil {
				placeholder := r.downloadMedia(ctx, item.VideoItem.CDNMedia, "video", "")
				if placeholder != "" {
					contentParts = append(contentParts, placeholder)
				}
			}
		}
	}

	content := strings.Join(contentParts, "\n")
	if content == "" {
		log.Warn("skipping empty message", zap.Int64("message_id", msg.MessageID))
		return nil
	}

	rawBytes, err := json.Marshal(msg)
	if err != nil {
		log.Error("marshal raw message", zap.Error(err))
		return nil
	}

	return &platform.InboundMessage{
		Platform:          "weixin",
		SenderID:          msg.FromUserID,
		SessionKey:        fmt.Sprintf("weixin:%s:%s", msg.FromUserID, msg.FromUserID),
		Content:           content,
		RawContent:        strings.Join(textParts, "\n"),
		Raw:               string(rawBytes),
		PlatformMessageID: strconv.FormatInt(msg.MessageID, 10),
		MessageTime:       msg.CreateTimeMs,
	}
}

func (r *WeixinReceiver) downloadMedia(ctx context.Context, cdn CDNMedia, mediaType, fileName string) string {
	if cdn.EncryptQueryParam == "" || cdn.AesKey == "" {
		log.Warn("missing CDN params, skipping media download")
		return ""
	}

	data, err := downloadAndDecrypt(ctx, r.client.cdnBaseUrl, cdn.EncryptQueryParam, cdn.AesKey)
	if err != nil {
		log.Error("download media failed", zap.Error(err), zap.String("type", mediaType))
		return ""
	}

	contentType := r.media.DetectMIME(data, fileName)
	ext := r.media.ExtensionFromMIME(contentType)

	path, err := r.media.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Error("save media failed", zap.Error(err))
		return ""
	}

	return r.media.BuildPlaceholder(mediaType, path, fileName)
}

func (r *WeixinReceiver) sendTypingIndicator(ctx context.Context, msg WeixinMessage) {
	configResp, err := r.client.GetConfig(ctx, msg.FromUserID, msg.ContextToken)
	if err != nil {
		log.Debug("get config for typing failed", zap.Error(err))
		return
	}
	if configResp.TypingTicket == "" {
		return
	}
	if err := r.client.SendTyping(ctx, msg.FromUserID, configResp.TypingTicket, TypingStatusTyping); err != nil {
		log.Debug("send typing failed", zap.Error(err))
	}
}

// --- Sender ---

// WeixinSender implements message sending to WeChat.
type WeixinSender struct {
	cfg    config.WeixinConfig
	media  *media.Service
	client *WeixinAPIClient
}

// Send sends an outbound message.
func (s *WeixinSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var rawMsg WeixinMessage
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &rawMsg); err != nil {
		return fmt.Errorf("weixin: parse reply context: %w", err)
	}

	toUserID := rawMsg.FromUserID
	contextToken := rawMsg.ContextToken

	// Handle media
	if msg.MediaPath != "" {
		return s.sendMedia(ctx, toUserID, contextToken, msg.MediaPath, msg.Content)
	}

	// Text only — strip markdown
	text := markdownToPlainText(msg.Content)
	return s.sendTextMessage(ctx, toUserID, contextToken, text)
}

func (s *WeixinSender) sendTextMessage(ctx context.Context, toUserID, contextToken, text string) error {
	weixinMsg := &WeixinMessage{
		ToUserID:     toUserID,
		MessageType:  MessageTypeBot,
		ContextToken: contextToken,
		ItemList: []MessageItem{
			{
				Type:     MessageItemTypeText,
				TextItem: &TextItem{Text: text},
			},
		},
	}
	return s.client.SendMessage(ctx, weixinMsg)
}

func (s *WeixinSender) sendMedia(ctx context.Context, toUserID, contextToken, mediaPath, caption string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("weixin: read media: %w", err)
	}
	if len(data) > s.cfg.MaxMediaSize {
		log.Warn("media too large, sending text only", zap.Int("size", len(data)), zap.Int("max", s.cfg.MaxMediaSize))
		return s.sendTextMessage(ctx, toUserID, contextToken, markdownToPlainText(caption))
	}

	contentType := s.media.DetectMIME(data, filepath.Base(mediaPath))
	cdnType := mediaCDNType(contentType)

	// Encrypt locally
	key := make([]byte, 16)
	if _, err := cryptoRandRead(key); err != nil {
		return fmt.Errorf("weixin: generate key: %w", err)
	}
	ciphertext := encryptAES128ECB(data, key)
	aesKeyHex := fmt.Sprintf("%x", key)

	// Get upload URL
	uploadReq := GetUploadUrlReq{
		FileKey:    fmt.Sprintf("weixin_%d", time.Now().UnixMilli()),
		MediaType:  cdnType,
		ToUserID:   toUserID,
		RawSize:    int64(len(data)),
		RawFileMD5: computeMD5(data),
		FileSize:   int64(len(ciphertext)),
		AesKey:     aesKeyHex,
	}

	uploadResp, err := s.client.GetUploadUrl(ctx, uploadReq)
	if err != nil {
		return fmt.Errorf("weixin: get upload url: %w", err)
	}

	// Upload ciphertext to CDN with retries
	var dlParam string
	var uploadErr error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		dlParam, uploadErr = doUpload(ctx, uploadResp.UploadParam, ciphertext)
		if uploadErr == nil {
			break
		}
	}
	if uploadErr != nil {
		log.Warn("CDN upload failed, falling back to text", zap.Error(uploadErr))
		return s.sendTextMessage(ctx, toUserID, contextToken, markdownToPlainText(caption))
	}

	// Build message with media item
	cdnMedia := CDNMedia{
		EncryptQueryParam: dlParam,
		AesKey:            base64.StdEncoding.EncodeToString(key),
	}

	var item MessageItem
	switch cdnType {
	case 1: // image
		item = MessageItem{Type: MessageItemTypeImage, ImageItem: &ImageItem{CDNMedia: cdnMedia}}
	case 2: // video
		item = MessageItem{Type: MessageItemTypeVideo, VideoItem: &VideoItem{CDNMedia: cdnMedia}}
	case 4: // voice
		item = MessageItem{Type: MessageItemTypeVoice, VoiceItem: &VoiceItem{CDNMedia: cdnMedia}}
	default: // file
		item = MessageItem{Type: MessageItemTypeFile, FileItem: &FileItem{CDNMedia: cdnMedia, FileName: filepath.Base(mediaPath), FileSize: int64(len(data))}}
	}

	weixinMsg := &WeixinMessage{
		ToUserID:     toUserID,
		MessageType:  MessageTypeBot,
		ContextToken: contextToken,
		ItemList:     []MessageItem{item},
	}

	// Add text caption if present
	text := markdownToPlainText(caption)
	if text != "" {
		weixinMsg.ItemList = append([]MessageItem{
			{Type: MessageItemTypeText, TextItem: &TextItem{Text: text}},
		}, weixinMsg.ItemList...)
	}

	return s.client.SendMessage(ctx, weixinMsg)
}

// cryptoRandRead wraps crypto/rand.Read for testability.
var cryptoRandRead = func(b []byte) (int, error) { return rand.Read(b) }

// --- Sync Buffer Persistence ---

func syncBufPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "weixin", "sync.json")
}

func loadSyncBuf() string {
	data, err := os.ReadFile(syncBufPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveSyncBuf(buf string) {
	path := syncBufPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Error("create sync dir", zap.Error(err))
		return
	}
	if err := os.WriteFile(path, []byte(buf), 0o644); err != nil {
		log.Error("save sync buf", zap.Error(err))
	}
}
