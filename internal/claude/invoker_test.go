// internal/claude/invoker_test.go
package claude

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewInvoker(t *testing.T) {
	inv := NewInvoker("/usr/bin/claude", "http://localhost:8080/mcp/bee", "test-key")
	if inv.binary != "/usr/bin/claude" {
		t.Errorf("binary: want /usr/bin/claude, got %s", inv.binary)
	}
	if inv.mcpURL != "http://localhost:8080/mcp/bee/sse" {
		t.Errorf("mcpURL: want http://localhost:8080/mcp/bee/sse, got %s", inv.mcpURL)
	}
	if inv.apiKey != "test-key" {
		t.Errorf("apiKey: want test-key, got %s", inv.apiKey)
	}
}

func TestInvoker_Run_WithEcho(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	proc, ch, err := inv.Run(ctx, t.TempDir(), "hello", RunOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.PID() == 0 {
		t.Error("expected non-zero PID")
	}

	var gotStdout bool
	var gotDone bool
	for out := range ch {
		switch out.Type {
		case OutputStdout:
			gotStdout = true
		case OutputDone:
			gotDone = true
		}
	}
	if !gotStdout {
		t.Error("expected stdout output")
	}
	if !gotDone {
		t.Error("expected done signal")
	}
}

func TestInvoker_Run_SessionFlags(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx := context.Background()

	// Test --session-id
	_, ch, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s1"})
	var output string
	for out := range ch {
		if out.Type == OutputStdout {
			output += out.Content
		}
	}
	if !strings.Contains(output, "--session-id") || !strings.Contains(output, "s1") {
		t.Errorf("expected --session-id s1 in output, got: %s", output)
	}

	// Test --resume
	_, ch2, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s2", Resume: true})
	var output2 string
	for out := range ch2 {
		if out.Type == OutputStdout {
			output2 += out.Content
		}
	}
	if !strings.Contains(output2, "--resume") || !strings.Contains(output2, "s2") {
		t.Errorf("expected --resume s2 in output, got: %s", output2)
	}
}

func TestProcess_Stop(t *testing.T) {
	inv := NewInvoker("sleep", "", "")
	ctx := context.Background()

	proc, ch, err := inv.Run(ctx, t.TempDir(), "60", RunOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Drain channel — should get an error since process was killed
	for range ch {
	}
}

func TestInvoker_ConcurrentRuns(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx := context.Background()

	proc1, ch1, err1 := inv.Run(ctx, t.TempDir(), "one", RunOptions{SessionID: "s1"})
	if err1 != nil {
		t.Fatalf("Run 1: %v", err1)
	}
	proc2, ch2, err2 := inv.Run(ctx, t.TempDir(), "two", RunOptions{SessionID: "s2"})
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
