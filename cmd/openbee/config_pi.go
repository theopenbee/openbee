package main

import "github.com/theopenbee/openbee/internal/infra/i18n"

func configurePiExecutable(vals *configValues) error {
	return configureEngineExecutable(
		"pi",
		i18n.M.Output.Config.PiFound,
		i18n.M.Output.Config.PiManualEntry,
		i18n.M.Prompt.PiPath,
		i18n.M.Prompt.PiTimeout,
		&vals.PiPath,
		&vals.PiTimeout,
	)
}
