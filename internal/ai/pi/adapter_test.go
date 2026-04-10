package pi

import (
	"os"
	"path/filepath"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestAdapter_SetupWorkspace_Bee(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("pi", "http://localhost:8080", nil)
	if err := a.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openbee.md")); !os.IsNotExist(err) {
		t.Errorf(".openbee.md must NOT be created by pi engine")
	}
}

func TestAdapter_SetupWorkspace_Worker(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("pi", "http://localhost:8080", nil)
	opts := ai.WorkspaceOptions{Name: "w1", Description: "desc", Memory: "mem"}
	if err := a.SetupWorkspace(dir, ai.RoleWorker, opts); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openbee.md")); !os.IsNotExist(err) {
		t.Errorf(".openbee.md must NOT be created by pi engine")
	}
}

func TestAdapter_SetupWorkspace_UnknownRole(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("pi", "http://localhost:8080", nil)
	err := a.SetupWorkspace(dir, ai.Role("unknown"), ai.WorkspaceOptions{})
	if err == nil {
		t.Error("expected error for unknown role, got nil")
	}
}

func TestAdapter_SetupWorkspace_Idempotent(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("pi", "http://localhost:8080", nil)
	if err := a.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := a.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("second call: %v", err)
	}
}
