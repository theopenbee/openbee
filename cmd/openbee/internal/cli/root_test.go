package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRootCommandIncludesTopLevelCommands(t *testing.T) {
	cmd := NewRoot(BuildInfo{Version: "test", Commit: "abc", Date: "2026-06-01"})
	want := []string{"config", "server", "stop", "restart", "status", "backup", "restore", "upgrade", "ctl"}
	for _, name := range want {
		_, _, err := cmd.Find([]string{name})
		require.NoError(t, err, "expected top-level command %q", name)
	}
}

func TestNewRootCommandVersionTemplate(t *testing.T) {
	cmd := NewRoot(BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-06-01T00:00:00Z"})
	require.Equal(t, "1.2.3", cmd.Version)
}
