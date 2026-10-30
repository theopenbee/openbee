package store_test

import (
	"slices"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

func team(st *fakeapi.State) {
	st.Engines = fakeapi.Engines("pi", "claude")
	st.Workers = []api.Worker{
		{ID: "w1", Name: "Alice", Description: "Writes docs", Engine: "claude", Status: "idle"},
		{ID: "w2", Name: "Bob", Engine: "pi", Status: "idle"},
	}
	st.SystemConfigs = map[string]string{"default_engine": "pi"}
}

func loaded(t *testing.T, seed func(st *fakeapi.State)) (*store.Store, *fakeapi.Server, *storetest.Fixture) {
	t.Helper()
	st, fake, fix := signedIn(t, seed)
	st.Workers.Ensure()
	st.Conversations.Ensure()
	fix.Settle(t)
	if !st.Workers.Loaded || !st.Conversations.Loaded {
		t.Fatalf("Workers.Loaded=%v Conversations.Loaded=%v, want both loaded", st.Workers.Loaded, st.Conversations.Loaded)
	}
	if !st.Online {
		t.Fatal("chat stream not online after sign-in")
	}
	return st, fake, fix
}

func summaryOf(t *testing.T, st *store.Store, cid string) api.ConversationSummary {
	t.Helper()
	i := slices.IndexFunc(st.Conversations.Data, func(c api.ConversationSummary) bool { return c.ID == cid })
	if i < 0 {
		t.Fatalf("conversation %q not in %v", cid, st.Conversations.Data)
	}
	return st.Conversations.Data[i]
}

func countCalls(fake *fakeapi.Server, method, path string) int {
	n := 0
	for _, c := range fake.Calls(method, path) {
		if c.Path == path {
			n++
		}
	}
	return n
}

func TestWorkersAndConversationsLoad(t *testing.T) {
	st, fake, fix := signedIn(t, team)
	if _, ok := st.Worker("w1"); ok {
		t.Fatal("Worker found before the list loaded")
	}
	if got := st.EnabledEngines(); len(got) != 0 {
		t.Fatalf("EnabledEngines before config = %v, want none", got)
	}

	st.Workers.Ensure()
	st.Conversations.Ensure()
	st.SystemConfigs.Ensure()
	fix.Settle(t)

	if w, ok := st.Worker("w1"); !ok || w.Name != "Alice" {
		t.Fatalf("Worker(w1) = %+v, %v", w, ok)
	}
	workers := st.WorkerMap()
	if len(workers) != 2 || workers["w2"].Name != "Bob" {
		t.Fatalf("WorkerMap = %v", workers)
	}
	var ids []string
	for _, c := range st.Conversations.Data {
		ids = append(ids, c.ID)
	}
	if !slices.Equal(ids, []string{"bee", "worker:w1", "worker:w2"}) {
		t.Fatalf("conversations = %v", ids)
	}
	if got := st.SystemConfigs.Data["default_engine"]; got != "pi" {
		t.Fatalf("system config default_engine = %q, want pi", got)
	}
	if got := model.EngineIDs(st.EnabledEngines()); !slices.Equal(got, []string{"pi", "claude"}) {
		t.Fatalf("EnabledEngines = %v, want [pi claude]", got)
	}

	detail := st.WorkerDetail("w2")
	if st.WorkerDetail("w2") != detail {
		t.Fatal("WorkerDetail created a new resource for the same id")
	}
	detail.Ensure()
	fix.Settle(t)
	if !detail.Loaded || detail.Data.Name != "Bob" {
		t.Fatalf("WorkerDetail(w2) Loaded=%v Data=%+v", detail.Loaded, detail.Data)
	}
	if n := countCalls(fake, "GET", "/api/workers/w2"); n != 1 {
		t.Fatalf("worker detail calls = %d, want 1", n)
	}
}

func TestFocusRefetchWhenStale(t *testing.T) {
	st, fake, fix := loaded(t, team)
	st.SetActive("worker:w1")
	detail := st.WorkerDetail("w1")
	detail.Ensure()
	fix.Settle(t)
	counts := func() []int {
		return []int{
			countCalls(fake, "GET", "/api/workers"),
			countCalls(fake, "GET", "/api/workers/w1"),
			countCalls(fake, "GET", "/api/local/conversations"),
		}
	}

	st.SetForeground(false)
	st.SetForeground(true)
	fix.Settle(t)
	if got := counts(); !slices.Equal(got, []int{1, 1, 1}) {
		t.Fatalf("calls after a fresh focus = %v, want no refetch", got)
	}

	fix.Clock.Advance(29 * time.Second)
	st.SetForeground(true)
	fix.Settle(t)
	if got := counts(); !slices.Equal(got, []int{1, 1, 1}) {
		t.Fatalf("calls after 29 s = %v, want no refetch", got)
	}

	fix.Clock.Advance(2 * time.Second)
	st.SetForeground(false)
	st.SetForeground(true)
	st.SetForeground(true)
	fix.Settle(t)
	if got := counts(); !slices.Equal(got, []int{2, 2, 2}) {
		t.Fatalf("calls after 31 s = %v, want one refetch each", got)
	}
}
