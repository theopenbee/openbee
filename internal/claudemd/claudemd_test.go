package claudemd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/claudemd"
)

func TestEnsureSystemRules_WritesBeeRules(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Bee\n"), 0644)

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleBee); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, claudemd.SystemRulesFile))
	if err != nil {
		t.Fatalf("read %s: %v", claudemd.SystemRulesFile, err)
	}
	content := string(data)

	if !strings.Contains(content, "coordinator and dispatcher") {
		t.Error("missing bee role description (coordinator and dispatcher)")
	}
	if !strings.Contains(content, "openbee-bee skill") {
		t.Error("missing openbee-bee skill reference")
	}
	if strings.Contains(content, "mark_task_complete") {
		t.Error("bee rules should not contain worker-specific mark_task_complete")
	}
}

func TestEnsureSystemRules_WritesWorkerRulesWithName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Worker\n"), 0644)

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleWorker, claudemd.WithName("test-assistant"), claudemd.WithDescription("responsible for testing")); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, claudemd.SystemRulesFile))
	if err != nil {
		t.Fatalf("read %s: %v", claudemd.SystemRulesFile, err)
	}
	content := string(data)

	if !strings.Contains(content, "Name: test-assistant") {
		t.Error("missing worker name")
	}
	if !strings.Contains(content, "Description: responsible for testing") {
		t.Error("missing worker description")
	}
	if strings.Contains(content, "clear context handling") {
		t.Error("worker rules should not contain bee-specific clear context handling")
	}
	if !strings.Contains(content, "openbee-worker skill") {
		t.Error("missing openbee-worker skill reference")
	}
	if !strings.Contains(content, "Worker") {
		t.Error("missing worker role description")
	}
}

func TestEnsureSystemRules_WritesWorkerRulesWithNameOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Worker\n"), 0644)

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleWorker, claudemd.WithName("mini-assistant")); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, claudemd.SystemRulesFile))
	if err != nil {
		t.Fatalf("read %s: %v", claudemd.SystemRulesFile, err)
	}
	content := string(data)

	if !strings.Contains(content, "Name: mini-assistant") {
		t.Error("missing worker name")
	}
	if strings.Contains(content, "Description:") {
		t.Error("description field should not appear when description is empty")
	}
}

func TestEnsureSystemRules_WritesWorkerRulesWithMemoryOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Worker\n"), 0644)

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleWorker, claudemd.WithMemory("user prefers English replies")); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, claudemd.SystemRulesFile))
	if err != nil {
		t.Fatalf("read %s: %v", claudemd.SystemRulesFile, err)
	}
	content := string(data)

	if strings.Contains(content, "---") {
		t.Error("frontmatter block should not appear when name and description are both empty")
	}
	if strings.Contains(content, "Description:") {
		t.Error("description field should not appear when name and description are both empty")
	}
	if !strings.Contains(content, "## Memory Constraints") {
		t.Error("memory section should appear")
	}
	if !strings.Contains(content, "user prefers English replies") {
		t.Error("memory content should appear")
	}
	if strings.Contains(content, "### Memory") {
		t.Error("old ### Memory heading should not appear")
	}
}

func TestEnsureSystemRules_WritesWorkerRulesWithoutName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Worker\n"), 0644)

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleWorker); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, claudemd.SystemRulesFile))
	if err != nil {
		t.Fatalf("read %s: %v", claudemd.SystemRulesFile, err)
	}
	content := string(data)

	if strings.Contains(content, "Non-Interactive Background Worker") {
		t.Error("worker preamble should have been removed")
	}
}

func TestEnsureSystemRules_AppendsImportWhenMissing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# My Bot\n\nSome user content\n"), 0644)

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleWorker); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, claudemd.ImportLine) {
		t.Error("CLAUDE.md should contain import line")
	}
	if !strings.Contains(content, "# My Bot") {
		t.Error("original CLAUDE.md content should be preserved")
	}
	if !strings.Contains(content, "Some user content") {
		t.Error("user content should be preserved")
	}
}

func TestEnsureSystemRules_DoesNotDuplicateImport(t *testing.T) {
	dir := t.TempDir()
	original := "# My Bot\n\n" + claudemd.ImportLine + "\n"
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(original), 0644)

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleWorker); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if string(data) != original {
		t.Errorf("CLAUDE.md should not be modified when import already exists.\nGot: %q\nWant: %q", string(data), original)
	}
}

func TestEnsureSystemRules_SkipsWhenNoCLAUDEMD(t *testing.T) {
	dir := t.TempDir()

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleWorker); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, claudemd.SystemRulesFile)); err != nil {
		t.Error(claudemd.SystemRulesFile + " should be created even without CLAUDE.md")
	}

	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("CLAUDE.md should not be created by EnsureSystemRules")
	}
}

func TestEnsureSystemRules_OverwritesExistingRulesFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Bot\n"), 0644)
	os.WriteFile(filepath.Join(dir, claudemd.SystemRulesFile), []byte("old content"), 0644)

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleWorker); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, claudemd.SystemRulesFile))
	if string(data) == "old content" {
		t.Error(claudemd.SystemRulesFile + " should be overwritten with latest rules")
	}
	if strings.Contains(string(data), "old content") {
		t.Error("overwritten file should not contain old content")
	}
}
