package codex_test

import (
	"os"
	"path/filepath"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/codex"
)

func TestAdapter_Prepare_NoOp(t *testing.T) {
	dir := t.TempDir()
	a, err := codex.NewAdapter("echo", "http://localhost:9999", nil)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}

	if err := a.Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	// Prepare must not create any files
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("Prepare must not create files, found: %v", entries)
	}
	_ = filepath.Join(dir, "AGENTS.md") // Ensure path helpers compile
}

func TestAdapter_Prepare_BothRoles(t *testing.T) {
	a, err := codex.NewAdapter("echo", "http://localhost:9999", nil)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		if err := a.Prepare(dir, ai.PrepareOptions{Role: role}); err != nil {
			t.Errorf("Prepare(%s): %v", role, err)
		}
	}
}

func TestAdapter_ExtraEnvInBaseEnv(t *testing.T) {
	a, err := codex.NewAdapter("echo", "http://localhost:9999", map[string]string{
		"CODEX_CUSTOM": "value",
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	var _ ai.EngineAdapter = a
}
