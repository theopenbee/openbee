package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

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

const usageMsg = "用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定 worker 的 engine"

// HandleCommand implements msgingest.CommandHandler.
// Returns true if content is a /engine command (whether or not it succeeded).
func (h *EngineCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != "/engine" {
		return false
	}

	switch len(fields) {
	case 1:
		h.reply(ctx, replyTo, usageMsg)
	case 2:
		h.handleBeeEngine(ctx, replyTo, fields[1])
	case 3:
		h.handleWorkerEngine(ctx, replyTo, fields[1], fields[2])
	default:
		h.reply(ctx, replyTo, usageMsg)
	}
	return true
}

func (h *EngineCommandHandler) handleBeeEngine(ctx context.Context, replyTo platform.InboundMessage, engineName string) {
	if !h.isValidEngine(ctx, replyTo, engineName) {
		return
	}
	if err := h.sysCfg.Set(ctx, model.SystemConfigKeyDefaultEngine, engineName); err != nil {
		h.reply(ctx, replyTo, "切换失败，请稍后重试")
		return
	}
	enginecfg.Set(engineName)
	h.reply(ctx, replyTo, fmt.Sprintf("已将默认 engine 切换为 %s", engineName))
}

func (h *EngineCommandHandler) handleWorkerEngine(ctx context.Context, replyTo platform.InboundMessage, engineName, workerName string) {
	if !h.isValidEngine(ctx, replyTo, engineName) {
		return
	}
	w, err := h.workers.GetByName(workerName)
	if err != nil {
		h.reply(ctx, replyTo, fmt.Sprintf("Worker %q 不存在", workerName))
		return
	}
	w.Engine = engineName
	if _, err := h.workers.Update(w); err != nil {
		h.reply(ctx, replyTo, "切换失败，请稍后重试")
		return
	}
	h.reply(ctx, replyTo, fmt.Sprintf("已将 Worker %q 的 engine 切换为 %s", workerName, engineName))
}

// isValidEngine validates engineName. On failure it sends an error reply and returns false.
func (h *EngineCommandHandler) isValidEngine(ctx context.Context, replyTo platform.InboundMessage, engineName string) bool {
	if err := h.validator.ValidateEngine(engineName); err != nil {
		h.reply(ctx, replyTo, fmt.Sprintf("未知的 engine: %s，支持的 engine：%s",
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
	_ = sender.Send(ctx, platform.OutboundMessage{
		Content:    text,
		ReplyTo:    replyTo,
		SourceType: "system",
	})
}
