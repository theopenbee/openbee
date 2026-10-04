package pi_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/pi"
)

func TestAdapter_Prepare_NoOp(t *testing.T) {
	dir := t.TempDir()
	a, err := pi.NewAdapter("echo", nil)
	require.NoError(t, err)

	require.NoError(t, a.Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}))
	entries, _ := os.ReadDir(dir)
	assert.Empty(t, entries, "Prepare must not create files")
}

func TestAdapter_Prepare_BothRoles(t *testing.T) {
	a, err := pi.NewAdapter("echo", nil)
	require.NoError(t, err)
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		err := a.Prepare(dir, ai.PrepareOptions{Role: role})
		assert.NoError(t, err, "Prepare(%s)", role)
	}
}
