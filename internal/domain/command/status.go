package command

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

const (
	maxInstructionRunes = 40
	shortExecIDLen      = 8
)

type StatusSessionLister interface {
	ListActiveSessionContexts(ctx context.Context, sessionKey, beeEngine string) ([]store.SessionAgent, error)
}

type StatusTaskLister interface {
	ListBySessionKey(ctx context.Context, sessionKey, status, taskType string) ([]model.Task, error)
}

type StatusWorkerLookup interface {
	GetByID(id string) (model.Worker, error)
}

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
	now := time.Now()
	nowSec := now.Unix()
	nowMs := now.UnixMilli()

	var b strings.Builder
	b.WriteString(m.Header)
	b.WriteByte('\n')

	fmt.Fprintf(&b, m.SectionBees, len(agents))
	b.WriteByte('\n')
	if len(agents) == 0 {
		b.WriteString(m.EmptyMarker)
		b.WriteByte('\n')
	} else {
		for _, a := range agents {
			fmt.Fprintf(&b, m.BeeLine, a.Name, a.Engine, formatRelative(nowSec-a.UpdatedAt))
			b.WriteByte('\n')
		}
	}

	fmt.Fprintf(&b, m.SectionTasks, len(tasks))
	b.WriteByte('\n')
	if len(tasks) == 0 {
		b.WriteString(m.EmptyMarker)
	} else {
		nameCache := make(map[string]string, len(tasks))
		for i, t := range tasks {
			workerName, ok := nameCache[t.WorkerID]
			if !ok {
				workerName = h.lookupWorkerName(t.WorkerID)
				nameCache[t.WorkerID] = workerName
			}
			runtimeSec := (nowMs - t.CreatedAt) / 1000
			fmt.Fprintf(&b, m.TaskLine,
				workerName,
				truncateInstruction(t.Instruction),
				formatRelative(runtimeSec),
				shortExecID(t.ExecutionID),
			)
			if i < len(tasks)-1 {
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

// lookupWorkerName falls back to the raw id on failure so the user can still
// correlate the line with logs.
func (h *StatusCommandHandler) lookupWorkerName(id string) string {
	if id == "" {
		return "?"
	}
	w, err := h.workers.GetByID(id)
	if err != nil || w.Name == "" {
		return id
	}
	return w.Name
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

func truncateInstruction(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= maxInstructionRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxInstructionRunes]) + "…"
}

func shortExecID(id string) string {
	if len(id) <= shortExecIDLen {
		return id
	}
	return id[:shortExecIDLen]
}
