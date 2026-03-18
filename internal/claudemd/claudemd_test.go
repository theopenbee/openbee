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

	if !strings.Contains(content, "任务通知规范") {
		t.Error("missing shared rules (任务通知规范)")
	}
	if !strings.Contains(content, "send_message") {
		t.Error("missing send_message reference in shared rules")
	}
	if !strings.Contains(content, "清除上下文处理") {
		t.Error("missing bee-specific rules (清除上下文处理)")
	}
	if !strings.Contains(content, "create_task") {
		t.Error("missing bee-specific create_task reference in 任务分发流程")
	}
	if !strings.Contains(content, "list_workers") {
		t.Error("missing bee-specific list_workers reference in 任务分发流程")
	}
	if !strings.Contains(content, "任务分发流程") {
		t.Error("missing bee-specific 任务分发流程 section")
	}
	if strings.Contains(content, "mark_task_complete") {
		t.Error("bee rules should not contain worker-specific mark_task_complete")
	}
}

func TestEnsureSystemRules_WritesWorkerRulesWithName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Worker\n"), 0644)

	if err := claudemd.EnsureSystemRules(dir, claudemd.RoleWorker, claudemd.WithName("测试助手"), claudemd.WithDescription("负责测试任务")); err != nil {
		t.Fatalf("EnsureSystemRules: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, claudemd.SystemRulesFile))
	if err != nil {
		t.Fatalf("read %s: %v", claudemd.SystemRulesFile, err)
	}
	content := string(data)

	if !strings.Contains(content, "任务通知规范") {
		t.Error("missing shared rules")
	}
	if !strings.Contains(content, "mark_task_complete") {
		t.Error("missing worker-specific rules (mark_task_complete)")
	}
	if strings.Contains(content, "mark_task_failed") {
		t.Error("worker rules must not contain mark_task_failed (failure is now system-handled)")
	}
	if !strings.Contains(content, "系统元数据") {
		t.Error("missing worker-specific 系统元数据 section")
	}
	if !strings.Contains(content, "Worker 配置") {
		t.Error("missing worker config block")
	}
	if !strings.Contains(content, "**名称:** 测试助手") {
		t.Error("missing worker name in config block")
	}
	if !strings.Contains(content, "**职责:** 负责测试任务") {
		t.Error("missing worker description in config block")
	}
	if strings.Contains(content, "清除上下文处理") {
		t.Error("worker rules should not contain bee-specific 清除上下文处理")
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

	if !strings.Contains(content, "mark_task_complete") {
		t.Error("missing worker-specific rules")
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
	if !strings.Contains(string(data), "任务通知规范") {
		t.Error("overwritten file should contain latest rules")
	}
}
