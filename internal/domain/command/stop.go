package command

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

type BeeStopper interface {
	StopSession(sessionKey string) bool
}

type StopMessageStore interface {
	FailReceived(ctx context.Context, sessionKey string) ([]string, error)
}

// WorkerStopper stops a single worker's in-flight work for a session without
// deleting its session context. Implemented by *session.ClearService.
type WorkerStopper interface {
	StopWorker(ctx context.Context, sessionKey string, w model.Worker) (session.StopWorkerResult, error)
}

type StopCommandHandler struct {
	feeder     BeeStopper
	msgs       StopMessageStore
	workers    WorkerNameLookup
	workerStop WorkerStopper
	senders    map[string]platform.PlatformSenderAdapter
}

func NewStopCommandHandler(
	feeder BeeStopper,
	msgs StopMessageStore,
	workers WorkerNameLookup,
	workerStop WorkerStopper,
	senders map[string]platform.PlatformSenderAdapter,
) *StopCommandHandler {
	return &StopCommandHandler{
		feeder:     feeder,
		msgs:       msgs,
		workers:    workers,
		workerStop: workerStop,
		senders:    senders,
	}
}

func (h *StopCommandHandler) IsCommand(content string) bool {
	return isExactOrPrefixed(content, CmdStop)
}

func (h *StopCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdStop {
		return false
	}

	switch len(fields) {
	case 1:
		h.handleStopBee(ctx, replyTo)
	case 2:
		h.handleStopWorker(ctx, replyTo, fields[1])
	default:
		sendReply(ctx, h.senders, replyTo, i18n.M.Runtime.StopCommand.Usage)
	}
	return true
}

func (h *StopCommandHandler) handleStopBee(ctx context.Context, replyTo platform.InboundMessage) {
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
}

func (h *StopCommandHandler) handleStopWorker(ctx context.Context, replyTo platform.InboundMessage, workerName string) {
	m := i18n.M.Runtime.StopCommand
	sessionKey := replyTo.SessionKey

	w, ok := resolveSingleWorker(ctx, h.senders, h.workers, replyTo, workerName, m.LookupFailed, m.WorkerNotFound, m.WorkerDuplicate)
	if !ok {
		return
	}

	result, err := h.workerStop.StopWorker(ctx, sessionKey, w)
	if err != nil {
		log.Error("stop worker", zap.String("workerID", w.ID), zap.Error(err))
		sendReply(ctx, h.senders, replyTo, m.LookupFailed)
		return
	}

	if result.CancelledTasks > 0 {
		sendReply(ctx, h.senders, replyTo, fmt.Sprintf(m.WorkerStopped, w.Name, result.CancelledTasks))
	} else {
		sendReply(ctx, h.senders, replyTo, fmt.Sprintf(m.WorkerNothingToStop, w.Name))
	}
}
