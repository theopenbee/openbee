package feishu

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/platform"
	"github.com/robobee/core/internal/utils"
)

// FeishuPlatform implements platform.Platform for Feishu/Lark.
type FeishuPlatform struct {
	receiver         *FeishuReceiver
	sender           *FeishuSender
	pendingReactions sync.Map
}

// NewPlatform constructs a FeishuPlatform from configuration.
func NewPlatform(cfg config.FeishuConfig) platform.Platform {
	larkClient := lark.NewClient(cfg.AppID, cfg.AppSecret)
	p := &FeishuPlatform{}
	p.receiver = &FeishuReceiver{larkClient: larkClient, cfg: cfg, pendingReactions: &p.pendingReactions}
	p.sender = &FeishuSender{larkClient: larkClient, pendingReactions: &p.pendingReactions}
	return p
}

func (f *FeishuPlatform) ID() string                                 { return "feishu" }
func (f *FeishuPlatform) Receiver() platform.PlatformReceiverAdapter { return f.receiver }
func (f *FeishuPlatform) Sender() platform.PlatformSenderAdapter     { return f.sender }

// FeishuReceiver connects to Feishu via WebSocket and dispatches inbound messages.
type FeishuReceiver struct {
	larkClient       *lark.Client
	cfg              config.FeishuConfig
	pendingReactions *sync.Map
}

func (r *FeishuReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			msg := event.Event.Message
			senderOpenId := "<nil>"
			if s := event.Event.Sender; s != nil && s.SenderId != nil {
				senderOpenId = utils.DerefStr(s.SenderId.OpenId)
			}
			slog.Info("received event", "component", "feishu",
				"messageId", utils.DerefStr(msg.MessageId),
				"chatId", utils.DerefStr(msg.ChatId),
				"chatType", utils.DerefStr(msg.ChatType),
				"messageType", utils.DerefStr(msg.MessageType),
				"content", utils.DerefStr(msg.Content),
				"senderOpenId", senderOpenId,
			)
			if msg == nil || *msg.MessageType != "text" {
				return nil
			}
			var content map[string]string
			if err := json.Unmarshal([]byte(*msg.Content), &content); err != nil {
				return nil
			}
			text := content["text"]
			if text == "" {
				return nil
			}
			sender := event.Event.Sender
			if sender == nil || sender.SenderId == nil || sender.SenderId.OpenId == nil {
				slog.Warn("skipping message with nil sender or OpenId", "component", "feishu")
				return nil
			}
			if msg.ChatId == nil {
				slog.Warn("skipping message with nil ChatId", "component", "feishu")
				return nil
			}
			senderID := *sender.SenderId.OpenId
			rawBytes, err := json.Marshal(event)
			if err != nil {
				slog.Error("failed to marshal event", "component", "feishu", "error", err)
				return nil
			}

			// Add "typing" reaction to acknowledge message receipt
			go func() {
				resp, err := r.larkClient.Im.MessageReaction.Create(ctx,
					larkim.NewCreateMessageReactionReqBuilder().
						MessageId(*msg.MessageId).
						Body(larkim.NewCreateMessageReactionReqBodyBuilder().
							ReactionType(larkim.NewEmojiBuilder().
								EmojiType("Typing").
								Build()).
							Build()).
						Build())
				if err != nil || !resp.Success() {
					slog.Error("add reaction error", "component", "feishu", "error", err, "resp", resp)
					return
				}
				if resp.Data != nil && resp.Data.ReactionId != nil {
					r.pendingReactions.Store(*msg.MessageId, *resp.Data.ReactionId)
				}
			}()

			dispatch(platform.InboundMessage{
				Platform:          "feishu",
				SenderID:          senderID,
				SessionKey:        "feishu:" + *msg.ChatId + ":" + senderID,
				Content:           text,
				Raw:               string(rawBytes),
				PlatformMessageID: utils.DerefStrOrEmpty(msg.MessageId),
				MessageTime:       utils.ParseMillis(msg.CreateTime),
			})

			return nil
		})

	wsClient := larkws.NewClient(r.cfg.AppID, r.cfg.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	slog.Info("Feishu bot starting...")
	return wsClient.Start(ctx)
}

// FeishuSender sends messages via the Feishu IM API.
type FeishuSender struct {
	larkClient       *lark.Client
	pendingReactions *sync.Map
}

func (s *FeishuSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var event larkim.P2MessageReceiveV1
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &event); err != nil {
		slog.Error("failed to unmarshal raw", "component", "feishu", "error", err)
		return nil
	}
	imMsg := event.Event.Message
	chatID := *imMsg.ChatId
	chatType := *imMsg.ChatType
	messageID := *imMsg.MessageId

	// Recall "typing" reaction before sending reply
	if reactionID, ok := s.pendingReactions.LoadAndDelete(messageID); ok {
		recallCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		resp, err := s.larkClient.Im.MessageReaction.Delete(recallCtx,
			larkim.NewDeleteMessageReactionReqBuilder().
				MessageId(messageID).
				ReactionId(reactionID.(string)).
				Build())
		cancel()
		if err != nil || !resp.Success() {
			slog.Warn("recall reaction error", "component", "feishu", "error", err, "resp", resp)
		}
	}

	content, _ := json.Marshal(map[string]string{"text": msg.Content})

	if chatType == "p2p" {
		resp, err := s.larkClient.Im.Message.Create(ctx,
			larkim.NewCreateMessageReqBuilder().
				ReceiveIdType(larkim.ReceiveIdTypeChatId).
				Body(larkim.NewCreateMessageReqBodyBuilder().
					MsgType(larkim.MsgTypeText).
					ReceiveId(chatID).
					Content(string(content)).
					Build()).
				Build())
		if err != nil || !resp.Success() {
			slog.Error("send message error", "component", "feishu", "error", err, "resp", resp)
		}
	} else {
		resp, err := s.larkClient.Im.Message.Reply(ctx,
			larkim.NewReplyMessageReqBuilder().
				MessageId(messageID).
				Body(larkim.NewReplyMessageReqBodyBuilder().
					MsgType(larkim.MsgTypeText).
					Content(string(content)).
					Build()).
				Build())
		if err != nil || !resp.Success() {
			slog.Error("reply message error", "component", "feishu", "error", err, "resp", resp)
		}
	}
	return nil
}

var _ platform.Platform = (*FeishuPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*FeishuReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*FeishuSender)(nil)

