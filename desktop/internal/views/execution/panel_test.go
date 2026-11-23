package execution_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/execution"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	wideW, wideH = 1440, 1400
	drawerW      = 1000
	e1Path       = "/api/executions/e1"
)

type fixture struct {
	*testkit.Harness
	t *testing.T
}

func team(st *fakeapi.State) {
	st.Workers = []api.Worker{
		{ID: "w1", Name: "Alice", Description: "Writes the release notes", Status: "idle", Engine: "claude", WorkDir: "/srv/w1"},
	}
}

func detail(status string, mutate ...func(e *api.ExecutionDetail)) api.ExecutionDetail {
	e := api.ExecutionDetail{
		ExecutionSummary: api.ExecutionSummary{ID: "e1", Status: status, StartedAt: time.Now().Add(-10 * time.Minute).UnixMilli()},
		TaskID:           "t1", WorkerID: "w1", Engine: "claude", Instruction: "Update `README` and run the tests",
	}
	if status != "running" {
		done := e.StartedAt + 192_000
		e.CompletedAt = &done
	}
	for _, fn := range mutate {
		fn(&e)
	}
	return e
}

func seeded(executions ...api.ExecutionDetail) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		team(st)
		st.Executions = map[string]api.ExecutionDetail{}
		for _, e := range executions {
			st.Executions[e.ID] = e
		}
	}
}

func open(t *testing.T, w int, param, run string, seed func(st *fakeapi.State)) *fixture {
	t.Helper()
	f := &fixture{Harness: testkit.SignedIn(t, w, wideH, seed), t: t}
	f.Root.Nav().Push(route.ChatRun(param, run))
	f.Motion()
	return f
}

func (f *fixture) shows(text string) bool {
	_, ok := f.T.Find(text)
	return ok
}

func (f *fixture) wantShown(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if !f.shows(text) {
			f.t.Fatalf("screen does not show %q; texts %q", text, f.T.Texts())
		}
	}
}

func (f *fixture) wantHidden(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if f.shows(text) {
			f.t.Fatalf("screen shows %q", text)
		}
	}
}

func (f *fixture) click(name string) {
	f.t.Helper()
	f.Click(name)
	f.Motion()
}

func (f *fixture) fetches() int {
	n := 0
	for _, c := range f.Fake.Calls(http.MethodGet, e1Path) {
		if c.Path == e1Path {
			n++
		}
	}
	return n
}

func (f *fixture) poll(want int) {
	f.t.Helper()
	f.Fix.Clock.Advance(2 * time.Second)
	f.Fix.Await(f.t, func() bool { return f.fetches() == want })
	f.Motion()
}

func runningTool() api.ExecutionStep {
	return api.ExecutionStep{Type: "tool", Kind: "command", Name: "Bash", Summary: "pnpm test", Status: "running"}
}

func TestSummaryOfAFinishedRun(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("completed")))
	f.wantShown(locales.ExecutionPanelTitle(), "Alice", locales.ExecutionStatusCompleted(), "Update README and run the tests",
		"Claude Code", locales.ExecutionTook(execution.Duration(192_000)))
	if got := execution.Duration(192_000); got != locales.ExecutionMinutesSeconds("3", "12") {
		t.Fatalf("Duration(192s) = %q", got)
	}
	f.wantShown(locales.ExecutionNoSteps())
	f.wantHidden(locales.ExecutionLive())
}

func TestStepsErrors(t *testing.T) {
	for code, text := range map[string]string{
		"transcript_unsupported": locales.ExecutionStepsErrorTranscriptUnsupported(),
		"engine_unavailable":     locales.ExecutionStepsErrorEngineUnavailable(),
		"transcript_failed":      locales.ExecutionStepsErrorTranscriptFailed(),
		"something_new":          locales.ExecutionStepsErrorTranscriptFailed(),
	} {
		t.Run(code, func(t *testing.T) {
			f := open(t, wideW, "w1", "e1", seeded(detail("completed", func(e *api.ExecutionDetail) { e.StepsError = code })))
			f.wantShown(text)
			f.wantHidden(locales.ExecutionNoSteps())
		})
	}
}

