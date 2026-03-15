package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/handler"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"

	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/media"
	"github.com/robobee/core/internal/platform"
)

// DingTalkPlatform implements platform.Platform for DingTalk.
type DingTalkPlatform struct {
	receiver      *DingTalkReceiver
	sender        *DingTalkSender
	pendingEmojis sync.Map
}

// NewPlatform constructs a DingTalkPlatform from configuration.
func NewPlatform(cfg config.DingTalkConfig, mediaSvc *media.Service) platform.Platform {
	p := &DingTalkPlatform{}
	p.receiver = &DingTalkReceiver{cfg: cfg, pendingEmojis: &p.pendingEmojis, mediaSvc: mediaSvc}
	p.sender = &DingTalkSender{cfg: cfg, pendingEmojis: &p.pendingEmojis}
	return p
}

func (d *DingTalkPlatform) ID() string                                 { return "dingtalk" }
func (d *DingTalkPlatform) Receiver() platform.PlatformReceiverAdapter { return d.receiver }
func (d *DingTalkPlatform) Sender() platform.PlatformSenderAdapter     { return d.sender }

// DingTalkReceiver connects to DingTalk via the stream SDK and dispatches inbound messages.
type DingTalkReceiver struct {
	cfg              config.DingTalkConfig
	pendingEmojis    *sync.Map
	mediaSvc         *media.Service
	mu               sync.Mutex
	cli              *client.StreamClient
	lastActivityTime atomic.Value // time.Time
}

func (r *DingTalkReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	r.lastActivityTime.Store(time.Now())
	if err := r.createAndStartClient(ctx, dispatch); err != nil {
		return fmt.Errorf("initial connection failed: %w", err)
	}
	slog.Info("DingTalk bot started with heartbeat supervisor", "component", "dingtalk")
	r.supervisorLoop(ctx, dispatch)
	return nil
}

func (r *DingTalkReceiver) createAndStartClient(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	cli := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(r.cfg.ClientID, r.cfg.ClientSecret)),
		client.WithKeepAlive(30*time.Second),
		client.WithAutoReconnect(true),
	)

	// Register system ping handler to track activity from server-side pings.
	cli.RegisterRouter("SYSTEM", "ping", handler.IFrameHandler(func(ctx context.Context, df *payload.DataFrame) (*payload.DataFrameResponse, error) {
		r.lastActivityTime.Store(time.Now())
		return cli.OnPing(ctx, df)
	}))

	cli.RegisterChatBotCallbackRouter(func(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		r.lastActivityTime.Store(time.Now())
		slog.Info("received message", "component", "dingtalk", "conversationId", data.ConversationId, "sender", data.SenderNick)

		msgtype := data.Msgtype
		if msgtype == "" {
			msgtype = "text"
		}
		content, _ := data.Content.(map[string]any)

		var textContent string
		switch msgtype {
		case "text":
			textContent = strings.TrimSpace(data.Text.Content)
		case "picture":
			textContent = r.handleDingTalkPicture(ctx, content)
		case "richText":
			textContent = r.handleDingTalkRichText(ctx, content)
		case "file":
			textContent = r.handleDingTalkFile(ctx, content)
		case "audio":
			textContent = r.handleDingTalkAudio(content)
		case "video":
			textContent = r.mediaSvc.BuildPlaceholder("video", "", "")
		default:
			slog.Warn("skipping unsupported message type", "component", "dingtalk", "msgtype", msgtype)
			return []byte(""), nil
		}

		if textContent == "" {
			return []byte(""), nil
		}
		if data.SenderStaffId == "" {
			slog.Warn("skipping message with empty SenderStaffId", "component", "dingtalk")
			return []byte(""), nil
		}

		rawBytes, err := json.Marshal(data)
		if err != nil {
			slog.Error("failed to marshal callback data", "component", "dingtalk", "error", err)
			return []byte(""), nil
		}

		msg := platform.InboundMessage{
			Platform:          "dingtalk",
			SenderID:          data.SenderStaffId,
			SessionKey:        "dingtalk:" + data.ConversationId + ":" + data.SenderStaffId,
			Content:           textContent,
			Raw:               string(rawBytes),
			PlatformMessageID: data.MsgId,
			MessageTime:       data.CreateAt,
		}
		go func() {
			addThinkingEmoji(ctx, r.cfg, data)
			r.pendingEmojis.Store(data.MsgId, struct{}{})
		}()

		dispatch(msg)
		slog.Info("dispatched message", "component", "dingtalk", "sessionKey", msg.SessionKey)
		return []byte(""), nil
	})

	if err := cli.Start(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	r.cli = cli
	r.mu.Unlock()

	slog.Info("DingTalk stream client connected", "component", "dingtalk")
	return nil
}

