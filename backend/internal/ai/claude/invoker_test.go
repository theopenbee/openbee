package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestNewInvoker(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	inv := NewInvoker("/usr/bin/claude", nil)
	assert.Equal(t, "/usr/bin/claude", inv.binary)
	wantURL := "OPENBEE_URL=http://localhost:8080"
	assert.Contains(t, inv.baseEnv, wantURL)
}

func TestInvoker_Run_SessionFlags(t *testing.T) {
	inv := NewInvoker("echo", nil)
	ctx := context.Background()

	// Test --session-id flag written to log file
	logPath1 := filepath.Join(t.TempDir(), "s1.log")
	_, ch, _ := inv.Run(ctx, t.TempDir(), "test", ai.RunOptions{SessionID: "s1"}, logPath1)
	for range ch {
	}
	data, _ := os.ReadFile(logPath1)
	output := string(data)
	assert.Contains(t, output, "--session-id")
	assert.Contains(t, output, "s1")

	// Test --resume flag written to log file
	logPath2 := filepath.Join(t.TempDir(), "s2.log")
	_, ch2, _ := inv.Run(ctx, t.TempDir(), "test", ai.RunOptions{SessionID: "s2", Resume: true}, logPath2)
	for range ch2 {
	}
	data2, _ := os.ReadFile(logPath2)
	output2 := string(data2)
	assert.Contains(t, output2, "--resume")
	assert.Contains(t, output2, "s2")
}

func TestProcess_Stop(t *testing.T) {
	inv := NewInvoker("sleep", nil)
	ctx := context.Background()

	logPath := filepath.Join(t.TempDir(), "stop.log")
	proc, ch, err := inv.Run(ctx, t.TempDir(), "60", ai.RunOptions{}, logPath)
	require.NoError(t, err)

	require.NoError(t, proc.Stop())

	// Drain channel — should get OutputError since process was killed
	for range ch {
	}
}

func TestInvoker_ConcurrentRuns(t *testing.T) {
	inv := NewInvoker("echo", nil)
	ctx := context.Background()

	logPath1 := filepath.Join(t.TempDir(), "one.log")
	logPath2 := filepath.Join(t.TempDir(), "two.log")

	proc1, ch1, err1 := inv.Run(ctx, t.TempDir(), "one", ai.RunOptions{SessionID: "s1"}, logPath1)
	require.NoError(t, err1)
	assert.NotZero(t, proc1.PID())
	proc2, ch2, err2 := inv.Run(ctx, t.TempDir(), "two", ai.RunOptions{SessionID: "s2"}, logPath2)
	require.NoError(t, err2)

	assert.NotEqual(t, proc1.PID(), proc2.PID(), "concurrent runs should have different PIDs")

	var gotDone bool
	for out := range ch1 {
		if out.Type == ai.OutputDone {
			gotDone = true
		}
	}
	assert.True(t, gotDone, "expected done signal")
	for range ch2 {
	}

	data, err := os.ReadFile(logPath1)
	require.NoError(t, err)
	assert.NotEmpty(t, data, "expected non-empty log file after echo")
}

func TestInvoker_Run_IsErrorEmitsOutputError(t *testing.T) {
	// Run the test binary itself as the subprocess. TestMain detects
	// GO_TEST_EMIT_IS_ERROR=1, writes is_error JSON to stdout, and exits 0.
	// BuildBaseEnv inherits os.Environ(), so t.Setenv propagates to the child.
	t.Setenv("GO_TEST_EMIT_IS_ERROR", "1")

	inv := NewInvoker(os.Args[0], nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "run.log")
	_, ch, err := inv.Run(ctx, dir, "", ai.RunOptions{}, logPath)
	require.NoError(t, err)

	var gotError bool
	var errorContent string
	for out := range ch {
		if out.Type == ai.OutputError {
			gotError = true
			errorContent = out.Content
		}
	}
	assert.True(t, gotError, "want OutputError, got none")
	assert.Contains(t, errorContent, "API Error: 400")
}

func TestScanResultLog_IsError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"result","is_error":true,"result":"API Error: 400 {\"error\":\"操作失败\"}"}` + "\n"
	require.NoError(t, os.WriteFile(logPath, []byte(content), 0o644))
	result, isError, _ := scanResultLog(logPath)
	assert.True(t, isError)
	assert.Equal(t, `API Error: 400 {"error":"操作失败"}`, result)
}

func TestScanResultLog_NoError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"result","is_error":false,"result":"all good"}` + "\n"
	require.NoError(t, os.WriteFile(logPath, []byte(content), 0o644))
	result, isError, _ := scanResultLog(logPath)
	assert.False(t, isError)
	assert.Equal(t, "all good", result)
}

func TestScanResultLog_NoResultEvent(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}` + "\n"
	require.NoError(t, os.WriteFile(logPath, []byte(content), 0o644))
	result, isError, _ := scanResultLog(logPath)
	assert.False(t, isError, "no result event present")
	assert.Empty(t, result)
}
