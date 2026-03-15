package dingtalk

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"

	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/platform"
)

// DingTalkPlatform implements platform.Platform for DingTalk.
type DingTalkPlatform struct {
	receiver *DingTalkReceiver
	sender   *DingTalkSender
}

// NewPlatform constructs a DingTalkPlatform from configuration.
func NewPlatform(cfg config.DingTalkConfig) platform.Platform {
	return &DingTalkPlatform{
		receiver: &DingTalkReceiver{cfg: cfg},
		sender:   &DingTalkSender{},
	}
}

func (d *DingTalkPlatform) ID() string                                 { return "dingtalk" }
func (d *DingTalkPlatform) Receiver() platform.PlatformReceiverAdapter { return d.receiver }
func (d *DingTalkPlatform) Sender() platform.PlatformSenderAdapter     { return d.sender }

// DingTalkReceiver connects to DingTalk via the stream SDK and dispatches inbound messages.
type DingTalkReceiver struct {
	cfg config.DingTalkConfig
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
		dispatch(msg)
		slog.Info("dispatched message", "component", "dingtalk", "sessionKey", msg.SessionKey)
		return []byte(""), nil
	})

	slog.Info("DingTalk bot starting...")
	return cli.Start(ctx)
}

// DingTalkSender sends messages via the DingTalk chatbot replier.
type DingTalkSender struct{}

const markdownTitle = "RoboBee"

func (s *DingTalkSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var data chatbot.BotCallbackDataModel
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &data); err != nil {
		slog.Error("failed to unmarshal raw", "component", "dingtalk", "error", err)
		return nil
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

var _ platform.Platform = (*DingTalkPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*DingTalkReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*DingTalkSender)(nil)
