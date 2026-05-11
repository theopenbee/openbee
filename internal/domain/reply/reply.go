// Package reply provides a shared helper for replying to an inbound message
// through the per-platform sender adapter.
package reply

import (
	"context"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

var log = logger.With(zap.String("component", "reply"))

// Send replies to replyTo with text via the platform-specific sender. If no
// sender is registered for replyTo.Platform the call is a no-op. Send errors
// are logged and not retried.
func Send(ctx context.Context, senders map[string]platform.PlatformSenderAdapter, replyTo platform.InboundMessage, text string) {
	sender, ok := senders[replyTo.Platform]
	if !ok {
		return
	}
	if err := sender.Send(ctx, platform.OutboundMessage{
		Content:    text,
		ReplyTo:    replyTo,
		SourceType: store.SourceTypeSystem,
	}); err != nil {
		log.Warn("reply send failed", zap.String("platform", replyTo.Platform), zap.Error(err))
	}
}
