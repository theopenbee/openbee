package task_dispatcher

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/robobee/core/internal/platform"
	"github.com/robobee/core/internal/store"
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

func (n *PlatformFailureNotifier) NotifyTaskFailure(ctx context.Context, messageID, reason string) error {
	stored, err := n.msgStore.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message for failure notification: %w", err)
	}

	sender, ok := n.senders[stored.Platform]
	if !ok {
		return fmt.Errorf("no sender for platform %q", stored.Platform)
	}

	content := fmt.Sprintf("[系统通知] 任务执行失败：%s", reason)
	// Truncate very long error messages to avoid exceeding platform limits.
	// Use rune slice to avoid splitting multi-byte UTF-8 characters.
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
	}
	if err := sender.Send(ctx, outbound); err != nil {
		slog.Error("send failure notification", "component", "failurenotifier", "messageID", messageID, "error", err)
		return fmt.Errorf("send failure notification: %w", err)
	}
	return nil
}
