package command

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// BeeStopper cancels the running bee for a session.
type BeeStopper interface {
	StopSession(sessionKey string) bool
}

// StopMessageStore cancels pending messages for a session.
type StopMessageStore interface {
	FailReceived(ctx context.Context, sessionKey string) ([]string, error)
}

// StopFailureNotifier sends failure notifications for individual messages.
type StopFailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}

// StopCommandHandler handles the /stop command.
type StopCommandHandler struct {
	feeder   BeeStopper
	msgs     StopMessageStore
	notifier StopFailureNotifier
	senders  map[string]platform.PlatformSenderAdapter
}

// NewStopCommandHandler creates a StopCommandHandler.
func NewStopCommandHandler(
	feeder BeeStopper,
	msgs StopMessageStore,
	notifier StopFailureNotifier,
	senders map[string]platform.PlatformSenderAdapter,
) *StopCommandHandler {
	return &StopCommandHandler{feeder: feeder, msgs: msgs, notifier: notifier, senders: senders}
}

func (h *StopCommandHandler) IsCommand(content string) bool {
	return content == CmdStop
}

func (h *StopCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	if content != CmdStop {
		return false
	}
	sessionKey := replyTo.SessionKey
	m := i18n.M.Runtime.StopCommand

	ids, err := h.msgs.FailReceived(ctx, sessionKey)
	if err != nil {
		log.Error("stop: fail received messages", zap.String("sessionKey", sessionKey), zap.Error(err))
	}

	for _, id := range ids {
		if notifyErr := h.notifier.NotifyTaskFailure(ctx, id, model.FailureInfo{
			Reason:     "stopped by /stop",
			WorkerName: "bee",
		}); notifyErr != nil {
			log.Error("stop: notify failure", zap.String("messageID", id), zap.Error(notifyErr))
		}
	}

	beeWasStopped := h.feeder.StopSession(sessionKey)

	var reply string
	switch {
	case beeWasStopped && len(ids) > 0:
		reply = fmt.Sprintf(m.StoppedWithMessages, len(ids))
	case beeWasStopped:
		reply = m.Stopped
	case len(ids) > 0:
		reply = fmt.Sprintf(m.CancelledMessages, len(ids))
	default:
		reply = m.NothingToStop
	}
	sendReply(ctx, h.senders, replyTo, reply)
	return true
}
