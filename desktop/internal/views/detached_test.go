package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/views"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const detachedW, detachedH = 600, 760

type detachedFixture struct {
	*testkit.Harness
	t      *testing.T
	view   *views.Detached
	win    *ui.Tester
	shown  int
	closed int
	titles []string
}

func openDetached(t *testing.T, cid string, seed func(st *fakeapi.State)) *detachedFixture {
	t.Helper()
	f := &detachedFixture{Harness: testkit.SignedIn(t, wideW, wideH, seed), t: t}
	f.view = f.Root.NewDetached(cid, views.DetachedOptions{
		ShowMain: func() { f.shown++ },
		Close:    func() { f.closed++ },
		SetTitle: func(title string) { f.titles = append(f.titles, title) },
	})
	f.Store.SetDetached(cid, true)
	f.win = testkit.NewTester(f.view.View, detachedW, detachedH)
	f.settle()
	return f
}

func (f *detachedFixture) settle() {
	f.t.Helper()
	for range 6 {
		f.Settle()
		f.win.Frame()
	}
}

func (f *detachedFixture) await(what string, ok func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			f.t.Fatalf("%s; window texts %q", what, f.win.Texts())
		}
		f.settle()
		time.Sleep(5 * time.Millisecond)
	}
}

func withAdaMessages(msgs ...api.ChatMessage) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		staffed(st)
		st.Messages = map[string][]api.ChatMessage{api.WorkerConversation("ada"): msgs}
	}
}

func TestDetachedWindowShowsOnlyItsChat(t *testing.T) {
	now := time.Now()
	f := openDetached(t, api.WorkerConversation("ada"), withAdaMessages(said("m1", "worker", "ada", "March report is ready", now.Add(-time.Minute))))
	f.await("the chat is not shown", func() bool { return f.win.HasText("March report is ready") })

	if !f.win.HasText("Ada Lovelace") {
		t.Fatalf("no worker name in the header; texts %q", f.win.Texts())
	}
	for _, hidden := range []string{locales.ChatBack(), locales.ChatSearch(), "张伟"} {
		if _, ok := f.win.Find(hidden); ok {
			t.Fatalf("the detached window shows %q", hidden)
		}
	}
	if err := f.win.Click("Ada Lovelace"); err == nil {
		f.settle()
	}
	if got := f.Root.Nav().Query(route.PanelQuery); got != "" {
		t.Fatalf("clicking the name opened the %q panel", got)
	}
	if !slices.Equal(f.titles, []string{"Ada Lovelace"}) {
		t.Fatalf("titles = %q, want the worker name once", f.titles)
	}
	if got := f.view.Title(); got != "Ada Lovelace" {
		t.Fatalf("Title() = %q, want Ada Lovelace", got)
	}
}

func TestDetachedWindowFollowsTheWorkerName(t *testing.T) {
	f := openDetached(t, api.WorkerConversation("ada"), staffed)
	f.Fake.Update(func(st *fakeapi.State) {
		for i := range st.Workers {
			if st.Workers[i].ID == "ada" {
				st.Workers[i].Name = "Ada King"
			}
		}
	})
	f.Store.Workers.Invalidate()
	f.await("the header kept the old name", func() bool { return f.win.HasText("Ada King") })
	if !slices.Equal(f.titles, []string{"Ada Lovelace", "Ada King"}) {
		t.Fatalf("titles = %q, want the rename to follow", f.titles)
	}
}

func TestDetachedWindowClosesWhenTheWorkerIsDeleted(t *testing.T) {
	f := openDetached(t, api.WorkerConversation("ada"), staffed)
	if f.closed != 0 {
		t.Fatalf("closed %d times while the worker exists", f.closed)
	}
	f.Fake.Update(func(st *fakeapi.State) {
		st.Workers = slices.DeleteFunc(st.Workers, func(w api.Worker) bool { return w.ID == "ada" })
	})
	f.Store.Workers.Invalidate()
	f.await("the window did not close", func() bool { return f.closed > 0 })
	if got := f.Root.Nav().Path(); got == route.Chat(api.BeeConversation) && f.shown > 0 {
		t.Fatal("a deleted worker moved the main window instead of closing the detached one")
	}
}

func TestDetachedHandoffOpensTheMainWindow(t *testing.T) {
	now := time.Now()
	f := openDetached(t, api.BeeConversation, func(st *fakeapi.State) {
		staffed(st)
		st.Messages = map[string][]api.ChatMessage{api.BeeConversation: {handedOff("h1", "ada", "immediate", "", "Rerun the March report", now.Add(-time.Minute))}}
	})
	title := locales.ChatHandedOffTo("Ada Lovelace")
	f.await("no handoff card", func() bool { return f.win.HasText(title) })
	if err := f.win.Click(title); err != nil {
		t.Fatalf("click the handoff card: %v", err)
	}
	f.settle()
	if f.shown != 1 {
		t.Fatalf("main window shown %d times, want once", f.shown)
	}
	if got := f.Root.Nav().Path(); got != route.Chat("ada") {
		t.Fatalf("main window path = %q, want the worker chat", got)
	}
}

func TestDetachedWindowHasNoProcessButton(t *testing.T) {
	f := openDetached(t, api.WorkerConversation("ada"), func(st *fakeapi.State) {
		staffed(st)
		for i := range st.Conversations {
			if st.Conversations[i].ID == api.WorkerConversation("ada") {
				st.Conversations[i].Activity = api.ConversationActivity{State: "running", ExecutionID: "run-1"}
			}
		}
	})
	f.await("no running status", func() bool { return f.win.HasText(locales.ChatStatusRunning()) })
	if _, ok := f.win.Find(locales.ExecutionShow()); ok {
		t.Fatal("the detached window offers the process panel")
	}
	f.Root.Nav().Push(route.Chat("ada"))
	f.Settle()
	if !f.T.HasText(locales.ExecutionShow()) {
		t.Fatalf("the main window lost its process button; texts %q", f.T.Texts())
	}
}

func TestToastsShowInTheFocusedWindow(t *testing.T) {
	f := openDetached(t, api.WorkerConversation("ada"), staffed)
	f.Store.Toast(kit.Toast{Title: "Saved to Downloads"})
	f.settle()
	if !f.T.HasText("Saved to Downloads") || f.win.HasText("Saved to Downloads") {
		t.Fatal("with the main window in front, the toast should show there only")
	}

	f.Store.FocusDetached(api.WorkerConversation("ada"))
	f.Store.Toast(kit.Toast{Title: "Couldn't copy the image"})
	f.settle()
	if f.T.HasText("Couldn't copy the image") || !f.win.HasText("Couldn't copy the image") {
		t.Fatal("with the detached window in front, a new toast should show there only")
	}
}
