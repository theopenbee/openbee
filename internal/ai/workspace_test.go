package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupWorkspace_Bee(t *testing.T) {
	dir := t.TempDir()
	if err := SetupWorkspace(dir, RoleBee, WorkspaceOptions{}); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
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
	if strings.Contains(content, "@.openbee.md") {
		t.Errorf("AGENTS.md must not contain @.openbee.md import line, got: %q", content)
	}
	if !strings.Contains(content, LoadInstruction) {
		t.Errorf("AGENTS.md missing mandatory load instruction, got: %q", content)
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
	opts := WorkspaceOptions{
		Name:        "my-worker",
		Description: "does things",
		Memory:      "remember X",
	}
	if err := SetupWorkspace(dir, RoleWorker, opts); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}

	agentsmd := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(agentsmd)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if strings.Contains(string(data), "@.openbee.md") {
		t.Errorf("AGENTS.md must not contain @.openbee.md import line, got: %q", string(data))
	}
	if !strings.Contains(string(data), LoadInstruction) {
		t.Errorf("AGENTS.md missing mandatory load instruction, got: %q", string(data))
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

func TestSetupWorkspace_UnknownRole(t *testing.T) {
	dir := t.TempDir()
	err := SetupWorkspace(dir, Role("unknown"), WorkspaceOptions{})
	if err == nil {
		t.Error("expected error for unknown role, got nil")
	}
}

func TestSetupWorkspace_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := SetupWorkspace(dir, RoleBee, WorkspaceOptions{}); err != nil {
		t.Fatalf("first SetupWorkspace: %v", err)
	}
	agentsmd := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsmd, []byte("custom content"), 0o644); err != nil {
		t.Fatalf("write custom content: %v", err)
	}
	if err := SetupWorkspace(dir, RoleBee, WorkspaceOptions{}); err != nil {
		t.Fatalf("second SetupWorkspace: %v", err)
	}
	data, _ := os.ReadFile(agentsmd)
	if string(data) != "custom content" {
		t.Errorf("SetupWorkspace overwrote existing AGENTS.md")
	}
}
