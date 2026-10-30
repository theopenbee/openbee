package configcmd

import (
	"testing"

	"github.com/AlecAivazis/survey/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineStep_AsksOnlyTimeoutsAndInstallsSkills(t *testing.T) {
	var messages []string
	installs := 0
	origAsk, origInstall := askOne, installSkills
	t.Cleanup(func() { askOne, installSkills = origAsk, origInstall })
	askOne = func(p survey.Prompt, response any, _ ...survey.AskOpt) error {
		in, ok := p.(*survey.Input)
		require.True(t, ok, "unexpected prompt %T", p)
		messages = append(messages, in.Message)
		*(response.(*string)) = "9m"
		return nil
	}
	installSkills = func() { installs++ }

	vals := defaultValues()
	require.NoError(t, runEngineStep(&vals))

	assert.Equal(t, []string{"Bee timeout:", "Worker timeout:"}, messages)
	assert.Equal(t, 1, installs)
	assert.Equal(t, "9m", vals.EngineTimeoutBee)
	assert.Equal(t, "9m", vals.EngineTimeoutWorker)
}
