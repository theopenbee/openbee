package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/media"
	"github.com/robobee/core/internal/platform"
)

// ─── Message body types ────────────────────────────────────────────────────

type messageBody struct {
	MsgID       string        `json:"msgid"`
	AiBotID     string        `json:"aibotid"`
	ChatID      string        `json:"chatid"`      // group only
	ChatType    string        `json:"chattype"`    // "single" | "group"
	From        messageFrom   `json:"from"`
	CreateTime  int64         `json:"create_time"` // Unix seconds
	ResponseURL string        `json:"response_url"`
	MsgType     string        `json:"msgtype"`
	Text        *textContent  `json:"text"`
	Voice       *voiceContent `json:"voice"`
	Image       *mediaContent `json:"image"`
	File        *fileContent  `json:"file"`
	Mixed       *mixedContent `json:"mixed"`
	Quote       *quoteContent `json:"quote"`
}

type messageFrom struct {
	UserID string `json:"userid"`
}

type textContent struct {
	Content string `json:"content"`
}

type voiceContent struct {
	Content string `json:"content"`
}

type mediaContent struct {
	URL    string `json:"url"`
	AesKey string `json:"aeskey"`
}

type fileContent struct {
	URL    string `json:"url"`
	AesKey string `json:"aeskey"`
}

type mixedContent struct {
	MsgItem []mixedItem `json:"msg_item"`
}

type mixedItem struct {
	MsgType string        `json:"msgtype"`
	Text    *textContent  `json:"text"`
	Image   *mediaContent `json:"image"`
}

type quoteContent struct {
	MsgType string        `json:"msgtype"`
	Text    *textContent  `json:"text"`
	Voice   *voiceContent `json:"voice"`
	Image   *mediaContent `json:"image"`
	File    *fileContent  `json:"file"`
	Mixed   *mixedContent `json:"mixed"`
}

// ─── Outbound body types ───────────────────────────────────────────────────

type streamBody struct {
	MsgType string     `json:"msgtype"`
	Stream  streamItem `json:"stream"`
}

type streamItem struct {
	ID      string `json:"id"`
	Finish  bool   `json:"finish"`
	Content string `json:"content"`
}

// sendMsgBody is the body of an aibot_send_msg frame.
// chatid must be snake_case to match the WeCom wire protocol.
type sendMsgBody struct {
	ChatID  string          `json:"chatid"`
	MsgType string          `json:"msgtype"`
	Image   *mediaIDContent `json:"image,omitempty"`
	Voice   *mediaIDContent `json:"voice,omitempty"`
	Video   *mediaIDContent `json:"video,omitempty"`
	File    *mediaIDContent `json:"file,omitempty"`
}

type mediaIDContent struct {
	MediaID string `json:"media_id"`
}

// ─── Upload body types ─────────────────────────────────────────────────────

type uploadInitBody struct {
	Type        string `json:"type"`
	Filename    string `json:"filename"`
	TotalSize   int    `json:"total_size"`
	TotalChunks int    `json:"total_chunks"`
	MD5         string `json:"md5"`
}

type uploadChunkBody struct {
	UploadID   string `json:"upload_id"`
	ChunkIndex int    `json:"chunk_index"`
	Base64Data string `json:"base64_data"`
}

type uploadFinishBody struct {
	UploadID string `json:"upload_id"`
}

// ─── sendReplyFn type ──────────────────────────────────────────────────────

// sendReplyFn matches WsConn.SendReply — injected for testing.
type sendReplyFn func(reqID, cmd string, body any) (WsFrame, error)

// ─── WeComPlatform ─────────────────────────────────────────────────────────

// WeComPlatform implements platform.Platform for WeCom AI Bot.
type WeComPlatform struct {
	receiver      *WeComReceiver
	sender        *WeComSender
	pendingStreams sync.Map // key: msgId → value: streamId
}

// NewPlatform constructs a WeComPlatform from configuration.
func NewPlatform(cfg config.WeComConfig, mediaSvc *media.Service) platform.Platform {
	wsConn := NewWsConn(WsConnConfig{
		BotID:  cfg.BotID,
		Secret: cfg.Secret,
		URL:    cfg.WebSocketURL,
	})
	p := &WeComPlatform{}
	p.receiver = &WeComReceiver{
		cfg:           cfg,
		pendingStreams: &p.pendingStreams,
		mediaSvc:      mediaSvc,
		wsConn:        wsConn,
		sendReplyFn:   wsConn.SendReply,
	}
	p.sender = &WeComSender{
		pendingStreams: &p.pendingStreams,
		wsConn:        wsConn,
		sendReplyFn:   wsConn.SendReply,
	}
	return p
}