func TestRunningWithoutStepsWaits(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("running")))
	f.wantShown(locales.ExecutionStatusRunning(), locales.ExecutionWaiting(), locales.ExecutionLive())
	f.wantHidden(locales.ExecutionNoSteps())
	if !f.T.HasText(locales.ExecutionElapsed("")) {
		t.Fatalf("no elapsed time; texts %q", f.T.Texts())
	}
}

func TestRunningToolSpinsWhileTheRunRuns(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("running", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{runningTool()}
	})))
	f.wantShown(locales.ExecutionToolCommand(), "pnpm test")
	f.wantHidden(locales.ExecutionToolStopped())
}

func TestUnfinishedToolIsStoppedAfterTheRun(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("failed", func(e *api.ExecutionDetail) {
		e.Error = "timed out after 10 minutes"
		e.Steps = []api.ExecutionStep{runningTool()}
	})))
	f.wantShown(locales.ExecutionStatusFailed(), locales.ExecutionToolStopped(), locales.ExecutionFailedTitle(), "timed out after 10 minutes")
	f.wantShown(locales.ExecutionStepCount(locales.ExecutionSteps(1), locales.ExecutionToolCalls(1)))
}

func TestTimelineAndToolOutput(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("completed", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{
			{Type: "thinking", Text: "Find the install section"},
			{Type: "tool", Kind: "edit", Name: "Edit", Summary: "README.md", Output: "+npm i -g openbee", Status: "done", DurationMS: 41_000},
			{Type: "tool", Kind: "message", Name: "send_message", Summary: "On it", Status: "done"},
			{Type: "tool", Kind: "browser", Name: "todo_write", Status: "error"},
			{Type: "error", Text: "rate limited"},
			{Type: "text", Text: "All **done**"},
			{Type: "image", Text: "ignored"},
		}
	})))
	f.wantShown(locales.ExecutionThinking(), "Find the install section", locales.ExecutionToolEdit(), "README.md",
		execution.Duration(41_000), locales.ExecutionToolMessage(), "On it", locales.ExecutionToolOther("todo_write"), "rate limited")
	if !f.T.HasText("All") || f.T.HasText("ignored") {
		t.Fatalf("text steps wrong; texts %q", f.T.Texts())
	}
	f.wantShown(locales.ExecutionStepCount(locales.ExecutionSteps(6), locales.ExecutionToolCalls(3)))
	f.wantHidden("+npm i -g openbee")

	f.click(locales.ExecutionToolEdit())
	f.wantShown("+npm i -g openbee")
	f.click(locales.ExecutionToolEdit())
	f.wantHidden("+npm i -g openbee")
}

func TestRunSwitcher(t *testing.T) {
	older := detail("completed", func(e *api.ExecutionDetail) {
		e.Runs = []api.ExecutionSummary{
			{ID: "e2", Status: "failed", StartedAt: time.Now().Add(-time.Minute).UnixMilli()},
			{ID: "e1", Status: "completed", StartedAt: e.StartedAt},
		}
	})
	newer := detail("failed", func(e *api.ExecutionDetail) {
		e.ID, e.Runs, e.Instruction = "e2", older.Runs, "The second attempt"
	})
	f := open(t, wideW, "w1", "e1", seeded(older, newer))
	f.wantShown(locales.ExecutionRuns())
	f.click(locales.ExecutionRuns())
	option := ""
	for _, text := range f.T.Texts() {
		if strings.HasSuffix(text, " · "+locales.ExecutionStatusFailed()) {
			option = text
		}
	}
	if option == "" {
		t.Fatalf("no failed run option; texts %q", f.T.Texts())
	}
	f.click(option)
	if got := f.Root.Nav().Query(route.RunQuery); got != "e2" {
		t.Fatalf("run = %q, want e2", got)
	}
	f.wantShown("The second attempt")

	single := open(t, wideW, "w1", "e1", seeded(detail("completed")))
	single.wantHidden(locales.ExecutionRuns())
}

