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

type pendingClear struct {
	workerID  string // empty = full /clear
	engine    string
	expiresAt time.Time
}

type WorkerNameLookup interface {
	ListByName(name string) ([]model.Worker, error)
}

type ClearSessionStore interface {
	ListSessionContexts(ctx context.Context, sessionKey string) ([]store.SessionAgent, error)
	GetSessionContextForEngine(ctx context.Context, sessionKey, agentID, engine string) (sessionID string, err error)
	DeleteSessionContextForEngine(ctx context.Context, sessionKey, agentID, engine string) error
}

type ClearTaskStore interface {
	ListBySessionKey(ctx context.Context, sessionKey, status, taskType string) ([]model.Task, error)
	CancelBySessionKey(ctx context.Context, sessionKey string) (int64, error)
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

	mu      sync.Mutex
	pending map[string]*pendingClear // key: sessionKey + "::" + normalized command
}

func NewClearCommandHandler(
	workers WorkerNameLookup,
	sessions ClearSessionStore,
	tasks ClearTaskStore,
	execStopper ClearExecStopper,
	sessionClear ClearSessionDispatcher,
	senders map[string]platform.PlatformSenderAdapter,
) *ClearCommandHandler {
	h := &ClearCommandHandler{
		workers:      workers,
		sessions:     sessions,
		tasks:        tasks,
		execStopper:  execStopper,
		sessionClear: sessionClear,
		senders:      senders,
		pending:      make(map[string]*pendingClear),
	}
	go h.sweepExpired()
	return h
}

func (h *ClearCommandHandler) sweepExpired() {
	ticker := time.NewTicker(clearConfirmTimeout)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		h.mu.Lock()
		for k, p := range h.pending {
			if now.After(p.expiresAt) {
				delete(h.pending, k)
			}
		}
		h.mu.Unlock()
	}
}

func (h *ClearCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != "/clear" {
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
	pendingKey := h.pendingKey(sessionKey, "/clear")

	h.mu.Lock()
	p, exists := h.pending[pendingKey]
	isValid := exists && time.Now().Before(p.expiresAt)
	if isValid {
		delete(h.pending, pendingKey)
	}
	h.mu.Unlock()

	if isValid {
		agents, err := h.sessions.ListSessionContexts(ctx, sessionKey)
		if err != nil {
			log.Error("list session contexts for /clear confirm", zap.Error(err))
		}

		runningTasks, err := h.tasks.ListBySessionKey(ctx, sessionKey, model.TaskStatusRunning, "")
		if err != nil {
			log.Error("list running tasks for /clear confirm", zap.Error(err))
		}
		for _, t := range runningTasks {
			if t.ExecutionID != "" {
				if err := h.execStopper.StopExecution(t.ExecutionID); err != nil {
					log.Error("stop execution for /clear", zap.String("executionID", t.ExecutionID), zap.Error(err))
				}
			}
		}

		cancelled, err := h.tasks.CancelBySessionKey(ctx, sessionKey)
		if err != nil {
			log.Error("cancel tasks for /clear confirm", zap.Error(err))
		}
		h.sessionClear.ClearSession(sessionKey)

		if len(agents) == 0 {
			h.reply(ctx, replyTo, m.NoContext)
			return
		}
		list := formatAgentList(agents)
		if cancelled > 0 {
			h.reply(ctx, replyTo, fmt.Sprintf(m.ClearedWithTasks, list, cancelled))
		} else {
			h.reply(ctx, replyTo, fmt.Sprintf(m.Cleared, list))
		}
		return
	}

	agents, err := h.sessions.ListSessionContexts(ctx, sessionKey)
	if err != nil {
		log.Error("list session contexts for /clear", zap.Error(err))
	}
	if len(agents) == 0 {
		h.reply(ctx, replyTo, m.NoContext)
		return
	}

	runningTasks, _ := h.tasks.ListBySessionKey(ctx, sessionKey, model.TaskStatusRunning, "")
	list := formatAgentList(agents)

	var confirmMsg string
	if len(runningTasks) > 0 {
		confirmMsg = fmt.Sprintf(m.ConfirmAllWithTasks, list, len(runningTasks))
	} else {
		confirmMsg = fmt.Sprintf(m.ConfirmAll, list)
	}

	h.mu.Lock()
	h.pending[pendingKey] = &pendingClear{expiresAt: time.Now().Add(clearConfirmTimeout)}
	h.mu.Unlock()

	h.reply(ctx, replyTo, confirmMsg)
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
	activeEngine := w.Engine
	if activeEngine == "" {
		activeEngine = enginecfg.Get()
	}

	pendingKey := h.pendingKey(sessionKey, "/clear "+workerName)

	h.mu.Lock()
	p, exists := h.pending[pendingKey]
	isValid := exists && time.Now().Before(p.expiresAt)
	if isValid {
		delete(h.pending, pendingKey)
	}
	h.mu.Unlock()

	if isValid {
		if err := h.sessions.DeleteSessionContextForEngine(ctx, sessionKey, p.workerID, p.engine); err != nil {
			log.Error("delete worker session context", zap.String("workerID", p.workerID), zap.Error(err))
			h.reply(ctx, replyTo, m.NoContext)
			return
		}
		h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerCleared, w.Name, p.engine))
		return
	}

	sessionID, err := h.sessions.GetSessionContextForEngine(ctx, sessionKey, w.ID, activeEngine)
	if err != nil {
		log.Error("get session context for /clear worker", zap.String("workerID", w.ID), zap.Error(err))
	}
	if sessionID == "" {
		h.reply(ctx, replyTo, m.NoContext)
		return
	}

	h.mu.Lock()
	h.pending[pendingKey] = &pendingClear{
		workerID:  w.ID,
		engine:    activeEngine,
		expiresAt: time.Now().Add(clearConfirmTimeout),
	}
	h.mu.Unlock()

	h.reply(ctx, replyTo, fmt.Sprintf(m.ConfirmWorker, w.Name, activeEngine, workerName))
}

func (h *ClearCommandHandler) pendingKey(sessionKey, cmd string) string {
	return sessionKey + "::" + cmd
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
