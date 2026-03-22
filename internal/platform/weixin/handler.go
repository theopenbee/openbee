package weixin

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/media"
	"github.com/theopenbee/openbee/internal/platform"
)

// ─── Platform ────────────────────────────────────────────────────────────────

type WeixinPlatform struct {
	receiver *WeixinReceiver
	sender   *WeixinSender
}

func NewPlatform(cfg config.WeixinConfig, mc config.MediaConfig, mediaSvc *media.Service) platform.Platform {
	client := newAPIClient(cfg)
	p := &WeixinPlatform{}
	p.receiver = &WeixinReceiver{
		cfg:      cfg,
		mc:       mc,
		mediaSvc: mediaSvc,
		client:   client,
	}
	p.sender = &WeixinSender{
		cfg:    cfg,
		client: client,
	}
	return p
}

func (p *WeixinPlatform) ID() string                                 { return "weixin" }
func (p *WeixinPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *WeixinPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// ─── Helpers ─────────────────────────────────────────────────────────────────

type weixinRaw struct {
	FromUserID   string `json:"from_user_id"`
	ToUserID     string `json:"to_user_id"`
	SessionID    string `json:"session_id"`
	ContextToken string `json:"context_token"`
}

func buildSessionKey(userID string) string {
	return fmt.Sprintf("weixin:%s:%s", userID, userID)
}

func buildPlatformMessageID(seq, messageID int64) string {
	return fmt.Sprintf("%d:%d", seq, messageID)
}

func parseWeixinRaw(raw string) (weixinRaw, error) {
	var r weixinRaw
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return weixinRaw{}, fmt.Errorf("parse weixin raw: %w", err)
	}
	return r, nil
}

func extractTextContent(items []messageItem) string {
	var parts []string
	for _, item := range items {
		if item.Type == 1 && item.TextItem != nil {
			parts = append(parts, item.TextItem.Text)
		}
	}
	return strings.Join(parts, "")
}

func sanitizeTokenForLog(token string) string {
	if len(token) <= 6 {
		return token
	}
	return token[:6] + "***"
}

// ─── Receiver ────────────────────────────────────────────────────────────────

type WeixinReceiver struct {
	cfg      config.WeixinConfig
	mc       config.MediaConfig
	mediaSvc *media.Service
	client   *apiClient

	// typing state
	typingTicket       string
	typingTicketMu     sync.Mutex
	ticketCachedAt     time.Time
	ticketBackoff      time.Duration // exponential backoff for getconfig failures
	ticketBackoffUntil time.Time

	// session pause
	pauseUntil time.Time
}

// Note: the `log` package-level variable is declared in api.go (same package).

const (
	messageTypeUser    = 1
	messageStateFinish = 2
	typingStart        = 1
	ticketCacheTTL     = 24 * time.Hour
)

func (r *WeixinReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	log.Info("weixin receiver started",
		zap.String("token", sanitizeTokenForLog(r.cfg.Token)))

	var cursor string
	var consecutiveFailures int

	for {
		if ctx.Err() != nil {
			return nil
		}

		// Check session pause
		if !r.pauseUntil.IsZero() && time.Now().Before(r.pauseUntil) {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(30 * time.Second):
				continue
			}
		}
		r.pauseUntil = time.Time{}

		resp, err := r.client.getUpdates(ctx, cursor)
		if err != nil {
			consecutiveFailures++
			delay := 5 * time.Second
			if consecutiveFailures >= 3 {
				delay = 30 * time.Second
			}
			log.Error("getUpdates error", zap.Error(err),
				zap.Int("consecutive_failures", consecutiveFailures))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
				continue
			}
		}
		consecutiveFailures = 0

		// Check session expiry
		if resp.Ret == -14 {
			log.Warn("weixin session expired, pausing for 60 minutes")
			r.pauseUntil = time.Now().Add(60 * time.Minute)
			continue
		}

		if resp.Ret != 0 {
			log.Warn("getUpdates non-zero ret", zap.Int("ret", resp.Ret))
			continue
		}

		// Update cursor
		if resp.GetUpdatesBuf != "" {
			cursor = resp.GetUpdatesBuf
		}

		for _, msg := range resp.Msgs {
			// Filter: only USER messages in FINISH state
			if msg.MessageType != messageTypeUser || msg.MessageState != messageStateFinish {
				continue
			}

			// Skip group messages
			if msg.GroupID != "" {
				log.Debug("skipping group message", zap.String("group_id", msg.GroupID))
				continue
			}

			// Skip empty item_list
			if len(msg.ItemList) == 0 {
				continue
			}

			// Send typing in background
			go r.sendTypingAction(ctx)

			inbound := r.buildInboundMessage(ctx, msg)
			if inbound == nil {
				continue
			}
			dispatch(*inbound)
		}
	}
}