func (p *WeComPlatform) ID() string                                 { return "wecom" }
func (p *WeComPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *WeComPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// ─── WeComReceiver ─────────────────────────────────────────────────────────

// WeComReceiver connects to WeCom via WebSocket and dispatches inbound messages.
type WeComReceiver struct {
	cfg           config.WeComConfig
	pendingStreams *sync.Map
	mediaSvc      *media.Service
	wsConn        *WsConn
	sendReplyFn   sendReplyFn // injectable for testing
	// downloadFn is injectable for testing; defaults to downloadDecryptSave.
	downloadFn func(ctx context.Context, url, aesKey, mediaType, filename string) string
}

// Start begins receiving messages and blocks until ctx is cancelled.
func (r *WeComReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	r.wsConn.OnMessage = func(frame WsFrame) {
		go r.processMessage(frame, dispatch)
	}
	slog.Info("WeCom bot starting", "component", "wecom")
	return r.wsConn.Connect(ctx)
}

// download is a helper that uses the injected downloadFn if available,
// otherwise falls back to downloadDecryptSave.
func (r *WeComReceiver) download(ctx context.Context, url, aesKey, mediaType, filename string) string {
	if r.downloadFn != nil {
		return r.downloadFn(ctx, url, aesKey, mediaType, filename)
	}
	return r.downloadDecryptSave(ctx, url, aesKey, mediaType, filename)
}

// processMessage extracts content from a callback frame, sends the thinking
// indicator, stores the stream ID, and dispatches the inbound message.
func (r *WeComReceiver) processMessage(frame WsFrame, dispatch func(platform.InboundMessage)) {
	var body messageBody
	if err := json.Unmarshal(frame.Body, &body); err != nil {
		slog.Warn("wecom: failed to parse message body", "component", "wecom", "error", err)
		return
	}

	chatID := body.From.UserID
	if body.ChatType == "group" {
		chatID = body.ChatID
	}
	senderID := body.From.UserID

	rawText, content := r.extractContent(context.Background(), &body)
	if content == "" {
		return
	}

	// Send thinking indicator.
	streamID := generateReqID("stream")
	thinking := streamBody{
		MsgType: "stream",
		Stream:  streamItem{ID: streamID, Finish: false, Content: "<think></think>"},
	}
	if _, err := r.sendReplyFn(frame.Headers.ReqID, WsCmdResponse, thinking); err != nil {
		slog.Warn("wecom: failed to send thinking message", "component", "wecom", "error", err)
	}

	// Store stream ID with TTL cleanup to prevent leaks when the downstream
	// pipeline drops a message without ever calling Sender.Send.
	r.pendingStreams.Store(body.MsgID, streamID)
	time.AfterFunc(10*time.Minute, func() { r.pendingStreams.Delete(body.MsgID) })

	rawBytes, _ := json.Marshal(frame)
	dispatch(platform.InboundMessage{
		Platform:          "wecom",
		SenderID:          senderID,
		SessionKey:        "wecom:" + chatID + ":" + senderID,
		Content:           content,
		RawContent:        rawText,
		Raw:               string(rawBytes),
		PlatformMessageID: body.MsgID,
		MessageTime:       body.CreateTime * 1000, // WeCom create_time is Unix seconds
	})
}

// extractContent determines rawText and content from the message body.
// rawText is plain text only (no media placeholders); content may include placeholders.
func (r *WeComReceiver) extractContent(ctx context.Context, body *messageBody) (rawText, content string) {
	switch body.MsgType {
	case "text":
		if body.Text == nil {
			return "", ""
		}
		rawText = body.Text.Content
		content = rawText

	case "voice":
		if body.Voice == nil {
			return "", ""
		}
		rawText = body.Voice.Content
		content = rawText

	case "image":
		if body.Image == nil {
			return "", r.mediaSvc.BuildPlaceholder("image", "", "")
		}
		content = r.download(ctx, body.Image.URL, body.Image.AesKey, "image", "")

	case "file":
		if body.File == nil {
			return "", r.mediaSvc.BuildPlaceholder("document", "", "")
		}
		content = r.download(ctx, body.File.URL, body.File.AesKey, "document", "")

	case "mixed":
		if body.Mixed == nil {
			return "", ""
		}
		rawText, content = r.extractMixedContent(ctx, body.Mixed.MsgItem)

	default:
		slog.Warn("wecom: skipping unsupported msgtype", "component", "wecom", "msgtype", body.MsgType)
		return "", ""
	}

	// Append quoted content if present.
	if body.Quote != nil {
		qContent := r.extractQuoteContent(ctx, body.Quote)
		if qContent != "" {
			if content != "" {
				content = content + "\n" + qContent
			} else {
				content = qContent
			}
		}
	}

	return rawText, content
}

// extractMixedContent processes a mixed message's items in parallel for images.
// rawText = joined plain text parts; content = text + image placeholders in order.
func (r *WeComReceiver) extractMixedContent(ctx context.Context, items []mixedItem) (rawText, content string) {
	type result struct {
		text  string
		image string
	}
	results := make([]result, len(items))

	g, gCtx := errgroup.WithContext(ctx)
	for i, item := range items {
		i, item := i, item
		switch item.MsgType {
		case "text":
			if item.Text != nil {
				results[i].text = item.Text.Content
			}
		case "image":
			if item.Image != nil {
				url, key := item.Image.URL, item.Image.AesKey
				g.Go(func() error {
					results[i].image = r.download(gCtx, url, key, "image", "")
					return nil
				})
			}
		}
	}
	_ = g.Wait()

	var textParts, allParts []string
	for _, res := range results {
		if res.text != "" {
			textParts = append(textParts, res.text)
			allParts = append(allParts, res.text)
		}
		if res.image != "" {
			allParts = append(allParts, res.image)
		}
	}
	return strings.Join(textParts, "\n"), strings.Join(allParts, "\n")
}

// extractQuoteContent extracts displayable content from a quote block.
func (r *WeComReceiver) extractQuoteContent(ctx context.Context, q *quoteContent) string {
	if q == nil {
		return ""
	}
	switch q.MsgType {
	case "text":
		if q.Text != nil {
			return q.Text.Content
		}
	case "voice":
		if q.Voice != nil {
			return q.Voice.Content
		}
	case "image":
		if q.Image != nil {
			return r.download(ctx, q.Image.URL, q.Image.AesKey, "image", "")
		}
	case "file":
		if q.File != nil {
			return r.download(ctx, q.File.URL, q.File.AesKey, "document", "")
		}
	case "mixed":
		if q.Mixed != nil {
			_, content := r.extractMixedContent(ctx, q.Mixed.MsgItem)
			return content
		}
	}
	return ""
}

// downloadDecryptSave downloads an encrypted WeCom media file, decrypts it,
// saves it via mediaSvc, and returns a placeholder string.
// On any error it logs and returns a placeholder without a path.
func (r *WeComReceiver) downloadDecryptSave(ctx context.Context, url, aesKey, mediaType, filename string) string {
	if url == "" {
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		slog.Error("wecom: create download request failed", "component", "wecom", "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("wecom: download media failed", "component", "wecom", "url", url, "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("wecom: read media body failed", "component", "wecom", "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}

	if aesKey != "" {
		data, err = DecryptFile(data, aesKey)
		if err != nil {
			slog.Error("wecom: decrypt media failed", "component", "wecom", "error", err)
			return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
		}
	}

	mime := r.mediaSvc.DetectMIME(data, filename)
	ext := r.mediaSvc.ExtensionFromMIME(mime)
	if ext == "" && filename != "" {
		ext = filepath.Ext(filename)
	}

	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		slog.Error("wecom: save media failed", "component", "wecom", "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}
	return r.mediaSvc.BuildPlaceholder(mediaType, path, filename)
}

// ─── WeComSender ───────────────────────────────────────────────────────────

// WeComSender sends replies via the WeCom AI Bot WebSocket.
type WeComSender struct {
	pendingStreams *sync.Map
	wsConn        *WsConn
	sendReplyFn   sendReplyFn
}

// Send delivers a reply to WeCom. Text replies use aibot_respond_msg streaming;
// media replies use 3-step upload + aibot_send_msg.
func (s *WeComSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var frame WsFrame
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &frame); err != nil {
		return fmt.Errorf("wecom sender: unmarshal raw frame: %w", err)
	}
	var body messageBody
	if err := json.Unmarshal(frame.Body, &body); err != nil {
		return fmt.Errorf("wecom sender: unmarshal message body: %w", err)
	}

	reqID := frame.Headers.ReqID
	chatID := body.From.UserID
	if body.ChatType == "group" {
		chatID = body.ChatID
	}
	msgID := body.MsgID

	streamID := generateReqID("stream") // fallback
	if val, ok := s.pendingStreams.LoadAndDelete(msgID); ok {
		streamID = val.(string)
	}

	if msg.MediaPath != "" {
		return s.sendMedia(ctx, msg.MediaPath, chatID, reqID, streamID)
	}
	return s.sendText(ctx, msg.Content, reqID, streamID)
}

// sendText sends a streaming text reply.
func (s *WeComSender) sendText(_ context.Context, content, reqID, streamID string) error {
	body := streamBody{
		MsgType: "stream",
		Stream:  streamItem{ID: streamID, Finish: true, Content: content},
	}
	_, err := s.sendReplyFn(reqID, WsCmdResponse, body)
	return err
}

// sendMedia uploads a media file and sends it via aibot_send_msg.
func (s *WeComSender) sendMedia(ctx context.Context, mediaPath, chatID, reqID, streamID string) error {
	// TODO: implemented in Task 5
	_ = ctx
	slog.Warn("wecom: sendMedia not yet implemented", "component", "wecom", "path", mediaPath)
	// Finish thinking stream with a placeholder so the user sees something.
	return s.finishStream(reqID, streamID, "⚠️ 媒体发送暂不支持。")
}

// finishStream closes the thinking stream with the given text.
func (s *WeComSender) finishStream(reqID, streamID, text string) error {
	body := streamBody{
		MsgType: "stream",
		Stream:  streamItem{ID: streamID, Finish: true, Content: text},
	}
	_, err := s.sendReplyFn(reqID, WsCmdResponse, body)
	return err
}

// suppress unused variable warning for chatID in sendMedia
var _ = (*WeComSender).sendMedia

// ─── Compile-time interface assertions ─────────────────────────────────────

var _ platform.Platform                = (*WeComPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*WeComReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*WeComSender)(nil)
