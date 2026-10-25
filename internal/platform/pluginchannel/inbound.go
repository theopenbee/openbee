package pluginchannel

import (
	"errors"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
)

var attachmentTypes = map[string]bool{"image": true, "audio": true, "video": true, "document": true, "sticker": true}

func ToInbound(channelID string, mediaSvc *media.Service, p InboundParams) (platform.InboundMessage, error) {
	if p.ChatID == "" {
		return platform.InboundMessage{}, errors.New("inbound chat_id is required")
	}
	if strings.Contains(p.ChatID, ":") {
		return platform.InboundMessage{}, errors.New("inbound chat_id must not contain ':'")
	}
	if p.SenderID == "" {
		return platform.InboundMessage{}, errors.New("inbound sender_id is required")
	}
	if p.Text == "" && len(p.Attachments) == 0 {
		return platform.InboundMessage{}, errors.New("inbound message needs text or attachments")
	}
	scope := p.SessionScope
	if scope == "" {
		scope = p.SenderID
	}
	parts := make([]string, 0, len(p.Attachments)+1)
	if p.Text != "" {
		parts = append(parts, p.Text)
	}
	for _, a := range p.Attachments {
		parts = append(parts, placeholder(channelID, mediaSvc, a))
	}
	msgID := ""
	if p.MessageID != "" {
		msgID = channelID + ":" + p.MessageID
	}
	return platform.InboundMessage{
		Platform:          channelID,
		SenderID:          p.SenderID,
		SessionKey:        BuildSessionKey(channelID, p.ChatID, scope),
		Content:           strings.Join(parts, "\n"),
		RawContent:        p.Text,
		Raw:               EncodeEnvelope(p.Raw, p.MessageID, p.Context),
		PlatformMessageID: msgID,
		MessageTime:       p.Time,
	}, nil
}

func placeholder(channelID string, mediaSvc *media.Service, a Attachment) string {
	typ := a.Type
	if !attachmentTypes[typ] {
		typ = "document"
	}
	name := a.Name
	if name == "" && a.Path != "" {
		name = filepath.Base(a.Path)
	}
	path := ""
	if insideDir(mediaSvc.InboundDir(), a.Path) {
		path = filepath.Clean(a.Path)
	} else if a.Path != "" {
		log.Warn("dropping attachment path outside media dir", zap.String("channel", channelID), zap.String("path", a.Path))
	}
	if a.Name == "" && path != "" {
		name = ""
	}
	return mediaSvc.BuildPlaceholder(typ, path, name)
}

func insideDir(dir, path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(dir, filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
