package chat_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/execution"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

func runningWith(executionID string) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		st.Conversations = append(st.Conversations, api.ConversationSummary{ID: workerCID, Kind: "worker", WorkerID: "w1",
			Activity: api.ConversationActivity{State: "running", ExecutionID: executionID}})
	}
}

func (f *fixture) wantLocation(want string) {
	f.t.Helper()
	if got := f.router.Location(); got != want {
		f.t.Fatalf("location = %q, want %q", got, want)
	}
}

func (f *fixture) inked(r ui.Rect) bool {
	img := f.T.Image()
	scale := float32(img.Bounds().Dx()) / wideW
	x0, y0 := int(r.X*scale), int(r.Y*scale)
	x1, y1 := int((r.X+r.W)*scale), int((r.Y+r.H)*scale)
	base := img.RGBAAt(x0, y0)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if img.RGBAAt(x, y) != base {
				return true
			}
		}
	}
	return false
}

func withRun(m api.ChatMessage, status string, took time.Duration) api.ChatMessage {
	run := &api.ExecutionSummary{ID: "run-" + m.ID, Status: status, StartedAt: m.TS + 1000}
	if took > 0 {
		done := run.StartedAt + took.Milliseconds()
		run.CompletedAt = &done
	}
	m.Handoff.Run = run
	return m
}

func TestActivityBarOpensTheProcess(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID), runningWith("e1")))
	f.wantShown(locales.ChatStatusRunning(), locales.ExecutionShow())
	f.click(locales.ExecutionShow())
	f.wantLocation(route.ChatRun("w1", "e1"))

	plain := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID), runningWith("")))
	plain.wantShown(locales.ChatStatusRunning())
	plain.wantHidden(locales.ExecutionShow())

	queued := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID), activity("queued")))
	queued.wantHidden(locales.ExecutionShow())
}

func TestTaskEventAddsTheProcessButton(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID), activity("idle")))
	f.Fake.Emit(api.ChatEvent{Type: "task", ConversationID: workerCID, TaskID: "t1", WorkerID: "w1", State: "running"})
	f.Fix.Await(t, func() bool { return f.activityOf(workerCID) == "running" })
	f.settleMotion()
	f.wantHidden(locales.ExecutionShow())

	f.Fake.Emit(api.ChatEvent{Type: "task", ConversationID: workerCID, TaskID: "t1", WorkerID: "w1", State: "running", ExecutionID: "e7"})
	f.Fix.Await(t, func() bool { f.T.Frame(); return f.shows(locales.ExecutionShow()) })
	f.settleMotion()
	f.click(locales.ExecutionShow())
	f.wantLocation(route.ChatRun("w1", "e7"))
}

func TestWorkerMessageOpensItsProcess(t *testing.T) {
	traced := message("m1", "worker", "w1", "Changed the install command", time.Now().Add(-time.Hour))
	traced.ExecutionID = "e1"
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID, traced))
	link, ok := f.T.Find(locales.ExecutionShow())
	if !ok {
		t.Fatalf("no process link; texts %q", f.T.Texts())
	}
	if f.inked(link) {
		t.Fatal("the process link shows before the reply is hovered")
	}
	reply, _ := f.T.Find("Changed the install command")
	f.T.Move(reply.X+reply.W/2, reply.Y+reply.H/2)
	f.settleMotion()
	if !f.inked(link) {
		t.Fatal("the process link stays hidden while the reply is hovered")
	}
	f.click(locales.ExecutionShow())
	f.wantLocation(route.ChatRun("w1", "e1"))

	plain := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID,
		message("m0", "worker", "w1", "Not linked to a run", time.Now().Add(-2*time.Hour))))
	plain.wantShown("Not linked to a run")
	plain.wantHidden(locales.ExecutionShow())
}

func TestFailureNoticeOpensItsProcess(t *testing.T) {
	failed := message("n1", "system", "", "Task execution failed", time.Now().Add(-time.Hour))
	failed.ExecutionID = "e1"
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID, failed))
	f.wantShown("Task execution failed", locales.ExecutionShow())
	f.click(locales.ExecutionShow())
	f.wantLocation(route.ChatRun("w1", "e1"))
}

func TestLiveStopNoticeOpensItsProcess(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.wantHidden(locales.ExecutionShow())
	f.Fake.Reply(workerCID, api.ChatMessage{ID: "n2", Sender: api.ChatSender{Type: "system"}, Content: "Task was cancelled", ExecutionID: "e2"})
	f.Fix.Await(t, func() bool { f.T.Frame(); return f.shows("Task was cancelled") })
	f.settleMotion()
	f.click(locales.ExecutionShow())
	f.wantLocation(route.ChatRun("w1", "e2"))
}

func TestHandoffRunRow(t *testing.T) {
	at := time.Now().Add(-time.Hour)
	scheduled := withRun(handoff("h1", "w1", "scheduled", "0 9 * * *", "Check the links", at), "completed", 108*time.Second)
	failed := withRun(handoff("h2", "w2", "immediate", "", "Translate the changelog", at.Add(time.Minute)), "failed", 10*time.Minute)
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation, scheduled, failed))

	f.wantShown(execution.RunLabel(*scheduled.Handoff.Run, true), execution.RunLabel(*failed.Handoff.Run, false))
	if got := execution.RunLabel(*scheduled.Handoff.Run, true); got != locales.ExecutionLatest()+locales.ExecutionStatusCompleted()+
		" · "+locales.ExecutionTook(locales.ExecutionMinutesSeconds("1", "48")) {
		t.Fatalf("scheduled run label = %q", got)
	}
	f.click(locales.ExecutionShow())
	f.wantLocation(route.ChatRun(api.BeeConversation, "run-h1"))

	f.click(locales.ChatHandedOffTo("Alice"))
	f.wantPath("/chat/w1")

	pending := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		handoff("h3", "w1", "countdown", "", "Not started yet", at)))
	pending.wantShown(locales.ChatHandedOffTo("Alice"))
	pending.wantHidden(locales.ExecutionShow())
}

func TestHandoffRunRowOnDeletedWorker(t *testing.T) {
	gone := withRun(handoff("h1", "gone", "immediate", "", "Archive the builds", time.Now().Add(-time.Hour)), "completed", time.Minute)
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation, gone))
	f.click(locales.ExecutionShow())
	f.wantLocation(route.ChatRun(api.BeeConversation, "run-h1"))
}

func TestHandoffRunRefreshesOnTaskEvents(t *testing.T) {
	h := withRun(handoff("h1", "w1", "scheduled", "0 9 * * *", "Check the links", time.Now().Add(-time.Hour)), "running", 0)
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation, h))
	f.wantShown(execution.RunLabel(*h.Handoff.Run, true))
	reads := func() int { return len(f.Fake.Calls(http.MethodGet, "/api/local/conversations/bee/messages")) }
	before := reads()

	done := withRun(handoff("h1", "w1", "scheduled", "0 9 * * *", "Check the links", time.Now().Add(-time.Hour)), "completed", 2*time.Minute)
	f.Fake.Update(func(st *fakeapi.State) { st.Messages[api.BeeConversation] = []api.ChatMessage{done} })
	f.Fake.Emit(api.ChatEvent{Type: "task", ConversationID: workerCID, TaskID: h.Handoff.TaskID, WorkerID: "w1", State: "completed"})
	f.Fix.Await(t, func() bool { return reads() > before })
	f.settleMotion()
	f.wantShown(execution.RunLabel(*done.Handoff.Run, true))
}
