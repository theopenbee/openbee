package command

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
)

const clearConfirmTimeout = 30 * time.Second

type WorkerNameLookup interface {
	ListByName(name string) ([]model.Worker, error)
	GetByIDs(ids []string) ([]model.Worker, error)
}

// ClearService is the destructive-ops surface used by the /clear handler.
// Implemented by *session.ClearService.
type ClearService interface {
	EvaluateClearSession(ctx context.Context, sessionKey string) (session.ClearSessionPreview, error)
	ClearSession(ctx context.Context, sessionKey string, preview session.ClearSessionPreview) (session.ClearSessionResult, error)
	EvaluateClearWorker(ctx context.Context, sessionKey string, w model.Worker) (session.ClearWorkerPreview, error)
	ClearWorker(ctx context.Context, sessionKey string, w model.Worker, preview session.ClearWorkerPreview) (session.ClearWorkerResult, error)
}

type ClearCommandHandler struct {
	workers      WorkerNameLookup
	svc          ClearService
	runningExecs utils.RunningExecLookup
	senders      map[string]platform.PlatformSenderAdapter

	now func() time.Time

	mu      sync.Mutex
	pending map[string]time.Time // key: sessionKey + "::" + normalized command → expiry
}

func NewClearCommandHandler(
	workers WorkerNameLookup,
	svc ClearService,
	senders map[string]platform.PlatformSenderAdapter,
	runningExecs utils.RunningExecLookup,
) *ClearCommandHandler {
	return &ClearCommandHandler{
		workers:      workers,
		svc:          svc,
		runningExecs: runningExecs,
		senders:      senders,
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

	preview, err := h.svc.EvaluateClearSession(ctx, sessionKey)
	if err != nil {
		log.Error("evaluate clear session", zap.Error(err))
		h.reply(ctx, replyTo, m.LookupFailed)
		return
	}
	if len(preview.Agents) == 0 {
		h.reply(ctx, replyTo, m.NoContext)
		return
	}
	if !confirmed && len(preview.ActiveTasks) > 0 {
		h.storePending(pendingKey)
		h.reply(ctx, replyTo, h.formatConfirmPrompt(ctx, preview.Agents, preview.ActiveTasks))
		return
	}

	result, err := h.svc.ClearSession(ctx, sessionKey, preview)
	if err != nil {
		log.Error("clear session", zap.Error(err))
		h.reply(ctx, replyTo, m.LookupFailed)
		return
	}

	list := formatAgentList(result.Agents)
	if result.CancelledTasks > 0 {
		h.reply(ctx, replyTo, fmt.Sprintf(m.ClearedWithTasks, list, result.CancelledTasks))
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
	pendingKey := h.pendingKey(sessionKey, CmdClear+" "+w.ID)
	confirmed := h.consumePending(pendingKey)

	preview, err := h.svc.EvaluateClearWorker(ctx, sessionKey, w)
	if err != nil {
		log.Error("evaluate clear worker session", zap.String("workerID", w.ID), zap.Error(err))
		h.reply(ctx, replyTo, m.NoContext)
		return
	}
	if !confirmed && len(preview.ActiveTasks) > 0 {
		h.storePending(pendingKey)
		h.reply(ctx, replyTo, h.formatWorkerConfirmPrompt(ctx, w, preview.Engine, preview.ActiveTasks))
		return
	}

	result, err := h.svc.ClearWorker(ctx, sessionKey, w, preview)
	if err != nil {
		log.Error("clear worker session", zap.String("workerID", w.ID), zap.Error(err))
		h.reply(ctx, replyTo, m.NoContext)
		return
	}

	if !result.DeletedContext && result.CancelledTasks == 0 {
		h.reply(ctx, replyTo, m.NoContext)
		return
	}

	if result.CancelledTasks > 0 {
		h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerClearedWithTasks, w.Name, result.Engine, result.CancelledTasks))
	} else {
		h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerCleared, w.Name, result.Engine))
	}
}

// renderConfirmPrompt builds the shared body of /clear confirmation prompts: a
// caller-supplied header, the task list with running exec IDs, and a footer.
func (h *ClearCommandHandler) renderConfirmPrompt(ctx context.Context, header []string, footer string, tasks []model.Task, workerNames map[string]string) string {
	nowMs := h.now().UnixMilli()
	execIDs := utils.RunningExecIDsForTasks(ctx, log, h.runningExecs, tasks)

	lines := make([]string, 0, len(header)+len(tasks)+4)
	lines = append(lines, header...)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf(i18n.M.Runtime.ClearCommand.ConfirmTasksHeader, len(tasks)))
	for _, t := range tasks {
		lines = append(lines, formatTaskLine(i18n.M.Runtime.StatusCommand.TaskLine, t, workerNames, execIDs, nowMs))
	}
	lines = append(lines, "")
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}

func (h *ClearCommandHandler) formatConfirmPrompt(ctx context.Context, agents []store.SessionAgent, tasks []model.Task) string {
	m := i18n.M.Runtime.ClearCommand
	header := make([]string, 0, 1+len(agents))
	header = append(header, m.ConfirmHeader)
	for _, a := range agents {
		header = append(header, fmt.Sprintf(m.ConfirmAgentLine, a.Name, a.Engine))
	}
	return h.renderConfirmPrompt(ctx, header, m.ConfirmFooter, tasks, resolveWorkerNames(h.workers, tasks))
}

func (h *ClearCommandHandler) formatWorkerConfirmPrompt(ctx context.Context, w model.Worker, engine string, tasks []model.Task) string {
	m := i18n.M.Runtime.ClearCommand
	header := []string{fmt.Sprintf(m.WorkerConfirmHeader, w.Name, engine)}
	footer := fmt.Sprintf(m.WorkerConfirmFooter, w.Name)
	return h.renderConfirmPrompt(ctx, header, footer, tasks, map[string]string{w.ID: w.Name})
}

func (h *ClearCommandHandler) pendingKey(sessionKey, cmd string) string {
	return sessionKey + "::" + cmd
}

func (h *ClearCommandHandler) consumePending(key string) bool {
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	expiresAt, exists := h.pending[key]
	if !exists || !now.Before(expiresAt) {
		return false
	}
	delete(h.pending, key)
	return true
}

// storePending opportunistically reaps expired entries — the only path that
// grows the map, so it bounds size.
func (h *ClearCommandHandler) storePending(key string) {
	now := h.now()
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
