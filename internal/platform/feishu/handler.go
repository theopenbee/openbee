package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
	"unsafe"

	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/infra/utils"
	retryutil "github.com/theopenbee/openbee/internal/utils"
)

var log = logger.With(zap.String("component", "feishu"))

const mentionPrefix = "@"

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
			log.Info("received event",
				zap.String("messageId", utils.DerefStr(msg.MessageId)),
				zap.String("chatId", utils.DerefStr(msg.ChatId)),
				zap.String("chatType", utils.DerefStr(msg.ChatType)),
				zap.String("messageType", utils.DerefStr(msg.MessageType)),
				zap.String("content", utils.DerefStr(msg.Content)),
				zap.String("senderOpenId", senderOpenId),
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
				log.Warn("skipping unsupported message type", zap.String("msgType", msgType))
				return nil
			}

			if textContent == "" {
				return nil
			}

			textContent = resolveMentions(textContent, msg.Mentions)
			sender := event.Event.Sender
			if sender == nil || sender.SenderId == nil || sender.SenderId.OpenId == nil {
				log.Warn("skipping message with nil sender or OpenId")
				return nil
			}
			if msg.ChatId == nil {
				log.Warn("skipping message with nil ChatId")
				return nil
			}
			senderID := *sender.SenderId.OpenId
			rawBytes, err := json.Marshal(event)
			if err != nil {
				log.Error("failed to marshal event", zap.Error(err))
				return nil
			}

			// Add "typing" reaction to acknowledge message receipt.
			// Use a buffered channel to coordinate with Send's reaction recall,
			// avoiding a race where LoadAndDelete runs before Store.
			reactionCh := make(chan string, 1)
			r.pendingReactions.Store(*msg.MessageId, reactionCh)
			go func() {
				defer time.AfterFunc(10*time.Minute, func() {
					r.pendingReactions.Delete(*msg.MessageId)
				})
				req := larkim.NewCreateMessageReactionReqBuilder().
					MessageId(*msg.MessageId).
					Body(larkim.NewCreateMessageReactionReqBodyBuilder().
						ReactionType(larkim.NewEmojiBuilder().
							EmojiType("Typing").
							Build()).
						Build()).
					Build()
				addCtx, addCancel := context.WithTimeout(ctx, 30*time.Second)
				defer addCancel()
				var reactionID string
				err := retryutil.RetryWithBackoff(addCtx, func() error {
					resp, e := r.larkClient.Im.MessageReaction.Create(addCtx, req)
					if e != nil {
						return e
					}
					if !resp.Success() {
						return fmt.Errorf("add reaction: %w", resp.CodeError)
					}
					if resp.Data != nil && resp.Data.ReactionId != nil {
						reactionID = *resp.Data.ReactionId
					}
					return nil
				}, retryutil.DefaultRetryCount, retryutil.DefaultRetryDelay)
				if err != nil {
					log.Error("add reaction failed after retries", zap.Error(err))
					close(reactionCh)
				} else if reactionID != "" {
					reactionCh <- reactionID
				} else {
					close(reactionCh)
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

	// The larkws SDK's Start() blocks with select{} and never checks ctx.Done().
	// Watch for shutdown and close the underlying WebSocket connection so the SDK's
	// read loop unblocks and the server receives a proper Close frame before exit.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			closeWSConn(wsClient)
		case <-done:
		}
	}()

	log.Info("Feishu bot starting...")
	return wsClient.Start(ctx)
}

// closeWSConn sends a WebSocket normal-closure frame and closes the underlying
// connection of a larkws.Client. The SDK exposes no public Stop method, so we
// use unsafe reflection to reach the private conn field.
func closeWSConn(client *larkws.Client) {
	v := reflect.ValueOf(client).Elem()
	connField := v.FieldByName("conn")
	if !connField.IsValid() || connField.IsNil() {
		return
	}
	// reflect.NewAt + unsafe.Pointer lets us read an unexported pointer field
	// without triggering the "unexported field" panic.
	connVal := reflect.NewAt(connField.Type(), unsafe.Pointer(connField.UnsafeAddr())).Elem().Interface()
	conn, ok := connVal.(*gorillaws.Conn)
	if !ok || conn == nil {
		return
	}
	closeMsg := gorillaws.FormatCloseMessage(gorillaws.CloseNormalClosure, "server shutdown")
	if err := conn.WriteControl(gorillaws.CloseMessage, closeMsg, time.Now().Add(3*time.Second)); err != nil {
		log.Debug("feishu ws close frame error", zap.Error(err))
	}
	if err := conn.Close(); err != nil {
		log.Debug("feishu ws conn close error", zap.Error(err))
	}
}