const (
	supervisorCheckInterval = 30 * time.Second
	activityTimeout         = 90 * time.Second
	reconnectDelay          = 5 * time.Second
)

func (r *DingTalkReceiver) supervisorLoop(ctx context.Context, dispatch func(platform.InboundMessage)) {
	ticker := time.NewTicker(supervisorCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("DingTalk supervisor shutting down", "component", "dingtalk")
			r.mu.Lock()
			if r.cli != nil {
				r.cli.AutoReconnect = false
				r.cli.Close()
			}
			r.mu.Unlock()
			return

		case <-ticker.C:
			last, ok := r.lastActivityTime.Load().(time.Time)
			if !ok {
				continue
			}
			elapsed := time.Since(last)
			if elapsed <= activityTimeout {
				continue
			}

			slog.Warn("DingTalk heartbeat timeout, triggering reconnect", "component", "dingtalk", "elapsed", elapsed)

			r.mu.Lock()
			if r.cli != nil {
				r.cli.AutoReconnect = false
				r.cli.Close()
				r.cli = nil
			}
			r.mu.Unlock()

			r.lastActivityTime.Store(time.Now())
			if err := r.createAndStartClient(ctx, dispatch); err != nil {
				slog.Error("DingTalk reconnect failed, retrying", "component", "dingtalk", "error", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(reconnectDelay):
				}
				r.lastActivityTime.Store(time.Now())
				if err := r.createAndStartClient(ctx, dispatch); err != nil {
					slog.Error("DingTalk reconnect retry failed", "component", "dingtalk", "error", err)
				} else {
					slog.Info("DingTalk reconnected successfully after retry", "component", "dingtalk")
				}
			} else {
				slog.Info("DingTalk reconnected successfully", "component", "dingtalk")
			}
		}
	}
}

// exchangeDownloadCode exchanges a downloadCode for a download URL via DingTalk API.
func exchangeDownloadCode(ctx context.Context, cfg config.DingTalkConfig, downloadCode string) (string, error) {
	token, err := getAccessToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]string{
		"downloadCode": downloadCode,
		"robotCode":    cfg.ClientID,
	})
	if err != nil {
		return "", fmt.Errorf("marshal download request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dingtalk.com/v1.0/robot/messageFiles/download",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchange download code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("exchange download code: status %d", resp.StatusCode)
	}
	var result struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode download URL: %w", err)
	}
	if result.DownloadURL == "" {
		return "", fmt.Errorf("empty download URL")
	}
	return result.DownloadURL, nil
}

