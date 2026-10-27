package configcmd

import (
	ai "github.com/theopenbee/openbee/internal/ai"
)

func configureCodexExecutable(vals *configValues) error {
	return configureEngineExecutable(
		ai.EngineCodex,
		"Found Codex in PATH: %s, using it automatically.",
		"Codex not found in PATH. Please enter the path manually.",
		"Codex executable path:",
		&vals.CodexPath,
	)
}
