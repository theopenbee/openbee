package surveyhint

import (
	"strings"

	"github.com/AlecAivazis/survey/v2"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

var originalMultiSelectTemplate = survey.MultiSelectQuestionTemplate

const multiSelectHintOld = `[Use arrows to move, space to select,{{- if not .Config.RemoveSelectAll }} <right> to all,{{end}}{{- if not .Config.RemoveSelectNone }} <left> to none,{{end}} type to filter{{- if and .Help (not .ShowHelp)}}, {{ .Config.HelpInput }} for more help{{end}}]`

func Apply() {
	survey.MultiSelectQuestionTemplate = strings.Replace(originalMultiSelectTemplate, multiSelectHintOld, i18n.M.Prompt.MultiSelectHint, 1)
}
