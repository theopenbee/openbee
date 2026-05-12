package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestMain(m *testing.M) {
	// Subprocess helper: when GO_TEST_EMIT_IS_ERROR=1, write an is_error result
	// event to stdout and exit immediately without running any tests.
	if os.Getenv("GO_TEST_EMIT_IS_ERROR") == "1" {
		os.Stdout.WriteString(`{"type":"result","is_error":true,"result":"API Error: 400 {}"}` + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func writeClaudeTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestNewBackend(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	b := NewBackend("/usr/bin/claude", nil)
	if b.binary != "/usr/bin/claude" {
		t.Errorf("binary: want /usr/bin/claude, got %s", b.binary)
	}
	wantURL := "OPENBEE_URL=http://localhost:8080"
	if !slices.Contains(b.baseEnv, wantURL) {
		t.Errorf("baseEnv missing %s", wantURL)
	}
}

func TestBackend_Run_WritesOutputToFile(t *testing.T) {
	inv := NewBackend("echo", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logPath := filepath.Join(t.TempDir(), "test.log")
	proc, ch, err := inv.Run(ctx, t.TempDir(), "hello", ai.RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.PID() == 0 {
		t.Error("expected non-zero PID")
	}

	var gotDone bool
	for out := range ch {
		if out.Type == ai.OutputDone {
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("expected done signal")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty log file after echo")
	}
}

func TestBackend_Run_SessionFlags(t *testing.T) {
	inv := NewBackend("echo", nil)
	ctx := context.Background()

	logPath1 := filepath.Join(t.TempDir(), "s1.log")
	_, ch, _ := inv.Run(ctx, t.TempDir(), "test", ai.RunOptions{SessionID: "s1"}, logPath1)
	for range ch {
	}
	data, _ := os.ReadFile(logPath1)
	output := string(data)
	if !strings.Contains(output, "--session-id") || !strings.Contains(output, "s1") {
		t.Errorf("expected --session-id s1 in log file, got: %s", output)
	}

	logPath2 := filepath.Join(t.TempDir(), "s2.log")
	_, ch2, _ := inv.Run(ctx, t.TempDir(), "test", ai.RunOptions{SessionID: "s2", Resume: true}, logPath2)
	for range ch2 {
	}
	data2, _ := os.ReadFile(logPath2)
	output2 := string(data2)
	if !strings.Contains(output2, "--resume") || !strings.Contains(output2, "s2") {
		t.Errorf("expected --resume s2 in log file, got: %s", output2)
	}
}

func TestProcess_Stop(t *testing.T) {
	inv := NewBackend("sleep", nil)
	ctx := context.Background()

	logPath := filepath.Join(t.TempDir(), "stop.log")
	proc, ch, err := inv.Run(ctx, t.TempDir(), "60", ai.RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	for range ch {
	}
}

func TestBackend_ConcurrentRuns(t *testing.T) {
	inv := NewBackend("echo", nil)
	ctx := context.Background()

	logPath1 := filepath.Join(t.TempDir(), "one.log")
	logPath2 := filepath.Join(t.TempDir(), "two.log")

	proc1, ch1, err1 := inv.Run(ctx, t.TempDir(), "one", ai.RunOptions{SessionID: "s1"}, logPath1)
	if err1 != nil {
		t.Fatalf("Run 1: %v", err1)
	}
	proc2, ch2, err2 := inv.Run(ctx, t.TempDir(), "two", ai.RunOptions{SessionID: "s2"}, logPath2)
	if err2 != nil {
		t.Fatalf("Run 2: %v", err2)
	}

	if proc1.PID() == proc2.PID() {
		t.Error("concurrent runs should have different PIDs")
	}

	for range ch1 {
	}
	for range ch2 {
	}
}

func TestBackend_Run_IsErrorEmitsOutputError(t *testing.T) {
	// Run the test binary itself as the subprocess. TestMain detects
	// GO_TEST_EMIT_IS_ERROR=1, writes is_error JSON to stdout, and exits 0.
	// BuildBaseEnv inherits os.Environ(), so t.Setenv propagates to the child.
	t.Setenv("GO_TEST_EMIT_IS_ERROR", "1")

	inv := NewBackend(os.Args[0], nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "run.log")
	_, ch, err := inv.Run(ctx, dir, "", ai.RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var gotError bool
	var errorContent string
	for out := range ch {
		if out.Type == ai.OutputError {
			gotError = true
			errorContent = out.Content
		}
	}
	if !gotError {
		t.Error("want OutputError, got none")
	}
	if !strings.Contains(errorContent, "API Error: 400") {
		t.Errorf("want error content to contain 'API Error: 400', got %q", errorContent)
	}
}

func TestScanResultLog_IsError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"result","is_error":true,"result":"API Error: 400 {\"error\":\"操作失败\"}"}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, isError, _ := scanResultLog(logPath)
	if !isError {
		t.Error("want isError=true, got false")
	}
	if result != `API Error: 400 {"error":"操作失败"}` {
		t.Errorf("want API Error string, got %q", result)
	}
}

func TestScanResultLog_NoError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"result","is_error":false,"result":"all good"}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, isError, _ := scanResultLog(logPath)
	if isError {
		t.Error("want isError=false, got true")
	}
	if result != "all good" {
		t.Errorf("want 'all good', got %q", result)
	}
}

func TestScanResultLog_NoResultEvent(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, isError, _ := scanResultLog(logPath)
	if isError {
		t.Error("want isError=false when no result event, got true")
	}
	if result != "" {
		t.Errorf("want empty result, got %q", result)
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

func TestClaudeCollector_Collect_AggregatesByModel(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/test-session.jsonl", `{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}
{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":5}}}
{"message":{"model":"claude-3-opus","usage":{"input_tokens":300,"output_tokens":150}}}
{"timestamp":"2025-01-01T00:00:00Z"}
`)
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	collector := NewBackend("", nil)

	usages, err := collector.Collect(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["claude-3-5-sonnet"]
	if sonnet.InputTokens != 300 {
		t.Errorf("sonnet InputTokens: want 300, got %d", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 150 {
		t.Errorf("sonnet OutputTokens: want 150, got %d", sonnet.OutputTokens)
	}
	if sonnet.CacheCreationTokens != 20 {
		t.Errorf("sonnet CacheCreationTokens: want 20, got %d", sonnet.CacheCreationTokens)
	}
	if sonnet.CacheReadTokens != 15 {
		t.Errorf("sonnet CacheReadTokens: want 15, got %d", sonnet.CacheReadTokens)
	}

	opus := byModel["claude-3-opus"]
	if opus.InputTokens != 300 {
		t.Errorf("opus InputTokens: want 300, got %d", opus.InputTokens)
	}
}

func TestClaudeCollector_Collect_FastSpeedSuffix(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/fast-session.jsonl",
		`{"message":{"model":"claude-3-5-sonnet","speed":"fast","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := NewBackend("", nil).Collect(context.Background(), "fast-session")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].Model != "claude-3-5-sonnet-fast" {
		t.Errorf("Model: want claude-3-5-sonnet-fast, got %s", usages[0].Model)
	}
}

func TestClaudeCollector_Collect_SkipsSyntheticModel(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/syn-session.jsonl",
		`{"message":{"model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n"+
			`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := NewBackend("", nil).Collect(context.Background(), "syn-session")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].Model != "claude-3-5-sonnet" {
		t.Errorf("Model: want claude-3-5-sonnet, got %s", usages[0].Model)
	}
}

func TestClaudeCollector_Collect_FileNotFound(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	_, err := NewBackend("", nil).Collect(context.Background(), "nonexistent-session")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Errorf("expected ErrSessionDataNotFound, got: %v", err)
	}
}
