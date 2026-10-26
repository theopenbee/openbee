package surveyhint

import (
	"testing"

	"github.com/AlecAivazis/survey/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func TestApplyUsesLoadedLanguage(t *testing.T) {
	require.NoError(t, i18n.Load("zh"))
	Apply()
	assert.Contains(t, survey.MultiSelectQuestionTemplate, i18n.M.Prompt.MultiSelectHint)
	assert.NotContains(t, survey.MultiSelectQuestionTemplate, "space to select")
}