func (r *WeixinReceiver) buildInboundMessage(ctx context.Context, msg weixinMessage) *platform.InboundMessage {
	content := r.extractContent(ctx, msg.ItemList)
	if content == "" {
		return nil
	}

	rawData := weixinRaw{
		FromUserID:   msg.FromUserID,
		ToUserID:     msg.ToUserID,
		SessionID:    msg.SessionID,
		ContextToken: msg.ContextToken,
	}
	rawBytes, err := json.Marshal(rawData)
	if err != nil {
		log.Error("marshal weixin raw", zap.Error(err))
		return nil
	}

	return &platform.InboundMessage{
		Platform:          "weixin",
		SenderID:          msg.FromUserID,
		SessionKey:        buildSessionKey(msg.FromUserID),
		Content:           content,
		RawContent:        content,
		Raw:               string(rawBytes),
		PlatformMessageID: buildPlatformMessageID(msg.Seq, msg.MessageID),
		MessageTime:       msg.CreateTimeMs,
	}
}

func (r *WeixinReceiver) extractContent(ctx context.Context, items []messageItem) string {
	var parts []string
	for _, item := range items {
		switch item.Type {
		case 1: // TEXT
			if item.TextItem != nil {
				parts = append(parts, item.TextItem.Text)
			}
		case 2: // IMAGE
			if item.ImageItem != nil {
				parts = append(parts, r.downloadMedia(ctx, item.ImageItem.Media, item.ImageItem.AesKey, "image", ""))
			}
		case 3: // VOICE
			if item.VoiceItem != nil {
				parts = append(parts, r.downloadVoice(ctx, item.VoiceItem))
			}
		case 4: // FILE
			if item.FileItem != nil {
				fileName := item.FileItem.FileName
				parts = append(parts, r.downloadMedia(ctx, item.FileItem.Media, "", "document", fileName))
			}
		case 5: // VIDEO
			if item.VideoItem != nil {
				parts = append(parts, r.downloadMedia(ctx, item.VideoItem.Media, "", "video", ""))
			}
		default:
			log.Warn("skipping unknown item type", zap.Int("type", item.Type))
		}
	}
	return strings.Join(parts, " ")
}

