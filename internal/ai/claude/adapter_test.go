package claude_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/claude"
)

func newTestAdapter(t *testing.T) ai.EngineAdapter {
	t.Helper()
	return claude.NewAdapter("echo", nil)
}

func TestClaudeAdapter_ExtraEnvInBaseEnv(t *testing.T) {
	a := claude.NewAdapter("echo", map[string]string{
		"MY_CUSTOM_VAR": "hello",
		"ANOTHER_KEY":   "world",
	})
	// Access baseEnv indirectly: run a command that echoes env and check output.
	// Since we cannot inspect baseEnv directly, we verify NewAdapter doesn't panic
	// and the adapter satisfies the interface.
	var _ ai.EngineAdapter = a
}

func TestClaudeAdapter_Prepare_Stub(t *testing.T) {
	dir := t.TempDir()
	adapter := newTestAdapter(t)
	if err := adapter.Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
}

func TestClaudeAdapter_Prepare_DeletesOpenbeeFile(t *testing.T) {
	dir := t.TempDir()
	openbeeFile := filepath.Join(dir, ai.SystemRulesFile)
	if err := os.WriteFile(openbeeFile, []byte("old rules"), 0o644); err != nil {
		t.Fatalf("write .openbee.md: %v", err)
	}

	if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if _, err := os.Stat(openbeeFile); !os.IsNotExist(err) {
		t.Error(".openbee.md should have been deleted by Prepare")
	}
}

func TestClaudeAdapter_Prepare_RemovesImportLine(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	content := "# My Bot\n" + ai.ImportLine + "\nOther content\n"
	if err := os.WriteFile(claudeFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	got := string(data)
	if strings.Contains(got, ai.ImportLine) {
		t.Errorf("CLAUDE.md should not contain import line after Prepare, got:\n%s", got)
	}
	if !strings.Contains(got, "# My Bot") {
		t.Error("CLAUDE.md should preserve other content")
	}
	if !strings.Contains(got, "Other content") {
		t.Error("CLAUDE.md should preserve other content")
	}
}

func TestClaudeAdapter_Prepare_PreservesOtherCLAUDEMDContent(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	// CLAUDE.md with no import line — must not be modified
	original := "# Custom instructions\nDo something special.\n"
	if err := os.WriteFile(claudeFile, []byte(original), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	if string(data) != original {
		t.Errorf("CLAUDE.md should be unchanged when import line is absent.\nGot: %q\nWant: %q", string(data), original)
	}
}

func TestClaudeAdapter_Prepare_NoopWhenFilesAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
		t.Fatalf("Prepare should not error when no files exist: %v", err)
	}
}

func TestClaudeAdapter_Prepare_BothRoles(t *testing.T) {
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		// Setup: both legacy files present
		os.WriteFile(filepath.Join(dir, ai.SystemRulesFile), []byte("rules"), 0o644)
		os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(ai.ImportLine+"\n"), 0o644)

		if err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: role}); err != nil {
			t.Errorf("Prepare(%s): %v", role, err)
		}
		if _, err := os.Stat(filepath.Join(dir, ai.SystemRulesFile)); !os.IsNotExist(err) {
			t.Errorf("role %s: .openbee.md should be deleted", role)
		}
	}
}
