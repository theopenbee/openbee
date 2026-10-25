package platform

import (
	"context"
)

// InboundMessage carries a parsed message from any platform.
type InboundMessage struct {
	Platform          string // channel id, e.g. "local" or a plugin id
	SenderID          string
	SessionKey        string // channel-prefixed session key, "<channel>:<chat_id>:<scope>"
	Content           string
	RawContent        string // original message text with formatting preserved (at-tags, markup)
	Raw               string // original platform event, used by the sender for reply metadata
	PlatformMessageID string // platform-native dedup ID; empty string means no dedup
	MessageTime       int64  // Unix milliseconds from platform; 0 = unknown (fallback to server time)
}

// OutboundMessage carries a reply to send back on a platform.
type OutboundMessage struct {
	ID           string
	SessionKey   string
	Content      string
	ReplyTo      InboundMessage
	Proactive    bool
	MediaPath    string // optional local file path to upload and send
	SourceType   string // "bee" | "worker" | "system" — who sent this message
	SourceID     string // worker_id when SourceType is "worker"
	InboundMsgID string // ID of the bee_platform_messages row that triggered this reply
}

// PlatformReceiverAdapter receives inbound messages and dispatches them.
type PlatformReceiverAdapter interface {
	Start(ctx context.Context, dispatch func(InboundMessage)) error
}

// PlatformSenderAdapter sends outbound messages on a platform.
type PlatformSenderAdapter interface {
	Send(ctx context.Context, msg OutboundMessage) error
}

// Platform bundles a receiver and sender for a single messaging platform.
type Platform interface {
	ID() string
	Receiver() PlatformReceiverAdapter
	Sender() PlatformSenderAdapter
}

type CombinedSender interface {
	SendsTextWithMedia() bool
}

func SendsTextWithMedia(s PlatformSenderAdapter) bool {
	c, ok := s.(CombinedSender)
	return ok && c.SendsTextWithMedia()
}
