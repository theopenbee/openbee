package i18n_test

import (
	"testing"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func TestLoad_zh(t *testing.T) {
	if err := i18n.Load("zh"); err != nil {
		t.Fatalf("Load zh: %v", err)
	}
	if i18n.M == nil {
		t.Fatal("M is nil after Load")
	}
	if i18n.M.Cmd.Root.Short == "" {
		t.Error("Cmd.Root.Short is empty for zh")
	}
	if i18n.M.Prompt.ServerPort == "" {
		t.Error("Prompt.ServerPort is empty for zh")
	}
}

func TestLoad_en(t *testing.T) {
	if err := i18n.Load("en"); err != nil {
		t.Fatalf("Load en: %v", err)
	}
	if i18n.M.Cmd.Root.Short != "OpenBee core service" {
		t.Errorf("Cmd.Root.Short: got %q, want %q", i18n.M.Cmd.Root.Short, "OpenBee core service")
	}
	if i18n.M.Prompt.ServerPort != "Server port:" {
		t.Errorf("Prompt.ServerPort: got %q, want %q", i18n.M.Prompt.ServerPort, "Server port:")
	}
	if got := i18n.M.Cmd.CtlWorker.Sub("list"); got != "List all workers" {
		t.Errorf("CtlWorker.Sub(list): got %q, want %q", got, "List all workers")
	}
	if i18n.M.Cmd.CtlDepartment.Short != "Manage departments" {
		t.Errorf("CtlDepartment.Short: got %q, want %q", i18n.M.Cmd.CtlDepartment.Short, "Manage departments")
	}
}

func TestLoad_unsupported_fallbacks_to_zh(t *testing.T) {
	// load zh as baseline
	if err := i18n.Load("zh"); err != nil {
		t.Fatalf("Load zh: %v", err)
	}
	zhShort := i18n.M.Cmd.Root.Short

	// loading an unsupported language should fall back to zh
	if err := i18n.Load("fr"); err != nil {
		t.Fatalf("Load fr (fallback): %v", err)
	}
	if i18n.M.Cmd.Root.Short != zhShort {
		t.Errorf("fallback: got %q, want zh value %q", i18n.M.Cmd.Root.Short, zhShort)
	}
}

func TestSupportedLangs(t *testing.T) {
	if len(i18n.SupportedLangs) < 2 {
		t.Errorf("SupportedLangs: expected at least 2, got %d", len(i18n.SupportedLangs))
	}
}
