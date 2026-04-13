package main

import "github.com/theopenbee/openbee/internal/infra/i18n"

func configureCodexExecutable(vals *configValues) error {
	return configureEngineExecutable(
		"codex",
		i18n.M.Output.Config.CodexFound,
		i18n.M.Output.Config.CodexManualEntry,
		i18n.M.Prompt.CodexPath,
		i18n.M.Prompt.CodexTimeout,
		&vals.CodexPath,
		&vals.CodexTimeout,
	)
}
