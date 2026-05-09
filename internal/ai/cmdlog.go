package ai

import (
	"github.com/theopenbee/openbee/internal/infra/logger"
	"go.uber.org/zap"
)

// LogCommand emits the full engine invocation (binary, args, work dir, stdin)
// at INFO so it is easy to inspect what each agent run actually saw. stdin may
// be "" when the prompt is passed via args instead.
func LogCommand(engine, binary, workDir string, args []string, stdin string) {
	logger.Info("engine command",
		zap.String("engine", engine),
		zap.String("binary", binary),
		zap.String("workDir", workDir),
		zap.Strings("args", args),
		zap.String("stdin", stdin),
	)
}
