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
	if len(results) != len(embeddedSkills) {
		t.Fatalf("expected %d results, got %d", len(embeddedSkills), len(results))
	}
	for _, r := range results {
		if r.Action != ActionInstalled {
			t.Errorf("skill %s: expected action 'installed', got %q", r.Name, r.Action)
		}
	}

	// Verify all embedded files were installed for each skill.
	for _, name := range embeddedSkills {
		embedded, err := collectEmbeddedFiles("skills/" + name)
		if err != nil {
			t.Fatalf("skill %s: collect embedded files: %v", name, err)
		}
		for relPath, wantContent := range embedded {
			p := filepath.Join(dir, name, filepath.FromSlash(relPath))
			got, err := os.ReadFile(p)
			if err != nil {
				t.Errorf("skill %s: file %s not created: %v", name, relPath, err)
				continue
			}
			if string(got) != wantContent {
				t.Errorf("skill %s: file %s content mismatch", name, relPath)
			}
		}
	}
}

func TestInstallSkills_UpToDate(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallSkills(dir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	results, err := InstallSkills(dir)
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	for _, r := range results {
		if r.Action != ActionUpToDate {
			t.Errorf("skill %s: expected 'up-to-date', got %q", r.Name, r.Action)
		}
	}
}

func TestInstallSkills_UpdatedWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallSkills(dir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	// Delete a reference file from openbee-bee.
	missing := filepath.Join(dir, "openbee-bee", "references", "cli-reference.md")
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	results, err := InstallSkills(dir)
	if err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
	var beeResult SkillResult
	for _, r := range results {
		if r.Name == "openbee-bee" {
			beeResult = r
		}
	}
	if beeResult.Action != ActionUpdated {
		t.Errorf("expected 'updated' for openbee-bee when file missing, got %q", beeResult.Action)
	}
	if _, err := os.Stat(missing); err != nil {
		t.Errorf("missing file was not restored: %v", err)
	}
}

func TestInstallSkills_PrunesStaleFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallSkills(dir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	// Plant a stale file that is not in the embedded FS.
	stale := filepath.Join(dir, "openbee-worker", "references", "old-removed.md")
	if err := os.WriteFile(stale, []byte("stale content"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := InstallSkills(dir)
	if err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
	var workerResult SkillResult
	for _, r := range results {
		if r.Name == "openbee-worker" {
			workerResult = r
		}
	}
	if workerResult.Action != ActionUpdated {
		t.Errorf("expected 'updated' for openbee-worker when stale file present, got %q", workerResult.Action)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("stale file was not pruned")
	}
}

func TestInstallSkills_Updated(t *testing.T) {
	dir := t.TempDir()
	// Write stale SKILL.md for openbee-bee.
	beeDir := filepath.Join(dir, "openbee-bee")
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
		if r.Name == "openbee-bee" {
			beeResult = r
		}
	}
	if beeResult.Action != ActionUpdated {
		t.Errorf("expected 'updated' for openbee-bee, got %q", beeResult.Action)
	}
	// Verify SKILL.md now matches embedded content.
	embedded, _ := collectEmbeddedFiles("skills/openbee-bee")
	got, _ := os.ReadFile(filepath.Join(beeDir, "SKILL.md"))
	if string(got) != embedded["SKILL.md"] {
		t.Error("openbee-bee SKILL.md content not updated to embedded content")
	}
	var workerResult SkillResult
	for _, r := range results {
		if r.Name == "openbee-worker" {
			workerResult = r
		}
	}
	if workerResult.Action != ActionInstalled {
		t.Errorf("expected 'installed' for openbee-worker, got %q", workerResult.Action)
	}
}

func TestInstallSkillsToDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	results, err := InstallSkillsToDefaults()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Two dirs × number of skills = total results.
	want := 2 * len(embeddedSkills)
	if len(results) != want {
		t.Fatalf("expected %d results (%d skills × 2 dirs), got %d", want, len(embeddedSkills), len(results))
	}
	for _, r := range results {
		if r.Action != ActionInstalled {
			t.Errorf("skill %s: expected 'installed', got %q", r.Name, r.Action)
		}
	}

	// Verify all embedded files exist in both target directories.
	claudeSkills := filepath.Join(home, ".claude", "skills")
	agentsSkills := filepath.Join(home, ".agents", "skills")
	for _, dir := range []string{claudeSkills, agentsSkills} {
		for _, name := range embeddedSkills {
			embedded, err := collectEmbeddedFiles("skills/" + name)
			if err != nil {
				t.Fatalf("skill %s: collect embedded files: %v", name, err)
			}
			for relPath := range embedded {
				p := filepath.Join(dir, name, filepath.FromSlash(relPath))
				if _, err := os.Stat(p); err != nil {
					t.Errorf("expected %s to exist: %v", p, err)
				}
			}
		}
	}
}

func TestInstallSkills_WriteError(t *testing.T) {
	dir := t.TempDir()
	// Block dir creation for openbee-bee by placing a file where the dir would go.
	if err := os.WriteFile(filepath.Join(dir, "openbee-bee"), []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := InstallSkills(dir)
	if err == nil {
		t.Fatal("expected error when write is blocked, got nil")
	}
}
