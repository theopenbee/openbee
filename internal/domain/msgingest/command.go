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
