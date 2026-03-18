package local

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
)

// LocalSender implements platform.PlatformSenderAdapter.
// It writes bee replies to the local_replies table and broadcasts via SSEHub.
type LocalSender struct {
	replyStore *store.LocalReplyStore
	hub        *SSEHub
}

// NewLocalSender constructs a LocalSender.
func NewLocalSender(replyStore *store.LocalReplyStore, hub *SSEHub) *LocalSender {
	return &LocalSender{replyStore: replyStore, hub: hub}
}

// Send stores the reply and broadcasts it to any connected SSE clients.
// The session key is read from msg.ReplyTo.SessionKey (msg.SessionKey is always empty).
func (s *LocalSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	sessionKey := msg.ReplyTo.SessionKey
	id := uuid.New().String()

	if err := s.replyStore.Create(ctx, id, sessionKey, msg.Content); err != nil {
		return err
	}

	data, _ := json.Marshal(map[string]any{
		"id":         id,
		"content":    msg.Content,
		"created_at": time.Now().UnixMilli(),
	})
	s.hub.Broadcast(sessionKey, string(data))
	return nil
}
