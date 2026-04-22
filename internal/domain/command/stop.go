package command

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/platform"
)

type BeeStopper interface {
	StopSession(sessionKey string) bool
}

type StopMessageStore interface {
	FailReceived(ctx context.Context, sessionKey string) ([]string, error)
}

type StopCommandHandler struct {
	feeder  BeeStopper
	msgs    StopMessageStore
	senders map[string]platform.PlatformSenderAdapter
}

func NewStopCommandHandler(
	feeder BeeStopper,
	msgs StopMessageStore,
	senders map[string]platform.PlatformSenderAdapter,
) *StopCommandHandler {
	return &StopCommandHandler{feeder: feeder, msgs: msgs, senders: senders}
}

func (h *StopCommandHandler) IsCommand(content string) bool {
	return isExactOrPrefixed(content, CmdStop)
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
