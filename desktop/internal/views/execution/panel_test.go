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

func (f *fixture) count(text string) int {
	n := 0
	for _, shown := range f.T.Texts() {
		if shown == text {
			n++
		}
	}
	return n
}

func TestRunningWithoutStepsWaits(t *testing.T) {
	e := detail("running")
	f := open(t, wideW, "w1", "e1", seeded(e))
	f.wantShown(locales.ExecutionStatusRunning(), locales.ExecutionWaiting(), locales.ExecutionLive())
	f.wantHidden(locales.ExecutionNoSteps())
	shown := func() string {
		for _, text := range f.T.Texts() {
			if strings.HasSuffix(text, locales.ExecutionElapsed("")) {
				return text
			}
		}
		t.Fatalf("no elapsed time; texts %q", f.T.Texts())
		return ""
	}
	first := shown()
	f.T.Advance(65 * time.Second)
	span := f.T.Now().UnixMilli() - e.StartedAt
	later := shown()
	if later == first || later != locales.ExecutionElapsed(execution.Duration(span)) &&
		later != locales.ExecutionElapsed(execution.Duration(span-1000)) {
		t.Fatalf("elapsed time %q after 65s, was %q; want about %q", later, first, locales.ExecutionElapsed(execution.Duration(span)))
	}
}

func TestRunningToolSpinsWhileTheRunRuns(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("running", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{runningTool()}
	})))
	f.wantShown(locales.ExecutionToolTitleCommand("pnpm test"))
	f.wantHidden(locales.ExecutionToolStopped())
	if n := f.count(locales.ExecutionStatusRunning()); n != 2 {
		t.Fatalf("%q shown %d times, want the badge and the tool's spinner; texts %q", locales.ExecutionStatusRunning(), n, f.T.Texts())
	}
}

func TestUnfinishedToolIsStoppedAfterTheRun(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("failed", func(e *api.ExecutionDetail) {
		e.Error = "timed out after 10 minutes"
		e.Steps = []api.ExecutionStep{runningTool()}
	})))
	f.wantShown(locales.ExecutionStatusFailed(), locales.ExecutionToolStopped(), locales.ExecutionFailedTitle(), "timed out after 10 minutes")
	f.wantShown(locales.ExecutionStepCount(locales.ExecutionSteps(1), locales.ExecutionToolCalls(1)))
	f.wantHidden(locales.ExecutionStatusRunning())
}

func TestToolToggleNamesItsStatus(t *testing.T) {
	withOutput := func(status string) api.ExecutionStep {
		step := runningTool()
		step.Status, step.Output = status, "log"
		return step
	}
	f := open(t, wideW, "w1", "e1", seeded(detail("failed", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{withOutput("error"), withOutput("running"), withOutput("done")}
	})))
	title := locales.ExecutionToolTitleCommand("pnpm test")
	f.wantShown(title+" · "+locales.ExecutionStatusFailed(), title+" · "+locales.ExecutionToolStopped())
	f.click(title + " · " + locales.ExecutionToolStopped())
	f.wantShown("log")
}

func TestRunGoneWhilePollingIsNotLive(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("running", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{runningTool()}
	})))
	f.wantShown(locales.ExecutionLive())
	f.Fake.Update(func(st *fakeapi.State) { delete(st.Executions, "e1") })
	f.poll(2)
	f.wantShown(locales.ExecutionNotFound())
	f.wantHidden(locales.ExecutionLive(), locales.ExecutionStatusRunning(), locales.ExecutionToolTitleCommand("pnpm test"))
	f.Fix.Clock.Advance(10 * time.Second)
	f.Motion()
	if n := f.fetches(); n != 2 {
		t.Fatalf("%d fetches after the run was gone, want 2", n)
	}
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
	edited := locales.ExecutionToolTitleEdit("README.md")
	sent := locales.ExecutionToolTitleMessage("On it")
	f.wantShown(locales.ExecutionThinking(), "Find the install section", edited, execution.Duration(41_000), sent,
		locales.ExecutionToolOther("todo_write"), "rate limited")
	if !f.T.HasText("All") || f.T.HasText("ignored") {
		t.Fatalf("text steps wrong; texts %q", f.T.Texts())
	}
	f.wantShown(locales.ExecutionStepCount(locales.ExecutionSteps(6), locales.ExecutionToolCalls(3)))
	f.wantHidden("+npm i -g openbee", "README.md", "On it", locales.ExecutionThinking()+" · Find the install section",
		locales.ExecutionToolOther("todo_write")+" · "+locales.ExecutionStatusFailed())

	f.click(edited)
	f.wantShown("README.md", "+npm i -g openbee")
	f.click(edited)
	f.wantHidden("README.md", "+npm i -g openbee")

	f.click(sent)
	f.wantShown("On it")
}

