// internal/claude/invoker_test.go
package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewInvoker(t *testing.T) {
	inv := NewInvoker("/usr/bin/claude", "http://localhost:8080", "test-key")
	if inv.binary != "/usr/bin/claude" {
		t.Errorf("binary: want /usr/bin/claude, got %s", inv.binary)
	}
	if inv.openbeeURL != "http://localhost:8080" {
		t.Errorf("openbeeURL: want http://localhost:8080, got %s", inv.openbeeURL)
	}
	if inv.apiKey != "test-key" {
		t.Errorf("apiKey: want test-key, got %s", inv.apiKey)
	}
}

func TestInvoker_Run_WritesOutputToFile(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logPath := filepath.Join(t.TempDir(), "test.log")
	proc, ch, err := inv.Run(ctx, t.TempDir(), "hello", RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.PID() == 0 {
		t.Error("expected non-zero PID")
	}

	var gotDone bool
	for out := range ch {
		if out.Type == OutputDone {
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

func TestInvoker_Run_SessionFlags(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx := context.Background()

	// Test --session-id flag written to log file
	logPath1 := filepath.Join(t.TempDir(), "s1.log")
	_, ch, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s1"}, logPath1)
	for range ch {
	}
	data, _ := os.ReadFile(logPath1)
	output := string(data)
	if !strings.Contains(output, "--session-id") || !strings.Contains(output, "s1") {
		t.Errorf("expected --session-id s1 in log file, got: %s", output)
	}

	// Test --resume flag written to log file
	logPath2 := filepath.Join(t.TempDir(), "s2.log")
	_, ch2, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s2", Resume: true}, logPath2)
	for range ch2 {
	}
	data2, _ := os.ReadFile(logPath2)
	output2 := string(data2)
	if !strings.Contains(output2, "--resume") || !strings.Contains(output2, "s2") {
		t.Errorf("expected --resume s2 in log file, got: %s", output2)
	}
}

func TestProcess_Stop(t *testing.T) {
	inv := NewInvoker("sleep", "", "")
	ctx := context.Background()

	logPath := filepath.Join(t.TempDir(), "stop.log")
	proc, ch, err := inv.Run(ctx, t.TempDir(), "60", RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Drain channel — should get OutputError since process was killed
	for range ch {
	}
}

func TestInvoker_ConcurrentRuns(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx := context.Background()

	logPath1 := filepath.Join(t.TempDir(), "one.log")
	logPath2 := filepath.Join(t.TempDir(), "two.log")

	proc1, ch1, err1 := inv.Run(ctx, t.TempDir(), "one", RunOptions{SessionID: "s1"}, logPath1)
	if err1 != nil {
		t.Fatalf("Run 1: %v", err1)
	}
	proc2, ch2, err2 := inv.Run(ctx, t.TempDir(), "two", RunOptions{SessionID: "s2"}, logPath2)
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
