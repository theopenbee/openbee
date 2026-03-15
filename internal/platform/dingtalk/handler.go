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
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"

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
	cfg           config.DingTalkConfig
	pendingEmojis *sync.Map
	mediaSvc      *media.Service
}

func (r *DingTalkReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	cli := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(r.cfg.ClientID, r.cfg.ClientSecret)),
	)
	cli.RegisterChatBotCallbackRouter(func(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		slog.Info("received message", "component", "dingtalk", "conversationId", data.ConversationId, "sender", data.SenderNick)

		// Parse raw data to get msgtype and content
		rawBytes, _ := json.Marshal(data)
		var rawData map[string]any
		json.Unmarshal(rawBytes, &rawData)

		msgtype := "text"
		if mt, ok := rawData["msgtype"].(string); ok && mt != "" {
			msgtype = mt
		}

		var textContent string
		switch msgtype {
		case "text":
			textContent = strings.TrimSpace(data.Text.Content)
		case "picture":
			textContent = r.handleDingTalkPicture(ctx, rawData)
		case "richText":
			textContent = r.handleDingTalkRichText(ctx, rawData)
		case "file":
			textContent = r.handleDingTalkFile(ctx, rawData)
		case "audio":
			textContent = r.handleDingTalkAudio(rawData)
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

	slog.Info("DingTalk bot starting...")
	return cli.Start(ctx)
}

// exchangeDownloadCode exchanges a downloadCode for a download URL via DingTalk API.
func exchangeDownloadCode(ctx context.Context, cfg config.DingTalkConfig, downloadCode string) (string, error) {
	token, err := getAccessToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{
		"downloadCode": downloadCode,
		"robotCode":    cfg.ClientID,
	})
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
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (r *DingTalkReceiver) handleDingTalkPicture(ctx context.Context, raw map[string]any) string {
	content, _ := raw["content"].(map[string]any)
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

func (r *DingTalkReceiver) handleDingTalkRichText(ctx context.Context, raw map[string]any) string {
	content, _ := raw["content"].(map[string]any)
	if content == nil {
		return ""
	}
	richTextArr, _ := content["richText"].([]any)
	var textParts []string
	for _, item := range richTextArr {
		itemMap, _ := item.(map[string]any)
		if itemMap == nil {
			continue
		}
		if text, ok := itemMap["text"].(string); ok && text != "" {
			textParts = append(textParts, text)
		}
		if picURL, ok := itemMap["pictureUrl"].(string); ok && picURL != "" {
			data, ct, err := httpDownload(ctx, picURL)
			if err != nil {
				slog.Error("download richtext image", "component", "dingtalk", "error", err)
				textParts = append(textParts, r.mediaSvc.BuildPlaceholder("image", "", ""))
				continue
			}
			ext := r.mediaSvc.ExtensionFromMIME(ct)
			path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
			if err != nil {
				slog.Error("save richtext image", "component", "dingtalk", "error", err)
				textParts = append(textParts, r.mediaSvc.BuildPlaceholder("image", "", ""))
				continue
			}
			textParts = append(textParts, r.mediaSvc.BuildPlaceholder("image", path, ""))
		}
	}
	return strings.Join(textParts, "\n")
}

func (r *DingTalkReceiver) handleDingTalkFile(ctx context.Context, raw map[string]any) string {
	content, _ := raw["content"].(map[string]any)
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
	extracted, _ := r.mediaSvc.ExtractText(ctx, path)
	if extracted != "" {
		return placeholder + "\n" + extracted
	}
	return placeholder
}

func (r *DingTalkReceiver) handleDingTalkAudio(raw map[string]any) string {
	content, _ := raw["content"].(map[string]any)
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

const markdownTitle = "RoboBee"

func (s *DingTalkSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var data chatbot.BotCallbackDataModel
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &data); err != nil {
		slog.Error("failed to unmarshal raw", "component", "dingtalk", "error", err)
		return nil
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
		return nil
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

// uploadMediaToDingTalk uploads a file to DingTalk's OAPI media endpoint.
func uploadMediaToDingTalk(ctx context.Context, cfg config.DingTalkConfig, filePath, mediaType string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	if len(data) > 20*1024*1024 {
		return "", fmt.Errorf("file too large: %d bytes (max 20MB)", len(data))
	}

	token, err := getOAPIToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return "", fmt.Errorf("get OAPI token: %w", err)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	ct := "application/octet-stream"
	if mediaType == "image" {
		ct = "image/jpeg"
	}

	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="media"; filename="%s"`, filepath.Base(filePath))},
		"Content-Type":        {ct},
	})
	if err != nil {
		return "", err
	}
	part.Write(data)
	writer.Close()

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
	fileName := filepath.Base(filePath)
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

	body, _ := json.Marshal(payload)

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