func TestToolTitles(t *testing.T) {
	tool := func(kind, summary string) api.ExecutionStep {
		return api.ExecutionStep{Type: "tool", Kind: kind, Name: "Tool", Summary: summary, Status: "done"}
	}
	f := open(t, wideW, "w1", "e1", seeded(detail("completed", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{
			tool("read", "a.md"), tool("search", "rate limit"), tool("web", "https://theopenbee.com"),
			tool("message", "  Done.\n\nSee   the PR "), tool("command", ""), tool("edit", "   "),
			{Type: "tool", Kind: "other", Name: "todo_write", Summary: "3 todos", Status: "done"},
			{Type: "tool", Kind: "browser", Name: "navigate", Status: "done"},
		}
	})))
	f.wantShown(locales.ExecutionToolTitleRead("a.md"), locales.ExecutionToolTitleSearch("rate limit"),
		locales.ExecutionToolTitleWeb("https://theopenbee.com"), locales.ExecutionToolTitleMessage("Done. See the PR"),
		locales.ExecutionToolCommand(), locales.ExecutionToolEdit(), locales.ExecutionToolOther("todo_write"),
		locales.ExecutionToolOther("navigate"))
	f.wantHidden("3 todos", "a.md")

	f.click(locales.ExecutionToolTitleRead("a.md"))
	f.wantHidden("a.md")
	f.click(locales.ExecutionToolOther("todo_write"))
	f.wantShown("3 todos")
}

func (f *fixture) pixel(x, y float32) (r, g, b uint32) {
	r, g, b, _ = f.T.Image().At(int(x), int(y)).RGBA()
	return r >> 8, g >> 8, b >> 8
}

func TestToolDetailShowsTheDiff(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("completed", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{{Type: "tool", Kind: "edit", Name: "Edit", Summary: "a.ts",
			Diff: " keep\n-old\n+new\n+more\n…", Status: "done"}}
	})))
	f.wantHidden("old", "+2")
	f.click(locales.ExecutionToolTitleEdit("a.ts"))
	f.wantShown("a.ts", "+2", "-1", "keep", "old", "new", "more", "…", "+", "-")

	tint := func(text string) (r, g, b uint32) {
		box, ok := f.T.Find(text)
		if !ok {
			t.Fatalf("no %q; texts %q", text, f.T.Texts())
		}
		return f.pixel(box.X+box.W+4, box.Y+box.H/2)
	}
	kr, kg, kb := tint("keep")
	if r, g, b := tint("old"); r <= g || r == kr && g == kg && b == kb {
		t.Errorf("removed line background rgb(%d,%d,%d), context rgb(%d,%d,%d); want a red tint", r, g, b, kr, kg, kb)
	}
	if r, g, b := tint("new"); g <= r || r == kr && g == kg && b == kb {
		t.Errorf("added line background rgb(%d,%d,%d), context rgb(%d,%d,%d); want a green tint", r, g, b, kr, kg, kb)
	}
}

func (f *fixture) inkOf(text string) (r, g, b uint32) {
	f.t.Helper()
	box, ok := f.T.Find(text)
	if !ok {
		f.t.Fatalf("no %q; texts %q", text, f.T.Texts())
	}
	best := -1 << 31
	for y := int(box.Y); y < int(box.Y+box.H); y++ {
		for x := int(box.X); x < int(box.X+box.W); x++ {
			pr, pg, pb := f.pixel(float32(x), float32(y))
			if score := int(pr) - max(int(pg), int(pb)); score > best {
				best, r, g, b = score, pr, pg, pb
			}
		}
	}
	return r, g, b
}

func TestToolDetailShowsTheErrorOfAFailedEdit(t *testing.T) {
	const diff = " keep\n-old\n+new"
	const failure = "oldString not found"
	f := open(t, wideW, "w1", "e1", seeded(detail("failed", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{{Type: "tool", Kind: "edit", Name: "Edit", Summary: "a.ts", Output: failure, Diff: diff, Status: "error"}}
	})))
	f.click(locales.ExecutionToolTitleEdit("a.ts") + " · " + locales.ExecutionStatusFailed())
	f.wantShown("keep", "old", "new", failure)
	last, _ := f.T.Find("new")
	text, _ := f.T.Find(failure)
	if text.Y <= last.Y {
		t.Errorf("error at y=%v, diff ends at y=%v; want the error under the diff", text.Y, last.Y)
	}
	if r, g, b := f.inkOf(failure); r < g+40 || r < b+40 {
		t.Errorf("error text rgb(%d,%d,%d); want the danger red", r, g, b)
	}
}

