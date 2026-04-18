package command

import (
	"context"
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

// EngineCommandHandler handles the /engine slash command.
type EngineCommandHandler struct {
	workers   WorkerRepository
	sysCfg    SystemConfigWriter
	validator EngineValidator
	senders   map[string]platform.PlatformSenderAdapter
}

// NewEngineCommandHandler constructs an EngineCommandHandler.
func NewEngineCommandHandler(
	workers WorkerRepository,
	sysCfg SystemConfigWriter,
	senders map[string]platform.PlatformSenderAdapter,
	validator EngineValidator,
) *EngineCommandHandler {
	return &EngineCommandHandler{workers: workers, sysCfg: sysCfg, validator: validator, senders: senders}
}

// HandleCommand implements msgingest.CommandHandler.
// Returns true if content is a /engine command (whether or not it succeeded).
func (h *EngineCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != "/engine" {
		return false
	}

	switch len(fields) {
	case 1:
		h.reply(ctx, replyTo, i18n.M.Runtime.EngineCommand.Usage)
	case 2:
		h.handleBeeEngine(ctx, replyTo, fields[1])
	case 3:
		h.handleWorkerEngine(ctx, replyTo, fields[1], fields[2])
	default:
		h.reply(ctx, replyTo, i18n.M.Runtime.EngineCommand.Usage)
	}
	return true
}

func (h *EngineCommandHandler) handleBeeEngine(ctx context.Context, replyTo platform.InboundMessage, engineName string) {
	if !h.isValidEngine(ctx, replyTo, engineName) {
		return
	}
	m := i18n.M.Runtime.EngineCommand
	if err := h.sysCfg.Set(ctx, model.SystemConfigKeyDefaultEngine, engineName); err != nil {
		h.reply(ctx, replyTo, m.SwitchFailed)
		return
	}
	enginecfg.Set(engineName)
	h.reply(ctx, replyTo, fmt.Sprintf(m.DefaultSwitched, engineName))
}

func (h *EngineCommandHandler) handleWorkerEngine(ctx context.Context, replyTo platform.InboundMessage, engineName, workerName string) {
	if !h.isValidEngine(ctx, replyTo, engineName) {
		return
	}
	m := i18n.M.Runtime.EngineCommand
	w, err := h.workers.GetByName(workerName)
	if err != nil {
		h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerNotFound, workerName))
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
	sender, ok := h.senders[replyTo.Platform]
	if !ok {
		return
	}
	if err := sender.Send(ctx, platform.OutboundMessage{
		Content:    text,
		ReplyTo:    replyTo,
		SourceType: store.SourceTypeSystem,
	}); err != nil {
		log.Warn("engine command reply failed", zap.String("platform", replyTo.Platform), zap.Error(err))
	}
}
