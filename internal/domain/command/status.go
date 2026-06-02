package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/session"
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

// SessionContextLister is shared by /status and /clear.
type SessionContextLister interface {
	ListActiveSessionContexts(ctx context.Context, sessionKey, beeEngine string) ([]store.SessionAgent, error)
}

// TaskBySessionLister is shared by /status and /clear.
type TaskBySessionLister interface {
	List(ctx context.Context, f store.TaskFilter) ([]model.Task, error)
}

type StatusCommandHandler struct {
	sessions      SessionContextLister
	tasks         TaskBySessionLister
	workers       WorkerByIDsLookup
	runningExecs  utils.RunningExecLookup
	senders       map[string]platform.PlatformSenderAdapter
	engineCfg     *enginecfg.Store
	now           func() time.Time
}

func NewStatusCommandHandler(
	sessions SessionContextLister,
	tasks TaskBySessionLister,
	workers WorkerByIDsLookup,
	senders map[string]platform.PlatformSenderAdapter,
	engineCfg *enginecfg.Store,
	runningExecs utils.RunningExecLookup,
) *StatusCommandHandler {
	return &StatusCommandHandler{
		sessions:     sessions,
		tasks:        tasks,
		workers:      workers,
		runningExecs: runningExecs,
		senders:      senders,
		engineCfg:    engineCfg,
		now:          time.Now,
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

	runningTasks, err := h.tasks.List(ctx, store.TaskFilter{
		SessionKey: sessionKey,
		Status:     model.TaskStatusRunning,
		Type:       model.TaskTypeImmediate,
	})
	if err != nil {
		log.Error("list tasks for /status", zap.String("sessionKey", sessionKey), zap.Error(err))
		h.reply(ctx, replyTo, m.LookupFailed)
		return true
	}

	h.reply(ctx, replyTo, h.formatStatus(ctx, agents, runningTasks))
	return true
}

func (h *StatusCommandHandler) formatStatus(ctx context.Context, agents []store.SessionAgent, tasks []model.Task) string {
	m := i18n.M.Runtime.StatusCommand
	now := h.now()
	// Both SessionAgent.UpdatedAt and Task.CreatedAt are in milliseconds.
	nowMs := now.UnixMilli()

	workerNames := resolveWorkerNames(h.workers, tasks)
	execIDs := utils.RunningExecIDsForTasks(ctx, log, h.runningExecs, tasks, session.OpStatus)

	lines := make([]string, 0, 2+max(1, len(agents))+max(1, len(tasks)))
	lines = append(lines, m.Header)
	lines = append(lines, fmt.Sprintf(m.SectionBees, len(agents)))
	if len(agents) == 0 {
		lines = append(lines, m.EmptyMarker)
	} else {
		for _, a := range agents {
			lines = append(lines, fmt.Sprintf(m.BeeLine, a.Name, a.Engine, formatRelative((nowMs-a.UpdatedAt)/1000)))
		}
	}
	lines = append(lines, fmt.Sprintf(m.SectionTasks, len(tasks)))
	if len(tasks) == 0 {
		lines = append(lines, m.EmptyMarker)
	} else {
		for _, t := range tasks {
			lines = append(lines, formatTaskLine(m.TaskLine, t, workerNames, execIDs, nowMs))
		}
	}
	return strings.Join(lines, "\n")
}

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
