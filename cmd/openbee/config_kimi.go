package main

import (
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func configureKimiExecutable(vals *configValues) error {
	return configureEngineExecutable(
		ai.EngineKimi,
		i18n.M.Output.Config.KimiFound,
		i18n.M.Output.Config.KimiManualEntry,
		i18n.M.Prompt.KimiPath,
		&vals.KimiPath,
	)
}
