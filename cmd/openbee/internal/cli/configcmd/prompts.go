package configcmd

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
)

const (
	langEN = "en"
	langZH = "zh"

	langOptEnglish = "English"
	langOptChinese = "Chinese"
)

func runLanguageStep(existingLang string) (string, error) {
	defaultOpt := langOptEnglish
	if existingLang == langZH {
		defaultOpt = langOptChinese
	}

	var selected string
	if err := survey.AskOne(&survey.Select{
		Message: "Language for the web console and bot replies:",
		Options: []string{langOptEnglish, langOptChinese},
		Default: defaultOpt,
	}, &selected); err != nil {
		return "", handleSurveyErr(err)
	}

	if selected == langOptChinese {
		return langZH, nil
	}
	return langEN, nil
}

func handleSurveyErr(err error) error {
	if errors.Is(err, terminal.InterruptErr) {
		fmt.Println("\nCancelled.")
		return errInterrupted
	}
	return err
}
