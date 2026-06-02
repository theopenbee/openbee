package cli

import "testing"

func TestNewRootCommandIncludesTopLevelCommands(t *testing.T) {
	cmd := NewRoot(BuildInfo{Version: "test", Commit: "abc", Date: "2026-06-01"})
	want := []string{"server", "stop", "restart", "status", "backup", "restore", "upgrade", "ctl"}
	for _, name := range want {
		if _, _, err := cmd.Find([]string{name}); err != nil {
			t.Fatalf("expected top-level command %q: %v", name, err)
		}
	}
}

func TestNewRootCommandVersionTemplate(t *testing.T) {
	cmd := NewRoot(BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-06-01T00:00:00Z"})
	if got := cmd.Version; got != "1.2.3" {
		t.Fatalf("Version = %q, want %q", got, "1.2.3")
	}
}
