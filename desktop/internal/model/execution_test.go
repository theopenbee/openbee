package model

import (
	"reflect"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func TestSplitDuration(t *testing.T) {
	cases := []struct {
		ms   int64
		want DurationParts
	}{
		{0, DurationParts{Seconds: 1}},
		{400, DurationParts{Seconds: 1}},
		{41_000, DurationParts{Seconds: 41}},
		{59_600, DurationParts{Minutes: 1}},
		{180_000, DurationParts{Minutes: 3}},
		{192_000, DurationParts{Minutes: 3, Seconds: 12}},
		{3_600_000, DurationParts{Hours: 1}},
		{3_720_000, DurationParts{Hours: 1, Minutes: 2}},
		{3_725_000, DurationParts{Hours: 1, Minutes: 2}},
	}
	for _, c := range cases {
		if got := SplitDuration(c.ms); got != c.want {
			t.Errorf("SplitDuration(%d) = %+v, want %+v", c.ms, got, c.want)
		}
	}
}

func TestToolKind(t *testing.T) {
	for _, kind := range []string{"command", "read", "edit", "search", "web", "message", "other"} {
		if got := ToolKind(kind); got != kind {
			t.Errorf("ToolKind(%q) = %q", kind, got)
		}
	}
	for _, kind := range []string{"", "browser", "Command"} {
		if got := ToolKind(kind); got != ToolOther {
			t.Errorf("ToolKind(%q) = %q, want other", kind, got)
		}
	}
}

func TestKnownStepsAndStats(t *testing.T) {
	steps := []api.ExecutionStep{
		{Type: StepThinking, Text: "plan"},
		{Type: "image"},
		{Type: StepTool, Kind: "read", Status: ToolDone},
		{Type: StepText, Text: "done"},
		{Type: StepError, Text: "rate limited"},
		{Type: StepTool, Kind: "edit", Status: ToolError},
	}
	known := KnownSteps(steps)
	if len(known) != 5 || known[1].Type != StepTool {
		t.Fatalf("KnownSteps = %+v", known)
	}
	if n, tools := StepStats(known); n != 5 || tools != 2 {
		t.Fatalf("StepStats = %d, %d; want 5, 2", n, tools)
	}
	if KnownSteps(nil) == nil {
		t.Fatal("KnownSteps(nil) is nil")
	}
}

func TestToolDurationShown(t *testing.T) {
	for ms, want := range map[int64]bool{0: false, 999: false, 1000: true, 41_000: true} {
		if got := ToolDurationShown(ms); got != want {
			t.Errorf("ToolDurationShown(%d) = %v, want %v", ms, got, want)
		}
	}
}

func TestSwitcherRuns(t *testing.T) {
	done := int64(10)
	detail := func(id string, runs ...api.ExecutionSummary) api.ExecutionDetail {
		return api.ExecutionDetail{ExecutionSummary: api.ExecutionSummary{ID: id, Status: ExecutionFailed, StartedAt: 1, CompletedAt: &done}, Runs: runs}
	}
	e3 := api.ExecutionSummary{ID: "e3", Status: ExecutionRunning, StartedAt: 9}
	e2 := api.ExecutionSummary{ID: "e2", Status: ExecutionCompleted, StartedAt: 5}
	if got := SwitcherRuns(detail("e1")); len(got) != 0 {
		t.Fatalf("no runs = %+v", got)
	}
	if got := SwitcherRuns(detail("e2", e3, e2)); !reflect.DeepEqual(got, []api.ExecutionSummary{e3, e2}) {
		t.Fatalf("listed runs = %+v", got)
	}
	older := detail("e1", e3, e2)
	want := []api.ExecutionSummary{e3, e2, older.ExecutionSummary}
	if got := SwitcherRuns(older); !reflect.DeepEqual(got, want) {
		t.Fatalf("older run = %+v, want %+v", got, want)
	}
}

func TestRunSpan(t *testing.T) {
	now := time.UnixMilli(50_000)
	done := int64(20_000)
	if ms, ok := RunSpan(api.ExecutionSummary{Status: ExecutionCompleted, StartedAt: 8_000, CompletedAt: &done}, now); !ok || ms != 12_000 {
		t.Fatalf("finished span = %d, %v", ms, ok)
	}
	if ms, ok := RunSpan(api.ExecutionSummary{Status: ExecutionRunning, StartedAt: 8_000}, now); !ok || ms != 42_000 {
		t.Fatalf("running span = %d, %v", ms, ok)
	}
	if _, ok := RunSpan(api.ExecutionSummary{Status: ExecutionFailed, StartedAt: 8_000}, now); ok {
		t.Fatal("a finished run without an end has a span")
	}
}

func TestPlainInstruction(t *testing.T) {
	if got := PlainInstruction("  Update `README` and run `pnpm test`\n"); got != "Update README and run pnpm test" {
		t.Fatalf("PlainInstruction = %q", got)
	}
}

func TestShowsHandoffTask(t *testing.T) {
	msgs := []Message{
		{ChatMessage: api.ChatMessage{ID: "m1", Kind: "message"}},
		{ChatMessage: api.ChatMessage{ID: "h1", Kind: "handoff", Handoff: &api.HandoffInfo{TaskID: "t1"}}},
	}
	if !ShowsHandoffTask(msgs, "t1") {
		t.Fatal("handoff task t1 not found")
	}
	if ShowsHandoffTask(msgs, "t2") || ShowsHandoffTask(msgs, "") || ShowsHandoffTask(nil, "t1") {
		t.Fatal("matched a task that no handoff shows")
	}
}
