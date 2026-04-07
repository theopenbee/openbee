package local

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/theopenbee/openbee/internal/platform"
)

// LocalSender implements platform.PlatformSenderAdapter.
// It broadcasts replies to connected SSE clients via SSEHub.
type LocalSender struct {
	hub *SSEHub
}

// NewLocalSender constructs a LocalSender.
func NewLocalSender(hub *SSEHub) *LocalSender {
	return &LocalSender{hub: hub}
}

// Send broadcasts the reply to any connected SSE clients for the session.
// The session key is read from msg.ReplyTo.SessionKey.
func (s *LocalSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	sessionKey := msg.ReplyTo.SessionKey

	data, err := json.Marshal(map[string]any{
		"content":    msg.Content,
		"created_at": time.Now().UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("marshal SSE payload: %w", err)
	}
	s.hub.Broadcast(sessionKey, string(data))
	return nil
}
