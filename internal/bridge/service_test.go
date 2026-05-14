package bridge_test

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/bridge"
)

func TestAllEnginesMatchesCanonicalOrder(t *testing.T) {
	got := bridge.AllEngines()
	want := []string{bridge.EngineClaude, bridge.EngineCodex, bridge.EnginePi, bridge.EngineKimi}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("engine[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestWorkerPersonaAndPrefix(t *testing.T) {
	persona := bridge.WorkerPersona{
		Name:        "xiao-qiao",
		Description: "openbee development",
		Constraints: "call the user boss",
	}
	prefix := bridge.BuildWorkerSessionPrefix(persona)
	for _, want := range []string{
		"## Step 1: Initialize your role",
		"openbee-worker",
		"<worker_persona>",
		"Name: xiao-qiao",
		"Description: openbee development",
		"call the user boss",
		"## Step 2: Execute the task",
	} {
		if !strings.Contains(prefix, want) {
			t.Fatalf("prefix missing %q:\n%s", want, prefix)
		}
	}
}

func TestParseEngineArgs(t *testing.T) {
	got, err := bridge.ParseEngineArgs(map[string]string{
		bridge.EngineClaude: `--model "sonnet 4" --verbose`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--model", "sonnet 4", "--verbose"}
	args := got[bridge.EngineClaude]
	if len(args) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg[%d]=%q want %q", i, args[i], want[i])
		}
	}
}
