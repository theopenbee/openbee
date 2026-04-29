package command

import (
	"context"
	"strings"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// StatusSessionLister is the subset of SessionStore needed by StatusCommandHandler.
type StatusSessionLister interface {
	ListActiveSessionContexts(ctx context.Context, sessionKey, beeEngine string) ([]store.SessionAgent, error)
}

// StatusTaskLister is the subset of TaskStore needed by StatusCommandHandler.
type StatusTaskLister interface {
	ListBySessionKey(ctx context.Context, sessionKey, status, taskType string) ([]model.Task, error)
}

// StatusWorkerLookup is the subset of WorkerStore needed by StatusCommandHandler
// to render task lines with the worker's display name.
type StatusWorkerLookup interface {
	GetByID(id string) (model.Worker, error)
}

// StatusCommandHandler handles the /status slash command.
type StatusCommandHandler struct {
	sessions  StatusSessionLister
	tasks     StatusTaskLister
	workers   StatusWorkerLookup
	senders   map[string]platform.PlatformSenderAdapter
	engineCfg *enginecfg.Store
}

func NewStatusCommandHandler(
	sessions StatusSessionLister,
	tasks StatusTaskLister,
	workers StatusWorkerLookup,
	senders map[string]platform.PlatformSenderAdapter,
	engineCfg *enginecfg.Store,
) *StatusCommandHandler {
	return &StatusCommandHandler{
		sessions:  sessions,
		tasks:     tasks,
		workers:   workers,
		senders:   senders,
		engineCfg: engineCfg,
	}
}

func (h *StatusCommandHandler) IsCommand(content string) bool {
	return isExactOrPrefixed(content, CmdStatus)
}

func (h *StatusCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdStatus {
		return false
	}
	if len(fields) != 1 {
		h.reply(ctx, replyTo, i18n.M.Runtime.StatusCommand.Usage)
		return true
	}
	// Full implementation follows in later tasks.
	h.reply(ctx, replyTo, i18n.M.Runtime.StatusCommand.Usage)
	return true
}

func (h *StatusCommandHandler) reply(ctx context.Context, replyTo platform.InboundMessage, text string) {
	sendReply(ctx, h.senders, replyTo, text)
}
