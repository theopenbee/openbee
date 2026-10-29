package store_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/kit"
)

func TestBeeBusyClearsWhenResyncFindsTheReply(t *testing.T) {
	st, fake, fix := loaded(t, team)
	fake.Reply("bee", api.ChatMessage{ID: "old", Content: "an earlier answer"})
	bee := ensured(t, st, fix, "bee")
	st.SetActive("bee")
	fix.Await(t, func() bool { return hasMessage(bee, "old") })

	if err := call(t, fix, func(done func(error)) { bee.Send("plan", nil, done) }); err != nil {
		t.Fatalf("Send: %v", err)
	}
	dropStream(t, fake, fix, st)
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return st.Online })
	fix.Settle(t)
	if !bee.BeeBusy {
		t.Fatal("BeeBusy cleared by a resync that holds only an earlier Bee reply")
	}

	dropStream(t, fake, fix, st)
	fake.Reply("bee", api.ChatMessage{ID: "b1", Content: "on it"})
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return st.Online && hasMessage(bee, "b1") })
	fix.Settle(t)
	if bee.BeeBusy {
		t.Fatal("BeeBusy still set after a resync loaded the Bee reply")
	}
}

func TestResyncRefreshesOnlyTheActiveChat(t *testing.T) {
	st, fake, fix := loaded(t, team)
	one := ensured(t, st, fix, "worker:w1")
	two := ensured(t, st, fix, "worker:w2")
	st.SetActive("worker:w1")
	fix.Settle(t)
	gets := func(cid string) int { return countCalls(fake, http.MethodGet, conversationPath(cid, "messages")) }

	dropStream(t, fake, fix, st)
	fake.Reply("worker:w2", api.ChatMessage{ID: "m2", Content: "while away"})
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return st.Online })
	fix.Settle(t)
	if a, b := gets("worker:w1"), gets("worker:w2"); a != 2 || b != 1 {
		t.Fatalf("message fetches after the resync: active=%d other=%d, want 2 and 1", a, b)
	}
	if !one.Loaded || !two.Loaded {
		t.Fatal("a resync dropped a loaded chat")
	}

	two.Ensure()
	fix.Await(t, func() bool { return hasMessage(two, "m2") })
	if n := gets("worker:w2"); n != 2 {
		t.Fatalf("fetches of the stale chat on Ensure = %d, want 2", n)
	}
	two.Ensure()
	fix.Settle(t)
	if n := gets("worker:w2"); n != 2 {
		t.Fatalf("fetches after a second Ensure = %d, want the refreshed chat kept", n)
	}
}

func TestResyncRefreshesOnlyTheOpenWorkerDetail(t *testing.T) {
	st, fake, fix := loaded(t, team)
	one, two := st.WorkerDetail("w1"), st.WorkerDetail("w2")
	one.Ensure()
	two.Ensure()
	st.SetActive("worker:w1")
	fix.Settle(t)
	gets := func(id string) int { return countCalls(fake, http.MethodGet, "/api/workers/"+id) }

	dropStream(t, fake, fix, st)
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return st.Online })
	fix.Settle(t)
	if a, b := gets("w1"), gets("w2"); a != 2 || b != 1 {
		t.Fatalf("detail fetches after the resync: open=%d other=%d, want 2 and 1", a, b)
	}
	two.Ensure()
	fix.Settle(t)
	if n := gets("w2"); n != 2 {
		t.Fatalf("fetches of the stale detail on Ensure = %d, want 2", n)
	}
}

func TestPluginChangesRefreshOnlyTheWatchedDetail(t *testing.T) {
	p, fake, fix, _ := activePlugins(t, func(st *fakeapi.State) {
		team(st)
		pluginCatalog(st)
		st.Plugins = append(st.Plugins, api.PluginDetail{PluginStatus: api.PluginStatus{ID: "linear", Package: linearPackage,
			Source: "npm", Version: "0.4.0", Name: api.LocalizedText{"en": "Linear"}, Enabled: true, State: "running"}})
	})
	telegram, linear := p.Detail("telegram"), p.Detail("linear")
	telegram.Ensure()
	linear.Ensure()
	p.Watch("telegram")
	fix.Settle(t)
	gets := func(id string) int { return countCalls(fake, http.MethodGet, pluginsPath+"/"+id) }

	if err := call(t, fix, func(done func(error)) { p.SaveSettings("telegram", api.PluginSettingsUpdate{}, done) }); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if err := call(t, fix, func(done func(error)) { p.Restart("telegram", done) }); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if a, b := gets("telegram"), gets("linear"); a != 3 || b != 1 {
		t.Fatalf("detail fetches: watched=%d other=%d, want 3 and 1", a, b)
	}
	linear.Ensure()
	fix.Settle(t)
	if n := gets("linear"); n != 2 {
		t.Fatalf("fetches of the stale detail on Ensure = %d, want 2", n)
	}
}

