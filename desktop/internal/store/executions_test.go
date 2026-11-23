package store_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

const executionPath = "/api/executions/e1"

func execution(status string) api.ExecutionDetail {
	return api.ExecutionDetail{
		ExecutionSummary: api.ExecutionSummary{ID: "e1", Status: status, StartedAt: 1700},
		TaskID:           "t1", WorkerID: "w1", Engine: "claude", Instruction: "Ship it",
	}
}

func withExecution(status string) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		team(st)
		st.Executions = map[string]api.ExecutionDetail{"e1": execution(status)}
	}
}

func setExecution(fake *fakeapi.Server, status string) {
	fake.Update(func(st *fakeapi.State) { st.Executions["e1"] = execution(status) })
}

func watched(t *testing.T, st *store.Store, fix *storetest.Fixture, id string) *store.Resource[api.ExecutionDetail] {
	t.Helper()
	st.WatchExecution(id)
	fix.Settle(t)
	r := st.Execution(id)
	if !r.Loaded || r.Err != nil {
		t.Fatalf("execution %q Loaded=%v Err=%v, want loaded", id, r.Loaded, r.Err)
	}
	return r
}

func pollOnce(t *testing.T, fake *fakeapi.Server, fix *storetest.Fixture, want int) {
	t.Helper()
	fix.Clock.AwaitSleepers(t, 1)
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return countCalls(fake, http.MethodGet, executionPath) == want })
}

func TestWatchedExecutionPollsWhileRunning(t *testing.T) {
	st, fake, fix := loaded(t, withExecution("running"))
	r := watched(t, st, fix, "e1")
	if r.Data.Status != "running" || countCalls(fake, http.MethodGet, executionPath) != 1 {
		t.Fatalf("status %q after %d fetches", r.Data.Status, countCalls(fake, http.MethodGet, executionPath))
	}

	pollOnce(t, fake, fix, 2)
	setExecution(fake, "completed")
	pollOnce(t, fake, fix, 3)
	fix.Settle(t)
	if r.Data.Status != "completed" {
		t.Fatalf("status = %q, want completed", r.Data.Status)
	}

	fix.Clock.Advance(10 * time.Second)
	fix.Settle(t)
	if n := countCalls(fake, http.MethodGet, executionPath); n != 3 {
		t.Fatalf("%d fetches after the run finished, want 3", n)
	}
}

func TestUnwatchedExecutionStopsPolling(t *testing.T) {
	st, fake, fix := loaded(t, withExecution("running"))
	watched(t, st, fix, "e1")
	fix.Clock.AwaitSleepers(t, 1)

	st.WatchExecution("")
	fix.Clock.Advance(10 * time.Second)
	fix.Settle(t)
	if n := countCalls(fake, http.MethodGet, executionPath); n != 1 {
		t.Fatalf("%d fetches after the panel closed, want 1", n)
	}

	setExecution(fake, "completed")
	r := watched(t, st, fix, "e1")
	if n := countCalls(fake, http.MethodGet, executionPath); n != 2 || r.Data.Status != "completed" {
		t.Fatalf("reopening a running execution: %d fetches, status %q; want a refetch", n, r.Data.Status)
	}
}

func TestFinishedExecutionIsNotRefetchedOnReopen(t *testing.T) {
	st, fake, fix := loaded(t, withExecution("failed"))
	watched(t, st, fix, "e1")
	st.WatchExecution("")
	watched(t, st, fix, "e1")
	fix.Clock.Advance(10 * time.Second)
	fix.Settle(t)
	if n := countCalls(fake, http.MethodGet, executionPath); n != 1 {
		t.Fatalf("%d fetches of a finished execution, want 1", n)
	}
}

func TestMissingExecutionStopsPolling(t *testing.T) {
	st, fake, fix := loaded(t, withExecution("running"))
	watched(t, st, fix, "e1")
	fake.Update(func(st *fakeapi.State) { delete(st.Executions, "e1") })
	pollOnce(t, fake, fix, 2)
	fix.Settle(t)
	if r := st.Execution("e1"); !api.IsStatus(r.Err, http.StatusNotFound) {
		t.Fatalf("Err = %v, want 404", r.Err)
	}
	fix.Clock.Advance(10 * time.Second)
	fix.Settle(t)
	if n := countCalls(fake, http.MethodGet, executionPath); n != 2 {
		t.Fatalf("%d fetches after a 404, want 2", n)
	}
}

func withHandoff(st *fakeapi.State) {
	team(st)
	st.Messages["bee"] = []api.ChatMessage{{
		ID: "h1", Sender: api.ChatSender{Type: "bee"}, Kind: "handoff", Content: "Check links", TS: 1000,
		Handoff: &api.HandoffInfo{WorkerID: "w1", TaskID: "t1", TaskType: "scheduled", ConversationID: "worker:w1"},
	}}
}

func TestTaskEventRefreshesHandoffChats(t *testing.T) {
	st, fake, fix := loaded(t, withHandoff)
	bee := ensured(t, st, fix, "bee")
	other := ensured(t, st, fix, "worker:w2")
	st.SetActive("bee")
	fix.Settle(t)
	gets := func(cid string) int { return countCalls(fake, http.MethodGet, conversationPath(cid, "messages")) }

	fake.Emit(api.ChatEvent{Type: "task", ConversationID: "worker:w1", TaskID: "t9", WorkerID: "w1", State: "running"})
	fix.Settle(t)
	if n := gets("bee"); n != 1 {
		t.Fatalf("%d bee fetches after an unrelated task, want 1", n)
	}

	fake.Update(func(st *fakeapi.State) {
		done := int64(1900)
		st.Messages["bee"][0].Handoff.Run = &api.ExecutionSummary{ID: "e1", Status: "completed", StartedAt: 1100, CompletedAt: &done}
	})
	fake.Emit(api.ChatEvent{Type: "task", ConversationID: "worker:w1", TaskID: "t1", WorkerID: "w1", State: "completed"})
	fix.Await(t, func() bool { return gets("bee") == 2 })
	fix.Settle(t)
	if run := bee.List.Msgs[0].Handoff.Run; run == nil || run.Status != "completed" {
		t.Fatalf("handoff run = %+v, want the completed run", run)
	}
	if n := gets("worker:w2"); n != 1 || !other.Loaded {
		t.Fatalf("%d fetches of a chat without the handoff, want 1", n)
	}
}

func TestTaskEventMarksInactiveHandoffChatStale(t *testing.T) {
	st, fake, fix := loaded(t, withHandoff)
	bee := ensured(t, st, fix, "bee")
	st.SetActive("worker:w2")
	gets := func() int { return countCalls(fake, http.MethodGet, conversationPath("bee", "messages")) }

	fake.Emit(api.ChatEvent{Type: "task", ConversationID: "worker:w1", TaskID: "t1", WorkerID: "w1", State: "running"})
	fix.Await(t, func() bool { return summaryOf(t, st, "worker:w1").Activity.State == "running" })
	fix.Settle(t)
	if n := gets(); n != 1 {
		t.Fatalf("%d fetches of an inactive chat, want 1", n)
	}
	bee.Ensure()
	fix.Settle(t)
	if n := gets(); n != 2 {
		t.Fatalf("%d fetches after showing the stale chat, want 2", n)
	}
}
