package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestSetupWorkspace_Bee(t *testing.T) {
	dir := t.TempDir()
	if err := setupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("setupWorkspace: %v", err)
	}

	agentsmd := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(agentsmd)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "You are") {
		t.Errorf("AGENTS.md missing bee persona, got: %q", content)
	}
	if !strings.Contains(content, "@.openbee.md") {
		t.Errorf("AGENTS.md missing @.openbee.md import, got: %q", content)
	}

	rules := filepath.Join(dir, ".openbee.md")
	rulesData, err := os.ReadFile(rules)
	if err != nil {
		t.Fatalf("read .openbee.md: %v", err)
	}
	if !strings.Contains(string(rulesData), "openbee-bee") {
		t.Errorf(".openbee.md missing bee rules, got: %q", string(rulesData))
	}
}

func TestSetupWorkspace_Worker(t *testing.T) {
	dir := t.TempDir()
	opts := ai.WorkspaceOptions{
		Name:        "my-worker",
		Description: "does things",
		Memory:      "remember X",
	}
	if err := setupWorkspace(dir, ai.RoleWorker, opts); err != nil {
		t.Fatalf("setupWorkspace: %v", err)
	}

	agentsmd := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(agentsmd)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(data), "@.openbee.md") {
		t.Errorf("AGENTS.md missing @.openbee.md import, got: %q", string(data))
	}

	rules := filepath.Join(dir, ".openbee.md")
	rulesData, err := os.ReadFile(rules)
	if err != nil {
		t.Fatalf("read .openbee.md: %v", err)
	}
	content := string(rulesData)
	if !strings.Contains(content, "my-worker") {
		t.Errorf(".openbee.md missing name, got: %q", content)
	}
	if !strings.Contains(content, "does things") {
		t.Errorf(".openbee.md missing description, got: %q", content)
	}
	if !strings.Contains(content, "remember X") {
		t.Errorf(".openbee.md missing memory, got: %q", content)
	}
	if !strings.Contains(content, "openbee-worker") {
		t.Errorf(".openbee.md missing worker rules, got: %q", content)
	}
}

func TestSetupWorkspace_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := setupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("first setupWorkspace: %v", err)
	}
	agentsmd := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsmd, []byte("custom content"), 0o644); err != nil {
		t.Fatalf("write custom content: %v", err)
	}
	if err := setupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("second setupWorkspace: %v", err)
	}
	data, _ := os.ReadFile(agentsmd)
	if string(data) != "custom content" {
		t.Errorf("setupWorkspace overwrote existing AGENTS.md")
	}
}
