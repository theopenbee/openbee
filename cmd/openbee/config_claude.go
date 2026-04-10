package main

import (
	"fmt"
	"os/exec"

	"github.com/AlecAivazis/survey/v2"
	claude "github.com/theopenbee/openbee/internal/ai/claude"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// configureClaudeExecutable handles Step 2a:
// 1. Auto-detect claude in PATH
// 2. If not found: manual input or download
// 3. Prompt for timeout
func configureClaudeExecutable(vals *configValues) error {
	if claudePath, err := exec.LookPath("claude"); err == nil {
		fmt.Printf(i18n.M.Output.Config.ClaudeFound+"\n", claudePath)
		vals.ClaudePath = claudePath
	} else {
		var method string
		if err := survey.AskOne(&survey.Select{
			Message: i18n.M.Prompt.ClaudeNotFound,
			Options: []string{i18n.M.Prompt.OptionEnterPathManually, i18n.M.Prompt.OptionDownloadClaude},
		}, &method); err != nil {
			return handleSurveyErr(err)
		}

		switch method {
		case i18n.M.Prompt.OptionEnterPathManually:
			if err := promptClaudeManualPath(vals); err != nil {
				return err
			}
		case i18n.M.Prompt.OptionDownloadClaude:
			path, err := claude.Download(openbeeStateDir(), false, "")
			if err != nil {
				fmt.Printf(i18n.M.Output.Config.ClaudeDownloadFailed+"\n", err)
				fmt.Println(i18n.M.Output.Config.ClaudeManualEntry)
				if err := promptClaudeManualPath(vals); err != nil {
					return err
				}
			} else {
				vals.ClaudePath = path
			}
		}
	}

	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.ClaudeTimeout,
		Default: vals.ClaudeTimeout,
	}, &vals.ClaudeTimeout); err != nil {
		return handleSurveyErr(err)
	}

	return nil
}

func promptClaudeManualPath(vals *configValues) error {
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.ClaudePath,
		Default: vals.ClaudePath,
	}, &vals.ClaudePath, survey.WithValidator(executablePathValidator)); err != nil {
		return handleSurveyErr(err)
	}
	return nil
}

func configureClaudeProvider(_ *configValues) error {
	if err := claude.ConfigureProvider(); err != nil {
		return err
	}
	return nil
}