// httpDownload downloads a file from a URL and returns its bytes and content type.
func httpDownload(ctx context.Context, url string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download failed: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxDownloadSize {
		return nil, "", fmt.Errorf("download too large: exceeds %d bytes", maxDownloadSize)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (r *DingTalkReceiver) handleDingTalkPicture(ctx context.Context, content map[string]any) string {
	if content == nil {
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	downloadCode, _ := content["downloadCode"].(string)
	if downloadCode == "" {
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	dlURL, err := exchangeDownloadCode(ctx, r.cfg, downloadCode)
	if err != nil {
		slog.Error("exchange download code failed", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	data, ct, err := httpDownload(ctx, dlURL)
	if err != nil {
		slog.Error("download image failed", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	ext := r.mediaSvc.ExtensionFromMIME(ct)
	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		slog.Error("save image failed", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	return r.mediaSvc.BuildPlaceholder("image", path, "")
}

func (r *DingTalkReceiver) handleDingTalkRichText(ctx context.Context, content map[string]any) string {
	if content == nil {
		return ""
	}
	richTextArr, _ := content["richText"].([]any)

	// Each item may have text, image, or both — store results by index for ordering.
	type itemResult struct{ text, image string }
	results := make([]itemResult, len(richTextArr))

	var wg sync.WaitGroup
	for i, item := range richTextArr {
		itemMap, _ := item.(map[string]any)
		if itemMap == nil {
			continue
		}
		if text, ok := itemMap["text"].(string); ok && text != "" {
			results[i].text = text
		}
		if picURL, ok := itemMap["pictureUrl"].(string); ok && picURL != "" {
			wg.Add(1)
			go func(idx int, url string) {
				defer wg.Done()
				data, ct, err := httpDownload(ctx, url)
				if err != nil {
					slog.Error("download richtext image", "component", "dingtalk", "error", err)
					results[idx].image = r.mediaSvc.BuildPlaceholder("image", "", "")
					return
				}
				ext := r.mediaSvc.ExtensionFromMIME(ct)
				path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
				if err != nil {
					slog.Error("save richtext image", "component", "dingtalk", "error", err)
					results[idx].image = r.mediaSvc.BuildPlaceholder("image", "", "")
					return
				}
				results[idx].image = r.mediaSvc.BuildPlaceholder("image", path, "")
			}(i, picURL)
		}
	}
	wg.Wait()

	var textParts []string
	for _, r := range results {
		if r.text != "" {
			textParts = append(textParts, r.text)
		}
		if r.image != "" {
			textParts = append(textParts, r.image)
		}
	}
	return strings.Join(textParts, "\n")
}

func (r *DingTalkReceiver) handleDingTalkFile(ctx context.Context, content map[string]any) string {
	if content == nil {
		return r.mediaSvc.BuildPlaceholder("document", "", "")
	}
	downloadCode, _ := content["downloadCode"].(string)
	fileName, _ := content["fileName"].(string)
	if downloadCode == "" {
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}
	dlURL, err := exchangeDownloadCode(ctx, r.cfg, downloadCode)
	if err != nil {
		slog.Error("exchange file download code", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}
	data, _, err := httpDownload(ctx, dlURL)
	if err != nil {
		slog.Error("download file", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}
	ext := ".bin"
	if fileName != "" {
		if origExt := filepath.Ext(fileName); origExt != "" {
			ext = origExt
		}
	}
	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		slog.Error("save file", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}
	placeholder := r.mediaSvc.BuildPlaceholder("document", path, fileName)
	extracted, err := r.mediaSvc.ExtractText(ctx, path)
	if err != nil {
		slog.Error("extract text failed", "component", "dingtalk", "path", path, "error", err)
	}
	if extracted != "" {
		return placeholder + "\n" + extracted
	}
	return placeholder
}

func (r *DingTalkReceiver) handleDingTalkAudio(content map[string]any) string {
	if content != nil {
		if recognition, ok := content["recognition"].(string); ok && recognition != "" {
			return recognition
		}
	}
	return r.mediaSvc.BuildPlaceholder("audio", "", "")
}

// DingTalkSender sends messages via the DingTalk chatbot replier.
type DingTalkSender struct {
	cfg           config.DingTalkConfig
	pendingEmojis *sync.Map
}

const (
	markdownTitle   = "RoboBee"
	maxDownloadSize = 100 * 1024 * 1024 // 100MB
	maxUploadSize   = 20 * 1024 * 1024  // 20MB
)

func (s *DingTalkSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var data chatbot.BotCallbackDataModel
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &data); err != nil {
		slog.Error("failed to unmarshal raw", "component", "dingtalk", "error", err)
		return fmt.Errorf("unmarshal raw: %w", err)
	}
	if _, ok := s.pendingEmojis.LoadAndDelete(data.MsgId); ok {
		recallThinkingEmoji(ctx, s.cfg, &data)
	}

	if msg.MediaPath != "" {
		mediaType := dingTalkMediaType(msg.MediaPath)
		mediaID, err := uploadMediaToDingTalk(ctx, s.cfg, msg.MediaPath, mediaType)
		if err != nil {
			slog.Error("upload media failed", "component", "dingtalk", "error", err)
			return fmt.Errorf("upload media: %w", err)
		}
		if err := sendMediaViaDingTalk(ctx, s.cfg, data.SessionWebhook, msg.MediaPath, mediaID); err != nil {
			slog.Error("send media failed", "component", "dingtalk", "error", err)
			return fmt.Errorf("send media: %w", err)
		}
		return nil
	}

	// Text message
	replier := chatbot.NewChatbotReplier()
	slog.Info("sending reply", "component", "dingtalk", "sessionKey", msg.ReplyTo.SessionKey, "webhookLen", len(data.SessionWebhook), "contentLen", len(msg.Content))
	if err := replier.SimpleReplyMarkdown(ctx, data.SessionWebhook, []byte(markdownTitle), []byte(msg.Content)); err != nil {
		slog.Error("reply send error", "component", "dingtalk", "error", err)
		return fmt.Errorf("reply send: %w", err)
	}
	slog.Info("reply sent ok", "component", "dingtalk")
	return nil
}

// dingTalkMediaType maps file extension to DingTalk upload media type.
func dingTalkMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return "image"
	case ".mp4", ".mov", ".avi":
		return "video"
	case ".mp3", ".wav", ".amr", ".ogg", ".aac", ".flac", ".m4a", ".opus":
		return "voice"
	default:
		return "file"
	}
}

// imageContentType returns the MIME content type for an image file path.
func imageContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "image/jpeg"
	}
}

// uploadMediaToDingTalk uploads a file to DingTalk's OAPI media endpoint.
func uploadMediaToDingTalk(ctx context.Context, cfg config.DingTalkConfig, filePath, mediaType string) (string, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if fi.Size() > maxUploadSize {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", fi.Size(), maxUploadSize)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	token, err := getOAPIToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return "", fmt.Errorf("get OAPI token: %w", err)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	ct := "application/octet-stream"
	if mediaType == "image" {
		ct = imageContentType(filePath)
	}

	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="media"; filename="%s"`, platform.SanitizeFileName(filepath.Base(filePath)))},
		"Content-Type":        {ct},
	})
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("write multipart data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	url := fmt.Sprintf("https://oapi.dingtalk.com/media/upload?access_token=%s&type=%s", token, mediaType)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload media: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		MediaID string `json:"media_id"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode upload response: %w", err)
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("upload error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return result.MediaID, nil
}

// sendMediaViaDingTalk sends a media message via sessionWebhook.
func sendMediaViaDingTalk(ctx context.Context, cfg config.DingTalkConfig, webhook, filePath, mediaID string) error {
	mediaType := dingTalkMediaType(filePath)
	fileName := platform.SanitizeFileName(filepath.Base(filePath))
	fileType := strings.TrimPrefix(filepath.Ext(filePath), ".")

	var payload map[string]any
	switch mediaType {
	case "image":
		payload = map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]string{"title": "Image", "text": fmt.Sprintf("![image](%s)", mediaID)},
		}
	case "voice":
		payload = map[string]any{
			"msgtype": "voice",
			"voice":   map[string]string{"mediaId": mediaID, "duration": "60000"},
		}
	case "video":
		payload = map[string]any{
			"msgtype": "video",
			"video":   map[string]string{"duration": "0", "videoMediaId": mediaID, "videoType": "mp4", "picMediaId": ""},
		}
	default:
		payload = map[string]any{
			"msgtype": "file",
			"file":    map[string]string{"mediaId": mediaID, "fileName": fileName, "fileType": fileType},
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal media payload: %w", err)
	}

	token, err := getAccessToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send media failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func buildEmojiPayload(cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel) []byte {
	payload, _ := json.Marshal(map[string]any{
		"robotCode":          cfg.ClientID,
		"openMsgId":          data.MsgId,
		"openConversationId": data.ConversationId,
		"emotionType":        2,
		"emotionName":        "🤔思考中",
		"textEmotion": map[string]string{
			"emotionId":    "2659900",
			"emotionName":  "🤔思考中",
			"text":         "🤔思考中",
			"backgroundId": "im_bg_1",
		},
	})
	return payload
}

func doEmojiRequest(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel, url string, timeout time.Duration, action string) {
	token, err := getAccessToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		slog.Warn("failed to get access token for emoji "+action, "component", "dingtalk", "error", err)
		return
	}

	payload := buildEmojiPayload(cfg, data)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		slog.Warn("failed to create emoji "+action+" request", "component", "dingtalk", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("failed to "+action+" emoji reaction", "component", "dingtalk", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("emoji "+action+" returned non-200", "component", "dingtalk", "status", resp.StatusCode)
	}
}

// addThinkingEmoji adds a 🤔思考中 emoji reaction to the user's message.
func addThinkingEmoji(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel) {
	doEmojiRequest(ctx, cfg, data, "https://api.dingtalk.com/v1.0/robot/emotion/reply", 5*time.Second, "reply")
}

// recallThinkingEmoji recalls the 🤔思考中 emoji reaction from the user's message.
func recallThinkingEmoji(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel) {
	doEmojiRequest(ctx, cfg, data, "https://api.dingtalk.com/v1.0/robot/emotion/recall", 3*time.Second, "recall")
}

var _ platform.Platform = (*DingTalkPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*DingTalkReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*DingTalkSender)(nil)
