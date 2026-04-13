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
		if r.Action != ActionInstalled {
			t.Errorf("skill %s: expected action 'installed', got %q", r.Name, r.Action)
		}
		content, err := os.ReadFile(filepath.Join(dir, r.Name, "SKILL.md"))
		if err != nil {
			t.Errorf("skill %s: SKILL.md not created: %v", r.Name, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("skill %s: SKILL.md is empty", r.Name)
		}
	}

	// Verify reference files are installed for each skill.
	for _, skill := range embeddedSkills {
		for refName, refContent := range skill.references {
			refPath := filepath.Join(dir, skill.name, "references", refName)
			got, err := os.ReadFile(refPath)
			if err != nil {
				t.Errorf("skill %s: reference %s not created: %v", skill.name, refName, err)
				continue
			}
			if string(got) != refContent {
				t.Errorf("skill %s: reference %s content mismatch", skill.name, refName)
			}
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
		if r.Action != ActionUpToDate {
			t.Errorf("skill %s: expected 'up-to-date', got %q", r.Name, r.Action)
		}
	}
}

func TestInstallSkills_UpdatedWhenReferenceMissing(t *testing.T) {
	dir := t.TempDir()
	// First install
	if _, err := InstallSkills(dir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	// Delete one reference file from openbee-bee
	beeRef := filepath.Join(dir, "openbee-bee", "references", "cli-reference.md")
	if err := os.Remove(beeRef); err != nil {
		t.Fatal(err)
	}
	// Second install should detect the missing reference and update
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
		t.Errorf("expected 'updated' for openbee-bee when reference missing, got %q", beeResult.Action)
	}
	// Verify the reference was restored
	got, err := os.ReadFile(beeRef)
	if err != nil {
		t.Fatalf("reference not restored: %v", err)
	}
	if len(got) == 0 {
		t.Error("restored reference is empty")
	}
}

func TestInstallSkills_Updated(t *testing.T) {
	dir := t.TempDir()
	// Write stale content for "openbee-bee"
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
	// Verify file now contains embedded content
	got, _ := os.ReadFile(filepath.Join(beeDir, "SKILL.md"))
	if string(got) != beeSkillMD {
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
	// Override home so both default paths resolve under our temp dir.
	home := t.TempDir()
	t.Setenv("HOME", home)

	results, err := InstallSkillsToDefaults()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Two dirs × two skills = four results.
	if len(results) != 4 {
		t.Fatalf("expected 4 results (2 skills × 2 dirs), got %d", len(results))
	}
	for _, r := range results {
		if r.Action != ActionInstalled {
			t.Errorf("skill %s: expected 'installed', got %q", r.Name, r.Action)
		}
	}

	// Verify SKILL.md and reference files exist in both target directories.
	claudeSkills := filepath.Join(home, ".claude", "skills")
	agentsSkills := filepath.Join(home, ".agents", "skills")
	for _, dir := range []string{claudeSkills, agentsSkills} {
		for _, skill := range embeddedSkills {
			p := filepath.Join(dir, skill.name, "SKILL.md")
			if _, err := os.Stat(p); err != nil {
				t.Errorf("expected file %s to exist: %v", p, err)
			}
			for refName := range skill.references {
				rp := filepath.Join(dir, skill.name, "references", refName)
				if _, err := os.Stat(rp); err != nil {
					t.Errorf("expected reference %s/%s to exist: %v", skill.name, refName, err)
				}
			}
		}
	}
}

func TestInstallSkills_WriteError(t *testing.T) {
	dir := t.TempDir()
	// Place a regular file at the path where the "openbee-bee" directory would be created,
	// so MkdirAll will fail with ENOTDIR.
	if err := os.WriteFile(filepath.Join(dir, "openbee-bee"), []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := InstallSkills(dir)
	if err == nil {
		t.Fatal("expected error when write is blocked, got nil")
	}
}
