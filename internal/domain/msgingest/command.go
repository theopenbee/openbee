package msgingest

import (
	"context"

	"github.com/theopenbee/openbee/internal/platform"
)

// CommandHandler processes slash commands extracted from inbound messages.
// HandleCommand returns true if the message was a recognized command and
// was handled (the caller should skip normal message processing).
type CommandHandler interface {
	// IsCommand reports whether content looks like a recognized command,
	// without side effects. Used for fast-path detection in Dispatch.
	IsCommand(content string) bool
	HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool
}

func ChainHandlers(handlers ...CommandHandler) CommandHandler {
	return &chainedHandler{handlers: handlers}
}

type chainedHandler struct {
	handlers []CommandHandler
}

func (c *chainedHandler) IsCommand(content string) bool {
	for _, h := range c.handlers {
		if h.IsCommand(content) {
			return true
		}
	}
	return false
}

func (c *chainedHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	for _, h := range c.handlers {
		if h.HandleCommand(ctx, content, replyTo) {
			return true
		}
	}
	return false
}
