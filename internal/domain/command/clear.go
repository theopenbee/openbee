package command

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

const clearConfirmTimeout = 30 * time.Second

type WorkerNameLookup interface {
	ListByName(name string) ([]model.Worker, error)
}

type ClearSessionStore interface {
	SessionContextLister
	DeleteSessionContextForEngine(ctx context.Context, sessionKey, agentID, engine string) (bool, error)
}

type ClearTaskStore interface {
	TaskBySessionLister
	CancelBySessionKey(ctx context.Context, sessionKey, taskType string) (int64, error)
}

type ClearExecStopper interface {
	StopExecution(executionID string) error
}

type ClearSessionDispatcher interface {
	ClearSession(sessionKey string)
}

type ClearCommandHandler struct {
	workers      WorkerNameLookup
	sessions     ClearSessionStore
	tasks        ClearTaskStore
	execStopper  ClearExecStopper
	sessionClear ClearSessionDispatcher
	senders      map[string]platform.PlatformSenderAdapter
	engineCfg    *enginecfg.Store

	now func() time.Time

	mu      sync.Mutex
	pending map[string]time.Time // key: sessionKey + "::" + normalized command → expiry
}

func NewClearCommandHandler(
	workers WorkerNameLookup,
	sessions ClearSessionStore,
	tasks ClearTaskStore,
	execStopper ClearExecStopper,
	sessionClear ClearSessionDispatcher,
	senders map[string]platform.PlatformSenderAdapter,
	engineCfg *enginecfg.Store,
) *ClearCommandHandler {
	return &ClearCommandHandler{
		workers:      workers,
		sessions:     sessions,
		tasks:        tasks,
		execStopper:  execStopper,
		sessionClear: sessionClear,
		senders:      senders,
		engineCfg:    engineCfg,
		now:          time.Now,
		pending:      make(map[string]time.Time),
	}
}

func (h *ClearCommandHandler) IsCommand(content string) bool {
	return isExactOrPrefixed(content, CmdClear)
}

func (h *ClearCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdClear {
		return false
	}

	switch len(fields) {
	case 1:
		h.handleClearAll(ctx, replyTo)
	case 2:
		h.handleClearWorker(ctx, replyTo, fields[1])
	default:
		h.reply(ctx, replyTo, i18n.M.Runtime.ClearCommand.Usage)
	}
	return true
}

func (h *ClearCommandHandler) handleClearAll(ctx context.Context, replyTo platform.InboundMessage) {
	m := i18n.M.Runtime.ClearCommand
	sessionKey := replyTo.SessionKey
	pendingKey := h.pendingKey(sessionKey, CmdClear)
	confirmed := h.consumePending(pendingKey)

	agents, err := h.sessions.ListActiveSessionContexts(ctx, sessionKey, h.engineCfg.Get())
	if err != nil {
		log.Error("list session contexts for /clear", zap.Error(err))
		h.reply(ctx, replyTo, m.LookupFailed)
		return
	}
	if len(agents) == 0 {
		h.reply(ctx, replyTo, m.NoContext)
		return
	}

	runningTasks, err := h.tasks.ListBySessionKey(ctx, sessionKey, model.TaskStatusRunning, model.TaskTypeImmediate)
	if err != nil {
		log.Error("list running tasks for /clear", zap.Error(err))
		h.reply(ctx, replyTo, m.LookupFailed)
		return
	}

	if len(runningTasks) > 0 && !confirmed {
		list := formatAgentList(agents)
		h.storePending(pendingKey)
		h.reply(ctx, replyTo, fmt.Sprintf(m.ConfirmAllWithTasks, list, len(runningTasks)))
		return
	}

	for _, t := range runningTasks {
		if t.ExecutionID == "" {
			continue
		}
		if err := h.execStopper.StopExecution(t.ExecutionID); err != nil {
			log.Error("stop execution for /clear", zap.String("executionID", t.ExecutionID), zap.Error(err))
		}
	}

	cancelled, err := h.tasks.CancelBySessionKey(ctx, sessionKey, model.TaskTypeImmediate)
	if err != nil {
		log.Error("cancel tasks for /clear exec", zap.Error(err))
	}
	h.sessionClear.ClearSession(sessionKey)

	list := formatAgentList(agents)
	if cancelled > 0 {
		h.reply(ctx, replyTo, fmt.Sprintf(m.ClearedWithTasks, list, cancelled))
	} else {
		h.reply(ctx, replyTo, fmt.Sprintf(m.Cleared, list))
	}
}

func (h *ClearCommandHandler) handleClearWorker(ctx context.Context, replyTo platform.InboundMessage, workerName string) {
	m := i18n.M.Runtime.ClearCommand
	sessionKey := replyTo.SessionKey

	workers, err := h.workers.ListByName(workerName)
	if err != nil {
		log.Error("list workers by name for /clear", zap.String("name", workerName), zap.Error(err))
		h.reply(ctx, replyTo, m.NoContext)
		return
	}
	if len(workers) == 0 {
		h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerNotFound, workerName))
		return
	}
	if len(workers) > 1 {
		var lines []string
		for _, w := range workers {
			lines = append(lines, fmt.Sprintf("  %s (%s)", w.Name, w.ID))
		}
		h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerDuplicate, workerName, strings.Join(lines, "\n")))
		return
	}

	w := workers[0]
	activeEngine := h.engineCfg.Resolve(w.Engine)

	deleted, err := h.sessions.DeleteSessionContextForEngine(ctx, sessionKey, w.ID, activeEngine)
	if err != nil {
		log.Error("delete worker session context", zap.String("workerID", w.ID), zap.Error(err))
		h.reply(ctx, replyTo, m.NoContext)
		return
	}
	if !deleted {
		h.reply(ctx, replyTo, m.NoContext)
		return
	}
	h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerCleared, w.Name, activeEngine))
}

func (h *ClearCommandHandler) pendingKey(sessionKey, cmd string) string {
	return sessionKey + "::" + cmd
}

// consumePending atomically retrieves and removes a valid (non-expired) pending entry.
func (h *ClearCommandHandler) consumePending(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	expiresAt, exists := h.pending[key]
	if !exists || !time.Now().Before(expiresAt) {
		return false
	}
	delete(h.pending, key)
	return true
}

// storePending records a confirmation deadline and opportunistically reaps any
// other expired entries — the only path that grows the map, so it bounds size.
func (h *ClearCommandHandler) storePending(key string) {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, expiresAt := range h.pending {
		if !now.Before(expiresAt) {
			delete(h.pending, k)
		}
	}
	h.pending[key] = now.Add(clearConfirmTimeout)
}

func (h *ClearCommandHandler) reply(ctx context.Context, replyTo platform.InboundMessage, text string) {
	sendReply(ctx, h.senders, replyTo, text)
}

func formatAgentList(agents []store.SessionAgent) string {
	parts := make([]string, 0, len(agents))
	for _, a := range agents {
		parts = append(parts, fmt.Sprintf("%s (%s)", a.Name, a.Engine))
	}
	return strings.Join(parts, ", ")
}
