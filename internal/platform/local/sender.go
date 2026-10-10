package local

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
)

// LocalSender implements platform.PlatformSenderAdapter.
// It broadcasts replies to connected SSE clients via SSEHub.
type LocalSender struct {
	hub       *SSEHub
	mediaRoot string
	legacy    *LegacySession
}

// NewLocalSender constructs a LocalSender.
func NewLocalSender(hub *SSEHub, mediaRoot string, legacy *LegacySession) *LocalSender {
	return &LocalSender{hub: hub, mediaRoot: mediaRoot, legacy: legacy}
}

func (s *LocalSender) PrepareOutbound(_ context.Context, msg platform.OutboundMessage) (platform.OutboundMessage, error) {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	msg.ReplyTo.SessionKey = s.legacy.Resolve(cmp.Or(msg.ReplyTo.SessionKey, msg.SessionKey))
	msg.SessionKey = s.legacy.Resolve(msg.SessionKey)
	if msg.MediaPath == "" {
		return msg, nil
	}
	dir, err := MediaDir(s.mediaRoot, msg.ReplyTo.SessionKey)
	if err != nil {
		return msg, err
	}
	if filepath.Dir(msg.MediaPath) == dir {
		return msg, nil
	}
	staged := filepath.Join(dir, msg.ID+"_"+platform.SanitizeFileName(filepath.Base(msg.MediaPath)))
	if err := utils.LinkOrCopyFile(msg.MediaPath, staged); err != nil {
		return msg, fmt.Errorf("stage media: %w", err)
	}
	msg.MediaPath = staged
	return msg, nil
}

// Send broadcasts the reply to any connected SSE clients for the session.
func (s *LocalSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	msg, err := s.PrepareOutbound(ctx, msg)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"id":         msg.ID,
		"content":    msg.Content,
		"created_at": time.Now().UnixMilli(),
	}
	if msg.MediaPath != "" {
		payload["media_paths"] = []string{filepath.Base(msg.MediaPath)}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal SSE payload: %w", err)
	}
	s.hub.Broadcast(msg.ReplyTo.SessionKey, string(data))
	return nil
}