func TestDeleteWorkerForgetsItsChatAndEnvs(t *testing.T) {
	st, fake, fix := loaded(t, team)
	ch := ensured(t, st, fix, "worker:w2")
	envs := st.Envs("worker", "w2")
	envs.Ensure()
	fix.Settle(t)
	if !ch.Loaded || !envs.Loaded {
		t.Fatalf("chat loaded=%v envs loaded=%v before the delete", ch.Loaded, envs.Loaded)
	}

	if err := call(t, fix, func(done func(error)) { st.DeleteWorker("w2", false, done) }); err != nil {
		t.Fatalf("DeleteWorker: %v", err)
	}
	if again := st.Chat("worker:w2"); again == ch || again.Loaded {
		t.Fatal("the deleted worker's chat is still kept")
	}
	if again := st.Envs("worker", "w2"); again == envs || again.Loaded {
		t.Fatal("the deleted worker's variables are still kept")
	}
	before := countCalls(fake, http.MethodGet, conversationPath("worker:w2", "messages"))
	dropStream(t, fake, fix, st)
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return st.Online })
	fix.Settle(t)
	if n := countCalls(fake, http.MethodGet, conversationPath("worker:w2", "messages")) - before; n != 0 {
		t.Fatalf("%d fetches of the deleted worker's chat after a resync, want none", n)
	}
}

func TestForegroundMarksTheOpenChatRead(t *testing.T) {
	st, fake, fix := loaded(t, team)
	ch := ensured(t, st, fix, "worker:w1")
	st.SetActive("worker:w1")
	fix.Settle(t)
	readPath := conversationPath("worker:w1", "read")
	before := countCalls(fake, http.MethodPost, readPath)

	st.SetForeground(false)
	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "while hidden"})
	fix.Await(t, func() bool { return hasMessage(ch, "m1") && summaryOf(t, st, "worker:w1").Unread == 1 })
	if n := countCalls(fake, http.MethodPost, readPath) - before; n != 0 {
		t.Fatalf("%d read marks in the background, want none", n)
	}

	st.SetForeground(true)
	fix.Await(t, func() bool { return summaryOf(t, st, "worker:w1").Unread == 0 })
	if n := countCalls(fake, http.MethodPost, readPath) - before; n != 1 {
		t.Fatalf("%d read marks on returning to the foreground, want 1", n)
	}
}

func TestWorkerStatusPatchesDetailWhenListAlreadyAgrees(t *testing.T) {
	st, fake, fix := loaded(t, team)
	detail := st.WorkerDetail("w1")
	detail.Ensure()
	fix.Settle(t)
	fake.Update(func(s *fakeapi.State) { s.Workers[0].Status = "working" })
	st.Workers.Invalidate()
	fix.Await(t, func() bool {
		w, _ := st.Worker("w1")
		return w.Status == "working"
	})
	if detail.Data.Status != "idle" {
		t.Fatalf("detail status = %q before the event, want the stale idle", detail.Data.Status)
	}

	fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: "w1", Status: "working"})
	fix.Await(t, func() bool { return detail.Data.Status == "working" })
}

func TestExpiredToastsAreDropped(t *testing.T) {
	st, _, fix := loaded(t, team)
	st.Toast(kit.Toast{Title: "old"})
	fix.Clock.Advance(kit.ToastTimeout + time.Second)
	st.Toast(kit.Toast{Title: "new"})
	st.PruneToasts()
	if len(st.Toasts) != 1 || st.Toasts[0].Title != "new" {
		t.Fatalf("toasts after pruning = %+v, want only the fresh one", st.Toasts)
	}
}
