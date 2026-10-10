package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/theopenbee/openbee/internal/platform"
)

// LocalSender implements platform.PlatformSenderAdapter.
// It broadcasts replies to connected SSE clients via SSEHub.
type LocalSender struct {
	hub       *SSEHub
	mediaRoot string
}

// NewLocalSender constructs a LocalSender.
func NewLocalSender(hub *SSEHub, mediaRoot string) *LocalSender {
	return &LocalSender{hub: hub, mediaRoot: mediaRoot}
}

// Send broadcasts the reply to any connected SSE clients for the session.
// The session key is read from msg.ReplyTo.SessionKey.
func (s *LocalSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	sessionKey := msg.ReplyTo.SessionKey

	payload := map[string]any{
		"id":         msg.ID,
		"content":    msg.Content,
		"created_at": time.Now().UnixMilli(),
	}
	if msg.MediaPath != "" {
		name, err := s.stageMedia(sessionKey, msg.ID, msg.MediaPath)
		if err != nil {
			return err
		}
		payload["media_paths"] = []string{name}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal SSE payload: %w", err)
	}
	s.hub.Broadcast(sessionKey, string(data))
	return nil
}

func (s *LocalSender) stageMedia(sessionKey, messageID, src string) (string, error) {
	dir, err := MediaDir(s.mediaRoot, sessionKey)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create media dir: %w", err)
	}
	name := MediaFileName(messageID, src)
	if err := copyFile(src, filepath.Join(dir, name)); err != nil {
		return "", fmt.Errorf("stage media: %w", err)
	}
	return name, nil
}
