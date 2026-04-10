package main

import (
	"fmt"
	"os/exec"

	"github.com/AlecAivazis/survey/v2"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func configurePiExecutable(vals *configValues) error {
	if piPath, err := exec.LookPath("pi"); err == nil {
		fmt.Printf(i18n.M.Output.Config.PiFound+"\n", piPath)
		vals.PiPath = piPath
	} else {
		fmt.Println(i18n.M.Output.Config.PiManualEntry)
		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.PiPath,
			Default: vals.PiPath,
		}, &vals.PiPath, survey.WithValidator(executablePathValidator)); err != nil {
			return handleSurveyErr(err)
		}
	}

	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.PiTimeout,
		Default: vals.PiTimeout,
	}, &vals.PiTimeout); err != nil {
		return handleSurveyErr(err)
	}

	return nil
}
