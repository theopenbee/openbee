package msgingest

import (
	"context"

	"github.com/theopenbee/openbee/internal/domain/reply"
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

// HandleEmpty sends the empty-message hint to the user via reply.Send.
func (h *DefaultEmptyMessageHandler) HandleEmpty(ctx context.Context, msg platform.InboundMessage) {
	reply.Send(ctx, h.senders, msg, i18n.M.Runtime.EmptyMessage.Hint)
}
