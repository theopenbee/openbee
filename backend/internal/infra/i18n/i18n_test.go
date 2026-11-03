package i18n_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func TestLoad_zh(t *testing.T) {
	require.NoError(t, i18n.Load("zh"))
	require.NotNil(t, i18n.M)
	assert.NotEmpty(t, i18n.M.Runtime.StatusCommand.Usage)
	assert.NotEmpty(t, i18n.M.Runtime.FailureNotifier.TaskFailed)
}

func TestLoad_en(t *testing.T) {
	require.NoError(t, i18n.Load("en"))
	assert.NotEmpty(t, i18n.M.Runtime.StatusCommand.Usage)
	assert.NotEmpty(t, i18n.M.Runtime.FailureNotifier.TaskFailed)
}

func TestLoad_unsupportedFallsBackToEn(t *testing.T) {
	require.NoError(t, i18n.Load("en"))
	enUsage := i18n.M.Runtime.StatusCommand.Usage

	require.NoError(t, i18n.Load("zh"))
	require.NotEqual(t, enUsage, i18n.M.Runtime.StatusCommand.Usage)

	require.NoError(t, i18n.Load("fr"))
	assert.Equal(t, enUsage, i18n.M.Runtime.StatusCommand.Usage)

	require.NoError(t, i18n.Load(""))
	assert.Equal(t, enUsage, i18n.M.Runtime.StatusCommand.Usage)
}
