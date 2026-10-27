package configcmd

import (
	ai "github.com/theopenbee/openbee/internal/ai"
)

func configurePiExecutable(vals *configValues) error {
	return configureEngineExecutable(
		ai.EnginePi,
		"Found Pi in PATH: %s, using it automatically.",
		"Pi not found in PATH. Please enter the path manually.",
		"Pi executable path:",
		&vals.PiPath,
	)
}