func (r *WeixinReceiver) downloadMedia(ctx context.Context, m *cdnMedia, hexAesKey, mediaType, fileName string) string {
	if m == nil || m.EncryptQueryParam == "" {
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	// Determine AES key: hex aeskey field takes priority, then media.aes_key (base64)
	var aesKeyB64 string
	if hexAesKey != "" {
		// Convert hex to raw bytes then base64
		keyBytes, err := hex.DecodeString(hexAesKey)
		if err != nil {
			log.Error("decode hex aeskey", zap.Error(err))
			return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
		}
		aesKeyB64 = base64.StdEncoding.EncodeToString(keyBytes)
	} else {
		aesKeyB64 = m.AesKey
	}

	if aesKeyB64 == "" {
		log.Error("no aes key for media download")
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	data, err := r.client.downloadFromCDN(ctx, m.EncryptQueryParam, aesKeyB64, r.cfg.MaxMediaSize)
	if err != nil {
		log.Error("download media from CDN", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	ext := ""
	if fileName != "" {
		if idx := strings.LastIndex(fileName, "."); idx >= 0 {
			ext = fileName[idx:]
		}
	}
	if ext == "" {
		mime := r.mediaSvc.DetectMIME(data, fileName)
		ext = r.mediaSvc.ExtensionFromMIME(mime)
	}

	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Error("save inbound media", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	return r.mediaSvc.BuildPlaceholder(mediaType, path, fileName)
}

func (r *WeixinReceiver) downloadVoice(ctx context.Context, v *voiceItem) string {
	if v.Media == nil || v.Media.EncryptQueryParam == "" {
		return r.mediaSvc.BuildPlaceholder("audio", "", "voice.silk")
	}

	aesKeyB64 := v.Media.AesKey
	if aesKeyB64 == "" {
		log.Error("no aes key for voice download")
		return r.mediaSvc.BuildPlaceholder("audio", "", "voice.silk")
	}

	data, err := r.client.downloadFromCDN(ctx, v.Media.EncryptQueryParam, aesKeyB64, r.cfg.MaxMediaSize)
	if err != nil {
		log.Error("download voice from CDN", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("audio", "", "voice.silk")
	}

	// Save raw SILK file first
	silkPath, err := r.mediaSvc.SaveInbound(ctx, data, ".silk")
	if err != nil {
		log.Error("save voice file", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("audio", "", "voice.silk")
	}

	// Try SILK→WAV transcoding via FFmpeg
	wavPath, err := r.transcodeSilkToWav(ctx, silkPath)
	if err != nil {
		log.Warn("SILK to WAV transcoding failed, using raw SILK file",
			zap.Error(err), zap.String("silk_path", silkPath))
		return r.mediaSvc.BuildPlaceholder("audio", silkPath, "voice.silk")
	}

	return r.mediaSvc.BuildPlaceholder("audio", wavPath, "voice.wav")
}

func (r *WeixinReceiver) transcodeSilkToWav(ctx context.Context, silkPath string) (string, error) {
	wavPath := strings.TrimSuffix(silkPath, filepath.Ext(silkPath)) + ".wav"
	cmd := exec.CommandContext(ctx, r.mc.FFmpegPath,
		"-i", silkPath,
		"-ar", "16000",
		"-ac", "1",
		"-y", wavPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg: %w: %s", err, string(out))
	}
	return wavPath, nil
}

func (r *WeixinReceiver) sendTypingAction(ctx context.Context) {
	ticket, err := r.getTypingTicket(ctx)
	if err != nil {
		log.Debug("get typing ticket failed", zap.Error(err))
		return
	}
	if err := r.client.sendTyping(ctx, ticket, typingStart); err != nil {
		log.Debug("send typing failed", zap.Error(err))
	}
}

func (r *WeixinReceiver) getTypingTicket(ctx context.Context) (string, error) {
	r.typingTicketMu.Lock()
	defer r.typingTicketMu.Unlock()

	// Return cached ticket if still valid
	if r.typingTicket != "" && time.Since(r.ticketCachedAt) < ticketCacheTTL {
		return r.typingTicket, nil
	}

	// Respect exponential backoff on getconfig failures
	if !r.ticketBackoffUntil.IsZero() && time.Now().Before(r.ticketBackoffUntil) {
		return "", fmt.Errorf("weixin: getconfig in backoff (until %s)", r.ticketBackoffUntil.Format(time.RFC3339))
	}

	resp, err := r.client.getConfig(ctx)
	if err != nil {
		// Exponential backoff: 2s → 4s → 8s → ... → 1h max
		if r.ticketBackoff == 0 {
			r.ticketBackoff = 2 * time.Second
		} else {
			r.ticketBackoff *= 2
			if r.ticketBackoff > time.Hour {
				r.ticketBackoff = time.Hour
			}
		}
		r.ticketBackoffUntil = time.Now().Add(r.ticketBackoff)
		return "", err
	}

	// Success: reset backoff, cache ticket
	r.ticketBackoff = 0
	r.ticketBackoffUntil = time.Time{}
	r.typingTicket = resp.TypingTicket
	r.ticketCachedAt = time.Now()
	return r.typingTicket, nil
}

// ─── Sender ──────────────────────────────────────────────────────────────────

type WeixinSender struct {
	cfg    config.WeixinConfig
	client *apiClient
}

func (s *WeixinSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	raw, err := parseWeixinRaw(msg.ReplyTo.Raw)
	if err != nil {
		return fmt.Errorf("parse reply context: %w", err)
	}

	if msg.MediaPath != "" {
		return s.sendMedia(ctx, raw, msg.MediaPath)
	}
	return s.sendText(ctx, raw, msg.Content)
}

func (s *WeixinSender) sendText(ctx context.Context, raw weixinRaw, text string) error {
	outMsg := weixinMessage{
		ToUserID:     raw.FromUserID,
		ContextToken: raw.ContextToken,
		MessageType:  2, // BOT
		ItemList: []messageItem{
			{
				Type:     1,
				TextItem: &textItem{Text: text},
			},
		},
	}
	return s.client.sendMessage(ctx, outMsg)
}

func (s *WeixinSender) sendMedia(ctx context.Context, raw weixinRaw, mediaPath string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("weixin: read media file: %w", err)
	}
	if len(data) > s.cfg.MaxMediaSize {
		return fmt.Errorf("weixin: file too large: %d bytes (max %d)", len(data), s.cfg.MaxMediaSize)
	}

	mime := http.DetectContentType(data)
	mediaType := mediaTypeFromMIME(mime)

	aesKey := generateAesKey()
	fileKey := generateFileKey()
	rawSize := len(data)
	rawMD5 := fileMD5(data)
	encSize := aesEcbPaddedSize(rawSize)

	uploadReq := getUploadURLReq{
		FileKey:    fileKey,
		MediaType:  mediaType,
		ToUserID:   raw.FromUserID,
		RawSize:    strconv.Itoa(rawSize),
		RawFileMD5: rawMD5,
		FileSize:   strconv.Itoa(encSize),
		AesKey:     hex.EncodeToString(aesKey),
	}

	// Retry up to 3 times for 5xx
	var uploadResp *getUploadURLResp
	for attempt := 0; attempt < 3; attempt++ {
		uploadResp, err = s.client.getUploadURL(ctx, uploadReq)
		if err == nil {
			break
		}
		log.Warn("getUploadURL failed, retrying", zap.Int("attempt", attempt+1), zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("weixin: getUploadURL: %w", err)
	}

	// Upload to CDN with retry (only 5xx; 4xx fails immediately)
	var downloadParam string
	for attempt := 0; attempt < 3; attempt++ {
		downloadParam, err = s.client.uploadToCDN(ctx, uploadResp.UploadParam, data, aesKey)
		if err == nil {
			break
		}
		if !isRetryable(err) {
			return fmt.Errorf("weixin: CDN upload (non-retryable): %w", err)
		}
		log.Warn("CDN upload 5xx, retrying", zap.Int("attempt", attempt+1), zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("weixin: CDN upload: %w", err)
	}

	cdnRef := &cdnMedia{
		EncryptQueryParam: downloadParam,
		AesKey:            base64.StdEncoding.EncodeToString(aesKey),
	}

	item := buildMediaItem(mediaType, cdnRef, aesKey, data, mediaPath)

	outMsg := weixinMessage{
		ToUserID:     raw.FromUserID,
		ContextToken: raw.ContextToken,
		MessageType:  2,
		ItemList:     []messageItem{item},
	}
	return s.client.sendMessage(ctx, outMsg)
}

func mediaTypeFromMIME(mime string) int {
	switch {
	case strings.HasPrefix(mime, "video/"):
		return 2
	case strings.HasPrefix(mime, "image/"):
		return 1
	default:
		return 3
	}
}

func buildMediaItem(mediaType int, cdn *cdnMedia, rawAesKey, data []byte, path string) messageItem {
	switch mediaType {
	case 1: // image
		return messageItem{
			Type: 2,
			ImageItem: &imageItem{
				Media:  cdn,
				AesKey: hex.EncodeToString(rawAesKey), // hex-encoded raw 16-byte key
			},
		}
	case 2: // video
		return messageItem{
			Type: 5,
			VideoItem: &videoItem{
				Media:    cdn,
				VideoMD5: fileMD5(data),
			},
		}
	default: // file
		fileName := filepath.Base(path)
		return messageItem{
			Type: 4,
			FileItem: &fileItem{
				Media:    cdn,
				FileName: platform.SanitizeFileName(fileName),
				MD5:      fileMD5(data),
				Len:      strconv.Itoa(len(data)),
			},
		}
	}
}

// Interface compliance guards.
var _ platform.Platform                = (*WeixinPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*WeixinReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*WeixinSender)(nil)
