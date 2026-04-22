package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/handler"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
	retryutil "github.com/theopenbee/openbee/internal/utils"
)

var log = logger.With(zap.String("component", "dingtalk"))

type dingtalkContextFields struct {
	ConversationID    string          `json:"conversationId,omitempty"`
	AtUsers           json.RawMessage `json:"atUsers,omitempty"`
	ChatbotCorpID     string          `json:"chatbotCorpId,omitempty"`
	ChatbotUserID     string          `json:"chatbotUserId,omitempty"`
	MsgID             string          `json:"msgId,omitempty"`
	SenderNick        string          `json:"senderNick,omitempty"`
	IsAdmin           *bool           `json:"isAdmin,omitempty"`
	SenderStaffID     string          `json:"senderStaffId,omitempty"`
	SenderCorpID      string          `json:"senderCorpId,omitempty"`
	ConversationType  string          `json:"conversationType,omitempty"`
	SenderID          string          `json:"senderId,omitempty"`
	ConversationTitle string          `json:"conversationTitle,omitempty"`
	MsgType           string          `json:"msgtype,omitempty"`
}

func ExtractContext(raw string) string {
	var ctx dingtalkContextFields
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		return ""
	}
	b, _ := json.Marshal(map[string]any{"dingtalk": ctx})
	return string(b)
}

// DingTalkPlatform implements platform.Platform for DingTalk.
type DingTalkPlatform struct {
	receiver      *DingTalkReceiver
	sender        *DingTalkSender
	pendingEmojis sync.Map
}

