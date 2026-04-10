package main

import (
	"fmt"
	"os/exec"

	"github.com/AlecAivazis/survey/v2"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func configureCodexExecutable(vals *configValues) error {
	if codexPath, err := exec.LookPath("codex"); err == nil {
		fmt.Printf(i18n.M.Output.Config.CodexFound+"\n", codexPath)
		vals.CodexPath = codexPath
	} else {
		fmt.Println(i18n.M.Output.Config.CodexManualEntry)
		if err := promptCodexManualPath(vals); err != nil {
			return err
		}
	}

	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.CodexTimeout,
		Default: vals.CodexTimeout,
	}, &vals.CodexTimeout); err != nil {
		return handleSurveyErr(err)
	}

	return nil
}

func promptCodexManualPath(vals *configValues) error {
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.CodexPath,
		Default: vals.CodexPath,
	}, &vals.CodexPath, survey.WithValidator(executablePathValidator)); err != nil {
		return handleSurveyErr(err)
	}
	return nil
}
