package configcmd

import (
	ai "github.com/theopenbee/openbee/internal/ai"
)

func configureClaudeExecutable(vals *configValues) error {
	return configureEngineExecutable(
		ai.EngineClaude,
		"Found Claude in PATH: %s, using it automatically.",
		"Please enter the Claude path manually.",
		"Agent executable path:",
		&vals.ClaudePath,
	)
}