// NewPlatform constructs a DingTalkPlatform from configuration.
func NewPlatform(cfg config.DingTalkConfig, mediaCfg config.MediaConfig, mediaSvc *media.Service) platform.Platform {
	p := &DingTalkPlatform{}
	p.receiver = &DingTalkReceiver{cfg: cfg, pendingEmojis: &p.pendingEmojis, mediaSvc: mediaSvc}
	p.sender = &DingTalkSender{cfg: cfg, mediaCfg: mediaCfg, pendingEmojis: &p.pendingEmojis}
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
		client.WithKeepAlive(30*time.Second),
		client.WithAutoReconnect(true),
	)

	cli.RegisterRouter("SYSTEM", "ping", handler.IFrameHandler(cli.OnPing))

	cli.RegisterChatBotCallbackRouter(func(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		log.Info("received message",
			zap.String("msgId", data.MsgId),
			zap.String("conversationId", data.ConversationId),
			zap.String("conversationType", data.ConversationType),
			zap.String("msgtype", data.Msgtype),
			zap.String("senderNick", data.SenderNick),
			zap.String("senderStaffId", data.SenderStaffId),
			zap.String("senderCorpId", data.SenderCorpId),
			zap.String("content", data.Text.Content),
		)

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
			textContent = r.handleDingTalkAudio(ctx, content)
		case "video":
			textContent = r.handleDingTalkVideo(ctx, content)
		default:
			log.Warn("skipping unsupported message type", zap.String("msgtype", msgtype))
			return []byte(""), nil
		}

		if textContent == "" {
			return []byte(""), nil
		}
		if data.SenderStaffId == "" {
			log.Warn("skipping message with empty SenderStaffId")
			return []byte(""), nil
		}

		rawBytes, err := json.Marshal(data)
		if err != nil {
			log.Error("failed to marshal callback data", zap.Error(err))
			return []byte(""), nil
		}

		msg := platform.InboundMessage{
			Platform:         "dingtalk",
			SenderID:         data.SenderStaffId,
			SessionKey:       "dingtalk:" + data.ConversationId + ":" + data.SenderStaffId,
			Content:          textContent,
			Raw:              string(rawBytes),
			PlatformMessageID: data.MsgId,
			MessageTime:      data.CreateAt,
		}
		go func() {
			addThinkingEmoji(ctx, r.cfg, data)
			r.pendingEmojis.Store(data.MsgId, struct{}{})
		}()

		dispatch(msg)
		log.Info("dispatched message", zap.String("sessionKey", msg.SessionKey))
		return []byte(""), nil
	})

	if err := cli.Start(ctx); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	log.Info("DingTalk bot started")
	<-ctx.Done()
	cli.AutoReconnect = false
	cli.Close()
	return nil
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
		log.Error("exchange download code failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	data, ct, err := httpDownload(ctx, dlURL)
	if err != nil {
		log.Error("download image failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	ext := r.mediaSvc.ExtensionFromMIME(ct)
	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Error("save image failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	return r.mediaSvc.BuildPlaceholder("image", path, "")
}

func (r *DingTalkReceiver) handleDingTalkVideo(ctx context.Context, content map[string]any) string {
	if content == nil {
		return r.mediaSvc.BuildPlaceholder("video", "", "")
	}
	downloadCode, _ := content["downloadCode"].(string)
	if downloadCode == "" {
		return r.mediaSvc.BuildPlaceholder("video", "", "")
	}
	dlURL, err := exchangeDownloadCode(ctx, r.cfg, downloadCode)
	if err != nil {
		log.Error("exchange download code failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("video", "", "")
	}
	data, ct, err := httpDownload(ctx, dlURL)
	if err != nil {
		log.Error("download video failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("video", "", "")
	}
	ext := r.mediaSvc.ExtensionFromMIME(ct)
	if ext == "" {
		if vt, _ := content["videoType"].(string); vt != "" {
			ext = "." + vt
		} else {
			ext = ".mp4"
		}
	}
	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Error("save video failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("video", "", "")
	}
	return r.mediaSvc.BuildPlaceholder("video", path, "")
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
					log.Error("download richtext image", zap.Error(err))
					results[idx].image = r.mediaSvc.BuildPlaceholder("image", "", "")
					return
				}
				ext := r.mediaSvc.ExtensionFromMIME(ct)
				path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
				if err != nil {
					log.Error("save richtext image", zap.Error(err))
					results[idx].image = r.mediaSvc.BuildPlaceholder("image", "", "")
					return
				}
				results[idx].image = r.mediaSvc.BuildPlaceholder("image", path, "")
			}(i, picURL)
		} else {
			dlCode, _ := itemMap["downloadCode"].(string)
			if dlCode == "" {
				dlCode, _ = itemMap["pictureDownloadCode"].(string)
			}
			if dlCode != "" {
				wg.Add(1)
				go func(idx int, code string) {
					defer wg.Done()
					dlURL, err := exchangeDownloadCode(ctx, r.cfg, code)
					if err != nil {
						log.Error("exchange richtext image download code", zap.Error(err))
						results[idx].image = r.mediaSvc.BuildPlaceholder("image", "", "")
						return
					}
					data, ct, err := httpDownload(ctx, dlURL)
					if err != nil {
						log.Error("download richtext image", zap.Error(err))
						results[idx].image = r.mediaSvc.BuildPlaceholder("image", "", "")
						return
					}
					ext := r.mediaSvc.ExtensionFromMIME(ct)
					path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
					if err != nil {
						log.Error("save richtext image", zap.Error(err))
						results[idx].image = r.mediaSvc.BuildPlaceholder("image", "", "")
						return
					}
					results[idx].image = r.mediaSvc.BuildPlaceholder("image", path, "")
				}(i, dlCode)
			}
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
		log.Error("exchange file download code", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}
	data, _, err := httpDownload(ctx, dlURL)
	if err != nil {
		log.Error("download file", zap.Error(err))
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
		log.Error("save file", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}
	return r.mediaSvc.BuildPlaceholder("document", path, fileName)
}

func (r *DingTalkReceiver) handleDingTalkAudio(ctx context.Context, content map[string]any) string {
	if content == nil {
		return r.mediaSvc.BuildPlaceholder("audio", "", "")
	}
	recognition, _ := content["recognition"].(string)
	downloadCode, _ := content["downloadCode"].(string)
	if downloadCode == "" {
		return r.mediaSvc.BuildPlaceholder("audio", "", recognition)
	}
	dlURL, err := exchangeDownloadCode(ctx, r.cfg, downloadCode)
	if err != nil {
		log.Error("exchange download code failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("audio", "", recognition)
	}
	data, ct, err := httpDownload(ctx, dlURL)
	if err != nil {
		log.Error("download audio failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("audio", "", recognition)
	}
	ext := r.mediaSvc.ExtensionFromMIME(ct)
	if ext == "" {
		ext = ".ogg"
	}
	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Error("save audio failed", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("audio", "", recognition)
	}
	return r.mediaSvc.BuildPlaceholder("audio", path, recognition)
}

// DingTalkSender sends messages via the DingTalk chatbot replier.
type DingTalkSender struct {
	cfg           config.DingTalkConfig
	mediaCfg      config.MediaConfig
	pendingEmojis *sync.Map
}

// mediaInfo holds duration/thumbnail metadata for voice and video messages.
type mediaInfo struct {
	durationMs  int    // voice: milliseconds
	durationSec int    // video: seconds
	picMediaID  string // video: thumbnail mediaId
}

const (
	markdownTitle      = "OpenBee"
	maxDownloadSize    = 100 * 1024 * 1024 // 100MB
	maxUploadSize      = 20 * 1024 * 1024  // 20MB
	maxVoiceUploadSize = 2 * 1024 * 1024   // 2MB — voice limit per DingTalk spec
)

func (s *DingTalkSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var data chatbot.BotCallbackDataModel
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &data); err != nil {
		log.Error("failed to unmarshal raw", zap.Error(err))
		return fmt.Errorf("unmarshal raw: %w", err)
	}
	if _, ok := s.pendingEmojis.LoadAndDelete(data.MsgId); ok {
		recallThinkingEmoji(ctx, s.cfg, &data)
	}

	expired := isWebhookExpired(&data)
	if expired {
		log.Info("sessionWebhook expired, using proactive API",
			zap.String("sessionKey", msg.ReplyTo.SessionKey),
			zap.String("conversationType", data.ConversationType))
	}

	if msg.MediaPath != "" {
		mediaType := dingTalkMediaType(msg.MediaPath)

		var info mediaInfo
		switch mediaType {
		case "voice":
			durationMs, err := media.AudioDurationMs(ctx, msg.MediaPath, s.mediaCfg.FFprobePath)
			if err != nil {
				log.Warn("could not probe audio duration, using 0", zap.Error(err))
			}
			info.durationMs = durationMs
		case "video":
			durationSec, err := media.VideoDurationSec(ctx, msg.MediaPath, s.mediaCfg.FFprobePath)
			if err != nil {
				log.Warn("could not probe video duration, using 0", zap.Error(err))
			}
			info.durationSec = durationSec

			thumbPath, cleanup, err := media.ExtractFirstFrame(ctx, msg.MediaPath, s.mediaCfg.FFmpegPath)
			if err == nil {
				defer cleanup()
				picMediaID, uploadErr := uploadMediaToDingTalk(ctx, s.cfg, thumbPath, "image")
				if uploadErr != nil {
					log.Warn("could not upload video thumbnail", zap.Error(uploadErr))
				} else {
					info.picMediaID = picMediaID
				}
			}
		}

		mediaID, err := uploadMediaToDingTalk(ctx, s.cfg, msg.MediaPath, mediaType)
		if err != nil {
			log.Error("upload media failed", zap.Error(err))
			return fmt.Errorf("upload media: %w", err)
		}
		if expired {
			if err := sendMediaProactive(ctx, s.cfg, &data, msg.MediaPath, mediaID, info); err != nil {
				log.Error("proactive media send failed", zap.Error(err))
				return fmt.Errorf("proactive media send: %w", err)
			}
			return nil
		}
		if err := sendMediaViaDingTalk(ctx, s.cfg, data.SessionWebhook, msg.MediaPath, mediaID, info); err != nil {
			log.Error("send media failed", zap.Error(err))
			return fmt.Errorf("send media: %w", err)
		}
		return nil
	}

	// Text message
	if expired {
		if err := sendTextProactive(ctx, s.cfg, &data, msg.Content); err != nil {
			log.Error("proactive text send failed", zap.Error(err))
			return fmt.Errorf("proactive text send: %w", err)
		}
		log.Info("proactive reply sent ok")
		return nil
	}

	replier := chatbot.NewChatbotReplier()
	log.Info("sending reply",
		zap.String("sessionKey", msg.ReplyTo.SessionKey),
		zap.Int("webhookLen", len(data.SessionWebhook)),
		zap.Int("contentLen", len(msg.Content)))
	if err := replier.SimpleReplyMarkdown(ctx, data.SessionWebhook, []byte(markdownTitle), []byte(msg.Content)); err != nil {
		log.Error("reply send error", zap.Error(err))
		return fmt.Errorf("reply send: %w", err)
	}
	log.Info("reply sent ok")
	return nil
}

// isWebhookExpired reports whether the sessionWebhook in data has expired.
// SessionWebhookExpiredTime is a Unix timestamp in milliseconds.
func isWebhookExpired(data *chatbot.BotCallbackDataModel) bool {
	if data.SessionWebhookExpiredTime <= 0 {
		return true
	}
	return time.Now().UnixMilli() >= data.SessionWebhookExpiredTime
}

// proactiveEndpoint returns the appropriate DingTalk API endpoint for proactive
// message sending based on conversation type ("1" = group, "2" = single).
func proactiveEndpoint(conversationType string) string {
	if conversationType == "2" {
		return "https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend"
	}
	return "https://api.dingtalk.com/v1.0/robot/groupMessages/send"
}

// buildProactiveTextPayload constructs the request body for proactive text (markdown) sending.
func buildProactiveTextPayload(cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel, content string) map[string]any {
	msgParam, _ := json.Marshal(map[string]string{
		"title": markdownTitle,
		"text":  content,
	})
	payload := map[string]any{
		"robotCode": cfg.ClientID,
		"msgKey":    "sampleMarkdown",
		"msgParam":  string(msgParam),
	}
	if data.ConversationType == "2" {
		payload["userIds"] = []string{data.SenderStaffId}
	} else {
		payload["openConversationId"] = data.ConversationId
	}
	return payload
}

// sendTextProactive sends a text (markdown) message via the proactive API.
func sendTextProactive(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel, content string) error {
	endpoint := proactiveEndpoint(data.ConversationType)
	p := buildProactiveTextPayload(cfg, data, content)
	return doProactiveRequest(ctx, cfg, endpoint, p)
}

// buildProactiveMediaPayload constructs the request body for proactive media sending.
func buildProactiveMediaPayload(cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel, filePath, mediaID string, info mediaInfo) map[string]any {
	mediaType := dingTalkMediaType(filePath)
	fileName := platform.SanitizeFileName(filepath.Base(filePath))
	fileType := strings.TrimPrefix(filepath.Ext(filePath), ".")

	var msgKey string
	var msgParam string

	switch mediaType {
	case "image":
		msgKey = "sampleMarkdown"
		p, _ := json.Marshal(map[string]string{
			"title": "Image",
			"text":  fmt.Sprintf("![image](%s)", mediaID),
		})
		msgParam = string(p)
	case "voice":
		msgKey = "sampleAudio"
		p, _ := json.Marshal(map[string]string{"mediaId": mediaID, "duration": strconv.Itoa(info.durationMs)})
		msgParam = string(p)
	case "video":
		msgKey = "sampleVideo"
		p, _ := json.Marshal(map[string]string{
			"duration":     strconv.Itoa(info.durationSec),
			"videoMediaId": mediaID,
			"videoType":    "mp4",
			"picMediaId":   info.picMediaID,
		})
		msgParam = string(p)
	default:
		msgKey = "sampleFile"
		p, _ := json.Marshal(map[string]string{"mediaId": mediaID, "fileName": fileName, "fileType": fileType})
		msgParam = string(p)
	}

	payload := map[string]any{
		"robotCode": cfg.ClientID,
		"msgKey":    msgKey,
		"msgParam":  msgParam,
	}
	if data.ConversationType == "2" {
		payload["userIds"] = []string{data.SenderStaffId}
	} else {
		payload["openConversationId"] = data.ConversationId
	}
	return payload
}

// sendMediaProactive sends a media message via the proactive API.
func sendMediaProactive(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel, filePath, mediaID string, info mediaInfo) error {
	endpoint := proactiveEndpoint(data.ConversationType)
	p := buildProactiveMediaPayload(cfg, data, filePath, mediaID, info)
	return doProactiveRequest(ctx, cfg, endpoint, p)
}

// doProactiveRequest sends an authenticated POST to a DingTalk API endpoint.
func doProactiveRequest(ctx context.Context, cfg config.DingTalkConfig, endpoint string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal proactive payload: %w", err)
	}

	token, err := getAccessToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return fmt.Errorf("get access token for proactive send: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("proactive send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("proactive send failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

// dingTalkMediaType maps file extension to DingTalk upload media type.
func dingTalkMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".png", ".gif", ".bmp":
		return "image"
	case ".mp4":
		return "video"
	case ".mp3", ".wav", ".amr":
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
	limit := int64(maxUploadSize)
	if mediaType == "voice" {
		limit = maxVoiceUploadSize
	}
	if fi.Size() > limit {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", fi.Size(), limit)
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
func sendMediaViaDingTalk(ctx context.Context, cfg config.DingTalkConfig, webhook, filePath, mediaID string, info mediaInfo) error {
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
			"voice":   map[string]string{"mediaId": mediaID, "duration": strconv.Itoa(info.durationMs)},
		}
	case "video":
		payload = map[string]any{
			"msgtype": "video",
			"video": map[string]string{
				"duration":     strconv.Itoa(info.durationSec),
				"videoMediaId": mediaID,
				"videoType":    "mp4",
				"picMediaId":   info.picMediaID,
			},
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

func doEmojiRequest(ctx context.Context, token string, payload []byte, url string, timeout time.Duration, action string) error {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create emoji %s request: %w", action, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s emoji reaction: %w", action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("emoji %s returned non-200: %d", action, resp.StatusCode)
	}
	return nil
}

func doEmojiRequestWithRetry(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel, url string, timeout time.Duration, action string, logFn func(string, ...zap.Field)) {
	token, err := getAccessToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		logFn("get access token for emoji "+action, zap.Error(err))
		return
	}
	payload := buildEmojiPayload(cfg, data)
	retryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := retryutil.RetryWithBackoff(retryCtx, func() error {
		return doEmojiRequest(retryCtx, token, payload, url, timeout, action)
	}, retryutil.DefaultRetryCount, retryutil.DefaultRetryDelay); err != nil {
		logFn(action+" emoji failed after retries", zap.Error(err))
	}
}

func addThinkingEmoji(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel) {
	doEmojiRequestWithRetry(ctx, cfg, data, "https://api.dingtalk.com/v1.0/robot/emotion/reply", 5*time.Second, "reply", log.Warn)
}

func recallThinkingEmoji(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel) {
	doEmojiRequestWithRetry(ctx, cfg, data, "https://api.dingtalk.com/v1.0/robot/emotion/recall", 3*time.Second, "recall", log.Warn)
}

var _ platform.Platform = (*DingTalkPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*DingTalkReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*DingTalkSender)(nil)
