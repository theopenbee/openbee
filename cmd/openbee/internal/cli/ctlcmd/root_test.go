package ctlcmd

import "testing"

func TestNewCommandIncludesCtlSubcommands(t *testing.T) {
	cmd := NewCommand()
	want := []string{"worker", "task", "constraint", "session", "system", "message", "department"}
	for _, name := range want {
		if _, _, err := cmd.Find([]string{name}); err != nil {
			t.Fatalf("expected ctl subcommand %q: %v", name, err)
		}
	}
}
