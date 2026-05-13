package main

import (
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func configureCodexExecutable(vals *configValues) error {
	return configureEngineExecutable(
		ai.EngineCodex,
		i18n.M.Output.Config.CodexFound,
		i18n.M.Output.Config.CodexManualEntry,
		i18n.M.Prompt.CodexPath,
		&vals.CodexPath,
	)
}