// validFeishuKey matches valid Feishu file/image keys.
var validFeishuKey = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)

// parseMediaKeys extracts image_key, file_key, and file_name from content JSON based on message type.
func parseMediaKeys(contentJSON, msgType string) (imageKey, fileKey, fileName string) {
	var content map[string]any
	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return "", "", ""
	}
	str := func(key string) string {
		v, _ := content[key].(string)
		return v
	}
	switch msgType {
	case "image", "sticker":
		return str("image_key"), "", ""
	default:
		return "", str("file_key"), str("file_name")
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
	maxSize := r.cfg.MaxMediaSize
	data, err := io.ReadAll(io.LimitReader(resp.File, int64(maxSize)+1))
	if err != nil {
		return nil, fmt.Errorf("read resource: %w", err)
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("resource too large: exceeds %d bytes", maxSize)
	}
	return data, nil
}

// downloadSaveAndPlaceholder downloads a resource, detects MIME, saves it, builds a placeholder,
// and appends extracted text if available. Returns the placeholder string.
func (r *FeishuReceiver) downloadSaveAndPlaceholder(
	ctx context.Context, messageID, key, resType, mediaType, fileName string,
) string {
	data, err := r.downloadMessageResource(ctx, messageID, key, resType)
	if err != nil {
		log.Error("download media failed", zap.String("key", key), zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	var ext string
	var mime string
	if fileName != "" {
		ext = filepath.Ext(fileName)
	}
	if ext == "" {
		mime = r.mediaSvc.DetectMIME(data, fileName)
		ext = r.mediaSvc.ExtensionFromMIME(mime)
	} else {
		mime = r.mediaSvc.DetectMIME(data, fileName)
	}

	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Error("save media failed", zap.String("key", key), zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	// Use MIME-based media type when caller passes empty mediaType (e.g. post media)
	mt := mediaType
	if mt == "" {
		mt = media.MediaTypeFromMIME(mime)
	}
	return r.mediaSvc.BuildPlaceholder(mt, path, fileName)
}

// resolveMediaContent handles download, save, text extraction, and placeholder building for media messages.
func (r *FeishuReceiver) resolveMediaContent(ctx context.Context, messageID, msgType, contentJSON string) string {
	imageKey, fileKey, fileName := parseMediaKeys(contentJSON, msgType)
	key := imageKey
	if key == "" {
		key = fileKey
	}
	if key == "" {
		log.Warn("no file key found in content", zap.String("msgType", msgType))
		return r.mediaSvc.BuildPlaceholder(mediaTypeForMsgType(msgType), "", fileName)
	}
	return r.downloadSaveAndPlaceholder(ctx, messageID, key, resourceType(msgType), mediaTypeForMsgType(msgType), fileName)
}

// resolvePostContent parses a post (rich text) message, downloads embedded images/media, and returns combined content.
func (r *FeishuReceiver) resolvePostContent(ctx context.Context, messageID, contentJSON string) string {
	result, err := ParsePostContent(contentJSON)
	if err != nil {
		log.Warn("failed to parse post content", zap.Error(err))
		return i18n.M.Runtime.Feishu.RichTextFallback
	}

	var parts []string
	if result.TextContent != "" {
		parts = append(parts, result.TextContent)
	}

	totalMedia := len(result.ImageKeys) + len(result.MediaKeys)
	if totalMedia > 0 {
		mediaParts := make([]string, totalMedia)
		g, gCtx := errgroup.WithContext(ctx)
		for i, imageKey := range result.ImageKeys {
			g.Go(func() error {
				mediaParts[i] = r.downloadSaveAndPlaceholder(gCtx, messageID, imageKey, "image", "image", "")
				return nil
			})
		}
		offset := len(result.ImageKeys)
		for i, mk := range result.MediaKeys {
			g.Go(func() error {
				mediaParts[offset+i] = r.downloadSaveAndPlaceholder(gCtx, messageID, mk.FileKey, "file", "", mk.FileName)
				return nil
			})
		}
		_ = g.Wait()
		parts = append(parts, mediaParts...)
	}

	if len(parts) == 0 {
		return i18n.M.Runtime.Feishu.RichTextFallback
	}
	return strings.Join(parts, "\n")
}


// fileTypeInfo holds Feishu upload metadata for a file extension.
type fileTypeInfo struct {
	category string // "image", "audio", "video", "file"
	fileType string // Feishu file_type for upload: "opus", "mp4", "pdf", etc.
	msgType  string // Feishu msg_type for sending: "audio", "media", "file"
}

var extFileTypeMap = map[string]fileTypeInfo{
	// Images
	".jpg": {"image", "stream", "file"}, ".jpeg": {"image", "stream", "file"},
	".png": {"image", "stream", "file"}, ".gif": {"image", "stream", "file"},
	".webp": {"image", "stream", "file"}, ".bmp": {"image", "stream", "file"},
	".ico": {"image", "stream", "file"}, ".tiff": {"image", "stream", "file"},
	// Audio
	".opus": {"audio", "opus", "audio"}, ".ogg": {"audio", "opus", "audio"},
	".mp3": {"audio", "stream", "file"}, ".wav": {"audio", "stream", "file"},
	".amr": {"audio", "stream", "file"}, ".aac": {"audio", "stream", "file"},
	".flac": {"audio", "stream", "file"}, ".m4a": {"audio", "stream", "file"},
	// Video
	".mp4": {"video", "mp4", "media"}, ".mov": {"video", "mp4", "media"},
	".avi": {"video", "mp4", "media"},
	// Documents
	".pdf": {"file", "pdf", "file"},
	".doc": {"file", "doc", "file"}, ".docx": {"file", "doc", "file"},
	".xls": {"file", "xls", "file"}, ".xlsx": {"file", "xls", "file"},
	".ppt": {"file", "ppt", "file"}, ".pptx": {"file", "ppt", "file"},
}

var defaultFileType = fileTypeInfo{"file", "stream", "file"}

func lookupFileType(path string) fileTypeInfo {
	ext := strings.ToLower(filepath.Ext(path))
	if info, ok := extFileTypeMap[ext]; ok {
		return info
	}
	return defaultFileType
}

func fileCategory(path string) string      { return lookupFileType(path).category }
func feishuFileType(path string) string     { return lookupFileType(path).fileType }
func feishuMediaMsgType(ft string) string {
	// ft is the result of feishuFileType, look up by matching fileType field
	switch ft {
	case "opus":
		return "audio"
	case "mp4":
		return "media"
	default:
		return "file"
	}
}

// FeishuSender sends messages via the Feishu IM API.
type FeishuSender struct {
	larkClient       *lark.Client
	pendingReactions *sync.Map
}

func (s *FeishuSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var event larkim.P2MessageReceiveV1
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &event); err != nil {
		return fmt.Errorf("unmarshal raw event: %w", err)
	}
	imMsg := event.Event.Message
	if imMsg == nil || imMsg.ChatId == nil || imMsg.ChatType == nil || imMsg.MessageId == nil {
		return fmt.Errorf("malformed event: missing required message fields")
	}
	chatID := *imMsg.ChatId
	chatType := *imMsg.ChatType
	messageID := *imMsg.MessageId

	// Recall "typing" reaction in background to avoid blocking the reply.
	if val, ok := s.pendingReactions.LoadAndDelete(messageID); ok {
		if ch, ok := val.(chan string); ok {
			go func() {
				recallCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				select {
				case reactionID, received := <-ch:
					if received && reactionID != "" {
						req := larkim.NewDeleteMessageReactionReqBuilder().
							MessageId(messageID).
							ReactionId(reactionID).
							Build()
						if err := retryutil.RetryWithBackoff(recallCtx, func() error {
							resp, e := s.larkClient.Im.MessageReaction.Delete(recallCtx, req)
							if e != nil {
								return e
							}
							if !resp.Success() {
								return fmt.Errorf("recall reaction: %w", resp.CodeError)
							}
							return nil
						}, retryutil.DefaultRetryCount, retryutil.DefaultRetryDelay); err != nil {
							log.Warn("recall reaction failed after retries", zap.Error(err))
						}
					}
				case <-timer.C:
					log.Warn("timed out waiting for reaction ID", zap.String("messageID", messageID))
				}
			}()
		}
	}

	if msg.MediaPath != "" {
		return s.sendMedia(ctx, msg.MediaPath, chatID, chatType, messageID)
	}

	// Text message: send as rich-text (post) with md tag to support Markdown rendering.
	return s.sendMessage(ctx, chatID, chatType, messageID, "post", BuildPostContent(msg.Content))
}

func (s *FeishuSender) createMessage(ctx context.Context, chatID, msgType, content string) error {
	resp, err := s.larkClient.Im.Message.Create(ctx,
		larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(larkim.ReceiveIdTypeChatId).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				MsgType(msgType).
				ReceiveId(chatID).
				Content(content).
				Build()).
			Build())
	if err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("create message failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

func (s *FeishuSender) sendMessage(ctx context.Context, chatID, chatType, messageID, msgType, content string) error {
	if chatType == "p2p" {
		return s.createMessage(ctx, chatID, msgType, content)
	}

	// Group chat: try reply first
	resp, err := s.larkClient.Im.Message.Reply(ctx,
		larkim.NewReplyMessageReqBuilder().
			MessageId(messageID).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				MsgType(msgType).
				Content(content).
				Build()).
			Build())
	if err == nil && resp.Success() {
		return nil
	}

	code := 0
	if resp != nil {
		code = resp.Code
	}
	if code == 230011 || code == 231003 {
		log.Warn("reply failed, falling back to direct send", zap.Int("code", code))
		return s.createMessage(ctx, chatID, msgType, content)
	}
	if err != nil {
		return fmt.Errorf("reply message: %w", err)
	}
	return fmt.Errorf("reply message failed: code=%d msg=%s", resp.Code, resp.Msg)
}

func (s *FeishuSender) sendMedia(ctx context.Context, mediaPath, chatID, chatType, messageID string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("read media file: %w", err)
	}
	if len(data) > 30*1024*1024 {
		return fmt.Errorf("file too large: %d bytes (max 30MB)", len(data))
	}

	category := fileCategory(mediaPath)
	fileName := platform.SanitizeFileName(filepath.Base(mediaPath))

	if category == "image" {
		return s.uploadAndSendImage(ctx, data, chatID, chatType, messageID)
	}
	return s.uploadAndSendFile(ctx, data, fileName, mediaPath, chatID, chatType, messageID)
}

func (s *FeishuSender) uploadAndSendImage(ctx context.Context, data []byte, chatID, chatType, messageID string) error {
	uploadCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := s.larkClient.Im.Image.Create(uploadCtx,
		larkim.NewCreateImageReqBuilder().
			Body(larkim.NewCreateImageReqBodyBuilder().
				ImageType(larkim.ImageTypeMessage).
				Image(bytes.NewReader(data)).
				Build()).
			Build())
	if err != nil || !resp.Success() {
		return fmt.Errorf("upload image: err=%v resp=%v", err, resp)
	}

	imageKey := *resp.Data.ImageKey
	content, _ := json.Marshal(map[string]string{"image_key": imageKey})
	return s.sendMessage(ctx, chatID, chatType, messageID, larkim.MsgTypeImage, string(content))
}

func (s *FeishuSender) uploadAndSendFile(ctx context.Context, data []byte, fileName, mediaPath, chatID, chatType, messageID string) error {
	ft := feishuFileType(mediaPath)

	uploadCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := s.larkClient.Im.File.Create(uploadCtx,
		larkim.NewCreateFileReqBuilder().
			Body(larkim.NewCreateFileReqBodyBuilder().
				FileType(ft).
				FileName(fileName).
				File(bytes.NewReader(data)).
				Build()).
			Build())
	if err != nil || !resp.Success() {
		return fmt.Errorf("upload file: err=%v resp=%v", err, resp)
	}

	fileKey := *resp.Data.FileKey
	msgType := feishuMediaMsgType(ft)
	var contentMap map[string]string
	switch msgType {
	case "media":
		contentMap = map[string]string{"file_key": fileKey, "file_name": fileName}
	default: // "audio", "file"
		contentMap = map[string]string{"file_key": fileKey}
	}
	content, _ := json.Marshal(contentMap)
	return s.sendMessage(ctx, chatID, chatType, messageID, msgType, string(content))
}

var _ platform.Platform = (*FeishuPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*FeishuReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*FeishuSender)(nil)

// resolveMentions replaces Feishu's opaque mention keys (e.g. "@_user_1") with
// human-readable names (e.g. "@Tom"). Feishu delivers @mentions as placeholder
// keys in the message text and resolves them separately in the Mentions slice;
// we need to stitch them back together before passing content upstream.
// Keys with no corresponding entry in mentions are left unchanged.
func resolveMentions(text string, mentions []*larkim.MentionEvent) string {
	if len(mentions) == 0 {
		return text
	}
	pairs := make([]string, 0, len(mentions)*2)
	for _, m := range mentions {
		if m.Key == nil || m.Name == nil {
			continue
		}
		pairs = append(pairs, *m.Key, mentionPrefix+*m.Name)
	}
	if len(pairs) == 0 {
		return text
	}
	return strings.NewReplacer(pairs...).Replace(text)
}
