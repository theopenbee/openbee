package configcmd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// originalMultiSelectTemplate holds the unmodified survey template so we can
// replace the hint text cleanly after each language selection.
var originalMultiSelectTemplate = survey.MultiSelectQuestionTemplate

// multiSelectHintOld is the exact hint fragment to replace inside the template.
const multiSelectHintOld = `[Use arrows to move, space to select,{{- if not .Config.RemoveSelectAll }} <right> to all,{{end}}{{- if not .Config.RemoveSelectNone }} <left> to none,{{end}} type to filter{{- if and .Help (not .ShowHelp)}}, {{ .Config.HelpInput }} for more help{{end}}]`

// applySurveyTemplates patches the survey MultiSelect hint text to use the
// currently loaded i18n locale. Must be called after i18n.Load().
func applySurveyTemplates() {
	survey.MultiSelectQuestionTemplate = strings.Replace(
		originalMultiSelectTemplate,
		multiSelectHintOld,
		i18n.M.Prompt.MultiSelectHint,
		1,
	)
}

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
	applySurveyTemplates()
	return lang, nil
}

func promptPassword(vals *configValues) error {
	var method string
	if err := survey.AskOne(&survey.Select{
		Message: i18n.M.Prompt.PasswordSetup,
		Options: []string{i18n.M.Prompt.OptionEnterManually, i18n.M.Prompt.OptionGenerateRandom},
	}, &method); err != nil {
		return handleSurveyErr(err)
	}
	switch method {
	case i18n.M.Prompt.OptionEnterManually:
		if err := survey.AskOne(&survey.Password{
			Message: i18n.M.Prompt.Password,
		}, &vals.AuthPassword, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
	case i18n.M.Prompt.OptionGenerateRandom:
		vals.AuthPassword = randomHex(16)
		fmt.Printf(i18n.M.Output.Config.PasswordGenerated+"\n", vals.AuthPassword)
	}
	return nil
}

// randomHex returns a hex-encoded random string with n bytes of entropy.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func promptBotName(fieldPtr *string) error {
	return handleSurveyErr(survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.BotName,
		Default: *fieldPtr,
	}, fieldPtr, survey.WithValidator(survey.Required)))
}

func handleSurveyErr(err error) error {
	if errors.Is(err, terminal.InterruptErr) {
		fmt.Println(i18n.M.Prompt.Cancelled)
		return errInterrupted
	}
	return err
}

// renderInlineYAMLList formats a comma-separated string into an inline YAML
// array body, e.g. `"a", "b"`. Empty input returns "".
func renderInlineYAMLList(csv string) string {
	parts := utils.SplitAndTrim(csv)
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = fmt.Sprintf("%q", p)
	}
	return strings.Join(out, ", ")
}
