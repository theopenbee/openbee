package local

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
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
}

// NewLocalSender constructs a LocalSender.
func NewLocalSender(hub *SSEHub, mediaRoot string) *LocalSender {
	return &LocalSender{hub: hub, mediaRoot: mediaRoot}
}

// Send broadcasts the reply to any connected SSE clients for the session.
func (s *LocalSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	sessionKey := cmp.Or(msg.ReplyTo.SessionKey, msg.SessionKey)

	payload := map[string]any{
		"id":         msg.ID,
		"content":    msg.Content,
		"created_at": time.Now().UnixMilli(),
	}
	if msg.MediaPath != "" {
		name, err := s.stageMedia(sessionKey, msg.ID, msg.MediaPath)
		if err != nil {
			return fmt.Errorf("stage media: %w", err)
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
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", src)
	}
	name := MediaFileName(messageID, src)
	if err := utils.CopyFile(src, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	return name, nil
}
