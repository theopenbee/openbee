package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/media"
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
func NewPlatform(cfg config.FeishuConfig, mediaSvc *media.Service) platform.Platform {
	larkClient := lark.NewClient(cfg.AppID, cfg.AppSecret)
	p := &FeishuPlatform{}
	p.receiver = &FeishuReceiver{larkClient: larkClient, cfg: cfg, pendingReactions: &p.pendingReactions, mediaSvc: mediaSvc}
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
	mediaSvc         *media.Service
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
			if msg == nil {
				return nil
			}
			msgType := utils.DerefStr(msg.MessageType)
			contentJSON := utils.DerefStr(msg.Content)
			messageID := utils.DerefStr(msg.MessageId)

			var textContent string
			switch msgType {
			case "text":
				var content map[string]string
				if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
					return nil
				}
				textContent = content["text"]
			case "image", "file", "audio", "video", "media", "sticker":
				textContent = r.resolveMediaContent(ctx, messageID, msgType, contentJSON)
			case "post":
				textContent = r.resolvePostContent(ctx, messageID, contentJSON)
			default:
				slog.Warn("skipping unsupported message type", "component", "feishu", "msgType", msgType)
				return nil
			}

			if textContent == "" {
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
				Content:           textContent,
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

// validFeishuKey matches valid Feishu file/image keys.
var validFeishuKey = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)

// parseMediaKeys extracts image_key, file_key, and file_name from content JSON based on message type.
func parseMediaKeys(contentJSON, msgType string) (imageKey, fileKey, fileName string) {
	var content map[string]string
	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return "", "", ""
	}
	switch msgType {
	case "image", "sticker":
		return content["image_key"], "", ""
	default:
		return "", content["file_key"], content["file_name"]
	}
}

// resourceType returns the Lark SDK resource type string for a given message type.
func resourceType(msgType string) string {
	switch msgType {
	case "image", "sticker":
		return "image"
	default:
		return "file"
	}
}

// mediaTypeForMsgType maps a Feishu msg_type to a media type string.
func mediaTypeForMsgType(msgType string) string {
	switch msgType {
	case "image":
		return "image"
	case "audio":
		return "audio"
	case "video", "media":
		return "video"
	case "sticker":
		return "sticker"
	default:
		return "document"
	}
}

// downloadMessageResource downloads a message resource (image or file) via the Lark SDK.
func (r *FeishuReceiver) downloadMessageResource(ctx context.Context, messageID, fileKey, resType string) ([]byte, error) {
	if !validFeishuKey.MatchString(fileKey) {
		return nil, fmt.Errorf("invalid file key: %s", fileKey)
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := r.larkClient.Im.MessageResource.Get(ctx,
		larkim.NewGetMessageResourceReqBuilder().
			MessageId(messageID).
			FileKey(fileKey).
			Type(resType).
			Build())
	if err != nil {
		return nil, fmt.Errorf("download resource: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("download resource failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if closer, ok := resp.File.(io.Closer); ok {
		defer closer.Close()
	}
	return io.ReadAll(resp.File)
}

// resolveMediaContent handles download, save, text extraction, and placeholder building for media messages.
func (r *FeishuReceiver) resolveMediaContent(ctx context.Context, messageID, msgType, contentJSON string) string {
	imageKey, fileKey, fileName := parseMediaKeys(contentJSON, msgType)
	key := imageKey
	if key == "" {
		key = fileKey
	}
	if key == "" {
		slog.Warn("no file key found in content", "component", "feishu", "msgType", msgType)
		return r.mediaSvc.BuildPlaceholder(mediaTypeForMsgType(msgType), "", fileName)
	}

	resType := resourceType(msgType)
	data, err := r.downloadMessageResource(ctx, messageID, key, resType)
	if err != nil {
		slog.Error("download media failed", "component", "feishu", "msgType", msgType, "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaTypeForMsgType(msgType), "", fileName)
	}

	// Determine extension: prefer original file extension, fall back to MIME detection
	mime := r.mediaSvc.DetectMIME(data, fileName)
	ext := r.mediaSvc.ExtensionFromMIME(mime)
	if fileName != "" {
		if origExt := filepath.Ext(fileName); origExt != "" {
			ext = origExt
		}
	}

	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		slog.Error("save media failed", "component", "feishu", "msgType", msgType, "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaTypeForMsgType(msgType), "", fileName)
	}

	placeholder := r.mediaSvc.BuildPlaceholder(mediaTypeForMsgType(msgType), path, fileName)

	// For documents, try text extraction and append
	extracted, err := r.mediaSvc.ExtractText(ctx, path)
	if err != nil {
		slog.Warn("text extraction failed", "component", "feishu", "path", path, "error", err)
	}
	if extracted != "" {
		placeholder += "\n" + extracted
	}

	return placeholder
}

// resolvePostContent parses a post (rich text) message, downloads embedded images/media, and returns combined content.
func (r *FeishuReceiver) resolvePostContent(ctx context.Context, messageID, contentJSON string) string {
	result, err := ParsePostContent(contentJSON)
	if err != nil {
		slog.Warn("failed to parse post content", "component", "feishu", "error", err)
		return "[富文本消息]"
	}

	var parts []string
	if result.TextContent != "" {
		parts = append(parts, result.TextContent)
	}

	// Download embedded images
	for _, imageKey := range result.ImageKeys {
		data, err := r.downloadMessageResource(ctx, messageID, imageKey, "image")
		if err != nil {
			slog.Error("download post image failed", "component", "feishu", "imageKey", imageKey, "error", err)
			parts = append(parts, r.mediaSvc.BuildPlaceholder("image", "", ""))
			continue
		}
		mime := r.mediaSvc.DetectMIME(data, "")
		ext := r.mediaSvc.ExtensionFromMIME(mime)
		path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
		if err != nil {
			slog.Error("save post image failed", "component", "feishu", "error", err)
			parts = append(parts, r.mediaSvc.BuildPlaceholder("image", "", ""))
			continue
		}
		parts = append(parts, r.mediaSvc.BuildPlaceholder("image", path, ""))
	}

	// Download embedded media files
	for _, mk := range result.MediaKeys {
		data, err := r.downloadMessageResource(ctx, messageID, mk.FileKey, "file")
		if err != nil {
			slog.Error("download post media failed", "component", "feishu", "fileKey", mk.FileKey, "error", err)
			parts = append(parts, r.mediaSvc.BuildPlaceholder("document", "", mk.FileName))
			continue
		}
		mime := r.mediaSvc.DetectMIME(data, mk.FileName)
		ext := r.mediaSvc.ExtensionFromMIME(mime)
		if mk.FileName != "" {
			if origExt := filepath.Ext(mk.FileName); origExt != "" {
				ext = origExt
			}
		}
		path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
		if err != nil {
			slog.Error("save post media failed", "component", "feishu", "error", err)
			parts = append(parts, r.mediaSvc.BuildPlaceholder("document", "", mk.FileName))
			continue
		}
		placeholder := r.mediaSvc.BuildPlaceholder(media.MediaTypeFromMIME(mime), path, mk.FileName)
		extracted, err := r.mediaSvc.ExtractText(ctx, path)
		if err != nil {
			slog.Warn("text extraction failed", "component", "feishu", "path", path, "error", err)
		}
		if extracted != "" {
			placeholder += "\n" + extracted
		}
		parts = append(parts, placeholder)
	}

	if len(parts) == 0 {
		return "[富文本消息]"
	}
	return strings.Join(parts, "\n")
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

