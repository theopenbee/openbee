package model

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	ExecutionRunning   = "running"
	ExecutionCompleted = "completed"
	ExecutionFailed    = "failed"

	StepText     = "text"
	StepThinking = "thinking"
	StepError    = "error"
	StepTool     = "tool"

	ToolRunning = "running"
	ToolDone    = "done"
	ToolError   = "error"

	ToolMessage = "message"
	ToolOther   = "other"

	toolDurationMin = 1000
)

var (
	toolKinds = []string{"command", "read", "edit", "search", "web", ToolMessage, ToolOther}
	stepTypes = []string{StepText, StepThinking, StepError, StepTool}
)

type DurationParts struct {
	Hours, Minutes, Seconds int
}

func SplitDuration(ms int64) DurationParts {
	seconds := max(1, int(math.Round(float64(ms)/1000)))
	if seconds < 60 {
		return DurationParts{Seconds: seconds}
	}
	minutes := seconds / 60
	if minutes < 60 {
		return DurationParts{Minutes: minutes, Seconds: seconds % 60}
	}
	return DurationParts{Hours: minutes / 60, Minutes: minutes % 60}
}

func ToolKind(kind string) string {
	if slices.Contains(toolKinds, kind) {
		return kind
	}
	return ToolOther
}

func ToolDurationShown(ms int64) bool {
	return ms >= toolDurationMin
}

func KnownSteps(steps []api.ExecutionStep) []api.ExecutionStep {
	out := make([]api.ExecutionStep, 0, len(steps))
	for _, step := range steps {
		if slices.Contains(stepTypes, step.Type) {
			out = append(out, step)
		}
	}
	return out
}

func StepStats(steps []api.ExecutionStep) (count, tools int) {
	for _, step := range steps {
		if step.Type == StepTool {
			tools++
		}
	}
	return len(steps), tools
}

func SwitcherRuns(e api.ExecutionDetail) []api.ExecutionSummary {
	if len(e.Runs) == 0 || slices.ContainsFunc(e.Runs, func(r api.ExecutionSummary) bool { return r.ID == e.ID }) {
		return e.Runs
	}
	return append(slices.Clone(e.Runs), e.ExecutionSummary)
}

func RunSpan(run api.ExecutionSummary, now time.Time) (int64, bool) {
	switch {
	case run.CompletedAt != nil:
		return *run.CompletedAt - run.StartedAt, true
	case run.Status == ExecutionRunning:
		return now.UnixMilli() - run.StartedAt, true
	}
	return 0, false
}

func PlainInstruction(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "`", ""))
}

func ShowsHandoffTask(msgs []Message, taskID string) bool {
	return taskID != "" && slices.ContainsFunc(msgs, func(m Message) bool {
		return m.Handoff != nil && m.Handoff.TaskID == taskID
	})
}
