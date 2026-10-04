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
	assert.NotEmpty(t, i18n.M.Cmd.Root.Short)
	assert.NotEmpty(t, i18n.M.Prompt.ServerPort)
}

func TestLoad_en(t *testing.T) {
	require.NoError(t, i18n.Load("en"))
	assert.Equal(t, "OpenBee core service", i18n.M.Cmd.Root.Short)
	assert.Equal(t, "Server port:", i18n.M.Prompt.ServerPort)
	got := i18n.M.Cmd.CtlWorker.Sub("list")
	assert.Equal(t, "List all workers", got)
	assert.Equal(t, "Manage departments", i18n.M.Cmd.CtlDepartment.Short)
}

func TestLoad_unsupported_fallbacks_to_zh(t *testing.T) {
	// load zh as baseline
	require.NoError(t, i18n.Load("zh"))
	zhShort := i18n.M.Cmd.Root.Short

	// loading an unsupported language should fall back to zh
	require.NoError(t, i18n.Load("fr"))
	assert.Equal(t, zhShort, i18n.M.Cmd.Root.Short, "fallback to zh")
}

func TestSupportedLangs(t *testing.T) {
	assert.GreaterOrEqual(t, len(i18n.SupportedLangs), 2)
}
