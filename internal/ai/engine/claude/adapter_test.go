package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupLegacyRules_Stub(t *testing.T) {
	dir := t.TempDir()
	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}
}

func TestCleanupLegacyRules_DeletesOpenbeeFile(t *testing.T) {
	dir := t.TempDir()
	openbeeFile := filepath.Join(dir, systemRulesFile)
	if err := os.WriteFile(openbeeFile, []byte("old rules"), 0o644); err != nil {
		t.Fatalf("write .openbee.md: %v", err)
	}

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}

	if _, err := os.Stat(openbeeFile); !os.IsNotExist(err) {
		t.Error(".openbee.md should have been deleted")
	}
}

func TestCleanupLegacyRules_RemovesImportLine(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	content := "# My Bot\n" + importLine + "\nOther content\n"
	if err := os.WriteFile(claudeFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	got := string(data)
	if strings.Contains(got, importLine) {
		t.Errorf("CLAUDE.md should not contain import line, got:\n%s", got)
	}
	if !strings.Contains(got, "# My Bot") {
		t.Error("CLAUDE.md should preserve other content")
	}
	if !strings.Contains(got, "Other content") {
		t.Error("CLAUDE.md should preserve other content")
	}
}

func TestCleanupLegacyRules_PreservesOtherCLAUDEMDContent(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	original := "# Custom instructions\nDo something special.\n"
	if err := os.WriteFile(claudeFile, []byte(original), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	if string(data) != original {
		t.Errorf("CLAUDE.md should be unchanged when import line is absent.\nGot: %q\nWant: %q", string(data), original)
	}
}

func TestCleanupLegacyRules_NoopWhenFilesAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules should not error when no files exist: %v", err)
	}
}

func TestCleanupLegacyRules_BothLegacyFilesPresent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, systemRulesFile), []byte("rules"), 0o644)
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(importLine+"\n"), 0o644)

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, systemRulesFile)); !os.IsNotExist(err) {
		t.Errorf(".openbee.md should be deleted")
	}
}
