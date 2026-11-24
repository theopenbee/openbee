package chat_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/locales"
)

func choicePath(cid, mid string) string {
	return messagesPath(cid) + "/" + mid + "/choice"
}

func question(id, senderType, senderID string, at time.Time, buttons ...api.ChatButton) api.ChatMessage {
	m := message(id, senderType, senderID, "Which release should go out first?", at)
	m.Buttons = buttons
	return m
}

func releaseButtons() []api.ChatButton {
	return []api.ChatButton{{Label: "Ship 1.2"}, {Label: "Hold both"}, {Label: "Drop 1.1", Style: api.ButtonDanger}}
}

func (f *fixture) choiceCalls(cid, mid string) int {
	return len(f.Fake.Calls(http.MethodPost, choicePath(cid, mid)))
}

func (f *fixture) choice(cid, mid string) *int {
	for _, m := range f.Store.Chat(cid).List.Msgs {
		if m.ID == mid {
			return m.Choice
		}
	}
	return nil
}

func (f *fixture) wantChoice(cid, mid string, want int) {
	f.t.Helper()
	if got := f.choice(cid, mid); got == nil || *got != want {
		f.t.Fatalf("choice of %s = %v, want %d", mid, got, want)
	}
}

func TestChoiceButtonsPostTheChoice(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		question("q1", "bee", "", time.Now().Add(-time.Minute), releaseButtons()...)))
	f.wantShown("Which release should go out first?", "Ship 1.2", "Hold both", "Drop 1.1")
	f.Shot("chat-choices-open")
	shown := f.count("Hold both")

	f.click("Hold both")
	f.history(api.BeeConversation).await(2)
	f.wantChoice(api.BeeConversation, "q1", 1)
	if n := f.choiceCalls(api.BeeConversation, "q1"); n != 1 {
		t.Fatalf("choice calls = %d, want 1", n)
	}
	posted := f.Store.Chat(api.BeeConversation).List.Msgs[1]
	if posted.Sender.Type != "user" || posted.Content != "Hold both" {
		t.Fatalf("posted message = %+v, want the user sending the button text", posted)
	}
	if n := f.count("Hold both"); n <= shown {
		t.Fatalf("%q shown %d times, want the posted message beside the button", "Hold both", n)
	}
	f.wantShown("Ship 1.2", "Drop 1.1")
	f.Shot("chat-choices-chosen")

	f.click("Ship 1.2")
	f.click("Drop 1.1")
	if n := f.choiceCalls(api.BeeConversation, "q1"); n != 1 {
		t.Fatalf("choice calls after clicking chosen buttons = %d, want 1", n)
	}
}

func TestChoiceButtonsInWorkerChat(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID,
		question("q1", "worker", "w1", time.Now().Add(-time.Minute), releaseButtons()...),
		message("u1", "user", "", "Ship 1.2", time.Now().Add(-30*time.Second))))
	f.wantShown("Hold both")
	f.click("Drop 1.1")
	f.history(workerCID).await(3)
	f.wantChoice(workerCID, "q1", 2)
}

func TestChoiceTakenElsewhereShowsTheRealChoice(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		question("q1", "bee", "", time.Now().Add(-time.Minute), releaseButtons()...)))
	f.Fake.Update(func(st *fakeapi.State) {
		taken := 0
		st.Messages[api.BeeConversation][0].Choice = &taken
	})

	f.click("Drop 1.1")
	f.wantShown(locales.ChatChoiceTaken())
	f.wantChoice(api.BeeConversation, "q1", 0)
	if n := f.history(api.BeeConversation).count(); n != 1 {
		t.Fatalf("%d messages, want nothing posted", n)
	}
	f.click("Hold both")
	if n := f.choiceCalls(api.BeeConversation, "q1"); n != 1 {
		t.Fatalf("choice calls = %d, want the refreshed buttons disabled", n)
	}
}

func TestChoiceFailureKeepsButtonsOpen(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		question("q1", "bee", "", time.Now().Add(-time.Minute), releaseButtons()...)))
	f.Fake.FailNext(http.MethodPost, choicePath(api.BeeConversation, "q1"), http.StatusServiceUnavailable,
		map[string]string{"error": "message queue is full"})

	f.click("Ship 1.2")
	f.wantShown(locales.ChatChoiceFailed(), "message queue is full")
	if f.choice(api.BeeConversation, "q1") != nil {
		t.Fatal("choice set after a failure")
	}

	f.click("Ship 1.2")
	f.history(api.BeeConversation).await(2)
	f.wantChoice(api.BeeConversation, "q1", 0)
	if n := f.choiceCalls(api.BeeConversation, "q1"); n != 2 {
		t.Fatalf("choice calls = %d, want the retry to go through", n)
	}
}

func TestChoiceButtonsLoadWhileInFlight(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		question("q1", "bee", "", time.Now().Add(-time.Minute), releaseButtons()...)))
	release := f.Fake.HoldNext(http.MethodPost, choicePath(api.BeeConversation, "q1"))

	if err := f.T.Click("Ship 1.2"); err != nil {
		t.Fatal(err)
	}
	f.T.Frame()
	if index, ok := f.Store.Chat(api.BeeConversation).Choosing("q1"); !ok || index != 0 {
		t.Fatalf("Choosing = %d, %v, want 0 in flight", index, ok)
	}
	if err := f.T.Click("Hold both"); err != nil {
		t.Fatal(err)
	}
	f.T.Frame()
	release()
	f.history(api.BeeConversation).await(2)
	f.wantChoice(api.BeeConversation, "q1", 0)
	if n := f.choiceCalls(api.BeeConversation, "q1"); n != 1 {
		t.Fatalf("choice calls = %d, want the other buttons disabled while in flight", n)
	}
}

func TestUserMessageButtonsHidden(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		question("u1", "user", "", time.Now().Add(-time.Minute), api.ChatButton{Label: "Never shown"})))
	f.wantShown("Which release should go out first?")
	f.wantHidden("Never shown")
}

func TestLongChoiceWrapsInsideTheLog(t *testing.T) {
	long := strings.Repeat("Roll the release back to the previous build ", 3)[:100]
	f := newFixture(t, narrowW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		question("q1", "bee", "", time.Now().Add(-time.Minute), api.ChatButton{Label: long}, api.ChatButton{Label: "Keep"})))
	button, ok := f.T.Find(long)
	if !ok {
		t.Fatalf("no long button; texts %q", f.T.Texts())
	}
	keep, _ := f.T.Find("Keep")
	if button.X != keep.X || button.X+button.W > narrowW || button.H <= keep.H {
		t.Fatalf("long button %+v beside short button %+v, want it wrapped inside the %d wide log", button, keep, narrowW)
	}
}
