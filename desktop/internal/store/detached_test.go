package store_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func TestFocusedDetachedChatIsTheOneRead(t *testing.T) {
	st, fake, fix := loaded(t, team)
	main := ensured(t, st, fix, "worker:w1")
	popped := ensured(t, st, fix, "worker:w2")
	st.SetActive("worker:w1")
	st.SetDetached("worker:w2", true)
	fix.Settle(t)
	reads := func(cid string) int { return countCalls(fake, http.MethodPost, conversationPath(cid, "read")) }
	mainBefore, poppedBefore := reads("worker:w1"), reads("worker:w2")

	st.FocusDetached("worker:w2")
	fix.Await(t, func() bool { return reads("worker:w2") == poppedBefore+1 })

	fake.Reply("worker:w2", api.ChatMessage{ID: "p1", Content: "in the pop-out"})
	fix.Await(t, func() bool { return hasMessage(popped, "p1") && reads("worker:w2") == poppedBefore+2 })
	if got := summaryOf(t, st, "worker:w2").Unread; got != 0 {
		t.Fatalf("unread of the focused pop-out = %d, want 0", got)
	}

	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "behind the pop-out"})
	fix.Await(t, func() bool { return hasMessage(main, "m1") && summaryOf(t, st, "worker:w1").Unread == 1 })
	if n := reads("worker:w1") - mainBefore; n != 0 {
		t.Fatalf("%d read marks for the main window's chat while a pop-out has the focus, want none", n)
	}

	st.FocusDetached("")
	fix.Await(t, func() bool { return summaryOf(t, st, "worker:w1").Unread == 0 })
	fake.Reply("worker:w2", api.ChatMessage{ID: "p2", Content: "after leaving"})
	fix.Await(t, func() bool { return hasMessage(popped, "p2") && summaryOf(t, st, "worker:w2").Unread == 1 })
}

func TestFocusDetachedIgnoresClosedPopOuts(t *testing.T) {
	st, _, fix := loaded(t, team)
	st.FocusDetached("worker:w2")
	if got := st.FocusedDetached(); got != "" {
		t.Fatalf("focused pop-out = %q without one open, want none", got)
	}
	st.SetDetached("worker:w2", true)
	st.FocusDetached("worker:w2")
	st.SetDetached("worker:w2", false)
	fix.Settle(t)
	if got := st.FocusedDetached(); got != "" {
		t.Fatalf("focused pop-out = %q after it closed, want none", got)
	}
}

func TestFocusDetachedClosesItsNotifications(t *testing.T) {
	st, _, fix := loaded(t, team)
	fix.Notifier.Visible = true
	st.SetActive("worker:w1")
	st.SetDetached("worker:w2", true)
	fix.Notifier.Closed = nil

	st.FocusDetached("worker:w2")
	if !slices.Equal(fix.Notifier.Closed, []string{"worker:w2"}) {
		t.Fatalf("closed = %v, want the pop-out's notifications", fix.Notifier.Closed)
	}
}

func TestResyncRefreshesDetachedChats(t *testing.T) {
	st, fake, fix := loaded(t, team)
	ensured(t, st, fix, "worker:w1")
	popped := ensured(t, st, fix, "worker:w2")
	st.SetActive("worker:w1")
	st.SetDetached("worker:w2", true)
	fix.Settle(t)
	gets := func(cid string) int { return countCalls(fake, http.MethodGet, conversationPath(cid, "messages")) }

	dropStream(t, fake, fix, st)
	fake.Reply("worker:w2", api.ChatMessage{ID: "m2", Content: "while away"})
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return st.Online && hasMessage(popped, "m2") })
	if a, b := gets("worker:w1"), gets("worker:w2"); a != 2 || b != 2 {
		t.Fatalf("message fetches after the resync: active=%d pop-out=%d, want both refreshed", a, b)
	}
}
