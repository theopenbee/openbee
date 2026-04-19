package msgingest

import (
	"context"

	"github.com/theopenbee/openbee/internal/platform"
)

// CommandHandler processes slash commands extracted from inbound messages.
// HandleCommand returns true if the message was a recognized command and
// was handled (the caller should skip normal message processing).
type CommandHandler interface {
	HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool
}

// ChainHandlers returns a CommandHandler that tries each handler in order,
// returning true on the first match.
func ChainHandlers(handlers ...CommandHandler) CommandHandler {
	return &chainedHandler{handlers: handlers}
}

type chainedHandler struct {
	handlers []CommandHandler
}

func (c *chainedHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	for _, h := range c.handlers {
		if h.HandleCommand(ctx, content, replyTo) {
			return true
		}
	}
	return false
}
