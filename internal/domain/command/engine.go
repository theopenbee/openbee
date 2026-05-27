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
	// CmdStop is the slash command that stops the running bee and cancels pending messages.
	CmdStop = "/stop"
	// CmdStatus is the slash command that prints the current session status.
	CmdStatus = "/status"
)

var log = logger.With(zap.String("component", "command"))

// WorkerRepository is the subset of WorkerStore needed by EngineCommandHandler.
type WorkerRepository interface {
	GetByName(name string) (model.Worker, error)
	UpdateEngine(id, engine string) error
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

// BeeExecutionActivityChecker reports whether active bee-owned executions exist.
type BeeExecutionActivityChecker interface {
	HasActiveBeeExecutions(ctx context.Context) (bool, error)
}

// WorkerExecutionActivityChecker reports whether active executions exist for a specific worker.
type WorkerExecutionActivityChecker interface {
	HasActiveExecutionsByWorkerID(ctx context.Context, workerID string) (bool, error)
}

// WorkerTaskActivityChecker reports whether active immediate tasks exist for a specific worker.
type WorkerTaskActivityChecker interface {
	HasActiveImmediateTasksByWorkerID(ctx context.Context, workerID string) (bool, error)
}

// BeeBusyChecker gates bee-level engine switches.
type BeeBusyChecker interface {
	MessageActivityChecker
	BeeExecutionActivityChecker
}

// WorkerBusyChecker gates worker-level engine switches.
type WorkerBusyChecker interface {
	WorkerExecutionActivityChecker
	WorkerTaskActivityChecker
}

type compositeBeeBusyChecker struct {
	MessageActivityChecker
	BeeExecutionActivityChecker
}

type compositeWorkerBusyChecker struct {
	WorkerExecutionActivityChecker
	WorkerTaskActivityChecker
}

func NewBeeBusyChecker(msg MessageActivityChecker, exec BeeExecutionActivityChecker) BeeBusyChecker {
	return compositeBeeBusyChecker{msg, exec}
}

func NewWorkerBusyChecker(exec WorkerExecutionActivityChecker, task WorkerTaskActivityChecker) WorkerBusyChecker {
	return compositeWorkerBusyChecker{exec, task}
}

// EngineCommandHandler handles the /engine slash command.
type EngineCommandHandler struct {
	workers    WorkerRepository
	sysCfg     SystemConfigWriter
	validator  EngineValidator
	senders    map[string]platform.PlatformSenderAdapter
	beeBusy    BeeBusyChecker
	workerBusy WorkerBusyChecker
	engineCfg  *enginecfg.Store
}

func isExactOrPrefixed(content, cmd string) bool {
	return content == cmd || strings.HasPrefix(content, cmd+" ")
}

func NewEngineCommandHandler(
	workers WorkerRepository,
	sysCfg SystemConfigWriter,
	senders map[string]platform.PlatformSenderAdapter,
	validator EngineValidator,
	beeBusy BeeBusyChecker,
	workerBusy WorkerBusyChecker,
	engineCfg *enginecfg.Store,
) *EngineCommandHandler {
	return &EngineCommandHandler{
		workers:    workers,
		sysCfg:     sysCfg,
		validator:  validator,
		senders:    senders,
		beeBusy:    beeBusy,
		workerBusy: workerBusy,
		engineCfg:  engineCfg,
	}
}

func (h *EngineCommandHandler) IsCommand(content string) bool {
	return isExactOrPrefixed(content, CmdEngine)
}

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

	if len(fields) == 2 {
		if busyMsg, busy := h.checkBeeBusy(ctx); busy {
			h.reply(ctx, replyTo, busyMsg)
			return true
		}
		h.handleBeeEngine(ctx, replyTo, engineName)
	} else {
		workerName := fields[2]
		m := i18n.M.Runtime.EngineCommand
		w, err := h.workers.GetByName(workerName)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerNotFound, workerName))
			} else {
				log.Error("get worker by name for /engine", zap.String("name", workerName), zap.Error(err))
				h.reply(ctx, replyTo, m.SwitchFailed)
			}
			return true
		}
		if busyMsg, busy := h.checkWorkerBusy(ctx, w.ID); busy {
			h.reply(ctx, replyTo, busyMsg)
			return true
		}
		h.handleWorkerEngine(ctx, replyTo, engineName, w)
	}
	return true
}

func (h *EngineCommandHandler) checkBeeBusy(ctx context.Context) (string, bool) {
	m := i18n.M.Runtime.EngineCommand
	if active, err := h.beeBusy.HasActiveMessages(ctx); err != nil {
		log.Warn("engine command: failed to check active messages", zap.Error(err))
	} else if active {
		return m.BusyMessages, true
	}
	if active, err := h.beeBusy.HasActiveBeeExecutions(ctx); err != nil {
		log.Warn("engine command: failed to check active bee executions", zap.Error(err))
	} else if active {
		return m.BusyExecutions, true
	}
	return "", false
}

func (h *EngineCommandHandler) checkWorkerBusy(ctx context.Context, workerID string) (string, bool) {
	m := i18n.M.Runtime.EngineCommand
	if active, err := h.workerBusy.HasActiveExecutionsByWorkerID(ctx, workerID); err != nil {
		log.Warn("engine command: failed to check active worker executions", zap.Error(err))
	} else if active {
		return m.BusyExecutions, true
	}
	if active, err := h.workerBusy.HasActiveImmediateTasksByWorkerID(ctx, workerID); err != nil {
		log.Warn("engine command: failed to check active worker tasks", zap.Error(err))
	} else if active {
		return m.BusyTasks, true
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

func (h *EngineCommandHandler) handleWorkerEngine(ctx context.Context, replyTo platform.InboundMessage, engineName string, w model.Worker) {
	m := i18n.M.Runtime.EngineCommand
	if err := h.workers.UpdateEngine(w.ID, engineName); err != nil {
		h.reply(ctx, replyTo, m.SwitchFailed)
		return
	}
	h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerSwitched, w.Name, engineName))
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
	key := platform.AccountKey(replyTo.Platform, replyTo.AccountName)
	sender, ok := senders[key]
	if !ok {
		// Tolerate legacy callers/tests that register by platform-only key.
		sender, ok = senders[replyTo.Platform]
		if !ok {
			return
		}
	}
	if err := sender.Send(ctx, platform.OutboundMessage{
		Content:     text,
		AccountName: replyTo.AccountName,
		ReplyTo:     replyTo,
		SourceType:  store.SourceTypeSystem,
	}); err != nil {
		log.Warn("command reply failed", zap.String("platform", replyTo.Platform), zap.String("account", replyTo.AccountName), zap.Error(err))
	}
}
