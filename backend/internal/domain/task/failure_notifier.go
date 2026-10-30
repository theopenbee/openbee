package task

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/delivery"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
)

// PlatformFailureNotifier implements FailureNotifier by looking up the originating
// message and sending a failure notification via the appropriate platform sender.
type PlatformFailureNotifier struct {
	resolver *delivery.Resolver
}

// NewPlatformFailureNotifier creates a PlatformFailureNotifier.
func NewPlatformFailureNotifier(msgStore *store.MessageStore, senders platform.Senders) *PlatformFailureNotifier {
	return &PlatformFailureNotifier{resolver: delivery.NewResolver(msgStore, senders)}
}

func (n *PlatformFailureNotifier) NotifyTaskFailure(ctx context.Context, target delivery.Target, info model.FailureInfo) error {
	m := i18n.M.Runtime.FailureNotifier
	var workerLine string
	if info.WorkerName != "" {
		workerLine = fmt.Sprintf(m.WorkerLine, info.WorkerName)
	} else {
		workerLine = m.ParseFailed
	}
	content := m.TaskFailed + workerLine + fmt.Sprintf(m.Failed, info.Reason)
	return n.sendNotification(ctx, target, content)
}

func (n *PlatformFailureNotifier) NotifyTaskCancelled(ctx context.Context, target delivery.Target, workerName string) error {
	m := i18n.M.Runtime.FailureNotifier
	content := m.TaskCancelled
	if workerName != "" {
		content += fmt.Sprintf(m.WorkerLine, workerName)
	}
	return n.sendNotification(ctx, target, content)
}

func (n *PlatformFailureNotifier) sendNotification(ctx context.Context, target delivery.Target, content string) error {
	sender, outbound, err := n.resolver.Resolve(ctx, target)
	if err != nil {
		return fmt.Errorf("resolve notification target: %w", err)
	}

	// 499 content runes + ellipsis = 500 visible runes, the platform ceiling.
	const maxContentRunes = 499
	outbound.Content = utils.TruncateRunes(content, maxContentRunes)
	outbound.SourceType = store.SourceTypeSystem

	if err := sender.Send(ctx, outbound); err != nil {
		log.Error("send notification", zap.String("messageID", target.MessageID), zap.String("sessionKey", target.SessionKey), zap.Error(err))
		return fmt.Errorf("send notification: %w", err)
	}
	return nil
}
