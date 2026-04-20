package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

const (
	// CmdEngine is the slash command that switches engines.
	CmdEngine = "/engine"
	// CmdClear is the slash command that clears session contexts.
	CmdClear = "/clear"
)

var log = logger.With(zap.String("component", "command"))

// WorkerRepository is the subset of WorkerStore needed by EngineCommandHandler.
type WorkerRepository interface {
	GetByName(name string) (model.Worker, error)
	Update(w model.Worker) (model.Worker, error)
}

// SystemConfigWriter is the subset of SystemConfigStore needed by EngineCommandHandler.
type SystemConfigWriter interface {
	Set(ctx context.Context, key, value string) error
}

// EngineValidator validates engine names against the set of enabled engines.
type EngineValidator interface {
	ValidateEngine(name string) error
	EnabledEngines() []string
}

// MessageActivityChecker reports whether active platform messages exist.
type MessageActivityChecker interface {
	HasActiveMessages(ctx context.Context) (bool, error)
}

// ExecutionActivityChecker reports whether active executions exist.
type ExecutionActivityChecker interface {
	HasActiveExecutions(ctx context.Context) (bool, error)
}

// TaskActivityChecker reports whether active immediate tasks exist.
type TaskActivityChecker interface {
	HasActiveImmediateTasks(ctx context.Context) (bool, error)
}

// SystemBusyChecker composes the activity checks that block engine switching
// while the system has in-flight work.
type SystemBusyChecker interface {
	MessageActivityChecker
	ExecutionActivityChecker
	TaskActivityChecker
}

func NewSystemBusyChecker(
	msg MessageActivityChecker,
	exec ExecutionActivityChecker,
	task TaskActivityChecker,
) SystemBusyChecker {
	return compositeBusyChecker{msg, exec, task}
}

type compositeBusyChecker struct {
	MessageActivityChecker
	ExecutionActivityChecker
	TaskActivityChecker
}

// EngineCommandHandler handles the /engine slash command.
type EngineCommandHandler struct {
	workers   WorkerRepository
	sysCfg    SystemConfigWriter
	validator EngineValidator
	senders   map[string]platform.PlatformSenderAdapter
	busy      SystemBusyChecker
	engineCfg *enginecfg.Store
}

func NewEngineCommandHandler(
	workers WorkerRepository,
	sysCfg SystemConfigWriter,
	senders map[string]platform.PlatformSenderAdapter,
	validator EngineValidator,
	busy SystemBusyChecker,
	engineCfg *enginecfg.Store,
) *EngineCommandHandler {
	return &EngineCommandHandler{
		workers:   workers,
		sysCfg:    sysCfg,
		validator: validator,
		senders:   senders,
		busy:      busy,
		engineCfg: engineCfg,
	}
}

func (h *EngineCommandHandler) IsCommand(content string) bool {
	return content == CmdEngine || strings.HasPrefix(content, CmdEngine+" ")
}

// Returns true if content is a /engine command (whether or not it succeeded).
func (h *EngineCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdEngine {
		return false
	}

	if len(fields) == 1 || len(fields) > 3 {
		h.reply(ctx, replyTo, i18n.M.Runtime.EngineCommand.Usage)
		return true
	}

	engineName := fields[1]
	if !h.isValidEngine(ctx, replyTo, engineName) {
		return true
	}
	if busyMsg, busy := h.checkBusy(ctx); busy {
		h.reply(ctx, replyTo, busyMsg)
		return true
	}

	if len(fields) == 2 {
		h.handleBeeEngine(ctx, replyTo, engineName)
	} else {
		h.handleWorkerEngine(ctx, replyTo, engineName, fields[2])
	}
	return true
}

// checkBusy returns a non-empty message and true on the first activity condition that
// blocks engine switching. Checks run sequentially and short-circuit; SQLite serialises
// reads anyway, so concurrent checks would not improve latency.
func (h *EngineCommandHandler) checkBusy(ctx context.Context) (string, bool) {
	m := i18n.M.Runtime.EngineCommand
	checks := []struct {
		fn   func(context.Context) (bool, error)
		busy string
		warn string
	}{
		{h.busy.HasActiveMessages, m.BusyMessages, "engine command: failed to check active messages"},
		{h.busy.HasActiveExecutions, m.BusyExecutions, "engine command: failed to check active executions"},
		{h.busy.HasActiveImmediateTasks, m.BusyTasks, "engine command: failed to check active immediate tasks"},
	}
	for _, c := range checks {
		active, err := c.fn(ctx)
		if err != nil {
			log.Warn(c.warn, zap.Error(err))
			continue
		}
		if active {
			return c.busy, true
		}
	}
	return "", false
}

func (h *EngineCommandHandler) handleBeeEngine(ctx context.Context, replyTo platform.InboundMessage, engineName string) {
	m := i18n.M.Runtime.EngineCommand
	if err := h.sysCfg.Set(ctx, model.SystemConfigKeyDefaultEngine, engineName); err != nil {
		h.reply(ctx, replyTo, m.SwitchFailed)
		return
	}
	h.engineCfg.Set(engineName)
	h.reply(ctx, replyTo, fmt.Sprintf(m.DefaultSwitched, engineName))
}

func (h *EngineCommandHandler) handleWorkerEngine(ctx context.Context, replyTo platform.InboundMessage, engineName, workerName string) {
	m := i18n.M.Runtime.EngineCommand
	w, err := h.workers.GetByName(workerName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerNotFound, workerName))
		} else {
			log.Error("get worker by name for /engine", zap.String("name", workerName), zap.Error(err))
			h.reply(ctx, replyTo, m.SwitchFailed)
		}
		return
	}
	w.Engine = engineName
	if _, err := h.workers.Update(w); err != nil {
		h.reply(ctx, replyTo, m.SwitchFailed)
		return
	}
	h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerSwitched, workerName, engineName))
}

func (h *EngineCommandHandler) isValidEngine(ctx context.Context, replyTo platform.InboundMessage, engineName string) bool {
	if err := h.validator.ValidateEngine(engineName); err != nil {
		h.reply(ctx, replyTo, fmt.Sprintf(i18n.M.Runtime.EngineCommand.UnknownEngine,
			engineName, strings.Join(h.validator.EnabledEngines(), " / ")))
		return false
	}
	return true
}

func (h *EngineCommandHandler) reply(ctx context.Context, replyTo platform.InboundMessage, text string) {
	sendReply(ctx, h.senders, replyTo, text)
}

func sendReply(ctx context.Context, senders map[string]platform.PlatformSenderAdapter, replyTo platform.InboundMessage, text string) {
	sender, ok := senders[replyTo.Platform]
	if !ok {
		return
	}
	if err := sender.Send(ctx, platform.OutboundMessage{
		Content:    text,
		ReplyTo:    replyTo,
		SourceType: store.SourceTypeSystem,
	}); err != nil {
		log.Warn("command reply failed", zap.String("platform", replyTo.Platform), zap.Error(err))
	}
}
