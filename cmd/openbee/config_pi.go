package main

import (
	"github.com/theopenbee/openbee/internal/bridge"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func configurePiExecutable(vals *configValues) error {
	return configureEngineExecutable(
		bridge.EnginePi,
		i18n.M.Output.Config.PiFound,
		i18n.M.Output.Config.PiManualEntry,
		i18n.M.Prompt.PiPath,
		&vals.PiPath,
	)
}
