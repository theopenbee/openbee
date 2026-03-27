package task_dispatcher

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
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
	stored, err := n.msgStore.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message for failure notification: %w", err)
	}

	sender, ok := n.senders[stored.Platform]
	if !ok {
		return fmt.Errorf("no sender for platform %q", stored.Platform)
	}

	var workerLine string
	if info.WorkerName != "" {
		workerLine = fmt.Sprintf("\nWorker：%s", info.WorkerName)
	} else {
		workerLine = "\n消息解析失败"
	}
	var content string
	if info.RetryCount >= 0 {
		content = fmt.Sprintf("❌ 任务执行失败%s\n已重试：%d/%d 次\n错误：%s",
			workerLine, info.RetryCount, info.MaxRetries, info.Reason)
	} else {
		content = fmt.Sprintf("❌ 任务执行失败%s\n错误：%s",
			workerLine, info.Reason)
	}
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
		log.Error("send failure notification", zap.String("messageID", messageID), zap.Error(err))
		return fmt.Errorf("send failure notification: %w", err)
	}
	return nil
}
