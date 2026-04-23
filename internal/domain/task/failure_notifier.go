package task

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/infra/store"
)

// PlatformFailureNotifier implements FailureNotifier by looking up the originating
// message and sending a failure notification via the appropriate platform sender.
type PlatformFailureNotifier struct {
	msgStore *store.MessageStore
	senders  map[string]platform.PlatformSenderAdapter
}

// NewPlatformFailureNotifier creates a PlatformFailureNotifier.
func NewPlatformFailureNotifier(msgStore *store.MessageStore, senders map[string]platform.PlatformSenderAdapter) *PlatformFailureNotifier {
	return &PlatformFailureNotifier{msgStore: msgStore, senders: senders}
}

func (n *PlatformFailureNotifier) NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error {
	m := i18n.M.Runtime.FailureNotifier
	var workerLine string
	if info.WorkerName != "" {
		workerLine = fmt.Sprintf(m.WorkerLine, info.WorkerName)
	} else {
		workerLine = m.ParseFailed
	}
	content := m.TaskFailed + workerLine + fmt.Sprintf(m.Failed, info.Reason)
	return n.sendNotification(ctx, messageID, content)
}

func (n *PlatformFailureNotifier) NotifyTaskCancelled(ctx context.Context, messageID string, workerName string) error {
	m := i18n.M.Runtime.FailureNotifier
	content := m.TaskCancelled
	if workerName != "" {
		content += fmt.Sprintf(m.WorkerLine, workerName)
	}
	return n.sendNotification(ctx, messageID, content)
}

func (n *PlatformFailureNotifier) sendNotification(ctx context.Context, messageID, content string) error {
	stored, err := n.msgStore.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message for notification: %w", err)
	}

	sender, ok := n.senders[stored.Platform]
	if !ok {
		return fmt.Errorf("no sender for platform %q", stored.Platform)
	}

	// Truncate to avoid exceeding platform limits; use rune slice to avoid splitting multi-byte UTF-8 characters.
	const maxRunes = 500
	runes := []rune(content)
	if len(runes) > maxRunes {
		content = string(runes[:maxRunes-1]) + "…"
	}

	outbound := platform.OutboundMessage{
		Content: content,
		ReplyTo: platform.InboundMessage{
			Platform:   stored.Platform,
			SessionKey: stored.SessionKey,
			Raw:        stored.Raw,
		},
		SourceType:   store.SourceTypeSystem,
		InboundMsgID: messageID,
	}
	if err := sender.Send(ctx, outbound); err != nil {
		log.Error("send notification", zap.String("messageID", messageID), zap.Error(err))
		return fmt.Errorf("send notification: %w", err)
	}
	return nil
}