func TestToolDetailShowsTheOutputUnderTheDiff(t *testing.T) {
	const note = "wrote 3 lines"
	f := open(t, wideW, "w1", "e1", seeded(detail("completed", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{{Type: "tool", Kind: "edit", Name: "Edit", Summary: "a.ts", Output: note, Diff: " keep\n-old\n+new", Status: "done"}}
	})))
	f.click(locales.ExecutionToolTitleEdit("a.ts"))
	f.wantShown("keep", "old", "new", note)
	last, _ := f.T.Find("new")
	text, _ := f.T.Find(note)
	if text.Y <= last.Y {
		t.Errorf("output at y=%v, diff ends at y=%v; want the output under the diff", text.Y, last.Y)
	}
	if r, g, b := f.inkOf(note); r >= g+40 || r >= b+40 {
		t.Errorf("output text rgb(%d,%d,%d); want a neutral color", r, g, b)
	}
}

func TestToolDetailOutput(t *testing.T) {
	f := open(t, wideW, "w1", "e1", seeded(detail("completed", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{
			{Type: "tool", Kind: "search", Name: "Grep", Summary: "grep -rn limit src", Output: "No matches", Status: "done"},
			{Type: "tool", Kind: "command", Name: "Bash", Output: "ok", Status: "done"},
			{Type: "tool", Kind: "message", Name: "send_message", Summary: "On it\nsoon", Status: "done"},
		}
	})))
	f.click(locales.ExecutionToolTitleSearch("grep -rn limit src"))
	f.wantShown("grep -rn limit src", "No matches")
	f.click(locales.ExecutionToolCommand())
	f.wantShown("ok")
	f.click(locales.ExecutionToolTitleMessage("On it soon"))
	f.wantShown("On it\nsoon")
}

func TestThinkingOpensToTheRest(t *testing.T) {
	thinking := func(text string) func(e *api.ExecutionDetail) {
		return func(e *api.ExecutionDetail) { e.Steps = []api.ExecutionStep{{Type: "thinking", Text: text}} }
	}
	short := open(t, wideW, "w1", "e1", seeded(detail("completed", thinking("Short thought"))))
	short.wantShown("Short thought", locales.ExecutionThinking())
	short.wantHidden(locales.ExecutionThinking() + " · Short thought")

	f := open(t, wideW, "w1", "e1", seeded(detail("completed", thinking("\n**Plan the change**\nCheck the routes first.\n\nThen the tests."))))
	toggle := locales.ExecutionThinking() + " · Plan the change"
	f.wantShown("Plan the change", toggle)
	f.wantHidden("**Plan the change**", "Check the routes first.\n\nThen the tests.")
	f.click(toggle)
	f.wantShown("Check the routes first.\n\nThen the tests.")
	if n := f.count("Plan the change"); n != 1 {
		t.Fatalf("title shown %d times when open, want once", n)
	}
	f.click(toggle)
	f.wantHidden("Check the routes first.\n\nThen the tests.")
}

func TestLongThinkingTitleOpensInFull(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("Check the install section and the release notes before shipping. ", 12))
	f := open(t, wideW, "w1", "e1", seeded(detail("completed", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{{Type: "thinking", Text: long}}
	})))
	toggle := locales.ExecutionThinking() + " · " + long
	f.wantShown(toggle)
	if n := f.count(long); n != 1 {
		t.Fatalf("long thought shown %d times before opening, want once", n)
	}
	f.click(toggle)
	if n := f.count(long); n != 2 {
		t.Fatalf("long thought shown %d times when open, want the title and the full text", n)
	}
}

func TestToolSummaryShowsInFullOnHover(t *testing.T) {
	summary := "pnpm vitest run src/components/chat --reporter verbose " + strings.Repeat("--project web ", 12)
	f := open(t, wideW, "w1", "e1", seeded(detail("completed", func(e *api.ExecutionDetail) {
		e.Steps = []api.ExecutionStep{
			{Type: "tool", Kind: "command", Name: "Bash", Summary: summary, Status: "done"},
			{Type: "tool", Kind: "read", Name: "Read", Summary: "a.md", Status: "done"},
		}
	})))
	title := locales.ExecutionToolTitleCommand(strings.TrimSpace(summary))
	hover := func(text string) {
		box, ok := f.T.Find(text)
		if !ok {
			t.Fatalf("no %q; texts %q", text, f.T.Texts())
		}
		f.T.Move(box.X+box.W/4, box.Y+box.H/2)
		f.T.Advance(2 * time.Second)
		f.Motion()
	}
	short := locales.ExecutionToolTitleRead("a.md")
	hover(short)
	if n := f.count(short); n != 1 {
		t.Fatalf("short title shown %d times on hover, want no tooltip", n)
	}
	before := f.count(title)
	if before == 0 {
		t.Fatalf("title missing; texts %q", f.T.Texts())
	}
	hover(title)
	if n := f.count(title); n != before+1 {
		t.Fatalf("title shown %d times on hover, was %d; want its tooltip too; texts %q", n, before, f.T.Texts())
	}
	f.click(title)
	f.wantShown(strings.TrimSpace(summary))
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
