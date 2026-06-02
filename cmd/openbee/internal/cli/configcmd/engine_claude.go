package configcmd

import (
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func configureClaudeExecutable(vals *configValues) error {
	return configureEngineExecutable(
		ai.EngineClaude,
		i18n.M.Output.Config.ClaudeFound,
		i18n.M.Output.Config.ClaudeManualEntry,
		i18n.M.Prompt.ClaudePath,
		&vals.ClaudePath,
	)
}
