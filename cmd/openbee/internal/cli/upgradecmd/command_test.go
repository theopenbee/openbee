package upgradecmd

import "testing"

func TestNewCommandFlags(t *testing.T) {
	cmd := NewCommand("v0.0.1")
	if cmd.Flags().Lookup("check") == nil {
		t.Error("expected --check flag")
	}
	for _, name := range []string{"cn", "cdn-url"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Errorf("flag --%s should be removed with the mainland China CDN channel", name)
		}
	}
}
