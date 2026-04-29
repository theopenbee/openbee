package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
)

const (
	maxInstructionRunes = 40
	shortExecIDLen      = 8
)

// SessionContextLister exposes the read used by both /status and /clear to
// enumerate the session's currently active per-engine bee contexts.
type SessionContextLister interface {
	ListActiveSessionContexts(ctx context.Context, sessionKey, beeEngine string) ([]store.SessionAgent, error)
}

// TaskBySessionLister exposes the read used by both /status and /clear to
// enumerate tasks scoped to a session, filtered by status and type.
type TaskBySessionLister interface {
	ListBySessionKey(ctx context.Context, sessionKey, status, taskType string) ([]model.Task, error)
}

// StatusWorkerLookup resolves worker IDs to worker rows in a single batch.
type StatusWorkerLookup interface {
	GetByIDs(ids []string) ([]model.Worker, error)
}

// StatusCommandHandler implements the /status slash command.
type StatusCommandHandler struct {
	sessions  SessionContextLister
	tasks     TaskBySessionLister
	workers   StatusWorkerLookup
	senders   map[string]platform.PlatformSenderAdapter
	engineCfg *enginecfg.Store
	now       func() time.Time
}

func NewStatusCommandHandler(
	sessions SessionContextLister,
	tasks TaskBySessionLister,
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
		now:       time.Now,
	}
}

// SetClockForTest overrides the time source used to compute relative durations.
// Test-only; production code should leave the default time.Now in place.
func (h *StatusCommandHandler) SetClockForTest(now func() time.Time) {
	h.now = now
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

	m := i18n.M.Runtime.StatusCommand
	sessionKey := replyTo.SessionKey

	agents, err := h.sessions.ListActiveSessionContexts(ctx, sessionKey, h.engineCfg.Get())
	if err != nil {
		log.Error("list session contexts for /status", zap.String("sessionKey", sessionKey), zap.Error(err))
		h.reply(ctx, replyTo, m.LookupFailed)
		return true
	}

	runningTasks, err := h.tasks.ListBySessionKey(ctx, sessionKey, model.TaskStatusRunning, model.TaskTypeImmediate)
	if err != nil {
		log.Error("list tasks for /status", zap.String("sessionKey", sessionKey), zap.Error(err))
		h.reply(ctx, replyTo, m.LookupFailed)
		return true
	}

	h.reply(ctx, replyTo, h.formatStatus(agents, runningTasks))
	return true
}

func (h *StatusCommandHandler) formatStatus(agents []store.SessionAgent, tasks []model.Task) string {
	m := i18n.M.Runtime.StatusCommand
	now := h.now()
	nowSec := now.Unix()
	nowMs := now.UnixMilli()

	workerNames := h.resolveWorkerNames(tasks)

	var lines []string
	lines = append(lines, m.Header)
	lines = append(lines, fmt.Sprintf(m.SectionBees, len(agents)))
	if len(agents) == 0 {
		lines = append(lines, m.EmptyMarker)
	} else {
		for _, a := range agents {
			lines = append(lines, fmt.Sprintf(m.BeeLine, a.Name, a.Engine, formatRelative(nowSec-a.UpdatedAt)))
		}
	}
	lines = append(lines, fmt.Sprintf(m.SectionTasks, len(tasks)))
	if len(tasks) == 0 {
		lines = append(lines, m.EmptyMarker)
	} else {
		for _, t := range tasks {
			runtimeSec := (nowMs - t.CreatedAt) / 1000
			lines = append(lines, fmt.Sprintf(m.TaskLine,
				workerNameOrFallback(workerNames, t.WorkerID),
				utils.TruncateRunes(strings.Join(strings.Fields(t.Instruction), " "), maxInstructionRunes),
				formatRelative(runtimeSec),
				shortExecID(t.ExecutionID),
			))
		}
	}
	return strings.Join(lines, "\n")
}

// resolveWorkerNames batches a single GetByIDs call for every distinct
// WorkerID across the running tasks. On failure it returns nil so the caller
// can fall back to raw IDs and still surface useful output to the user.
func (h *StatusCommandHandler) resolveWorkerNames(tasks []model.Task) map[string]string {
	if len(tasks) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tasks))
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t.WorkerID == "" {
			continue
		}
		if _, ok := seen[t.WorkerID]; ok {
			continue
		}
		seen[t.WorkerID] = struct{}{}
		ids = append(ids, t.WorkerID)
	}
	if len(ids) == 0 {
		return nil
	}
	workers, err := h.workers.GetByIDs(ids)
	if err != nil {
		log.Error("batch lookup workers for /status", zap.Error(err))
		return nil
	}
	out := make(map[string]string, len(workers))
	for _, w := range workers {
		if w.Name != "" {
			out[w.ID] = w.Name
		}
	}
	return out
}

// workerNameOrFallback returns the looked-up name when present, "?" for an
// empty id (so callers can still correlate the line with logs), and the raw
// id otherwise.
func workerNameOrFallback(names map[string]string, id string) string {
	if id == "" {
		return "?"
	}
	if name, ok := names[id]; ok {
		return name
	}
	return id
}

func (h *StatusCommandHandler) reply(ctx context.Context, replyTo platform.InboundMessage, text string) {
	sendReply(ctx, h.senders, replyTo, text)
}

// formatRelative clamps negative inputs to "0s" to tolerate clock skew.
func formatRelative(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh", seconds/3600)
	default:
		return fmt.Sprintf("%dd", seconds/86400)
	}
}

func shortExecID(id string) string {
	if len(id) <= shortExecIDLen {
		return id
	}
	return id[:shortExecIDLen]
}
