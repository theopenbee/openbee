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
	return claude.NewAdapter("echo", "http://localhost:9999")
}

func TestClaudeAdapter_SetupWorkspace_Worker(t *testing.T) {
	dir := t.TempDir()
	// pre-create CLAUDE.md so EnsureSystemRules can find it
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("@.openbee.md\n"), 0644)

	adapter := newTestAdapter(t)
	err := adapter.SetupWorkspace(dir, ai.RoleWorker, ai.WorkspaceOptions{
		Name:        "test-worker",
		Description: "runs tests",
		Memory:      "",
	})
	if err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, claude.SystemRulesFile))
	if err != nil {
		t.Fatalf("read system rules: %v", err)
	}
	if !strings.Contains(string(data), "test-worker") {
		t.Error("worker name not found in system rules")
	}
}

func TestClaudeAdapter_SetupWorkspace_Bee_CreatesCLAUDEMD(t *testing.T) {
	dir := t.TempDir()

	adapter := newTestAdapter(t)
	err := adapter.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{})
	if err != nil {
		t.Fatalf("SetupWorkspace bee: %v", err)
	}

	claudeMD := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
		t.Error("CLAUDE.md was not created for bee workspace")
	}

	data, err := os.ReadFile(filepath.Join(dir, claude.SystemRulesFile))
	if err != nil {
		t.Fatalf("read system rules: %v", err)
	}
	if !strings.Contains(string(data), "coordinator") {
		t.Error("bee role rules missing 'coordinator'")
	}
}

func TestClaudeAdapter_SetupWorkspace_Bee_DoesNotOverwriteCLAUDEMD(t *testing.T) {
	dir := t.TempDir()
	existing := "# my custom persona\n"
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(existing), 0644)

	adapter := newTestAdapter(t)
	adapter.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{})

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if string(data) != existing {
		t.Error("SetupWorkspace overwrote existing CLAUDE.md")
	}
}
