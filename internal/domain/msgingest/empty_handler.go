package msgingest

import (
	"context"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/platform"
)

// EmptyMessageHandler is invoked when an inbound message is empty after the
// bot @mention is stripped. Implementations should reply to the user (or
// no-op) without performing any DB writes or pipeline state mutations.
type EmptyMessageHandler interface {
	HandleEmpty(ctx context.Context, msg platform.InboundMessage)
}

// DefaultEmptyMessageHandler replies with a localized hint via the per-platform
// sender adapter.
type DefaultEmptyMessageHandler struct {
	senders map[string]platform.PlatformSenderAdapter
}

// NewDefaultEmptyMessageHandler constructs a handler bound to the given
// per-platform sender map.
func NewDefaultEmptyMessageHandler(senders map[string]platform.PlatformSenderAdapter) *DefaultEmptyMessageHandler {
	return &DefaultEmptyMessageHandler{senders: senders}
}

// HandleEmpty sends the empty-message hint to the user. If no sender is
// registered for msg.Platform, the call logs a warning and returns. Send
// errors are logged at warn level and not retried.
func (h *DefaultEmptyMessageHandler) HandleEmpty(ctx context.Context, msg platform.InboundMessage) {
	sender, ok := h.senders[msg.Platform]
	if !ok {
		log.Warn("no sender for empty-message reply",
			zap.String("platform", msg.Platform),
			zap.String("sessionKey", msg.SessionKey))
		return
	}
	out := platform.OutboundMessage{
		SessionKey: msg.SessionKey,
		Content:    i18n.M.Runtime.EmptyMessage.Hint,
		ReplyTo:    msg,
		SourceType: "system",
	}
	if err := sender.Send(ctx, out); err != nil {
		log.Warn("send empty-message reply failed",
			zap.String("platform", msg.Platform),
			zap.String("sessionKey", msg.SessionKey),
			zap.Error(err))
	}
}
