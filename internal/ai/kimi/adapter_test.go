package kimi_test

import (
	"os"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/kimi"
)

func TestAdapter_Prepare_NoOp(t *testing.T) {
	dir := t.TempDir()
	a := kimi.NewAdapter("echo", "http://localhost:9999", nil)

	if err := a.Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("Prepare must not create files, found: %v", entries)
	}
}

func TestAdapter_Prepare_BothRoles(t *testing.T) {
	a := kimi.NewAdapter("echo", "http://localhost:9999", nil)
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		if err := a.Prepare(dir, ai.PrepareOptions{Role: role}); err != nil {
			t.Errorf("Prepare(%s): %v", role, err)
		}
	}
}