func TestPollingStopsWhenTheRunFinishes(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("running", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{runningTool()}
	})))
	if n := f.fetches(); n != 1 {
		t.Fatalf("%d fetches on open, want 1", n)
	}
	f.poll(2)
	f.Fake.Update(func(st *fakeapi.State) {
		st.Executions["e1"] = detail("completed", func(e *api.ExecutionDetail) {
			e.Steps = []api.ExecutionStep{{Type: "tool", Kind: "command", Name: "Bash", Summary: "pnpm test", Status: "done"}}
		})
	})
	f.poll(3)
	f.wantShown(locales.ExecutionStatusCompleted())
	f.wantHidden(locales.ExecutionLive())

	f.Fix.Clock.Advance(10 * time.Second)
	f.Motion()
	if n := f.fetches(); n != 3 {
		t.Fatalf("%d fetches after the run finished, want 3", n)
	}
}

func TestClosingThePanelStopsPolling(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("running")))
	f.click(locales.ChatPanelClose())
	f.wantHidden(locales.ExecutionPanelTitle())
	f.Fix.Clock.Advance(10 * time.Second)
	f.Motion()
	if n := f.fetches(); n != 1 {
		t.Fatalf("%d fetches after closing the panel, want 1", n)
	}
}

func TestMissingExecution(t *testing.T) {
	f := open(t, wideW, "bee", "gone", seeded())
	f.wantShown(locales.ExecutionPanelTitle(), locales.ExecutionNotFound())
	f.wantHidden(locales.CommonRetry())
}

func TestLoadErrorRetries(t *testing.T) {
	f := &fixture{Harness: testkit.SignedIn(t, wideW, wideH, seeded(detail("completed"))), t: t}
	f.Fake.FailNext(http.MethodGet, e1Path, http.StatusInternalServerError, map[string]string{"error": "boom"})
	f.Root.Nav().Push(route.ChatRun("w1", "e1"))
	f.Motion()
	f.wantShown(locales.CommonLoadError(), locales.CommonRetry())
	f.click(locales.CommonRetry())
	f.wantShown(locales.ExecutionStatusCompleted())
}

func TestDrawerShowsThePanel(t *testing.T) {
	f := open(t, drawerW, "w1", "e1", seeded(detail("completed")))
	f.wantShown(locales.ExecutionPanelTitle(), locales.ExecutionStatusCompleted())
}

func TestRunLabel(t *testing.T) {
	done := int64(108_000 + 1000)
	cases := []struct {
		run    api.ExecutionSummary
		latest bool
		want   string
	}{
		{api.ExecutionSummary{Status: "running", StartedAt: 1000}, false, locales.ExecutionStatusRunning()},
		{api.ExecutionSummary{Status: "running", StartedAt: 1000}, true, locales.ExecutionLatest() + locales.ExecutionStatusRunning()},
		{api.ExecutionSummary{Status: "completed", StartedAt: 1000, CompletedAt: &done}, true,
			locales.ExecutionLatest() + locales.ExecutionStatusCompleted() + " · " + locales.ExecutionTook(locales.ExecutionMinutesSeconds("1", "48"))},
	}
	for _, c := range cases {
		if got := execution.RunLabel(c.run, c.latest); got != c.want {
			t.Errorf("RunLabel(%+v, %v) = %q, want %q", c.run, c.latest, got, c.want)
		}
	}
	for ms, want := range map[int64]string{
		500:       locales.ExecutionSeconds(1),
		180_000:   locales.ExecutionMinutes(3),
		3_600_000: locales.ExecutionHours(1),
		3_720_000: locales.ExecutionHoursMinutes("1", "2"),
	} {
		if got := execution.Duration(ms); got != want {
			t.Errorf("Duration(%d) = %q, want %q", ms, got, want)
		}
	}
}
