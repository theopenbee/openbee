package skillinstall

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallSkills_FirstInstall(t *testing.T) {
	dir := t.TempDir()
	results, err := InstallSkills(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Action != "installed" {
			t.Errorf("skill %s: expected action 'installed', got %q", r.Name, r.Action)
		}
		content, err := os.ReadFile(filepath.Join(dir, r.Name, "SKILL.md"))
		if err != nil {
			t.Errorf("skill %s: file not created: %v", r.Name, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("skill %s: file is empty", r.Name)
		}
	}
}

func TestInstallSkills_UpToDate(t *testing.T) {
	dir := t.TempDir()
	// First install
	if _, err := InstallSkills(dir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	// Second install — content unchanged
	results, err := InstallSkills(dir)
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	for _, r := range results {
		if r.Action != "up-to-date" {
			t.Errorf("skill %s: expected 'up-to-date', got %q", r.Name, r.Action)
		}
	}
}

func TestInstallSkills_Updated(t *testing.T) {
	dir := t.TempDir()
	// Write stale content for "bee"
	beeDir := filepath.Join(dir, "bee")
	if err := os.MkdirAll(beeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beeDir, "SKILL.md"), []byte("stale content"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := InstallSkills(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var beeResult SkillResult
	for _, r := range results {
		if r.Name == "bee" {
			beeResult = r
		}
	}
	if beeResult.Action != "updated" {
		t.Errorf("expected 'updated' for bee, got %q", beeResult.Action)
	}
	// Verify file now contains embedded content
	got, _ := os.ReadFile(filepath.Join(beeDir, "SKILL.md"))
	if string(got) != beeSkillMD {
		t.Error("bee SKILL.md content not updated to embedded content")
	}
	var workerResult SkillResult
	for _, r := range results {
		if r.Name == "worker" {
			workerResult = r
		}
	}
	if workerResult.Action != "installed" {
		t.Errorf("expected 'installed' for worker, got %q", workerResult.Action)
	}
}

func TestInstallSkills_WriteError(t *testing.T) {
	dir := t.TempDir()
	// Place a regular file at the path where the "bee" directory would be created,
	// so MkdirAll will fail with ENOTDIR.
	if err := os.WriteFile(filepath.Join(dir, "bee"), []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := InstallSkills(dir)
	if err == nil {
		t.Fatal("expected error when write is blocked, got nil")
	}
}
