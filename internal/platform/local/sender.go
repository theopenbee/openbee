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

type LocalSender struct {
	pub       *EventPublisher
	mediaRoot string
}

func NewLocalSender(pub *EventPublisher, mediaRoot string) *LocalSender {
	return &LocalSender{pub: pub, mediaRoot: mediaRoot}
}

func (s *LocalSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	sessionKey := cmp.Or(msg.ReplyTo.SessionKey, msg.SessionKey)

	chat := ChatMessage{
		ID:      msg.ID,
		Sender:  Sender{Type: cmp.Or(msg.SourceType, SenderBee), ID: msg.SourceID},
		Kind:    cmp.Or(msg.Kind, KindMessage),
		Content: msg.Content,
		TS:      time.Now().UnixMilli(),
	}
	if chat.Kind == KindHandoff {
		var info HandoffInfo
		if err := json.Unmarshal([]byte(msg.Payload), &info); err != nil {
			return fmt.Errorf("decode handoff payload: %w", err)
		}
		chat.Handoff = &info
	}
	if msg.MediaPath != "" {
		name, err := s.stageMedia(sessionKey, msg.ID, msg.MediaPath)
		if err != nil {
			return fmt.Errorf("stage media: %w", err)
		}
		chat.MediaPaths = []string{name}
	}

	s.pub.Message(sessionKey, chat)
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
