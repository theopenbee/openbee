package main

import (
	"github.com/theopenbee/openbee/internal/bridge"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func configureClaudeExecutable(vals *configValues) error {
	return configureEngineExecutable(
		bridge.EngineClaude,
		i18n.M.Output.Config.ClaudeFound,
		i18n.M.Output.Config.ClaudeManualEntry,
		i18n.M.Prompt.ClaudePath,
		&vals.ClaudePath,
	)
}
