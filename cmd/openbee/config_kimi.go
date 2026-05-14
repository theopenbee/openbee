package main

import (
	"github.com/theopenbee/openbee/internal/bridge"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func configureKimiExecutable(vals *configValues) error {
	return configureEngineExecutable(
		bridge.EngineKimi,
		i18n.M.Output.Config.KimiFound,
		i18n.M.Output.Config.KimiManualEntry,
		i18n.M.Prompt.KimiPath,
		&vals.KimiPath,
	)
}
