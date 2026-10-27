package configcmd

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/surveyhint"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

const (
	langOptEnglish = "English"
	langOptChinese = "Chinese"
)

// runLanguageStep shows a bilingual language-selection prompt and reloads i18n
// with the chosen language. existingLang should be "" or a previously saved
// language code (i18n.LangEN or i18n.LangZH); it determines the default selection.
func runLanguageStep(existingLang string) (string, error) {
	defaultOpt := langOptEnglish
	if existingLang == i18n.LangZH {
		defaultOpt = langOptChinese
	}

	var selected string
	if err := survey.AskOne(&survey.Select{
		Message: "Select language",
		Options: []string{langOptEnglish, langOptChinese},
		Default: defaultOpt,
	}, &selected); err != nil {
		return "", handleSurveyErr(err)
	}

	lang := i18n.LangEN
	if selected == langOptChinese {
		lang = i18n.LangZH
	}

	if err := i18n.Load(lang); err != nil {
		return "", fmt.Errorf("load i18n: %w", err)
	}
	surveyhint.Apply()
	return lang, nil
}

func handleSurveyErr(err error) error {
	if errors.Is(err, terminal.InterruptErr) {
		fmt.Println(i18n.M.Prompt.Cancelled)
		return errInterrupted
	}
	return err
}
