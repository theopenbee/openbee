package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
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
		text := strings.TrimSpace(data.Text.Content)
		slog.Info("received message", "component", "dingtalk", "conversationId", data.ConversationId, "sender", data.SenderNick, "text", text)
		if text == "" {
			return []byte(""), nil
		}
		if data.SenderStaffId == "" {
			slog.Warn("skipping message with empty SenderStaffId", "component", "dingtalk", "conversationId", data.ConversationId)
			return []byte(""), nil
		}
		rawBytes, err := json.Marshal(data)
		if err != nil {
			slog.Error("failed to marshal raw callback data", "component", "dingtalk", "error", err)
		}
		msg := platform.InboundMessage{
			Platform:          "dingtalk",
			SenderID:          data.SenderStaffId,
			SessionKey:        "dingtalk:" + data.ConversationId + ":" + data.SenderStaffId,
			Content:           text,
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
	replier := chatbot.NewChatbotReplier()
	slog.Info("sending reply", "component", "dingtalk", "sessionKey", msg.ReplyTo.SessionKey, "webhookLen", len(data.SessionWebhook), "contentLen", len(msg.Content))
	if err := replier.SimpleReplyMarkdown(ctx, data.SessionWebhook, []byte(markdownTitle), []byte(msg.Content)); err != nil {
		slog.Error("reply send error", "component", "dingtalk", "error", err)
		return nil
	}
	slog.Info("reply sent ok", "component", "dingtalk")
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
