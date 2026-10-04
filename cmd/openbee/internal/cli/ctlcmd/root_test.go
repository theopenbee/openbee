package ctlcmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCommandIncludesCtlSubcommands(t *testing.T) {
	cmd := NewCommand()
	want := []string{"worker", "task", "constraint", "session", "system", "message", "department"}
	for _, name := range want {
		_, _, err := cmd.Find([]string{name})
		require.NoError(t, err, "expected ctl subcommand %q", name)
	}
}
